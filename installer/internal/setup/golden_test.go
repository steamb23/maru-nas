package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"maru/installer/internal/prompt"
)

// goldenCase는 testdata/golden/<이름>의 케이스다(만든 방법과 케이스 설명은 그 디렉터리의 README).
type goldenCase struct {
	name, dir string
	args      []string
	group     string
}

func goldenCases(t *testing.T) []goldenCase {
	t.Helper()
	entries, err := os.ReadDir("testdata/golden")
	if err != nil {
		t.Fatal(err)
	}
	var out []goldenCase
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join("testdata/golden", e.Name())
		args, err := os.ReadFile(filepath.Join(dir, "args"))
		if err != nil {
			t.Fatal(err)
		}
		group, err := os.ReadFile(filepath.Join(dir, "group"))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, goldenCase{
			name:  e.Name(),
			dir:   dir,
			args:  strings.Split(strings.TrimSuffix(string(args), "\n"), "\n"),
			group: strings.TrimSpace(string(group)),
		})
	}
	if len(out) == 0 {
		t.Fatal("no golden cases")
	}
	return out
}

// newGoldenFake는 케이스의 기존 파일과 서비스 사용자를 둔 가짜 시스템이다.
func newGoldenFake(t *testing.T, gc goldenCase, user string) *fakeSystem {
	t.Helper()
	f := newFake(t)
	f.putRepo(t)
	if _, err := os.Stat(filepath.Join(gc.dir, "existing")); err == nil {
		f.copyTree(t, filepath.Join(gc.dir, "existing"))
	}
	f.users[user] = Account{Name: user, UID: 999, Home: "/nonexistent"}
	f.groups[user] = gc.group
	return f
}

// 설정을 모두 준 --service-install은 묻지 않고, bash install.sh와 바이트 단위로 같은 파일을 같은 명령 순서로 만든다.
func TestGoldenServiceInstall(t *testing.T) {
	for _, gc := range goldenCases(t) {
		t.Run(gc.name, func(t *testing.T) {
			args := append([]string{"--service-install"}, gc.args...)
			_, values, err := parseArgs(settings, args)
			if err != nil {
				t.Fatal(err)
			}
			f := newGoldenFake(t, gc, values.Get(nameUser))
			c, out, _ := newTestContext(t, f, prompt.Fail{}, args...)
			c.IsRoot = true
			if err := Run(c); err != nil {
				t.Fatalf("Run: %v\n%s", err, out)
			}
			compareTree(t, f, filepath.Join(gc.dir, "want"))
			want, err := os.ReadFile(filepath.Join(gc.dir, "commands"))
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(f.log, "\n") + "\n"; got != string(want) {
				t.Errorf("commands differ\n--- got\n%s--- want\n%s", got, want)
			}
			// 바뀐 환경 파일만 백업한다. 같은 내용으로 다시 설치하면 백업이 없다.
			if gc.name == "rerun-unchanged" {
				if b := backups(t, f); len(b) != 0 {
					t.Errorf("unexpected backups %v", b)
				}
				if !strings.Contains(out.String(), envFile+" unchanged") {
					t.Errorf("no unchanged message:\n%s", out)
				}
			}
		})
	}
}

// 기존 환경 파일이 바뀌면 /etc/default/maru.bak-<시각>으로 남기고, 유닛은 백업하지 않는다.
func TestGoldenBackup(t *testing.T) {
	for _, gc := range goldenCases(t) {
		if gc.name != "helper-anyaddr-terminal" {
			continue
		}
		args := append([]string{"--service-install"}, gc.args...)
		f := newGoldenFake(t, gc, "dash")
		old, _ := f.read(t, envFile)
		f.put(t, unitFile, "# old unit\nUser=dash\n", 0o644)
		c, out, _ := newTestContext(t, f, prompt.Fail{}, args...)
		c.IsRoot = true
		if err := Run(c); err != nil {
			t.Fatalf("Run: %v\n%s", err, out)
		}
		if b := backups(t, f); len(b) != 1 || b[0] != envFile+".bak-20261018-093000" {
			t.Fatalf("backups %v", b)
		}
		if got, _ := f.read(t, envFile+".bak-20261018-093000"); got != old {
			t.Errorf("backup content %q", got)
		}
	}
}

// 레지스트리가 관리하지 않는 기존 MARU_* 줄은 파일 끝에 보존한다(bash install.sh에서 바뀐 유일한 출력).
func TestKeepUnmanagedLines(t *testing.T) {
	for _, gc := range goldenCases(t) {
		if gc.name != "helper-loopback" {
			continue
		}
		args := append([]string{"--service-install"}, gc.args...)
		f := newGoldenFake(t, gc, "maru")
		f.put(t, envFile, "MARU_LISTEN=127.0.0.1:8090\nMARU_COOKIE_PATH=/desk/\n#MARU_STATE_DIR=/x\nOTHER=1\nMARU_TERMINAL=true\n", 0o644)
		f.put(t, helperEnvFile, "MARU_HELPER_CLIENT_USER=old\nMARU_HELPER_IDLE=5m\n", 0o644)
		c, out, _ := newTestContext(t, f, prompt.Fail{}, args...)
		c.IsRoot = true
		if err := Run(c); err != nil {
			t.Fatalf("Run: %v\n%s", err, out)
		}
		want, _ := os.ReadFile(filepath.Join(gc.dir, "want", envFile))
		wantEnv := strings.Replace(string(want), "#MARU_TERMINAL=false", "MARU_TERMINAL=true", 1) + keptComment + "MARU_COOKIE_PATH=/desk/\n"
		if got, _ := f.read(t, envFile); got != wantEnv {
			t.Errorf("env\n--- got\n%s--- want\n%s", got, wantEnv)
		}
		wantHelper, _ := os.ReadFile(filepath.Join(gc.dir, "want", helperEnvFile))
		if got, _ := f.read(t, helperEnvFile); got != string(wantHelper)+keptComment+"MARU_HELPER_IDLE=5m\n" {
			t.Errorf("helper env\n%s", got)
		}
	}
}
