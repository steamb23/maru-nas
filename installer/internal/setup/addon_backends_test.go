package setup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"maru/backend/config"
	"maru/installer/internal/prompt"
)

const (
	instA = "0123456789abcdef0123456789abcdef"
	instB = "fedcba9876543210fedcba9876543210"
	// addonPatternStop은 끌 때 마지막으로 올라와 있는 모든 백엔드 인스턴스를 멈추는 명령이다.
	addonPatternStop = "systemctl stop 'maru-addon@*.socket' 'maru-addon-*@*.socket' 'maru-addon@*.service' 'maru-addon-*@*.service'"
)

// addonFake는 helper-addon-backends 골든 결과(애드온 백엔드를 켠 설치)가 설치된 가짜 시스템이다.
func addonFake(t *testing.T) *fakeSystem {
	t.Helper()
	f := newFake(t)
	f.putRepo(t)
	f.copyTree(t, filepath.Join(goldenDir("helper-addon-backends"), "want"))
	f.put(t, binDst, "old\n", 0o755)
	f.put(t, helperBinDst, "old\n", 0o755)
	f.put(t, upgradeInstallerDst, "old\n", 0o755)
	f.users["maru"] = Account{Name: "maru", UID: 999, Home: "/nonexistent"}
	f.groups["maru"] = "maru"
	return f
}

// putAddonRecord는 동기화 기록을 쓴다.
func putAddonRecord(t *testing.T, f *fakeSystem, r addonSyncRecord) {
	t.Helper()
	if err := writeAddonSyncRecord(f, r); err != nil {
		t.Fatal(err)
	}
}

// putWants는 sockets.target.wants에 소켓 인스턴스 링크를 만든다(systemctl enable이 만드는 것처럼).
func putWants(t *testing.T, f *fakeSystem, names ...string) {
	t.Helper()
	dir := f.path(addonWantsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		unit, _, _ := strings.Cut(n, "@")
		if err := os.Symlink("/etc/systemd/system/"+unit+"@.socket", filepath.Join(dir, n)); err != nil {
			t.Fatal(err)
		}
	}
}

func lexists(f *fakeSystem, path string) bool {
	_, err := os.Lstat(f.path(path))
	return err == nil
}

// serviceDirectives는 유닛의 [Service] 절 지시어다(키별 값 목록, 주석 제외).
func serviceDirectives(t *testing.T, unit []byte) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	section := ""
	for _, line := range strings.Split(string(unit), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "["):
			section = line
		case section == "[Service]":
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				t.Fatalf("bad line %q", line)
			}
			out[k] = append(out[k], v)
		}
	}
	return out
}

