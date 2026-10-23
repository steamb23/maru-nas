package setup

import (
	"os"
	"strings"
	"testing"
)

// docs/install-options.ko.md의 옵션 표(| `--...` 로 시작하는 줄)에 레지스트리의 모든 설정 옵션과 설치기 옵션이 있어야 한다.
// 설정을 더하면 그 표도 함께 고친다.
func TestDocsListEveryOption(t *testing.T) {
	data, err := os.ReadFile("../../../docs/install-options.ko.md")
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "| `--") || strings.HasPrefix(line, "| `-h`") {
			rows = append(rows, line)
		}
	}
	table := strings.Join(rows, "\n")
	want := []string{"--upgrade", "--reinstall", "--no-build", "--uninstall", "--service-install", "--dry-run", "--upgrade-plan", "--upgrade-service", "--plain", "--lang", "--help"}
	for _, s := range settings {
		switch s.Flag {
		case FlagNone:
		case FlagToggle:
			want = append(want, "--"+s.Name, "--no-"+s.Name)
		default:
			want = append(want, "--"+s.Name)
		}
	}
	for _, flag := range want {
		if !strings.Contains(table, "`"+flag+"`") && !strings.Contains(table, "`"+flag+" ") {
			t.Errorf("docs/install-options.ko.md option table misses %s", flag)
		}
	}
}
