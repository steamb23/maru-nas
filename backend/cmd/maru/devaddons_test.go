package main

import (
	"os"
	"path/filepath"
	"testing"

	"maru/backend/config"
	"maru/backend/internal/addonpkg"
)

// 운영 백엔드 감지는 경로를 받아 임시 디렉터리로 본다(실제 /var/lib/maru-addons를 보지 않는다).
func TestInstalledAddonBackends(t *testing.T) {
	root := t.TempDir()
	installed := filepath.Join(root, "var", "lib", "maru")
	status := filepath.Join(root, "status.json")

	// 결과 파일이 없으면 꺼짐이다.
	if host, sync := installedAddonBackends(installed, installed, status); host != nil || sync != nil {
		t.Fatalf("without a status file: %v %v", host, sync)
	}
	if err := os.WriteFile(status, []byte(`{"format":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	host, sync := installedAddonBackends(installed+"/", installed, status)
	if host == nil || sync == nil {
		t.Fatal("installed backends not detected")
	}
	want := addonpkg.InstalledBackendPaths()
	if host.Backend("hello") != filepath.Join(config.AddonSocketDir, "hello.sock") ||
		host.Host("hello") != filepath.Join(config.AddonHostDir, "hello", config.AddonHostSocketName) ||
		host.HostMode != want.HostMode || host.HostDirFromUnit != want.HostDirFromUnit || host.HostPeer == nil {
		t.Errorf("backend paths = %+v", host)
	}
	if sync.Desired != filepath.Join(installed, config.AddonBackendsFile) ||
		sync.Request != filepath.Join(installed, config.AddonSyncRequest) || sync.Status != status {
		t.Errorf("sync paths = %+v", sync)
	}

	// 상태 디렉터리가 설치한 경로가 아니면(개발 서버의 .dev-state, 메모리 모드) 운영 파일을 쓰지 않는다.
	for _, dir := range []string{"", filepath.Join(root, ".dev-state", "fake"), installed + "-other"} {
		if host, sync := installedAddonBackends(dir, installed, status); host != nil || sync != nil {
			t.Errorf("state dir %q: backends on", dir)
		}
	}

	// 결과가 심볼릭 링크나 디렉터리면 켜지 않는다.
	link := filepath.Join(root, "link.json")
	if err := os.Symlink(status, link); err != nil {
		t.Fatal(err)
	}
	if host, _ := installedAddonBackends(installed, installed, link); host != nil {
		t.Error("symlinked status file accepted")
	}
	if host, _ := installedAddonBackends(installed, installed, root); host != nil {
		t.Error("directory accepted as the status file")
	}
}

func TestAddonBackendsDevFirst(t *testing.T) {
	state := t.TempDir()
	t.Setenv(envDevAddonBackends, "1")
	host, sync := addonBackends(state)
	if host == nil || sync != nil {
		t.Fatalf("dev backends = %v, sync %v", host, sync)
	}
	if got := host.Host("hello"); got != filepath.Join(state, addonpkg.DevHostDir, "hello.sock") || host.HostPeer != nil || host.HostMode != 0o600 {
		t.Errorf("dev host socket = %q peer %v mode %v", got, host.HostPeer != nil, host.HostMode)
	}
	for _, d := range []string{addonpkg.DevBackendDir, addonpkg.DevHostDir} {
		if fi, err := os.Stat(filepath.Join(state, d)); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v %v", d, fi, err)
		}
	}
	// 개발 변수가 있어도 상태 디렉터리가 없으면 꺼짐이고 운영 경로로 넘어가지 않는다.
	if host, sync := addonBackends(""); host != nil || sync != nil {
		t.Errorf("dev without a state dir = %v %v", host, sync)
	}
	// 개발 변수가 1이 아니면 운영 감지로 간다(개발 상태 디렉터리는 설치한 경로가 아니므로 꺼짐).
	t.Setenv(envDevAddonBackends, "0")
	if host, sync := addonBackends(state); host != nil || sync != nil {
		t.Errorf("MARU_DEV_ADDON_BACKENDS=0 = %v %v", host, sync)
	}
}

func TestInstalledHostPeer(t *testing.T) {
	for uid, want := range map[int]bool{0: true, 61184: true, 65519: true, 63000: true, 1: false, 1000: false, 61183: false, 65520: false, 65534: false} {
		if got := addonpkg.InstalledHostPeer(uid); got != want {
			t.Errorf("InstalledHostPeer(%d) = %v", uid, got)
		}
	}
}
