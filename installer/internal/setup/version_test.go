package setup

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"testing"

	"maru/backend/config"
)

func TestParseVersion(t *testing.T) {
	for s, want := range map[string]version{"0.3.1": {0, 3, 1}, "v22.12.0": {22, 12, 0}, "0.10.0": {0, 10, 0}} {
		if got, ok := parseVersion(s); !ok || got != want {
			t.Errorf("parseVersion(%q) = %v %v", s, got, ok)
		}
	}
	for _, s := range []string{"", "1.2", "1.2.3.4", "1.2.x", "1.2.3-rc1", "v", "1..3", "+1.2.3"} {
		if _, ok := parseVersion(s); ok {
			t.Errorf("parseVersion(%q) accepted", s)
		}
	}
	a, _ := parseVersion("0.9.9")
	b, _ := parseVersion("0.10.0")
	if a.cmp(b) >= 0 || b.cmp(a) <= 0 || a.cmp(a) != 0 {
		t.Error("cmp compares numerically")
	}
}

func TestVersionRange(t *testing.T) {
	r, err := parseVersionRange(requireNode)
	if err != nil {
		t.Fatal(err)
	}
	for s, want := range map[string]bool{
		"20.19.0": true, "20.30.1": true, "22.12.0": true, "24.1.0": true,
		"20.18.9": false, "21.7.0": false, "22.11.0": false, "18.20.0": false,
	} {
		v, _ := parseVersion(s)
		if r.contains(v) != want {
			t.Errorf("%s in %q = %v", s, requireNode, !want)
		}
	}
	for _, s := range []string{"~20.19.0", "20.19.0", "^0.1.0", ">=20", "^20.19.0 ||"} {
		if _, err := parseVersionRange(s); err == nil {
			t.Errorf("range %q accepted", s)
		}
	}
}

// 버전은 업그레이드 계획이 비교할 수 있는 X.Y.Z여야 한다.
func TestVersionConstantParses(t *testing.T) {
	if _, ok := parseVersion(config.Version); !ok {
		t.Errorf("config.Version %q is not X.Y.Z", config.Version)
	}
}

var goDirectiveRE = regexp.MustCompile(`(?m)^go (\S+)$`)

// requireGo는 두 모듈의 go 지시어와 같다(설치기 규칙상 두 지시어는 같다).
func TestRequireGo(t *testing.T) {
	for _, p := range []string{"../../go.mod", "../../../backend/go.mod"} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		m := goDirectiveRE.FindSubmatch(data)
		if m == nil || string(m[1]) != requireGo {
			t.Errorf("%s go directive %q, requireGo %q", p, m, requireGo)
		}
	}
}

// requireNode는 frontend/package.json의 engines.node와 같다.
func TestRequireNode(t *testing.T) {
	data, err := os.ReadFile("../../../frontend/package.json")
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Engines.Node != requireNode {
		t.Errorf("package.json engines.node %q, requireNode %q", pkg.Engines.Node, requireNode)
	}
}

// 수동 재설치 목록과 업그레이드 뒤 할 일은 버전을 읽을 수 있고, 두 언어 문구와 정해진 명령이 있어야 한다.
func TestUpgradeListsValid(t *testing.T) {
	commands := []string{config.UpgradeCommandInstall, config.UpgradeCommandReinstall}
	for _, m := range manualUpgrades {
		if _, ok := parseVersion(m.Below); !ok || m.Reason.En == "" || m.Reason.Ko == "" || !slices.Contains(commands, m.Command) {
			t.Errorf("manual upgrade %+v", m)
		}
	}
	for _, n := range upgradeNotes {
		if _, ok := parseVersion(n.Since); !ok || n.Text.En == "" || n.Text.Ko == "" {
			t.Errorf("upgrade note %+v", n)
		}
	}
}