// 백엔드 서비스 템플릿은 어느 네트워크 단계에서도 maru.service보다 느슨하지 않다. maru.service의 하드닝 지시어마다 같은
// 값이 있거나 더 엄격한 지시어로 바뀌어 있어야 한다(maru.service에 하드닝을 더하면 백엔드에도 더해야 이 시험이 통과한다).
// 그리고 /run, maru 상태 디렉터리, /srv, /home을 다시 열지 않는다.
func TestAddonBackendUnitsNotLooserThanMaru(t *testing.T) {
	c := &Context{Values: Values{nameUser: "maru", nameAddonBackends: "yes", nameHelper: "yes"}, group: "maru"}
	base := c.unitData()
	maruUnit, err := renderUnit("maru.service", base)
	if err != nil {
		t.Fatal(err)
	}
	maru := serviceDirectives(t, maruUnit)
	// 하드닝이 아닌 maru.service 지시어(실행할 것, 계정, 디렉터리, 다시 시작). ExecStartPre·ReadWritePaths는 호스트 소켓
	// 디렉터리를 만들고 maru에 여는 줄이다(백엔드에는 없어야 한다. 아래에서 본다).
	notHardening := map[string]bool{
		"Type": true, "User": true, "Group": true, "EnvironmentFile": true, "ExecStart": true, "Restart": true, "RestartSec": true,
		"StateDirectory": true, "RuntimeDirectory": true, "RuntimeDirectoryPreserve": true, "ExecStartPre": true, "ReadWritePaths": true,
	}
	if len(maru) <= len(notHardening) {
		t.Fatalf("maru.service has no hardening? %v", maru)
	}
	for _, tier := range addonTiers() {
		t.Run(tier.Prefix, func(t *testing.T) {
			data := base
			data.Tier = tier
			b, err := renderUnit("maru-addon.service", data)
			if err != nil {
				t.Fatal(err)
			}
			got := serviceDirectives(t, b)
			for key, values := range maru {
				if notHardening[key] {
					continue
				}
				switch key {
				case "DevicePolicy":
					// PrivateDevices=yes는 장치 노드를 아예 보이지 않게 하므로 DevicePolicy=closed보다 엄격하다.
					if !slices.Equal(got["PrivateDevices"], []string{"yes"}) && !slices.Equal(got[key], values) {
						t.Errorf("DevicePolicy=%v is not kept or replaced by PrivateDevices=yes", values)
					}
				case "RestrictAddressFamilies":
					allowed := strings.Fields(strings.Join(values, " "))
					if len(got[key]) != 1 {
						t.Fatalf("RestrictAddressFamilies %v", got[key])
					}
					for _, fam := range strings.Fields(got[key][0]) {
						if !slices.Contains(allowed, fam) {
							t.Errorf("address family %s is not allowed for maru.service", fam)
						}
					}
				default:
					if !slices.Equal(got[key], values) {
						t.Errorf("%s=%v, maru.service has %v", key, got[key], values)
					}
				}
			}
			for key, want := range map[string]string{
				"DynamicUser": "yes", "PrivateUsers": "yes", "CapabilityBoundingSet": "", "PrivateDevices": "yes",
				"ProtectProc": "invisible", "MemoryDenyWriteExecute": "yes", "SystemCallFilter": "@system-service", "UMask": "0077",
				"PrivateIPC": "yes",
			} {
				if !slices.Equal(got[key], []string{want}) {
					t.Errorf("%s=%v, want %q", key, got[key], want)
				}
			}
			// /dev/shm은 백엔드마다 따로 둔다(호스트의 /dev/shm을 다른 서비스와 함께 쓰지 않게).
			if !slices.Equal(got["TemporaryFileSystem"], []string{"/run:ro", stateDir + ":ro", "/dev/shm:mode=1777,nosuid,nodev,noexec"}) {
				t.Errorf("TemporaryFileSystem %v", got["TemporaryFileSystem"])
			}
			for _, key := range []string{"ReadWritePaths", "ReadOnlyPaths", "ExecPaths", "SupplementaryGroups", "AmbientCapabilities", "ExecStartPre"} {
				if len(got[key]) != 0 {
					t.Errorf("%s=%v", key, got[key])
				}
			}
			if !slices.Equal(got["BindPaths"], []string{config.AddonHostDir + "/%i:/run/maru-host"}) ||
				!slices.Equal(got["BindReadOnlyPaths"], []string{stateDir + "/addons/%i"}) {
				t.Errorf("binds %v %v", got["BindPaths"], got["BindReadOnlyPaths"])
			}
			hidden := strings.Fields(strings.Join(got["InaccessiblePaths"], " "))
			for _, p := range []string{"-/srv", "-/var/lib/openmediavault", "-/etc/openmediavault", "-" + config.UpgradeStateDir, "-" + config.AddonSyncDir} {
				if !slices.Contains(hidden, p) {
					t.Errorf("InaccessiblePaths misses %s", p)
				}
			}
			if !slices.Equal(got["ExecStart"], []string{"/var/lib/maru/addons/%i/current/backend/run"}) {
				t.Errorf("ExecStart %v", got["ExecStart"])
			}
			// 네트워크 단계
			allow := strings.Join(got["IPAddressAllow"], " ")
			switch {
			case tier.None:
				if !slices.Equal(got["PrivateNetwork"], []string{"yes"}) || !slices.Equal(got["RestrictAddressFamilies"], []string{"AF_UNIX"}) ||
					len(got["IPAddressAllow"]) != 0 {
					t.Errorf("no-network tier %v", got)
				}
			default:
				if len(got["PrivateNetwork"]) != 0 || !slices.Equal(got["IPAddressDeny"], []string{"any"}) {
					t.Errorf("network tier %v", got)
				}
				if strings.Contains(allow, "192.168.0.0/16") != tier.LAN || strings.Contains(allow, "localhost") != tier.Loopback {
					t.Errorf("IPAddressAllow %q for %+v", allow, tier)
				}
			}
		})
	}
}

