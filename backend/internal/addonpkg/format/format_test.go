package format

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func valid() map[string]any {
	return map[string]any{
		"format":  1,
		"id":      "hello",
		"version": "1.0.0",
		"sdk":     1,
		"name":    map[string]any{"en": "Hello"},
		"author":  map[string]any{"name": "Example", "url": "https://example.com/hello"},
		"icon":    "icon.svg",
		"views": map[string]any{
			"window": map[string]any{"entry": "window.html", "defaultSize": map[string]any{"width": 480, "height": 320}},
			"widget": map[string]any{"entry": "widget.html", "defaultSize": map[string]any{"cols": 2, "rows": 2},
				"minSize": map[string]any{"cols": 1, "rows": 1}, "maxSize": map[string]any{"cols": 4, "rows": 3}},
		},
	}
}

func decode(t *testing.T, m map[string]any) (*Manifest, error) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return Decode(b)
}

func TestValidateAccepts(t *testing.T) {
	cases := map[string]func(map[string]any){
		"minimal":    nil,
		"prerelease": func(m map[string]any) { m["version"] = "2.10.0-rc.1" },
		"backend and actions": func(m map[string]any) {
			m["rpc"] = map[string]any{"read": []string{"Nut.getStats"}, "write": []string{"Nut.setUps"}}
			m["actions"] = []any{map[string]any{"name": "test-ups", "admin": true, "confirm": true, "timeoutSeconds": 600,
				"label": map[string]any{"en": "Test", "ko": "시험"}, "confirmText": map[string]any{"en": "Run a UPS test?"}}}
			m["backend"] = map[string]any{"network": []string{"lan", "loopback"}}
		},
		"plugins": func(m map[string]any) {
			m["requires"] = map[string]any{"admin": true, "plugins": []any{map[string]any{"package": "openmediavault-nut", "rpcFile": "nut.inc"}}}
		},
		"backend without network": func(m map[string]any) { m["backend"] = map[string]any{"network": []string{}} },
		"actions without writes": func(m map[string]any) {
			m["actions"] = []any{map[string]any{"name": "ping"}}
			m["backend"] = map[string]any{"network": []string{}}
		},
	}
	for name, edit := range cases {
		m := valid()
		if edit != nil {
			edit(m)
		}
		got, err := decode(t, m)
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if err := Validate(got); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]struct {
		edit  func(map[string]any)
		field string
	}{
		"format":        {func(m map[string]any) { m["format"] = 2 }, "format"},
		"id":            {func(m map[string]any) { m["id"] = "Hello" }, "id"},
		"id length":     {func(m map[string]any) { m["id"] = strings.Repeat("a", 25) }, "id"},
		"version":       {func(m map[string]any) { m["version"] = "1.0" }, "version"},
		"version build": {func(m map[string]any) { m["version"] = "1.0.0+abc" }, "version"},
		"version zero":  {func(m map[string]any) { m["version"] = "01.0.0" }, "version"},
		"sdk":           {func(m map[string]any) { m["sdk"] = 0 }, "sdk"},
		"no en":         {func(m map[string]any) { m["name"] = map[string]any{"ko": "안녕"} }, "name"},
		"language":      {func(m map[string]any) { m["name"] = map[string]any{"en": "Hi", "xx": "Hi"} }, "name.xx"},
		"long name":     {func(m map[string]any) { m["name"] = map[string]any{"en": strings.Repeat("가", 41)} }, "name.en"},
		"empty name":    {func(m map[string]any) { m["name"] = map[string]any{"en": "  "} }, "name.en"},
		"control":       {func(m map[string]any) { m["name"] = map[string]any{"en": "Hi\x07"} }, "name.en"},
		"bidi":          {func(m map[string]any) { m["name"] = map[string]any{"en": "Hi‮"} }, "name.en"},
		"author":        {func(m map[string]any) { m["author"] = map[string]any{} }, "author.name"},
		"author url":    {func(m map[string]any) { m["author"] = map[string]any{"name": "A", "url": "javascript:alert(1)"} }, "author.url"},
		"icon":          {func(m map[string]any) { m["icon"] = "../icon.svg" }, "icon"},
		"no views":      {func(m map[string]any) { m["views"] = map[string]any{} }, "views"},
		"entry dir": {func(m map[string]any) {
			m["views"].(map[string]any)["window"].(map[string]any)["entry"] = "pages/x.html"
		}, "views.window.entry"},
		"entry ext": {func(m map[string]any) { m["views"].(map[string]any)["window"].(map[string]any)["entry"] = "x.js" }, "views.window.entry"},
		"window size": {func(m map[string]any) {
			m["views"].(map[string]any)["window"].(map[string]any)["defaultSize"] = map[string]any{"width": 0, "height": 1}
		}, "views.window.defaultSize"},
		"widget size": {func(m map[string]any) {
			m["views"].(map[string]any)["widget"].(map[string]any)["maxSize"] = map[string]any{"cols": 13, "rows": 3}
		}, "views.widget"},
		"plugin name": {func(m map[string]any) {
			m["requires"] = map[string]any{"plugins": []any{map[string]any{"package": "nut", "rpcFile": "nut.inc"}}}
		}, "requires.plugins[0].package"},
		"plugin rpcFile": {func(m map[string]any) {
			m["requires"] = map[string]any{"plugins": []any{map[string]any{"package": "openmediavault-nut", "rpcFile": "../x"}}}
		}, "requires.plugins[0].rpcFile"},
		"method":          {func(m map[string]any) { m["rpc"] = map[string]any{"read": []string{"getStats"}} }, "rpc.read[0]"},
		"write no action": {func(m map[string]any) { m["rpc"] = map[string]any{"write": []string{"Nut.set"}} }, "rpc.write"},
		"action backend":  {func(m map[string]any) { m["actions"] = []any{map[string]any{"name": "go"}} }, "actions"},
		"action name": {func(m map[string]any) {
			m["actions"] = []any{map[string]any{"name": "Go"}}
			m["backend"] = map[string]any{"network": []string{}}
		}, "actions[0].name"},
		"action timeout": {func(m map[string]any) {
			m["actions"] = []any{map[string]any{"name": "go", "timeoutSeconds": 601}}
			m["backend"] = map[string]any{"network": []string{}}
		}, "actions[0].timeoutSeconds"},
		"confirm text": {func(m map[string]any) {
			m["actions"] = []any{map[string]any{"name": "go", "confirm": true}}
			m["backend"] = map[string]any{"network": []string{}}
		}, "actions[0].confirmText"},
		"network":     {func(m map[string]any) { m["backend"] = map[string]any{"network": []string{"internet"}} }, "backend.network[0]"},
		"network dup": {func(m map[string]any) { m["backend"] = map[string]any{"network": []string{"lan", "lan"}} }, "backend.network[1]"},
	}
	for name, c := range cases {
		m := valid()
		c.edit(m)
		got, err := decode(t, m)
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		err = Validate(got)
		var p *Problem
		if !errors.As(err, &p) || p.Field != c.field {
			t.Errorf("%s: err = %v, want field %s", name, err, c.field)
		}
	}
}

