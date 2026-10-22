package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"maru/backend/config"
	"maru/installer/internal/prompt"
)

const (
	revA = "0123456789abcdef0123456789abcdef01234567"
	revB = "89abcdef0123456789abcdef0123456789abcdef"
)

// runRecordUpgrade는 설치된 가짜 시스템에서 root로 --upgrade --no-build를 실행하고 설치 기록을 돌려준다.
func runRecordUpgrade(t *testing.T, f *fakeSystem, self *fakeBuild) (config.InstallRecord, string) {
	t.Helper()
	c, out, _ := newTestContext(t, f, prompt.Fail{}, "--upgrade", "--no-build")
	c.IsRoot = true
	if self != nil {
		c.SelfBuild = vcsBuild(self.rev, self.modified)
	}
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	data, ok := f.read(t, config.InstallRecordFile)
	if !ok {
		t.Fatalf("%s not written\n%s", config.InstallRecordFile, out)
	}
	var rec config.InstallRecord
	if err := json.Unmarshal([]byte(data), &rec); err != nil {
		t.Fatalf("record %q: %v", data, err)
	}
	return rec, out.String()
}

type fakeBuild struct {
	rev      string
	modified bool
}

// 설치기와 bin/maru가 같은 커밋에서 빌드됐으면 버전과 커밋을 기록한다. 일반 사용자가 읽을 수 있어야 한다(0755/0644).
func TestInstallRecord(t *testing.T) {
	f := installedFake(t)
	f.buildInfo["/repo/bin/maru"] = vcsBuild(revA, true)
	rec, out := runRecordUpgrade(t, f, &fakeBuild{rev: revA, modified: true})
	want := config.InstallRecord{
		Format: config.InstallRecordFormat, Version: config.Version, Revision: revA,
		RevisionTime: "2026-10-01T02:03:04Z", Modified: true, InstalledAt: "2026-10-18T09:30:00Z",
	}
	if rec != want {
		t.Errorf("record %+v, want %+v", rec, want)
	}
	if strings.Contains(out, "install record:") {
		t.Errorf("unexpected warning:\n%s", out)
	}
	for path, mode := range map[string]os.FileMode{config.UpgradeStateDir: 0o755, config.InstallRecordFile: 0o644} {
		fi, err := os.Stat(f.path(path))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != mode {
			t.Errorf("%s mode %v, want %v", path, fi.Mode().Perm(), mode)
		}
	}
	// 파일 쓰기는 명령 기록에 남지 않으므로 골든의 명령 순서는 그대로다.
	for _, l := range f.log {
		if strings.Contains(l, filepath.Base(config.InstallRecordFile)) {
			t.Errorf("record write logged as a command: %q", l)
		}
	}
}

// 버전을 확인하지 못하면 비워 두고 경고한다. 설치는 실패하지 않는다.
func TestInstallRecordUnconfirmed(t *testing.T) {
	cases := []struct {
		name     string
		binary   *fakeBuild // nil이면 bin/maru를 읽을 수 없다
		self     *fakeBuild
		revision string
		warning  string
	}{
		{"older binary (--no-build)", &fakeBuild{rev: revB}, &fakeBuild{rev: revA}, revB, "built from a different commit than the installer"},
		{"installer without vcs info", &fakeBuild{rev: revA}, nil, revA, "the installer has no git commit information"},
		{"binary without vcs info", &fakeBuild{rev: ""}, &fakeBuild{rev: revA}, "", "has no git commit information"},
		{"not a Go executable", nil, &fakeBuild{rev: revA}, "", "cannot read the build information"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := installedFake(t)
			if tc.binary != nil {
				f.buildInfo["/repo/bin/maru"] = vcsBuild(tc.binary.rev, false)
			}
			rec, out := runRecordUpgrade(t, f, tc.self)
			if rec.Version != "" || rec.Revision != tc.revision || rec.Format != config.InstallRecordFormat || rec.InstalledAt == "" {
				t.Errorf("record %+v", rec)
			}
			if !strings.Contains(out, tc.warning) {
				t.Errorf("no warning %q:\n%s", tc.warning, out)
			}
		})
	}
}