// 백엔드 서비스가 바인드하는 호스트 소켓 디렉터리(<AddonHostDir>/<id>)는 그 위까지 root 것이다. maru가 <id>를 다른
// 곳으로 바꿔 root(PID 1)가 그곳을 바인드하게 하지 못하게, maru 유닛은 그 디렉터리를 RuntimeDirectory로 갖지 않고 root로
// 만든 뒤 쓰기만 연다. <id>는 소켓 유닛이 root로 만들어 maru에게 준다. 끈 설치의 maru 유닛에는 이 줄들이 없다.
func TestAddonHostDirOwnedByRoot(t *testing.T) {
	for _, on := range []bool{true, false} {
		c := &Context{Values: Values{nameUser: "maru", nameAddonBackends: yesNo(on), nameHelper: "yes"}, group: "maru"}
		b, err := renderUnit("maru.service", c.unitData())
		if err != nil {
			t.Fatal(err)
		}
		got := serviceDirectives(t, b)
		if !slices.Equal(got["RuntimeDirectory"], []string{"maru"}) {
			t.Errorf("on=%v: RuntimeDirectory %v", on, got["RuntimeDirectory"])
		}
		wantPre, wantRW := []string(nil), []string(nil)
		if on {
			wantPre = []string{"+/usr/bin/install -d -o root -g root -m 0755 " + config.AddonHostDir}
			wantRW = []string{"-" + config.AddonHostDir}
		}
		if !slices.Equal(got["ExecStartPre"], wantPre) || !slices.Equal(got["ReadWritePaths"], wantRW) {
			t.Errorf("on=%v: ExecStartPre %v ReadWritePaths %v", on, got["ExecStartPre"], got["ReadWritePaths"])
		}
	}
	c := &Context{Values: Values{nameUser: "svc", nameAddonBackends: "yes"}, group: "svcgroup"}
	for _, tier := range addonTiers() {
		data := c.unitData()
		data.Tier = tier
		b, err := renderUnit("maru-addon.socket", data)
		if err != nil {
			t.Fatal(err)
		}
		var pre []string
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(line, "ExecStartPre="); ok {
				pre = append(pre, v)
			}
		}
		want := []string{
			"/usr/bin/install -d -o root -g root -m 0755 " + config.AddonHostDir,
			"/usr/bin/install -d -o svc -g svcgroup -m 0755 " + config.AddonHostDir + "/%i",
		}
		if !slices.Equal(pre, want) {
			t.Errorf("%s socket ExecStartPre %q", tier.Prefix, pre)
		}
	}
}

// 설정은 묻지 않는다. 새 설치는 끔, 기존 설치는 동기화 디렉터리가 있는지로 읽고, 업그레이드 계획에도 나오지 않는다.
func TestAddonBackendsSetting(t *testing.T) {
	if settingAddonBackends.asked() {
		t.Fatal("addon backends must never be asked")
	}
	f := newFake(t)
	c := &Context{Lang: "en", Sys: f, Installed: &Installed{}, Values: Values{}}
	if got := settingAddonBackends.defaultValue(c); got != "no" {
		t.Errorf("fresh default %q", got)
	}
	f2 := addonFake(t)
	inst, err := readInstalled(f2)
	if err != nil {
		t.Fatal(err)
	}
	c2 := &Context{Lang: "en", Sys: f2, Installed: inst, Values: Values{}}
	if v, ok := settingAddonBackends.Load(c2); v != "yes" || !ok {
		t.Errorf("load %q %v", v, ok)
	}
	if got := settingAddonBackends.defaultValue(c2); got != "yes" {
		t.Errorf("installed default %q", got)
	}
	// 기존 값이 잘못됐을 때의 기본값은 기존 설치를 보지 않는다.
	if got := settingAddonBackends.Default(c2.withoutInstalled()); got != "no" {
		t.Errorf("default without installation %q", got)
	}

	f3 := planFake(t)
	f3.copyTree(t, filepath.Join(goldenDir("helper-addon-backends"), "want"))
	p, _, _ := runPlan(t, f3)
	if !p.Web || len(p.Questions) != 0 || len(p.Manual) != 0 {
		t.Errorf("plan %+v", p)
	}
}

