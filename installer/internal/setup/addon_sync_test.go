package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"maru/backend/config"
	"maru/installer/internal/prompt"
)

// 설치기가 애드온 백엔드를 켤 때 둔 빈 결과다.
const seedStatus = `{"format":1,"updatedAt":"2026-10-01T00:00:00Z","digest":"","backends":[],"purged":[]}` + "\n"

// syncFake는 애드온 백엔드를 켠 설치(동기화 디렉터리와 빈 결과)다. systemctl의 enable·disable·clean이 링크와 데이터를
// 바꾼다(fakeSystem.units).
func syncFake(t *testing.T) *fakeSystem {
	t.Helper()
	f := newFake(t)
	f.units = true
	f.put(t, config.AddonSyncStatusFile, seedStatus, 0o644)
	return f
}

// runSync는 root로 동기화를 한 번 실행하고 그동안 실행한 명령을 돌려준다.
func runSync(t *testing.T, f *fakeSystem) []string {
	t.Helper()
	f.log = nil
	c, out, _ := newTestContext(t, f, prompt.Fail{}, "--addon-sync")
	c.IsRoot = true
	if err := syncAddonBackends(c, time.Second); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	return f.log
}

// ab는 원하는 상태의 백엔드 항목이다. rev는 버전 1.0.0과 hash(8자리 16진수)로 만든다.
func ab(id, hash, instance string, network ...string) config.AddonBackend {
	if network == nil {
		network = []string{}
	}
	return config.AddonBackend{ID: id, Version: "1.0.0", Rev: "1.0.0-" + hash, Instance: instance, Network: network}
}

