package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"maru/backend/config"
	"maru/backend/internal/api"
	"maru/backend/internal/upgrade"
)

func TestUpgradePaths(t *testing.T) {
	installed := upgrade.Paths{EnvFile: config.UpgradeEnvFile, StatusFile: config.UpgradeStatusFile, RecordFile: config.InstallRecordFile}
	withRequest := installed
	withRequest.CheckRequest = "/var/lib/maru/upgrade-check.json"
	cases := []struct {
		devRoot, stateDir string
		want              upgrade.Paths
	}{
		// 설치한 상태 디렉터리에서만 확인 요청을 쓴다(경로 유닛이 그곳만 본다).
		{stateDir: "/var/lib/maru", want: withRequest},
		{stateDir: "/var/lib/maru/", want: withRequest},
		{stateDir: "/srv/maru-state", want: installed},
		{want: installed},
		// 개발 서버는 가짜 루트 아래를 읽고 지금 상태 디렉터리에 쓴다.
		{devRoot: "/tmp/dev", stateDir: "/tmp/state", want: upgrade.Paths{EnvFile: "/tmp/dev" + config.UpgradeEnvFile,
			StatusFile: "/tmp/dev" + config.UpgradeStatusFile, RecordFile: "/tmp/dev" + config.InstallRecordFile,
			CheckRequest: "/tmp/state/upgrade-check.json"}},
	}
	for _, c := range cases {
		if got := upgradePaths(c.devRoot, c.stateDir); got != c.want {
			t.Errorf("upgradePaths(%q, %q) = %+v, want %+v", c.devRoot, c.stateDir, got, c.want)
		}
	}
}

func TestParseListen(t *testing.T) {
	cases := []struct {
		addr             string
		allowNonLoopback bool
		network, address string
		wantErr          bool
	}{
		{addr: "127.0.0.1:8090", network: "tcp", address: "127.0.0.1:8090"},
		{addr: "[::1]:8090", network: "tcp", address: "[::1]:8090"},
		{addr: "unix:/run/maru/maru.sock", network: "unix", address: "/run/maru/maru.sock"},
		{addr: "unix:maru.sock", wantErr: true},
		{addr: "0.0.0.0:8090", wantErr: true},
		{addr: ":8090", wantErr: true},
		{addr: "192.0.2.1:8090", wantErr: true},
		{addr: "localhost:8090", wantErr: true},
		{addr: "127.0.0.1", wantErr: true},
		{addr: "0.0.0.0:8090", allowNonLoopback: true, network: "tcp", address: "0.0.0.0:8090"},
		{addr: ":8090", allowNonLoopback: true, network: "tcp", address: ":8090"},
		{addr: "127.0.0.1", allowNonLoopback: true, wantErr: true},
	}
	for _, c := range cases {
		network, address, err := parseListen(c.addr, c.allowNonLoopback)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseListen(%q, %v) = %s %s, want error", c.addr, c.allowNonLoopback, network, address)
			}
			continue
		}
		if err != nil || network != c.network || address != c.address {
			t.Errorf("parseListen(%q, %v) = %q %q %v, want %q %q", c.addr, c.allowNonLoopback, network, address, err, c.network, c.address)
		}
	}
}

func TestListenOnUnix(t *testing.T) {
	// Unix 소켓 경로 길이 제한(108바이트) 때문에 짧은 임시 디렉터리를 쓴다.
	dir, err := os.MkdirTemp("", "maru")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "o.sock")

	ln, err := listenOn("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o660 {
		t.Errorf("socket mode = %o, want 660", got)
	}

	// 쓰고 있는 소켓은 빼앗지 않는다.
	if _, err := listenOn("unix", path); err == nil {
		t.Fatal("listenOn on a socket in use succeeded")
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	// 비정상 종료로 남은 소켓 파일은 지우고 다시 수신한다.
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	ln.Close()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("stale socket should remain: %v", err)
	}
	ln, err = listenOn("unix", path)
	if err != nil {
		t.Fatalf("listenOn over stale socket: %v", err)
	}
	ln.Close()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("socket file should be removed on close, err = %v", err)
	}

	// 소켓이 아닌 파일은 지우지 않는다.
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenOn("unix", path); err == nil {
		t.Fatal("listenOn over a regular file succeeded")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "x" {
		t.Errorf("regular file was modified: %q %v", b, err)
	}
}

func TestRuntimeSettings(t *testing.T) {
	env := map[string]string{
		"MARU_LISTEN":        "unix:/run/maru/maru.sock",
		"MARU_OMV_URL":       "",
		"MARU_WORKBENCH_URL": "",
		"STATE_DIRECTORY":    "/var/lib/maru",
	}
	lookup := func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
	got := runtimeSettings(map[string]bool{"cookie-path": true}, lookup, []runtimeEntry{
		{flag: "listen", env: "MARU_LISTEN", value: "unix:/run/maru/maru.sock"},
		{flag: "omv-url", env: "MARU_OMV_URL", value: "http://127.0.0.1"},
		{flag: "cookie-path", env: "MARU_COOKIE_PATH", value: "/desk/"},
		{flag: "state-dir", env: "MARU_STATE_DIR", value: "/var/lib/maru", systemdEnv: "STATE_DIRECTORY"},
		{flag: "workbench-url", env: "MARU_WORKBENCH_URL", value: "", emptyIsSet: true},
		{flag: "smart-refresh-at", env: "MARU_SMART_REFRESH_AT", value: "00:00"},
	})
	want := map[string]string{
		"listen":           api.RuntimeSourceEnv,
		"omv-url":          api.RuntimeSourceDefault, // 빈 환경 변수는 기본값을 쓴다
		"cookie-path":      api.RuntimeSourceFlag,
		"state-dir":        api.RuntimeSourceSystemd,
		"workbench-url":    api.RuntimeSourceEnv, // 빈 값은 "끔"으로 설정한 것이다
		"smart-refresh-at": api.RuntimeSourceDefault,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries", len(got))
	}
	for _, s := range got {
		if s.Source != want[s.Flag] {
			t.Errorf("%s: source %q, want %q", s.Flag, s.Source, want[s.Flag])
		}
	}
	if got[0].Env != "MARU_LISTEN" || got[0].Value != "unix:/run/maru/maru.sock" {
		t.Errorf("first entry %+v", got[0])
	}
}
