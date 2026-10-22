package addons

import (
	"fmt"
	"strings"
	"testing"

	"maru/backend/internal/api"
)

func shortcuts(apps ...string) []api.Shortcut {
	list := make([]api.Shortcut, len(apps))
	for i, a := range apps {
		list[i] = api.Shortcut{App: a}
	}
	return list
}

func manyShortcuts(n int) []api.Shortcut {
	list := make([]api.Shortcut, n)
	for i := range list {
		list[i] = api.Shortcut{App: fmt.Sprintf("app-%d", i)}
	}
	return list
}

func TestValidateShortcutsAccepts(t *testing.T) {
	cases := map[string][]api.Shortcut{
		"empty":       {},
		"nil":         nil,
		"placeholder": shortcuts("files", "system-info", "a1-b2"),
		"max":         manyShortcuts(MaxShortcuts),
		"max length":  shortcuts("a" + strings.Repeat("b", maxAppIDLen-1)),
	}
	for name, list := range cases {
		if err := ValidateShortcuts(list); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestValidateShortcutsRejects(t *testing.T) {
	cases := map[string][]api.Shortcut{
		"too many":    manyShortcuts(MaxShortcuts + 1),
		"empty id":    shortcuts(""),
		"upper case":  shortcuts("Files"),
		"underscore":  shortcuts("a_b"),
		"leading -":   shortcuts("-a"),
		"double -":    shortcuts("a--b"),
		"trailing -":  shortcuts("a-"),
		"path":        shortcuts("../x"),
		"digit first": shortcuts("1a"),
		"too long":    shortcuts("a" + strings.Repeat("b", maxAppIDLen)),
		"duplicate":   shortcuts("files", "storage", "files"),
	}
	for name, list := range cases {
		if err := ValidateShortcuts(list); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