// putDesired는 maru처럼 원하는 상태 파일을 쓰고 그 digest를 돌려준다.
func putDesired(t *testing.T, f *fakeSystem, backends []config.AddonBackend, purge ...string) string {
	t.Helper()
	if backends == nil {
		backends = []config.AddonBackend{}
	}
	if purge == nil {
		purge = []string{}
	}
	data, err := json.MarshalIndent(config.AddonBackends{Format: config.AddonBackendsFormat, Backends: backends, Purge: purge}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	f.put(t, addonBackendsFile, string(data), 0o600)
	return config.AddonBackendsDigest(data)
}

// putData는 systemd가 DynamicUser의 StateDirectory로 두는 백엔드 데이터를 만든다.
func putData(t *testing.T, f *fakeSystem, id string) {
	t.Helper()
	f.put(t, addonPrivateDataDir+"/"+id+"/data.json", "{}", 0o600)
	if err := os.MkdirAll(f.path(addonDataDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../private/"+config.AddonStateDirName+"/"+id, f.path(addonDataDir+"/"+id)); err != nil && !errors.Is(err, fs.ErrExist) {
		t.Fatal(err)
	}
}

func hasData(f *fakeSystem, id string) bool {
	return lexists(f, addonPrivateDataDir+"/"+id) || lexists(f, addonDataDir+"/"+id)
}

// syncStatus는 결과 파일을 maru처럼 읽는다.
func syncStatus(t *testing.T, f *fakeSystem) config.AddonSyncStatus {
	t.Helper()
	data, ok := f.read(t, config.AddonSyncStatusFile)
	if !ok {
		t.Fatal("no sync status")
	}
	st, err := config.ParseAddonSyncStatus([]byte(data))
	if err != nil {
		t.Fatalf("status: %v\n%s", err, data)
	}
	return st
}

// syncRecord는 동기화 기록이다.
func syncRecord(t *testing.T, f *fakeSystem) addonSyncRecord {
	t.Helper()
	rec, err := readAddonSyncRecord(f)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func enabledStatus(id, hash, unit string) config.AddonBackendStatus {
	return config.AddonBackendStatus{ID: id, Rev: "1.0.0-" + hash, Unit: unit, State: config.AddonBackendEnabled}
}

// isActive는 켜 둔 백엔드의 소켓이 떠 있는지 보는 읽기 전용 확인이다.
func isActive(socket string) string { return "systemctl is-active --quiet " + socket }

// stopAll은 데이터를 지우기 전에 그 id의 모든 네트워크 단계 인스턴스를 멈추는 명령이다(단계 목록을 따로 적어 둔다).
func stopAll(id string) string {
	var names []string
	for _, unit := range []string{"maru-addon", "maru-addon-lan", "maru-addon-loopback", "maru-addon-lan-loopback"} {
		names = append(names, unit+"@"+id+".socket", unit+"@"+id+".service")
	}
	return "systemctl stop " + strings.Join(names, " ")
}

func wantLog(t *testing.T, got []string, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if got == nil {
		got = []string{}
	}
	if !slices.Equal(got, want) {
		t.Errorf("commands\n got %q\nwant %q", got, want)
	}
}

// 새 id는 그 네트워크 단계의 소켓을 켠다(id 순). 결과와 기록을 쓰고 요청을 지운다. 같은 원하는 상태로 다시 실행하면
// 소켓이 떠 있는지만 보고 결과·기록도 그대로다.
func TestAddonSyncEnable(t *testing.T) {
	f := syncFake(t)
	f.put(t, addonSyncRequest, config.AddonSyncRequestBody, 0o600)
	digest := putDesired(t, f, []config.AddonBackend{
		ab("weather", "0123abcd", instA),
		ab("radio", "89abcdef", instB, config.AddonNetworkLAN, config.AddonNetworkLoopback),
	})
	wantLog(t, runSync(t, f),
		"systemctl enable --now maru-addon-lan-loopback@radio.socket",
		"systemctl enable --now maru-addon@weather.socket")
	if _, ok := f.read(t, addonSyncRequest); ok {
		t.Error("request not removed")
	}
	want := config.AddonSyncStatus{
		Format:    config.AddonSyncStatusFormat,
		UpdatedAt: f.Now().UTC(),
		Digest:    digest,
		Backends: []config.AddonBackendStatus{
			enabledStatus("radio", "89abcdef", "maru-addon-lan-loopback@radio.service"),
			enabledStatus("weather", "0123abcd", "maru-addon@weather.service"),
		},
		Purged: []string{},
	}
	if got := syncStatus(t, f); !reflect.DeepEqual(got, want) {
		t.Errorf("status\n got %+v\nwant %+v", got, want)
	}
	for path, mode := range map[string]fs.FileMode{config.AddonSyncStatusFile: 0o644, config.AddonSyncRecordFile: 0o600} {
		if fi, err := os.Stat(f.path(path)); err != nil || fi.Mode().Perm() != mode {
			t.Errorf("%s: %v %v", path, fi, err)
		}
	}
	wantRec := addonSyncRecord{Format: addonRecordFormat, Backends: []addonRecordBackend{
		{ID: "radio", Unit: "maru-addon-lan-loopback", Rev: "1.0.0-89abcdef", Instance: instB},
		{ID: "weather", Unit: "maru-addon", Rev: "1.0.0-0123abcd", Instance: instA},
	}, Data: []addonRecordData{}}
	if got := syncRecord(t, f); !reflect.DeepEqual(got, wantRec) {
		t.Errorf("record %+v", got)
	}
	if !lexists(f, addonWantsDir+"/maru-addon@weather.socket") {
		t.Error("socket not enabled")
	}

	status, _ := f.read(t, config.AddonSyncStatusFile)
	record, _ := f.read(t, config.AddonSyncRecordFile)
	wantLog(t, runSync(t, f), isActive("maru-addon-lan-loopback@radio.socket"), isActive("maru-addon@weather.socket"))
	if got, _ := f.read(t, config.AddonSyncStatusFile); got != status {
		t.Errorf("status changed on the second run:\n%s", got)
	}
	if got, _ := f.read(t, config.AddonSyncRecordFile); got != record {
		t.Errorf("record changed on the second run:\n%s", got)
	}
}

// 켜 둔 백엔드가 바뀌는 경우마다 정해진 명령만 실행한다. 업데이트·네트워크 변경은 데이터를 남기고, 설치 인스턴스가
// 바뀌면 이전 설치의 데이터를 지운 뒤 켠다. 원하는 상태에서 빠지면 끄고 데이터와 그 설치 인스턴스를 남긴다.
func TestAddonSyncChanges(t *testing.T) {
	f := syncFake(t)
	putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
	runSync(t, f)
	putData(t, f, "weather")

	steps := []struct {
		name    string
		desired []config.AddonBackend
		log     []string
		data    bool
		rec     addonSyncRecord
	}{{
		name:    "update",
		desired: []config.AddonBackend{ab("weather", "4567cdef", instA)},
		log:     []string{isActive("maru-addon@weather.socket"), "systemctl try-restart maru-addon@weather.service"},
		data:    true,
		rec:     addonSyncRecord{Backends: []addonRecordBackend{{ID: "weather", Unit: "maru-addon", Rev: "1.0.0-4567cdef", Instance: instA}}},
	}, {
		name:    "network",
		desired: []config.AddonBackend{ab("weather", "4567cdef", instA, config.AddonNetworkLoopback)},
		log: []string{
			"systemctl stop maru-addon@weather.socket maru-addon@weather.service",
			"systemctl disable maru-addon@weather.socket",
			"systemctl enable --now maru-addon-loopback@weather.socket",
		},
		data: true,
		rec:  addonSyncRecord{Backends: []addonRecordBackend{{ID: "weather", Unit: "maru-addon-loopback", Rev: "1.0.0-4567cdef", Instance: instA}}},
	}, {
		// 다시 설치했다(설치 인스턴스가 바뀜). 네트워크도 함께 바뀌어도 이전 단계를 끄고 데이터를 지운 뒤 새 단계를 켠다.
		name:    "reinstalled",
		desired: []config.AddonBackend{ab("weather", "4567cdef", instB, config.AddonNetworkLAN)},
		log: []string{
			"systemctl stop maru-addon-loopback@weather.socket maru-addon-loopback@weather.service",
			"systemctl disable maru-addon-loopback@weather.socket",
			stopAll("weather"),
			"systemctl clean --what=state maru-addon@weather.service",
			"systemctl enable --now maru-addon-lan@weather.socket",
		},
		data: false,
		rec:  addonSyncRecord{Backends: []addonRecordBackend{{ID: "weather", Unit: "maru-addon-lan", Rev: "1.0.0-4567cdef", Instance: instB}}},
	}, {
		name: "removed",
		log: []string{
			"systemctl stop maru-addon-lan@weather.socket maru-addon-lan@weather.service",
			"systemctl disable maru-addon-lan@weather.socket",
		},
		data: true,
		rec:  addonSyncRecord{Data: []addonRecordData{{ID: "weather", Instance: instB}}},
	}, {
		name:    "same installation again",
		desired: []config.AddonBackend{ab("weather", "4567cdef", instB)},
		log:     []string{"systemctl enable --now maru-addon@weather.socket"},
		data:    true,
		rec:     addonSyncRecord{Backends: []addonRecordBackend{{ID: "weather", Unit: "maru-addon", Rev: "1.0.0-4567cdef", Instance: instB}}},
	}, {
		name: "removed again",
		log: []string{
			"systemctl stop maru-addon@weather.socket maru-addon@weather.service",
			"systemctl disable maru-addon@weather.socket",
		},
		data: true,
		rec:  addonSyncRecord{Data: []addonRecordData{{ID: "weather", Instance: instB}}},
	}, {
		// 남긴 데이터와 다른 설치(다른 작성자의 같은 id일 수도 있다)는 그 데이터를 읽지 못한다.
		name:    "another installation",
		desired: []config.AddonBackend{ab("weather", "4567cdef", instA)},
		log: []string{
			stopAll("weather"),
			"systemctl clean --what=state maru-addon@weather.service",
			"systemctl enable --now maru-addon@weather.socket",
		},
		data: false,
		rec:  addonSyncRecord{Backends: []addonRecordBackend{{ID: "weather", Unit: "maru-addon", Rev: "1.0.0-4567cdef", Instance: instA}}},
	}}
	for _, step := range steps {
		putDesired(t, f, step.desired)
		wantLog(t, runSync(t, f), step.log...)
		if hasData(f, "weather") != step.data {
			t.Errorf("%s: data present %v", step.name, !step.data)
		}
		want := step.rec
		want.Format = addonRecordFormat
		if want.Backends == nil {
			want.Backends = []addonRecordBackend{}
		}
		if want.Data == nil {
			want.Data = []addonRecordData{}
		}
		if got := syncRecord(t, f); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: record %+v", step.name, got)
		}
		st := syncStatus(t, f)
		if st.Error != "" || len(st.Backends) != len(step.desired) {
			t.Errorf("%s: status %+v", step.name, st)
		}
		for _, b := range st.Backends {
			if b.State != config.AddonBackendEnabled {
				t.Errorf("%s: %+v", step.name, b)
			}
		}
		// 백엔드가 다시 데이터를 만든다.
		if !step.data && len(step.desired) > 0 {
			putData(t, f, "weather")
		}
	}
}

// 기록에 없는 데이터는 다른 설치의 것이다. 그 id를 켤 때만 먼저 지우고, 켜지 않는 id의 데이터는 그대로 둔다.
func TestAddonSyncUnrecordedData(t *testing.T) {
	f := syncFake(t)
	putData(t, f, "weather")
	putData(t, f, "radio")
	putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
	wantLog(t, runSync(t, f),
		"systemctl stop maru-addon@weather.socket maru-addon@weather.service maru-addon-lan@weather.socket maru-addon-lan@weather.service "+
			"maru-addon-loopback@weather.socket maru-addon-loopback@weather.service maru-addon-lan-loopback@weather.socket maru-addon-lan-loopback@weather.service",
		"systemctl clean --what=state maru-addon@weather.service",
		"systemctl enable --now maru-addon@weather.socket")
	if hasData(f, "weather") || !hasData(f, "radio") {
		t.Errorf("data weather=%v radio=%v", hasData(f, "weather"), hasData(f, "radio"))
	}
}

// purge는 backends에 없는 id의 데이터를 지운다(남긴 데이터 항목이 없어도). 데이터가 없으면 명령 없이 지운 것으로 본다.
// backends에 있는 id는 무시한다.
func TestAddonSyncPurge(t *testing.T) {
	f := syncFake(t)
	putAddonRecord(t, f, addonSyncRecord{Data: []addonRecordData{{ID: "old", Instance: instA}, {ID: "kept", Instance: instB}}})
	putData(t, f, "old")
	putData(t, f, "kept")
	putData(t, f, "stray")
	putDesired(t, f, []config.AddonBackend{ab("live", "0123abcd", instA)}, "stray", "old", "live", "gone")
	wantLog(t, runSync(t, f),
		"systemctl enable --now maru-addon@live.socket",
		stopAll("old"),
		"systemctl clean --what=state maru-addon@old.service",
		stopAll("stray"),
		"systemctl clean --what=state maru-addon@stray.service")
	st := syncStatus(t, f)
	if !slices.Equal(st.Purged, []string{"gone", "old", "stray"}) {
		t.Errorf("purged %q", st.Purged)
	}
	if hasData(f, "old") || hasData(f, "stray") || !hasData(f, "kept") {
		t.Error("purged the wrong data")
	}
	rec := syncRecord(t, f)
	if !reflect.DeepEqual(rec.Data, []addonRecordData{{ID: "kept", Instance: instB}}) || len(rec.Backends) != 1 {
		t.Errorf("record %+v", rec)
	}
	// 같은 purge가 남아 있어도(maru가 결과를 아직 반영하지 않음) 다시 실행하면 명령 없이 같은 결과다.
	wantLog(t, runSync(t, f), isActive("maru-addon@live.socket"))
	if st := syncStatus(t, f); !slices.Equal(st.Purged, []string{"gone", "old", "stray"}) {
		t.Errorf("purged on the second run %q", st.Purged)
	}

	// 원하는 상태에서 빠지면서 purge된 id는 끈 뒤 지운다.
	putDesired(t, f, nil, "live")
	putData(t, f, "live")
	wantLog(t, runSync(t, f),
		"systemctl stop maru-addon@live.socket maru-addon@live.service",
		"systemctl disable maru-addon@live.socket",
		stopAll("live"),
		"systemctl clean --what=state maru-addon@live.service")
	if st := syncStatus(t, f); !slices.Equal(st.Purged, []string{"live"}) || hasData(f, "live") {
		t.Errorf("purged %q", st.Purged)
	}
}

// 기록과 켜진 소켓 링크를 맞춘다. 기록에 없는 켜진 인스턴스는 끄고(그 데이터는 다른 설치의 것으로 본다), 링크가 없는
// 기록 항목은 멈춘 뒤(링크만 지우고 돌 수 있다) 같은 설치의 데이터를 둔 채 다시 켠다.
func TestAddonSyncLinks(t *testing.T) {
	f := syncFake(t)
	putWants(t, f, "maru-addon-lan@ghost.socket", "maru-addon@other.socket")
	putData(t, f, "ghost")
	putAddonRecord(t, f, addonSyncRecord{Backends: []addonRecordBackend{{ID: "weather", Unit: "maru-addon", Rev: "1.0.0-0123abcd", Instance: instA}}})
	putData(t, f, "weather")
	putDesired(t, f, []config.AddonBackend{ab("ghost", "0123abcd", instB, config.AddonNetworkLAN), ab("weather", "0123abcd", instA)})
	wantLog(t, runSync(t, f),
		"systemctl stop maru-addon-lan@ghost.socket maru-addon-lan@ghost.service",
		"systemctl disable maru-addon-lan@ghost.socket",
		"systemctl stop maru-addon@other.socket maru-addon@other.service",
		"systemctl disable maru-addon@other.socket",
		"systemctl stop maru-addon@weather.socket maru-addon@weather.service",
		"systemctl disable maru-addon@weather.socket",
		stopAll("ghost"),
		"systemctl clean --what=state maru-addon@ghost.service",
		"systemctl enable --now maru-addon-lan@ghost.socket",
		"systemctl enable --now maru-addon@weather.socket")
	if hasData(f, "ghost") || !hasData(f, "weather") {
		t.Errorf("data ghost=%v weather=%v", hasData(f, "ghost"), hasData(f, "weather"))
	}
	if lexists(f, addonWantsDir+"/maru-addon@other.socket") {
		t.Error("unrecorded instance still enabled")
	}
	wantLog(t, runSync(t, f), isActive("maru-addon-lan@ghost.socket"), isActive("maru-addon@weather.socket"))
}

// 기록·링크와 상관없이 돌고 있는 같은 id의 인스턴스가 그 데이터를 쓰는 채로 지우거나, 새 설치로 기록되지 않는다.
func TestAddonSyncStrayInstances(t *testing.T) {
	// 링크만 지워지고 유닛은 도는 기록 항목(손으로 --now 없이 disable)은 멈춘 뒤 데이터를 남긴 것으로 옮긴다. 다른
	// 설치면 데이터를 지우고 켜고, 원하는 상태에 없으면 멈춘 채 둔다.
	t.Run("recorded without link", func(t *testing.T) {
		f := syncFake(t)
		putAddonRecord(t, f, addonSyncRecord{Backends: []addonRecordBackend{
			{ID: "radio", Unit: "maru-addon", Rev: "1.0.0-0123abcd", Instance: instA},
			{ID: "weather", Unit: "maru-addon-lan", Rev: "1.0.0-0123abcd", Instance: instA},
		}})
		f.active["maru-addon@radio.socket"] = true
		f.active["maru-addon-lan@weather.socket"] = true
		putData(t, f, "radio")
		putData(t, f, "weather")
		putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instB, config.AddonNetworkLAN)})
		wantLog(t, runSync(t, f),
			"systemctl stop maru-addon@radio.socket maru-addon@radio.service",
			"systemctl disable maru-addon@radio.socket",
			"systemctl stop maru-addon-lan@weather.socket maru-addon-lan@weather.service",
			"systemctl disable maru-addon-lan@weather.socket",
			stopAll("weather"),
			"systemctl clean --what=state maru-addon@weather.service",
			"systemctl enable --now maru-addon-lan@weather.socket")
		if f.active["maru-addon@radio.socket"] || hasData(f, "weather") || !hasData(f, "radio") {
			t.Errorf("radio active %v, data weather=%v radio=%v", f.active["maru-addon@radio.socket"], hasData(f, "weather"), hasData(f, "radio"))
		}
		want := addonSyncRecord{Format: addonRecordFormat,
			Backends: []addonRecordBackend{{ID: "weather", Unit: "maru-addon-lan", Rev: "1.0.0-0123abcd", Instance: instB}},
			Data:     []addonRecordData{{ID: "radio", Instance: instA}}}
		if got := syncRecord(t, f); !reflect.DeepEqual(got, want) {
			t.Errorf("record %+v", got)
		}
	})

	// 기록에도 링크에도 없이 손으로 시작한 다른 단계가 데이터를 쓰고 있어도, 지우기 전에 그 id의 모든 단계를 멈춘다.
	t.Run("started by hand", func(t *testing.T) {
		f := syncFake(t)
		f.active["maru-addon-loopback@weather.socket"] = true
		f.active["maru-addon-loopback@weather.service"] = true
		putData(t, f, "weather")
		putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
		wantLog(t, runSync(t, f),
			stopAll("weather"),
			"systemctl clean --what=state maru-addon@weather.service",
			"systemctl enable --now maru-addon@weather.socket")
		if f.active["maru-addon-loopback@weather.socket"] || f.active["maru-addon-loopback@weather.service"] || hasData(f, "weather") {
			t.Errorf("active %v, data %v", f.active, hasData(f, "weather"))
		}
	})

	// 기록과 맞지 않는 인스턴스를 멈추지 못하면 그 id를 켜지도, 데이터를 지우지도 않는다(failed, purge는 미룸). 기록은
	// 그대로라 다음 동기화가 다시 맞춘다.
	t.Run("stop fails", func(t *testing.T) {
		f := syncFake(t)
		putAddonRecord(t, f, addonSyncRecord{Backends: []addonRecordBackend{{ID: "weather", Unit: "maru-addon-lan", Rev: "1.0.0-0123abcd", Instance: instA}}})
		putWants(t, f, "maru-addon-loopback@radio.socket", "maru-addon@clock.socket")
		putData(t, f, "weather")
		putData(t, f, "clock")
		stops := []string{
			"systemctl stop maru-addon-loopback@radio.socket maru-addon-loopback@radio.service",
			"systemctl stop maru-addon@clock.socket maru-addon@clock.service",
			"systemctl stop maru-addon-lan@weather.socket maru-addon-lan@weather.service",
		}
		for _, cmd := range stops {
			f.fail[cmd] = fmt.Errorf("%s: exit status 1", cmd)
		}
		putDesired(t, f, []config.AddonBackend{ab("radio", "0123abcd", instB), ab("weather", "0123abcd", instB, config.AddonNetworkLAN)}, "clock")
		record, _ := f.read(t, config.AddonSyncRecordFile)
		wantLog(t, runSync(t, f), stops...)
		st := syncStatus(t, f)
		if st.Error != "" || len(st.Purged) != 0 || len(st.Backends) != 2 ||
			st.Backends[0].State != config.AddonBackendFailed || !strings.Contains(st.Backends[0].Error, "cannot stop maru-addon-loopback@radio.socket, which is enabled but not recorded") ||
			st.Backends[1].State != config.AddonBackendFailed || !strings.Contains(st.Backends[1].Error, "cannot stop maru-addon-lan@weather.socket, which is recorded but not enabled") {
			t.Errorf("status %+v", st)
		}
		if got, _ := f.read(t, config.AddonSyncRecordFile); got != record || !hasData(f, "weather") || !hasData(f, "clock") {
			t.Errorf("record or data changed:\n%s", got)
		}

		for _, cmd := range stops {
			delete(f.fail, cmd)
		}
		wantLog(t, runSync(t, f),
			stops[0], "systemctl disable maru-addon-loopback@radio.socket",
			stops[1], "systemctl disable maru-addon@clock.socket",
			stops[2], "systemctl disable maru-addon-lan@weather.socket",
			"systemctl enable --now maru-addon@radio.socket",
			stopAll("weather"),
			"systemctl clean --what=state maru-addon@weather.service",
			"systemctl enable --now maru-addon-lan@weather.socket",
			stopAll("clock"),
			"systemctl clean --what=state maru-addon@clock.service")
		if st := syncStatus(t, f); !slices.Equal(st.Purged, []string{"clock"}) || st.Backends[0].State != config.AddonBackendEnabled || st.Backends[1].State != config.AddonBackendEnabled {
			t.Errorf("status after retry %+v", st)
		}
	})
}

