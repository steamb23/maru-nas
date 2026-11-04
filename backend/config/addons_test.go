package config

import (
	"strings"
	"testing"
)

func TestValidAddonID(t *testing.T) {
	for _, id := range []string{"a", "hello-widget", "ups2", strings.Repeat("a", AddonIDMaxLen)} {
		if !ValidAddonID(id) {
			t.Errorf("ValidAddonID(%q) = false", id)
		}
	}
	for _, id := range []string{"", "Hello", "-a", "a-", "a--b", "2a", "a_b", "a b", "a;b", strings.Repeat("a", AddonIDMaxLen+1)} {
		if ValidAddonID(id) {
			t.Errorf("ValidAddonID(%q) = true", id)
		}
	}
	// 동적 사용자 이름 maru-a-<id>는 31자를 넘지 않는다.
	if n := len("maru-a-") + AddonIDMaxLen; n > 31 {
		t.Errorf("dynamic user name can be %d characters", n)
	}
}

func TestValidAddonRev(t *testing.T) {
	for _, rev := range []string{"0.1.0-0123abcd", "1.2.3-beta.1-deadbeef", "dev-0123abcd"} {
		if !ValidAddonRev(rev) {
			t.Errorf("ValidAddonRev(%q) = false", rev)
		}
	}
	for _, rev := range []string{"", "0.1.0", "0.1.0-0123ABCD", "../0123abcd", "0..1-0123abcd", "0.1.0/x-0123abcd", ".1-0123abcd"} {
		if ValidAddonRev(rev) {
			t.Errorf("ValidAddonRev(%q) = true", rev)
		}
	}
}

func TestAddonUnitPrefix(t *testing.T) {
	cases := map[string][]string{
		"maru-addon":              nil,
		"maru-addon-lan":          {"lan"},
		"maru-addon-loopback":     {"loopback"},
		"maru-addon-lan-loopback": {"loopback", "lan"},
	}
	for want, network := range cases {
		if got := AddonUnitPrefix(network); got != want {
			t.Errorf("AddonUnitPrefix(%v) = %q, want %q", network, got, want)
		}
	}
}

func TestParseAddonBackends(t *testing.T) {
	ok := `{"format":1,"backends":[{"id":"ups","version":"1.0.0","rev":"1.0.0-0123abcd","network":["lan"]}],"purge":["old"]}`
	b, err := ParseAddonBackends([]byte(ok))
	if err != nil || len(b.Backends) != 1 || b.Backends[0].ID != "ups" || len(b.Purge) != 1 {
		t.Fatalf("ParseAddonBackends(ok) = %+v, %v", b, err)
	}
	bad := map[string]string{
		"format":          `{"format":2,"backends":[],"purge":[]}`,
		"unknown field":   `{"format":1,"backends":[],"purge":[],"extra":1}`,
		"entry field":     `{"format":1,"backends":[{"id":"a","version":"1.0.0","rev":"1.0.0-0123abcd","network":[],"user":"root"}],"purge":[]}`,
		"id":              `{"format":1,"backends":[{"id":"a;b","version":"1.0.0","rev":"1.0.0-0123abcd","network":[]}],"purge":[]}`,
		"duplicate":       `{"format":1,"backends":[{"id":"a","version":"1.0.0","rev":"1.0.0-0123abcd","network":[]},{"id":"a","version":"1.0.0","rev":"1.0.0-0123abcd","network":[]}],"purge":[]}`,
		"rev":             `{"format":1,"backends":[{"id":"a","version":"1.0.0","rev":"../x","network":[]}],"purge":[]}`,
		"rev version":     `{"format":1,"backends":[{"id":"a","version":"1.0.0","rev":"2.0.0-0123abcd","network":[]}],"purge":[]}`,
		"network":         `{"format":1,"backends":[{"id":"a","version":"1.0.0","rev":"1.0.0-0123abcd","network":["internet"]}],"purge":[]}`,
		"network twice":   `{"format":1,"backends":[{"id":"a","version":"1.0.0","rev":"1.0.0-0123abcd","network":["lan","lan"]}],"purge":[]}`,
		"purge id":        `{"format":1,"backends":[],"purge":["../x"]}`,
		"purge duplicate": `{"format":1,"backends":[],"purge":["a","a"]}`,
		"trailing":        `{"format":1,"backends":[],"purge":[]} {}`,
		"too large":       `{"format":1,"backends":[],"purge":[]}` + strings.Repeat(" ", AddonBackendsMaxBytes),
	}
	for name, data := range bad {
		if _, err := ParseAddonBackends([]byte(data)); err == nil {
			t.Errorf("%s: ParseAddonBackends accepted %.80s", name, data)
		}
	}
	many := AddonBackends{Format: 1}
	for i := 0; i <= AddonBackendsMax; i++ {
		many.Purge = append(many.Purge, "a"+strings.Repeat("b", i%20)+string(rune('a'+i%26)))
	}
	if err := many.Validate(); err == nil {
		t.Error("Validate accepted more than AddonBackendsMax purge entries")
	}
}
