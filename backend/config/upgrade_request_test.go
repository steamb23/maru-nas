package config

import (
	"strings"
	"testing"
)

func TestValidUpgradeTarget(t *testing.T) {
	ok := strings.Repeat("0123456789", 4)
	if !ValidUpgradeTarget(ok) {
		t.Errorf("%q rejected", ok)
	}
	for _, bad := range []string{"", ok[:39], ok + "a", strings.ToUpper("abcdef") + ok[6:], "HEAD", "-" + ok[1:], ok[:39] + "g"} {
		if ValidUpgradeTarget(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidUpgradeChannel(t *testing.T) {
	for _, ok := range []string{UpgradeChannelCurrent, UpgradeChannelStable, UpgradeChannelMain} {
		if !ValidUpgradeChannel(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"Stable", "origin/stable", "dev", "-main", "main ", "refs/heads/main"} {
		if ValidUpgradeChannel(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidUpgradeSetting(t *testing.T) {
	cases := []struct {
		name, value string
		ok          bool
	}{
		{"public-origin", "https://desk.example.lan", true},
		{"workbench-url", "", true},
		{"helper", "yes", true},
		{"smart-refresh-at", "03:30", true},
		{"workbench-url", "-", true},
		{"a", strings.Repeat("x", MaxUpgradeSettingValue), true},
		{"a", strings.Repeat("x", MaxUpgradeSettingValue+1), false},
		{"", "x", false},
		{"Public-origin", "x", false},
		{"-x", "x", false},
		{"1x", "x", false},
		{"no-helper", "yes", false},
		{"public_origin", "x", false},
		{"service-install x", "", false},
		{strings.Repeat("a", 49), "x", false},
		{"x", "a b", false},
		{"x", "a\nb", false},
		{"x", "a\x00", false},
		{"x", "\x7f", false},
		{"x", "한글", false},
	}
	for _, c := range cases {
		if got := ValidUpgradeSetting(c.name, c.value); got != c.ok {
			t.Errorf("ValidUpgradeSetting(%q, %q) = %v, want %v", c.name, c.value, got, c.ok)
		}
	}
}