// 켜 둔 백엔드의 소켓이 떠 있지 않으면(계속 죽는 백엔드가 시작 한도에 걸려 소켓까지 실패했다) 실패 상태를 지우고 다시
// 시작한다. rev가 바뀌었으면 다시 시작한 소켓이 다음 연결 때 새 실행 파일로 서비스를 시작하므로 try-restart는 하지
// 않는다. 다시 시작하지 못하면 failed이고 기록의 rev는 그대로다.
func TestAddonSyncSocketDown(t *testing.T) {
	socket := "maru-addon@weather.socket"
	reset := "systemctl reset-failed maru-addon@weather.socket maru-addon@weather.service"
	start := "systemctl start " + socket
	f := syncFake(t)
	putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
	runSync(t, f)
	putData(t, f, "weather")

	delete(f.active, socket)
	wantLog(t, runSync(t, f), isActive(socket), reset, start)
	if st := syncStatus(t, f); st.Backends[0].State != config.AddonBackendEnabled || !f.active[socket] {
		t.Errorf("status %+v, active %v", st.Backends, f.active[socket])
	}

	delete(f.active, socket)
	putDesired(t, f, []config.AddonBackend{ab("weather", "4567cdef", instA)})
	wantLog(t, runSync(t, f), isActive(socket), reset, start)
	if rec := syncRecord(t, f); rec.Backends[0].Rev != "1.0.0-4567cdef" {
		t.Errorf("record %+v", rec)
	}

	delete(f.active, socket)
	f.fail[start] = fmt.Errorf("%s: exit status 1", start)
	for _, hash := range []string{"89abcdef", "4567cdef"} {
		putDesired(t, f, []config.AddonBackend{ab("weather", hash, instA)})
		wantLog(t, runSync(t, f), isActive(socket), reset, start)
		if st := syncStatus(t, f); st.Backends[0].State != config.AddonBackendFailed || st.Backends[0].Error != start+": exit status 1" {
			t.Errorf("%s: status %+v", hash, st.Backends)
		}
		if rec := syncRecord(t, f); len(rec.Backends) != 1 || rec.Backends[0].Rev != "1.0.0-4567cdef" || !hasData(f, "weather") {
			t.Errorf("%s: record %+v", hash, rec)
		}
	}

	// reset-failed가 실패해도(실패 상태가 아니었다) 다시 시작되면 켜진 것이다.
	delete(f.fail, start)
	f.fail[reset] = fmt.Errorf("%s: exit status 1", reset)
	wantLog(t, runSync(t, f), isActive(socket), reset, start)
	if st := syncStatus(t, f); st.Backends[0].State != config.AddonBackendEnabled {
		t.Errorf("status %+v", st.Backends)
	}
}