func TestValidateIncompatible(t *testing.T) {
	m := valid()
	m["sdk"] = 2
	got, err := decode(t, m)
	if err != nil {
		t.Fatal(err)
	}
	var p *Problem
	if err := Validate(got); !errors.As(err, &p) || !p.Incompatible {
		t.Errorf("err = %v", err)
	}
}

func TestDecodeStrict(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field": `{"format":1,"id":"x","extra":true}`,
		"trailing":      `{"format":1} {}`,
		"not object":    `[]`,
		"too large":     `{"id":"` + strings.Repeat("a", MaxManifestBytes) + `"}`,
	} {
		if _, err := Decode([]byte(body)); !errors.Is(err, ErrManifest) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	order := []string{"0.9.9", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.2.0", "1.10.0", "2.0.0"}
	for i := range order {
		for j := range order {
			got := CompareVersions(order[i], order[j])
			if (i < j && got >= 0) || (i > j && got <= 0) || (i == j && got != 0) {
				t.Errorf("CompareVersions(%s, %s) = %d", order[i], order[j], got)
			}
		}
	}
}

// 패키지가 쓸 수 있는 언어는 프론트의 지원 언어와 같아야 한다.
func TestLanguagesMatchFrontend(t *testing.T) {
	src, err := os.ReadFile("../../../../frontend/src/i18n/languages.ts")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)SUPPORTED_LANGUAGES = \[(.*?)\] as const`).FindSubmatch(src)
	if block == nil {
		t.Fatal("SUPPORTED_LANGUAGES not found in languages.ts")
	}
	var front []string
	for _, m := range regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(block[1], -1) {
		front = append(front, string(m[1]))
	}
	if !slices.Equal(front, Languages) {
		t.Errorf("format.Languages = %v, frontend = %v", Languages, front)
	}
}