// 켠 설치를 같은 설정으로 다시 설치하면 경로 유닛은 다시 시작하지 않고, 템플릿이 바뀐 단계의 켜진 인스턴스만 다시
// 시작한다(소켓이 바뀌면 소켓, 서비스만 바뀌면 서비스). 켜진 인스턴스는 기록과 sockets.target.wants의 링크에서 찾는다.
// 동기화가 쓴 결과는 덮어쓰지 않는다.
func TestAddonBackendsReinstallRestarts(t *testing.T) {
	f := addonFake(t)
	putAddonRecord(t, f, addonSyncRecord{Backends: []addonRecordBackend{
		{ID: "weather", Unit: "maru-addon-lan", Rev: "1.0.0-0123abcd", Instance: instA},
		{ID: "notes", Unit: "maru-addon", Rev: "0.2.0-89abcdef", Instance: instB},
		{ID: "clock", Unit: "maru-addon-loopback", Rev: "0.1.0-deadbeef", Instance: instA},
	}})
	// 기록에도 있는 링크와 링크에만 있는 인스턴스(기록을 잃은 경우)
	putWants(t, f, "maru-addon-lan@weather.socket", "maru-addon-lan@radio.socket", "maru-addon-lan-loopback@feed.socket")
	f.put(t, addonSocketUnit("maru-addon-lan"), "# old socket\n", 0o644)
	f.put(t, addonServiceUnit("maru-addon"), "# old service\n", 0o644)
	status := `{"format":1,"updatedAt":"2026-10-17T00:00:00Z","digest":"abc","backends":[],"purged":[]}` + "\n"
	f.put(t, config.AddonSyncStatusFile, status, 0o644)
	c, out, _ := newTestContext(t, f, prompt.Fail{}, "--service-install", "--upgrade")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	var addonCmds []string
	for _, l := range f.log {
		if strings.Contains(l, "maru-addon") {
			addonCmds = append(addonCmds, l)
		}
	}
	want := []string{
		"systemctl enable --now maru-addons.path",
		// 기록 순서(id 순), 그다음 링크에만 있는 것(이름 순)
		"systemctl try-restart maru-addon@notes.service",
		"systemctl try-restart maru-addon-lan@weather.socket",
		"systemctl try-restart maru-addon-lan@radio.socket",
	}
	if !slices.Equal(addonCmds, want) {
		t.Errorf("addon commands %q", addonCmds)
	}
	if got, _ := f.read(t, config.AddonSyncStatusFile); got != status {
		t.Errorf("status overwritten: %s", got)
	}
	// 바뀐 템플릿은 다시 골든과 같다.
	for _, p := range append(addonUnitFiles(), unitFile) {
		want, err := os.ReadFile(filepath.Join(goldenDir("helper-addon-backends"), "want", p))
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := f.read(t, p); got != string(want) {
			t.Errorf("%s differs:\n%s", p, got)
		}
	}
}

// 끄면 경로 유닛을 멈추고, 기록과 sockets.target.wants의 링크에 있는 인스턴스를 모두 끄고, 유닛과 동기화·소켓
// 디렉터리, 남은 요청을 지운다. 백엔드 데이터는 남긴다. 대시보드에서 업그레이드도 꺼져 있으면 설치기 복사본도 지운다.
func TestAddonBackendsRemoved(t *testing.T) {
	f := addonFake(t)
	putAddonRecord(t, f, addonSyncRecord{Backends: []addonRecordBackend{
		{ID: "weather", Unit: "maru-addon-lan", Rev: "1.0.0-0123abcd", Instance: instA},
	}, Data: []addonRecordData{{ID: "old", Instance: instB}}})
	putWants(t, f, "maru-addon-lan@weather.socket", "maru-addon@stale.socket", "maru-addon@Bad_ID.socket", "maru-addon-wan@x.socket")
	f.put(t, addonWantsDir+"/other.socket", "", 0o644)
	f.put(t, addonSyncRequest, config.AddonSyncRequestBody, 0o600)
	f.put(t, config.AddonSocketDir+"/weather.sock", "", 0o600)
	f.put(t, config.AddonHostDir+"/weather/host.sock", "", 0o666)
	f.put(t, addonPrivateDataDir+"/weather/data.json", "{}", 0o600)
	c, out, _ := newTestContext(t, f, prompt.Fail{}, "--service-install", "--upgrade", "--no-addon-backends")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	want := []string{
		"systemctl is-active --quiet maru.service",
		"install -o root -g root -m 0755 /repo/bin/maru /usr/local/bin/maru",
		"install -o root -g root -m 0755 /repo/bin/maru-helper /usr/local/bin/maru-helper",
		"systemctl stop maru-addons.path",
		"systemctl disable maru-addons.path",
		"systemctl stop maru-addon-lan@weather.socket maru-addon-lan@weather.service",
		"systemctl disable maru-addon-lan@weather.socket",
		"systemctl stop maru-addon@stale.socket maru-addon@stale.service",
		"systemctl disable maru-addon@stale.socket",
		addonPatternStop,
		"systemctl daemon-reload",
		"systemctl daemon-reload",
	}
	if len(f.log) < len(want) || !slices.Equal(f.log[:len(want)], want) {
		t.Errorf("log %q", f.log)
	}
	for _, p := range append(addonUnitFiles(), addonSyncRequest, upgradeInstallerDst, config.AddonSyncStatusFile) {
		if _, ok := f.read(t, p); ok {
			t.Errorf("%s not removed", p)
		}
	}
	for _, d := range []string{config.AddonSyncDir, config.AddonSocketDir, config.AddonHostDir, upgradeInstallerDir} {
		if _, err := os.Lstat(f.path(d)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s not removed (%v)", d, err)
		}
	}
	for _, n := range []string{"maru-addon-lan@weather.socket", "maru-addon@stale.socket"} {
		if lexists(f, addonWantsDir+"/"+n) {
			t.Errorf("link %s not removed", n)
		}
	}
	for _, n := range []string{"maru-addon@Bad_ID.socket", "maru-addon-wan@x.socket", "other.socket"} {
		if !lexists(f, addonWantsDir+"/"+n) {
			t.Errorf("unrelated %s removed", n)
		}
	}
	if _, ok := f.read(t, addonPrivateDataDir+"/weather/data.json"); !ok {
		t.Error("backend data removed")
	}
	compareTree(t, f, filepath.Join(goldenDir("helper-loopback"), "want"))
}