// 남긴 데이터 항목이 상한이어도, 단계를 바꾸려고 잠깐 끈 원하는 id는 원하는 상태에 없는 항목 하나를 밀어내고 적는다(켜기가
// 실패해도 다음 동기화가 같은 설치의 데이터를 지우지 않게). 원하는 상태에서 빠진 id는 상한이면 적지 않는다.
func TestAddonSyncRetainFull(t *testing.T) {
	f := syncFake(t)
	putDesired(t, f, []config.AddonBackend{ab("radio", "0123abcd", instA), ab("weather", "0123abcd", instA)})
	runSync(t, f)
	rec := syncRecord(t, f)
	for i := range addonRecordMaxData {
		rec.Data = append(rec.Data, addonRecordData{ID: fmt.Sprintf("d%03d", i), Instance: instB})
	}
	putAddonRecord(t, f, rec)
	putData(t, f, "weather")
	enableLAN := "systemctl enable --now maru-addon-lan@weather.socket"
	f.fail[enableLAN] = fmt.Errorf("%s: exit status 1", enableLAN)
	putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA, config.AddonNetworkLAN)})
	runSync(t, f)
	rec = syncRecord(t, f)
	hasID := func(id string) bool {
		return slices.ContainsFunc(rec.Data, func(d addonRecordData) bool { return d.ID == id })
	}
	if len(rec.Backends) != 0 || len(rec.Data) != addonRecordMaxData || !slices.Contains(rec.Data, addonRecordData{ID: "weather", Instance: instA}) ||
		hasID("radio") || hasID("d000") || !hasID("d001") {
		t.Errorf("record backends %+v, %d data, weather %v radio %v d000 %v", rec.Backends, len(rec.Data), hasID("weather"), hasID("radio"), hasID("d000"))
	}
	delete(f.fail, enableLAN)
	wantLog(t, runSync(t, f), enableLAN)
	if !hasData(f, "weather") {
		t.Error("data of the same installation deleted")
	}
}

