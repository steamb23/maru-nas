package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDevNotices(t *testing.T) {
	const doc = `{
		"admin": [{"id": "sample-a", "code": "sample", "level": "warning", "count": 3}],
		"everyone": [{"id": "sample-b", "code": "sample", "level": "info", "revision": "fixed"}]
	}`
	f, err := parseDevNotices(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Admin) != 1 || f.Admin[0].Revision == "" || *f.Admin[0].Count != 3 {
		t.Errorf("admin = %+v", f.Admin)
	}
	if len(f.Everyone) != 1 || f.Everyone[0].Revision != "fixed" {
		t.Errorf("everyone = %+v", f.Everyone)
	}
	// 내용이 바뀌면 채운 리비전도 바뀐다.
	g, err := parseDevNotices(strings.NewReader(strings.Replace(doc, `"count": 3`, `"count": 4`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if g.Admin[0].Revision == f.Admin[0].Revision {
		t.Errorf("revision did not change with content")
	}
	if _, err := parseDevNotices(strings.NewReader(`{"admin": [`)); err == nil {
		t.Errorf("malformed file accepted")
	}
}

func TestDevNoticesCollect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notices.json")
	admin, everyone := devNotices{path: path, admin: true}, devNotices{path: path}

	// 가짜 OMV가 파일을 쓰기 전에는 알릴 것이 없다.
	if got, err := admin.Collect(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("missing file: %v, %v", got, err)
	}
	doc := `{"admin": [{"id": "a", "code": "sample", "level": "error"}], "everyone": [{"id": "b", "code": "sample", "level": "info"}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	check := func(s devNotices, want string) {
		t.Helper()
		got, err := s.Collect(context.Background())
		if err != nil || len(got) != 1 || got[0].ID != want {
			t.Errorf("%s: %+v, %v", s.Name(), got, err)
		}
	}
	check(admin, "a")
	check(everyone, "b")
	if !admin.Admin() || everyone.Admin() {
		t.Errorf("admin flags wrong")
	}
}