// 요청 자리에 일반 파일이 아닌 것(maru가 만든 비어 있지 않은 디렉터리, 심볼릭 링크)이 있어도 끄기는 끝까지 가고, 링크는
// 따라가지 않는다(동기화의 takeFile과 같다).
func TestAddonBackendsRemovedRequestNotFile(t *testing.T) {
	for name, put := range map[string]func(t *testing.T, f *fakeSystem){
		"directory": func(t *testing.T, f *fakeSystem) { f.put(t, addonSyncRequest+"/inner/x", "x", 0o644) },
		"symlink": func(t *testing.T, f *fakeSystem) {
			f.put(t, "/target.json", "keep", 0o644)
			if err := os.MkdirAll(filepath.Dir(f.path(addonSyncRequest)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(f.path("/target.json"), f.path(addonSyncRequest)); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := addonFake(t)
			put(t, f)
			c, out, _ := newTestContext(t, f, prompt.Fail{})
			c.IsRoot = true
			if err := removeAddonBackends(c); err != nil {
				t.Fatalf("removeAddonBackends: %v\n%s", err, out)
			}
			if lexists(f, addonSyncRequest) {
				t.Error("request not removed")
			}
			if !slices.Contains(f.log, "systemctl daemon-reload") {
				t.Errorf("log %q", f.log)
			}
			for _, d := range []string{config.AddonSyncDir, config.AddonSocketDir, config.AddonHostDir} {
				if lexists(f, d) {
					t.Errorf("%s not removed", d)
				}
			}
			if name == "symlink" {
				if got, ok := f.read(t, "/target.json"); !ok || got != "keep" {
					t.Errorf("link target changed: %q %v", got, ok)
				}
			}
		})
	}
}

// 기록을 읽지 못해도 sockets.target.wants의 링크로 찾은 인스턴스는 다시 시작하고 끈다(경고한다). 일반 사용자의 dry-run은
// root 0600 기록을 읽지 못하는 것이 정상이라 경고하지 않는다.
func TestAddonInstancesWithoutRecord(t *testing.T) {
	f := addonFake(t)
	f.put(t, config.AddonSyncRecordFile, "not json", 0o600)
	putWants(t, f, "maru-addon-lan@radio.socket")
	c, out, _ := newTestContext(t, f, prompt.Fail{})
	c.IsRoot = true
	c.addonChanged = map[string]addonTemplateChange{"maru-addon-lan": {Socket: true}}
	restartAddonInstances(c)
	if !slices.Equal(f.log, []string{"systemctl try-restart maru-addon-lan@radio.socket"}) {
		t.Errorf("log %q", f.log)
	}
	if !strings.Contains(out.String(), "warning: cannot read "+config.AddonSyncRecordFile) {
		t.Errorf("no warning:\n%s", out.String())
	}
	// 템플릿이 바뀌지 않았으면 기록을 읽지도 않는다.
	f.log = nil
	c2, out2, _ := newTestContext(t, f, prompt.Fail{})
	c2.addonChanged = map[string]addonTemplateChange{}
	restartAddonInstances(c2)
	if len(f.log) != 0 || out2.String() != "" {
		t.Errorf("unchanged templates: log %q out %q", f.log, out2.String())
	}

	if err := os.Chmod(f.path(config.AddonSyncRecordFile), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(f.path(config.AddonSyncRecordFile)); err == nil {
		t.Skip("running as root: the record stays readable")
	}
	var dout testOut
	sys := NewDryRunSystem(f, &dout, "en")
	c3, _, _ := newTestContext(t, sys, prompt.Fail{})
	c3.Out, c3.Err = &dout, &dout
	if got := knownAddonInstances(c3); !slices.Equal(got, []addonInstance{{Unit: "maru-addon-lan", ID: "radio"}}) {
		t.Errorf("dry-run instances %v", got)
	}
	if strings.Contains(dout.String(), "warning") {
		t.Errorf("dry-run warned:\n%s", dout.String())
	}
}

// 아무것도 설치하지 않았으면 끄는 단계는 명령도 파일도 만들지 않는다(동기화 디렉터리를 만들면 다음 업그레이드가 켜진
// 것으로 읽는다). 데이터가 없으면 묻지 않는다.
func TestAddonBackendsNothingInstalled(t *testing.T) {
	f := newFake(t)
	c, _, _ := newTestContext(t, f, prompt.Fail{})
	c.IsRoot = true
	for _, step := range []func(*Context) error{removeAddonBackends, removeInstallerCopy, stepRemoveAddonData} {
		if err := step(c); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.log) != 0 {
		t.Errorf("log %q", f.log)
	}
	if dirExists(f, config.AddonSyncDir) {
		t.Error("sync dir created")
	}
}

// 설치기 복사본은 대시보드에서 업그레이드와 애드온 백엔드가 함께 쓴다. 하나만 끄면 남고, 둘 다 끄면 지운다.
func TestInstallerCopyShared(t *testing.T) {
	f := webUpgradeFake(t)
	run := func(args ...string) {
		t.Helper()
		c, out, _ := newTestContext(t, f, prompt.Fail{}, append([]string{"--service-install", "--upgrade"}, args...)...)
		c.IsRoot = true
		if err := Run(c); err != nil {
			t.Fatalf("Run %q: %v\n%s", args, err, out)
		}
	}
	run("--addon-backends")
	if _, ok := f.read(t, config.AddonSyncStatusFile); !ok {
		t.Fatal("addon backends not turned on")
	}
	run("--no-web-upgrade")
	if _, ok := f.read(t, config.UpgradeEnvFile); ok {
		t.Error("web upgrade not turned off")
	}
	if got, ok := f.read(t, upgradeInstallerDst); !ok || got != "#!/bin/sh\n" {
		t.Errorf("installer copy %q %v", got, ok)
	}
	run("--no-addon-backends")
	if _, err := os.Lstat(f.path(upgradeInstallerDir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s not removed (%v)", upgradeInstallerDir, err)
	}
}

// 제거: 백엔드 유닛은 maru.service를 지우는 files 단계보다 먼저 끄고, 백엔드 데이터는 따로 확인을 받은 뒤에만 지운다.
func TestAddonBackendsUninstall(t *testing.T) {
	for _, deleteData := range []bool{true, false} {
		t.Run(map[bool]string{true: "delete-data", false: "keep-data"}[deleteData], func(t *testing.T) {
			f := addonFake(t)
			f.unitFiles["maru.service"] = true
			putAddonRecord(t, f, addonSyncRecord{Backends: []addonRecordBackend{
				{ID: "weather", Unit: "maru-addon-lan", Rev: "1.0.0-0123abcd", Instance: instA},
			}})
			putWants(t, f, "maru-addon-lan@weather.socket")
			f.put(t, addonPrivateDataDir+"/weather/data.json", "{}", 0o600)
			if err := os.MkdirAll(f.path(addonDataDir), 0o755); err != nil {
				t.Fatal(err)
			}
			f.put(t, stateDir+"/site.json", "{}", 0o600)
			answer := map[bool]string{true: "y", false: "n"}[deleteData]
			script := &prompt.Script{Answers: map[string]string{"uninstall": "y", "uninstall-state-dir": "n", "uninstall-addon-data": answer, "uninstall-user": "n"}}
			c, out, _ := newTestContext(t, f, script, "--uninstall")
			c.IsRoot = true
			if err := Run(c); err != nil {
				t.Fatalf("Run: %v\n%s", err, out)
			}
			want := []string{
				"systemctl list-unit-files maru.service",
				"systemctl disable --now maru.service",
				"systemctl stop maru-addons.path",
				"systemctl disable maru-addons.path",
				"systemctl stop maru-addon-lan@weather.socket maru-addon-lan@weather.service",
				"systemctl disable maru-addon-lan@weather.socket",
				addonPatternStop,
				"systemctl daemon-reload",
				"systemctl disable --now maru-helper.socket maru-helper.service",
				"systemctl daemon-reload",
				"systemctl daemon-reload",
			}
			if !slices.Equal(f.log, want) {
				t.Errorf("log %q", f.log)
			}
			if !slices.Equal(script.Asked, []string{"uninstall", "uninstall-state-dir", "uninstall-addon-data", "uninstall-user"}) {
				t.Errorf("asked %v", script.Asked)
			}
			for _, p := range append(slices.Clone(configFiles), upgradeInstallerDst) {
				if _, ok := f.read(t, p); ok {
					t.Errorf("%s not removed", p)
				}
			}
			for _, d := range []string{config.AddonSyncDir, upgradeInstallerDir} {
				if dirExists(f, d) {
					t.Errorf("%s not removed", d)
				}
			}
			_, kept := f.read(t, addonPrivateDataDir+"/weather/data.json")
			if kept == deleteData || dirExists(f, addonDataDir) == deleteData {
				t.Errorf("data kept=%v after answering %q", kept, answer)
			}
		})
	}
}

// dry-run으로 켜면 쓸 유닛과 명령을 보여 주기만 하고 파일·잠금 파일을 만들지 않는다.
func TestAddonBackendsDryRun(t *testing.T) {
	f := installedFake(t)
	var out testOut
	sys := NewDryRunSystem(f, &out, "en")
	c, _, _ := newTestContext(t, sys, prompt.Fail{}, "--service-install", "--upgrade", "--dry-run", "--addon-backends")
	c.Out, c.Err = &out, &out
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"[dry-run] write /etc/systemd/system/maru-addon-lan@.service (0644, new file):",
		"[dry-run] write " + config.AddonSyncStatusFile + " (0644, new file):",
		"[dry-run] run: install -o root -g root -m 0755 /repo/bin/maru-install /usr/local/lib/maru/maru-install",
		"[dry-run] run: systemctl enable maru-addons.path",
		"    + ExecStartPre=+/usr/bin/install -d -o root -g root -m 0755 /run/maru-addon-host",
		"    + ReadWritePaths=-/run/maru-addon-host",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output misses %q:\n%s", want, out.String())
		}
	}
	if _, err := os.Lstat(f.path(config.AddonSyncDir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("dry run created %s", config.AddonSyncDir)
	}
}

// 동기화 기록은 id 순으로 0600으로 쓰고, 읽을 때 형식·id·단계·rev·인스턴스·개수·모르는 필드를 엄격하게 본다.
func TestAddonSyncRecord(t *testing.T) {
	f := newFake(t)
	if r, err := readAddonSyncRecord(f); err != nil || r.Format != addonRecordFormat || r.Backends == nil || r.Data == nil {
		t.Fatalf("missing record: %+v %v", r, err)
	}
	rec := addonSyncRecord{
		Backends: []addonRecordBackend{
			{ID: "weather", Unit: "maru-addon-lan-loopback", Rev: "1.0.0-0123abcd", Instance: instA},
			{ID: "clock", Unit: "maru-addon", Rev: "dev-89abcdef", Instance: instB},
		},
		Data: []addonRecordData{{ID: "old", Instance: instB}},
	}
	if err := writeAddonSyncRecord(f, rec); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(f.path(config.AddonSyncRecordFile))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("record mode %v %v", fi, err)
	}
	got, err := readAddonSyncRecord(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Backends) != 2 || got.Backends[0].ID != "clock" || got.Backends[1] != rec.Backends[0] || len(got.Data) != 1 || got.Data[0] != rec.Data[0] {
		t.Errorf("roundtrip %+v", got)
	}

	ok := `{"format":1,"backends":[{"id":"a","unit":"maru-addon","rev":"1-0123abcd","instance":"` + instA + `"}],"data":[]}`
	if _, err := parseAddonSyncRecord([]byte(ok + "\n")); err != nil {
		t.Fatalf("valid record: %v", err)
	}
	backend := func(id, unit, rev, inst string) string {
		return `{"id":"` + id + `","unit":"` + unit + `","rev":"` + rev + `","instance":"` + inst + `"}`
	}
	many := make([]string, config.AddonBackendsMax+1)
	for i := range many {
		many[i] = backend("a"+strings.Repeat("b", i), "maru-addon", "1-0123abcd", instA)
	}
	for name, bad := range map[string]string{
		"format":        `{"format":2,"backends":[],"data":[]}`,
		"unknown field": `{"format":1,"backends":[],"data":[],"extra":1}`,
		"trailing":      ok + `{}`,
		"trailing ]":    ok + `]`,
		"trailing }":    ok + `}`,
		"injection id":  `{"format":1,"backends":[` + backend("a;rm -rf", "maru-addon", "1-0123abcd", instA) + `],"data":[]}`,
		"option id":     `{"format":1,"backends":[` + backend("--now", "maru-addon", "1-0123abcd", instA) + `],"data":[]}`,
		"unit":          `{"format":1,"backends":[` + backend("a", "maru-addon-wan", "1-0123abcd", instA) + `],"data":[]}`,
		"rev":           `{"format":1,"backends":[` + backend("a", "maru-addon", "../x", instA) + `],"data":[]}`,
		"instance":      `{"format":1,"backends":[` + backend("a", "maru-addon", "1-0123abcd", "ABC") + `],"data":[]}`,
		"duplicate":     `{"format":1,"backends":[` + backend("a", "maru-addon", "1-0123abcd", instA) + `,` + backend("a", "maru-addon-lan", "1-0123abcd", instA) + `],"data":[]}`,
		"both lists":    `{"format":1,"backends":[` + backend("a", "maru-addon", "1-0123abcd", instA) + `],"data":[{"id":"a","instance":"` + instB + `"}]}`,
		"data instance": `{"format":1,"backends":[],"data":[{"id":"a","instance":"x"}]}`,
		"too many":      `{"format":1,"backends":[` + strings.Join(many, ",") + `],"data":[]}`,
		"too large":     `{"format":1,"backends":[],"data":[]}` + strings.Repeat(" ", addonRecordMaxBytes),
	} {
		if _, err := parseAddonSyncRecord([]byte(bad)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := writeAddonSyncRecord(f, addonSyncRecord{Backends: []addonRecordBackend{{ID: "Bad", Unit: "maru-addon", Rev: "1-0123abcd", Instance: instA}}}); err == nil {
		t.Error("invalid record written")
	}
	// 일반 파일이 아니면 읽지 않는다.
	if err := os.Remove(f.path(config.AddonSyncRecordFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(f.path(config.AddonSyncRecordFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readAddonSyncRecord(f); err == nil {
		t.Error("directory read as record")
	}
}

// 동기화 잠금은 배타적이고, 기다리는 동안 잠금 파일이 지워지면 새 파일을 잡는다. 동기화 디렉터리를 만들지 않는다.
func TestAddonSyncLock(t *testing.T) {
	f := newFake(t)
	if _, err := f.Lock(config.AddonSyncLockFile, 10*time.Millisecond); err == nil {
		t.Fatal("locked without the sync directory")
	}
	if dirExists(f, config.AddonSyncDir) {
		t.Fatal("lock created the sync directory")
	}
	if err := os.MkdirAll(f.path(config.AddonSyncDir), 0o755); err != nil {
		t.Fatal(err)
	}
	unlockA, err := f.Lock(config.AddonSyncLockFile, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Lock(config.AddonSyncLockFile, 150*time.Millisecond); err == nil || !strings.Contains(err.Error(), "still held") {
		t.Fatalf("second lock: %v", err)
	}
	// 기다리지 않는 잠금은 바로 실패한다(lockAddonSync가 기다린다고 알리기 전에 먼저 해 본다).
	start := time.Now()
	if _, err := f.Lock(config.AddonSyncLockFile, 0); err == nil || time.Since(start) > 50*time.Millisecond {
		t.Fatalf("try lock: %v after %s", err, time.Since(start))
	}
	type result struct {
		unlock func()
		err    error
	}
	done := make(chan result, 1)
	go func() {
		u, err := f.Lock(config.AddonSyncLockFile, 5*time.Second)
		done <- result{u, err}
	}()
	time.Sleep(150 * time.Millisecond)
	// 기다리는 쪽이 연 파일을 지운 뒤 잠금을 푼다. 기다리던 쪽은 지금 경로의 새 파일을 잡아야 한다.
	if err := os.Remove(f.path(config.AddonSyncLockFile)); err != nil {
		t.Fatal(err)
	}
	unlockA()
	r := <-done
	if r.err != nil {
		t.Fatal(r.err)
	}
	if _, err := f.Lock(config.AddonSyncLockFile, 150*time.Millisecond); err == nil {
		t.Error("the waiter holds a stale lock file")
	}
	r.unlock()
	unlockC, err := f.Lock(config.AddonSyncLockFile, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	unlockC()
	fi, err := os.Stat(f.path(config.AddonSyncLockFile))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("lock file %v %v", fi, err)
	}
}