// systemctl이 실패하면 그 항목을 failed(실패한 동작의 요약)로 남기고 0으로 끝낸다. 기록은 실제로 된 것만 적으므로 다음
// 동기화가 다시 시도한다.
func TestAddonSyncFailures(t *testing.T) {
	exit := func(cmd string) error { return fmt.Errorf("%s: exit status 1", cmd) }
	enableCmd := "systemctl enable --now maru-addon@weather.socket"

	t.Run("enable", func(t *testing.T) {
		f := syncFake(t)
		f.fail[enableCmd] = exit(enableCmd)
		putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA), ab("zoo", "0123abcd", instA)})
		wantLog(t, runSync(t, f),
			enableCmd,
			"systemctl stop maru-addon@weather.socket maru-addon@weather.service",
			"systemctl disable maru-addon@weather.socket",
			"systemctl enable --now maru-addon@zoo.socket")
		st := syncStatus(t, f)
		if st.Error != "" || len(st.Backends) != 2 || st.Backends[0].State != config.AddonBackendFailed ||
			st.Backends[0].Error != enableCmd+": exit status 1" || st.Backends[1].State != config.AddonBackendEnabled {
			t.Errorf("status %+v", st)
		}
		if _, on := (&addonSyncer{rec: syncRecord(t, f)}).enabled("weather"); on {
			t.Error("failed backend recorded as enabled")
		}
		delete(f.fail, enableCmd)
		wantLog(t, runSync(t, f), enableCmd, isActive("maru-addon@zoo.socket"))
		if st := syncStatus(t, f); st.Backends[0].State != config.AddonBackendEnabled {
			t.Errorf("retry %+v", st.Backends[0])
		}
	})

	t.Run("try-restart", func(t *testing.T) {
		f := syncFake(t)
		putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
		runSync(t, f)
		restart := "systemctl try-restart maru-addon@weather.service"
		f.fail[restart] = exit(restart)
		putDesired(t, f, []config.AddonBackend{ab("weather", "4567cdef", instA)})
		wantLog(t, runSync(t, f), isActive("maru-addon@weather.socket"), restart)
		if st := syncStatus(t, f); st.Backends[0].State != config.AddonBackendFailed || st.Backends[0].Rev != "1.0.0-4567cdef" {
			t.Errorf("status %+v", st.Backends)
		}
		if rec := syncRecord(t, f); rec.Backends[0].Rev != "1.0.0-0123abcd" {
			t.Errorf("record %+v", rec)
		}
		delete(f.fail, restart)
		wantLog(t, runSync(t, f), isActive("maru-addon@weather.socket"), restart)
		if rec := syncRecord(t, f); rec.Backends[0].Rev != "1.0.0-4567cdef" {
			t.Errorf("record after retry %+v", rec)
		}
	})

	// 이전 설치의 데이터를 지우지 못하면(실패, 또는 성공했다는데 남음) 켜지 않고 데이터와 그 설치 인스턴스를 남긴다.
	for _, keeps := range []bool{false, true} {
		t.Run(fmt.Sprintf("clean keeps=%v", keeps), func(t *testing.T) {
			f := syncFake(t)
			putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
			runSync(t, f)
			putData(t, f, "weather")
			clean := "systemctl clean --what=state maru-addon@weather.service"
			if keeps {
				f.cleanKeeps = true
			} else {
				f.fail[clean] = exit(clean)
			}
			putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instB)})
			wantLog(t, runSync(t, f),
				"systemctl stop maru-addon@weather.socket maru-addon@weather.service",
				"systemctl disable maru-addon@weather.socket",
				stopAll("weather"),
				clean)
			st := syncStatus(t, f)
			if st.Backends[0].State != config.AddonBackendFailed || !strings.Contains(st.Backends[0].Error, "delete the data of another installation") {
				t.Errorf("status %+v", st.Backends)
			}
			rec := syncRecord(t, f)
			if len(rec.Backends) != 0 || !reflect.DeepEqual(rec.Data, []addonRecordData{{ID: "weather", Instance: instA}}) || !hasData(f, "weather") {
				t.Errorf("record %+v", rec)
			}
		})
	}

	// 끄지 못한 백엔드는 기록에 그대로 두고, 그 id의 purge는 미룬다.
	t.Run("stop", func(t *testing.T) {
		f := syncFake(t)
		putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA), ab("radio", "0123abcd", instA)})
		runSync(t, f)
		stop := "systemctl stop maru-addon@weather.socket maru-addon@weather.service"
		f.fail[stop] = exit(stop)
		putDesired(t, f, []config.AddonBackend{ab("radio", "0123abcd", instA, config.AddonNetworkLAN)}, "weather")
		radioStop := "systemctl stop maru-addon@radio.socket maru-addon@radio.service"
		f.fail[radioStop] = exit(radioStop)
		wantLog(t, runSync(t, f), stop, radioStop)
		st := syncStatus(t, f)
		if st.Error != "" || len(st.Purged) != 0 || len(st.Backends) != 1 || st.Backends[0].State != config.AddonBackendFailed {
			t.Errorf("status %+v", st)
		}
		rec := syncRecord(t, f)
		if len(rec.Backends) != 2 || rec.Backends[0].Unit != "maru-addon" {
			t.Errorf("record %+v", rec)
		}
	})

	// 끄지 못한 백엔드로 기록이 차 있으면 새 백엔드를 켜지 않는다(기록의 상한).
	t.Run("record full", func(t *testing.T) {
		f := syncFake(t)
		var full []config.AddonBackend
		for i := range config.AddonBackendsMax {
			id := fmt.Sprintf("b%02d", i)
			full = append(full, ab(id, "0123abcd", instA))
			stop := "systemctl stop maru-addon@" + id + ".socket maru-addon@" + id + ".service"
			f.fail[stop] = exit(stop)
		}
		putDesired(t, f, full)
		runSync(t, f)
		putDesired(t, f, []config.AddonBackend{ab("zz", "0123abcd", instA)})
		log := runSync(t, f)
		if len(log) != config.AddonBackendsMax || slices.Contains(log, "systemctl enable --now maru-addon@zz.socket") {
			t.Errorf("log %q", log)
		}
		if st := syncStatus(t, f); st.Backends[0].State != config.AddonBackendFailed || !strings.Contains(st.Backends[0].Error, "more than 32") {
			t.Errorf("status %+v", st.Backends)
		}
	})
}

// 백엔드의 동적 사용자 이름으로 동적 범위 밖의 고정 계정·그룹이 있으면 켜지 않는다(켜져 있었으면 끈다). 도는 동안의
// 동적 사용자(범위 안)는 받는다.
func TestAddonSyncStaticAccount(t *testing.T) {
	f := syncFake(t)
	putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA), ab("radio", "0123abcd", instA), ab("clock", "0123abcd", instA)})
	runSync(t, f)
	f.users["maru-a-weather"] = Account{Name: "maru-a-weather", UID: 1001, Home: "/home/maru-a-weather"}
	f.groupIDs["maru-a-radio"] = 100
	f.users["maru-a-clock"] = Account{Name: "maru-a-clock", UID: config.AddonDynamicUIDMin + 5}
	f.groupIDs["maru-a-clock"] = config.AddonDynamicUIDMin + 5
	wantLog(t, runSync(t, f),
		isActive("maru-addon@clock.socket"),
		"systemctl stop maru-addon@radio.socket maru-addon@radio.service",
		"systemctl disable maru-addon@radio.socket",
		"systemctl stop maru-addon@weather.socket maru-addon@weather.service",
		"systemctl disable maru-addon@weather.socket")
	st := syncStatus(t, f)
	byID := map[string]config.AddonBackendStatus{}
	for _, b := range st.Backends {
		byID[b.ID] = b
	}
	if b := byID["weather"]; b.State != config.AddonBackendFailed || !strings.Contains(b.Error, "account named maru-a-weather already exists (uid 1001)") {
		t.Errorf("weather %+v", b)
	}
	if b := byID["radio"]; b.State != config.AddonBackendFailed || !strings.Contains(b.Error, "group named maru-a-radio already exists (gid 100)") {
		t.Errorf("radio %+v", b)
	}
	if b := byID["clock"]; b.State != config.AddonBackendEnabled {
		t.Errorf("clock %+v", b)
	}
	rec := syncRecord(t, f)
	if len(rec.Backends) != 1 || len(rec.Data) != 2 {
		t.Errorf("record %+v", rec)
	}
	// 계정을 지우면 같은 설치의 데이터를 둔 채 다시 켠다.
	delete(f.users, "maru-a-weather")
	delete(f.groupIDs, "maru-a-radio")
	wantLog(t, runSync(t, f),
		isActive("maru-addon@clock.socket"),
		"systemctl enable --now maru-addon@radio.socket",
		"systemctl enable --now maru-addon@weather.socket")
}

// 원하는 상태 파일을 믿지 않는다. 심볼릭 링크·일반 파일이 아님·상한 넘음·형식 오류는 아무것도 바꾸지 않고 결과의
// error에 남긴다(내용을 읽었으면 그 digest와 함께). 파일이 없으면 빈 원하는 상태라 모두 끈다.
func TestAddonSyncDesiredInvalid(t *testing.T) {
	valid := func(mut func(m map[string]any)) string {
		m := map[string]any{"format": 1, "purge": []any{}, "backends": []any{map[string]any{
			"id": "weather", "version": "1.0.0", "rev": "1.0.0-0123abcd", "instance": instA, "network": []any{},
		}}}
		if mut != nil {
			mut(m)
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	entry := func(m map[string]any) map[string]any { return m["backends"].([]any)[0].(map[string]any) }
	many := func(m map[string]any) {
		var list []any
		for i := range config.AddonBackendsMax + 1 {
			list = append(list, map[string]any{"id": fmt.Sprintf("b%02d", i), "version": "1.0.0", "rev": "1.0.0-0123abcd", "instance": instA, "network": []any{}})
		}
		m["backends"] = list
	}
	contents := map[string]string{
		"bad id":           valid(func(m map[string]any) { entry(m)["id"] = "Weather" }),
		"long id":          valid(func(m map[string]any) { entry(m)["id"] = strings.Repeat("a", config.AddonIDMaxLen+1) }),
		"injection":        valid(func(m map[string]any) { entry(m)["id"] = "x.socket --now" }),
		"bad rev":          valid(func(m map[string]any) { entry(m)["rev"] = "1.0.0-xyz" }),
		"rev of other ver": valid(func(m map[string]any) { entry(m)["rev"] = "2.0.0-0123abcd" }),
		"bad network":      valid(func(m map[string]any) { entry(m)["network"] = []any{"internet"} }),
		"twice network":    valid(func(m map[string]any) { entry(m)["network"] = []any{"lan", "lan"} }),
		"bad instance":     valid(func(m map[string]any) { entry(m)["instance"] = "ABC" }),
		"unknown field":    valid(func(m map[string]any) { entry(m)["exec"] = "/bin/sh" }),
		"bad purge":        valid(func(m map[string]any) { m["purge"] = []any{"../x"} }),
		"format":           valid(func(m map[string]any) { m["format"] = 2 }),
		"too many":         valid(many),
		"duplicate":        valid(func(m map[string]any) { m["backends"] = append(m["backends"].([]any), entry(m)) }),
		"trailing ]":       valid(nil) + "]",
		"trailing }":       valid(nil) + "}",
		"two values":       valid(nil) + valid(nil),
		"not json":         "weather",
	}
	setup := func(t *testing.T) *fakeSystem {
		f := syncFake(t)
		putDesired(t, f, []config.AddonBackend{ab("radio", "0123abcd", instA)})
		runSync(t, f)
		return f
	}
	check := func(t *testing.T, f *fakeSystem, digest string) {
		t.Helper()
		record, _ := f.read(t, config.AddonSyncRecordFile)
		wantLog(t, runSync(t, f))
		st := syncStatus(t, f)
		if st.Error == "" || st.Digest != digest || len(st.Backends) != 0 || len(st.Purged) != 0 {
			t.Errorf("status %+v", st)
		}
		if got, _ := f.read(t, config.AddonSyncRecordFile); got != record {
			t.Errorf("record changed:\n%s", got)
		}
	}
	for name, content := range contents {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			f.put(t, addonBackendsFile, content, 0o600)
			check(t, f, config.AddonBackendsDigest([]byte(content)))
		})
	}
	// 형식 오류 예시는 원래 통과하는 내용에서 만들었다.
	if _, err := config.ParseAddonBackends([]byte(valid(nil))); err != nil {
		t.Fatal(err)
	}

	t.Run("symlink", func(t *testing.T) {
		f := setup(t)
		f.put(t, "/elsewhere.json", valid(nil), 0o644)
		if err := os.Remove(f.path(addonBackendsFile)); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(f.path("/elsewhere.json"), f.path(addonBackendsFile)); err != nil {
			t.Fatal(err)
		}
		check(t, f, "")
	})
	t.Run("directory", func(t *testing.T) {
		f := setup(t)
		if err := os.Remove(f.path(addonBackendsFile)); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(f.path(addonBackendsFile), 0o755); err != nil {
			t.Fatal(err)
		}
		check(t, f, "")
	})
	t.Run("fifo", func(t *testing.T) {
		f := setup(t)
		if err := os.Remove(f.path(addonBackendsFile)); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(f.path(addonBackendsFile), 0o600); err != nil {
			t.Fatal(err)
		}
		check(t, f, "")
	})
	t.Run("too large", func(t *testing.T) {
		f := setup(t)
		f.put(t, addonBackendsFile, valid(nil)+strings.Repeat(" ", config.AddonBackendsMaxBytes), 0o600)
		check(t, f, "")
	})
	t.Run("missing", func(t *testing.T) {
		f := setup(t)
		if err := os.Remove(f.path(addonBackendsFile)); err != nil {
			t.Fatal(err)
		}
		wantLog(t, runSync(t, f),
			"systemctl stop maru-addon@radio.socket maru-addon@radio.service",
			"systemctl disable maru-addon@radio.socket")
		if st := syncStatus(t, f); st.Error != "" || st.Digest != "" || len(st.Backends) != 0 {
			t.Errorf("status %+v", st)
		}
	})
}

// 기록을 읽지 못하면 어느 설치의 데이터인지 모르므로 아무것도 바꾸지 않는다.
func TestAddonSyncRecordUnreadable(t *testing.T) {
	f := syncFake(t)
	f.put(t, config.AddonSyncRecordFile, "not json", 0o600)
	putData(t, f, "weather")
	putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
	wantLog(t, runSync(t, f))
	if st := syncStatus(t, f); !strings.Contains(st.Error, "addon sync record") {
		t.Errorf("status %+v", st)
	}
	if got, _ := f.read(t, config.AddonSyncRecordFile); got != "not json" || !hasData(f, "weather") {
		t.Error("record or data changed")
	}
}

// 요청은 내용과 상관없이 따라가지 않고 지운다(심볼릭 링크는 링크만, 디렉터리는 통째로). 동기화는 그대로 한다.
func TestAddonSyncRequest(t *testing.T) {
	cases := map[string]func(t *testing.T, f *fakeSystem){
		"symlink": func(t *testing.T, f *fakeSystem) {
			f.put(t, "/target.json", "keep\n", 0o644)
			if err := os.MkdirAll(f.path(stateDir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(f.path("/target.json"), f.path(addonSyncRequest)); err != nil {
				t.Fatal(err)
			}
		},
		"directory": func(t *testing.T, f *fakeSystem) { f.put(t, addonSyncRequest+"/inner", "x", 0o644) },
		"too large": func(t *testing.T, f *fakeSystem) {
			f.put(t, addonSyncRequest, strings.Repeat("x", config.AddonSyncRequestMaxBytes+1), 0o600)
		},
		"garbage": func(t *testing.T, f *fakeSystem) { f.put(t, addonSyncRequest, "\x00\x01", 0o600) },
		"missing": func(t *testing.T, f *fakeSystem) {},
	}
	for name, put := range cases {
		t.Run(name, func(t *testing.T) {
			f := syncFake(t)
			put(t, f)
			digest := putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
			wantLog(t, runSync(t, f), "systemctl enable --now maru-addon@weather.socket")
			if lexists(f, addonSyncRequest) {
				t.Error("request not removed")
			}
			if got, ok := f.read(t, "/target.json"); name == "symlink" && (!ok || got != "keep\n") {
				t.Error("symlink target changed")
			}
			if st := syncStatus(t, f); st.Digest != digest || st.Error != "" {
				t.Errorf("status %+v", st)
			}
		})
	}
}

// 요청을 지우지 못하면 지우지 못했다고 경고하고(경로 유닛이 다시 시작한다) 동기화는 그대로 한다.
func TestAddonSyncRequestKept(t *testing.T) {
	f := syncFake(t)
	f.keepRequests = true
	f.put(t, addonSyncRequest, config.AddonSyncRequestBody, 0o600)
	putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
	c, out, _ := newTestContext(t, f, prompt.Fail{}, "--addon-sync")
	c.IsRoot = true
	if err := syncAddonBackends(c, time.Second); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if s := out.String(); !strings.Contains(s, "could not be removed") || !strings.Contains(s, "starts the sync again") || strings.Contains(s, "(removed;") {
		t.Errorf("out %q", s)
	}
	if !slices.Contains(f.log, "systemctl enable --now maru-addon@weather.socket") {
		t.Errorf("log %q", f.log)
	}
}

// takeFile은 지우지 못한 것을 errRequestKept로 알리고, 읽기 오류(지운 뒤)와 나눈다.
func TestTakeFileKeptRequest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove files from a read-only directory")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "request.json")
	if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	_, err := takeFile(p, 64)
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(err, errRequestKept) {
		t.Errorf("read-only directory: %v", err)
	}
	if err := os.WriteFile(p, []byte(strings.Repeat("x", 100)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := takeFile(p, 64); err == nil || errors.Is(err, errRequestKept) {
		t.Errorf("too large: %v", err)
	}
	if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("too large request kept: %v", err)
	}
}

// 애드온 백엔드가 꺼져 있으면(동기화 디렉터리 없음) 요청만 지우고 결과를 쓰지 않는다. 디렉터리도 만들지 않는다.
func TestAddonSyncFeatureOff(t *testing.T) {
	f := newFake(t)
	f.units = true
	f.put(t, addonSyncRequest, config.AddonSyncRequestBody, 0o600)
	putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
	wantLog(t, runSync(t, f))
	if lexists(f, addonSyncRequest) || lexists(f, config.AddonSyncDir) {
		t.Errorf("request kept %v, sync dir created %v", lexists(f, addonSyncRequest), lexists(f, config.AddonSyncDir))
	}
}

// 맞춘 뒤 원하는 상태 파일이 바뀌었으면 다시 맞춘다(3번까지). 결과의 digest는 마지막으로 맞춘 파일의 것이다.
func TestAddonSyncRounds(t *testing.T) {
	t.Run("changed once", func(t *testing.T) {
		f := syncFake(t)
		putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
		var second string
		f.onRun = func(cmd string) {
			if cmd == "systemctl enable --now maru-addon@weather.socket" && second == "" {
				second = putDesired(t, f, []config.AddonBackend{ab("radio", "0123abcd", instB), ab("weather", "0123abcd", instA)})
			}
		}
		wantLog(t, runSync(t, f),
			"systemctl enable --now maru-addon@weather.socket",
			"systemctl enable --now maru-addon@radio.socket",
			isActive("maru-addon@weather.socket"))
		if st := syncStatus(t, f); st.Digest != second || len(st.Backends) != 2 {
			t.Errorf("status %+v", st)
		}
	})
	t.Run("keeps changing", func(t *testing.T) {
		f := syncFake(t)
		putDesired(t, f, []config.AddonBackend{ab("weather", "00000000", instA)})
		var digests []string
		n := 0
		f.onRun = func(string) {
			n++
			digests = append(digests, putDesired(t, f, []config.AddonBackend{ab("weather", fmt.Sprintf("%08d", n), instA)}))
		}
		wantLog(t, runSync(t, f),
			"systemctl enable --now maru-addon@weather.socket",
			isActive("maru-addon@weather.socket"),
			"systemctl try-restart maru-addon@weather.service",
			isActive("maru-addon@weather.socket"),
			"systemctl try-restart maru-addon@weather.service")
		st := syncStatus(t, f)
		if st.Digest != digests[1] || st.Backends[0].Rev != "1.0.0-00000002" {
			t.Errorf("status %+v, digests %q", st, digests)
		}
	})
}

// 동기화는 기록을 다루는 동안 잠금을 쥔다. 설치기가 잠금을 쥐고 있으면 기다리고, 끝내 얻지 못하면 1로 끝낸다(요청은
// 이미 지웠다). 기다리는 동안 설치기가 애드온 백엔드를 껐으면 0으로 끝내고 디렉터리를 다시 만들지 않는다.
func TestAddonSyncTakesLock(t *testing.T) {
	t.Run("held while syncing", func(t *testing.T) {
		f := syncFake(t)
		putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
		var lockErr error
		f.onRun = func(string) {
			unlock, err := f.fsOps.Lock(config.AddonSyncLockFile, 0)
			if err == nil {
				unlock()
			}
			lockErr = err
		}
		runSync(t, f)
		if lockErr == nil {
			t.Error("the sync lock was free while the sync ran")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		f := syncFake(t)
		f.put(t, addonSyncRequest, config.AddonSyncRequestBody, 0o600)
		putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
		unlock, err := f.fsOps.Lock(config.AddonSyncLockFile, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		c, out, _ := newTestContext(t, f, prompt.Fail{}, "--addon-sync")
		c.IsRoot = true
		err = syncAddonBackends(c, 200*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "cannot lock") {
			t.Fatalf("sync with a held lock: %v", err)
		}
		if code, _ := ExitCode(err); code != 1 {
			t.Errorf("exit code %d", code)
		}
		if !strings.Contains(out.String(), "Waiting for the installer") || lexists(f, addonSyncRequest) || len(f.log) != 0 {
			t.Errorf("out %q log %q", out.String(), f.log)
		}
		if got, _ := f.read(t, config.AddonSyncStatusFile); got != seedStatus {
			t.Errorf("status written:\n%s", got)
		}
	})
	t.Run("turned off while waiting", func(t *testing.T) {
		f := syncFake(t)
		putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
		unlock, err := f.fsOps.Lock(config.AddonSyncLockFile, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		c, _, _ := newTestContext(t, f, prompt.Fail{}, "--addon-sync")
		c.IsRoot = true
		done := make(chan error, 1)
		go func() { done <- syncAddonBackends(c, 5*time.Second) }()
		time.Sleep(150 * time.Millisecond)
		if err := os.RemoveAll(f.path(config.AddonSyncDir)); err != nil {
			t.Fatal(err)
		}
		unlock()
		if err := <-done; err != nil {
			t.Fatalf("sync: %v", err)
		}
		if lexists(f, config.AddonSyncDir) || len(f.log) != 0 {
			t.Errorf("sync dir recreated %v, log %q", lexists(f, config.AddonSyncDir), f.log)
		}
	})
}

// --addon-sync로 실행하면 Run이 동기화로 보낸다.
func TestAddonSyncRun(t *testing.T) {
	f := syncFake(t)
	putDesired(t, f, []config.AddonBackend{ab("weather", "0123abcd", instA)})
	c, out, _ := newTestContext(t, f, prompt.Fail{}, "--addon-sync")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	if !slices.Contains(f.log, "systemctl enable --now maru-addon@weather.socket") {
		t.Errorf("log %q", f.log)
	}
}

// --addon-sync는 root로만 돈다(maru-addons.service가 실행한다). 요청도 지우지 않는다.
func TestAddonSyncNeedsRoot(t *testing.T) {
	f := newFake(t)
	f.put(t, addonSyncRequest, config.AddonSyncRequestBody, 0o600)
	c, _, _ := newTestContext(t, f, prompt.Fail{})
	if err := runAddonSync(c); err == nil || !strings.Contains(err.Error(), "must run as root") {
		t.Fatalf("runAddonSync as a regular user: %v", err)
	}
	if _, ok := f.read(t, addonSyncRequest); !ok {
		t.Error("request removed")
	}
}

// 결과의 오류 문구는 한 줄이고 상한 안에서 UTF-8 경계로 자른다.
func TestBoundedSyncError(t *testing.T) {
	if got := boundedSyncError(errors.New("a\nb  c")); got != "a b c" {
		t.Errorf("%q", got)
	}
	got := boundedSyncError(errors.New(strings.Repeat("가", 200)))
	if len(got) > addonSyncErrorMax || !utf8.ValidString(got) || !strings.HasSuffix(got, "...") {
		t.Errorf("%d bytes %q", len(got), got)
	}
}

// dry-run의 TakeFile은 읽기만 하고 지우는 것은 보여 준다.
func TestDryRunTakeFile(t *testing.T) {
	f := newFake(t)
	f.put(t, addonSyncRequest, config.AddonSyncRequestBody, 0o600)
	var out testOut
	data, err := NewDryRunSystem(f, &out, "en").TakeFile(addonSyncRequest, config.AddonSyncRequestMaxBytes)
	if err != nil || string(data) != config.AddonSyncRequestBody || !lexists(f, addonSyncRequest) {
		t.Fatalf("%q %v", data, err)
	}
	if !strings.Contains(out.String(), "[dry-run] run: rm -rf -- "+addonSyncRequest) {
		t.Errorf("out %q", out.String())
	}
}

// getent passwd 출력을 읽지 못하면 "계정 없음"이 아니라 오류다(고정 계정 확인이 통과하지 않게).
func TestPasswdAccount(t *testing.T) {
	a, ok, err := passwdAccount("maru-a-x", []byte("maru-a-x:x:61200:61200:Dynamic User:/:/usr/sbin/nologin\n"))
	if err != nil || !ok || a.UID != 61200 || a.Name != "maru-a-x" {
		t.Errorf("%+v %v %v", a, ok, err)
	}
	for _, out := range []string{"", "maru-a-x:x", "maru-a-x:x:abc:1::/:/bin/sh", "maru-a-x:x:-1:1::/:/bin/sh", "maru-a-x:x:4294967296:1::/:/bin/sh"} {
		if _, ok, err := passwdAccount("maru-a-x", []byte(out)); err == nil || ok {
			t.Errorf("%q accepted", out)
		}
	}
}

// getent group 출력의 gid를 읽는다.
func TestParseGroupID(t *testing.T) {
	for line, want := range map[string]int{"maru-a-x:x:61200:": 61200, "g:x:0:root": 0} {
		if got, ok := parseGroupID(line); !ok || got != want {
			t.Errorf("%q: %d %v", line, got, ok)
		}
	}
	for _, line := range []string{"g:x", "g:x:-1:", "g:x:4294967296:", "g:x:abc:"} {
		if _, ok := parseGroupID(line); ok {
			t.Errorf("%q accepted", line)
		}
	}
}
