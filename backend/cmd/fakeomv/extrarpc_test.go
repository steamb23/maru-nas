package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExtraRPC(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a.json", `{"Example.getStatus": {"result": 1}, "Other.get": {"result": null, "admin": true}}`)
	b := write("b.json", `{"example.getStatus": {"result": 2}, "Third.get": {"result": []}}`)

	var logs []string
	logf := func(f string, args ...any) { logs = append(logs, fmt.Sprintf(f, args...)) }
	got, err := loadExtraRPC(a+"::"+b, logf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || string(got["Example.getStatus"].Result) != "1" || !got["Other.get"].Admin || got["Third.get"].Result == nil {
		t.Errorf("merged = %v", got)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "example.getStatus in "+b+" is ignored") {
		t.Errorf("logs = %v", logs)
	}

	if _, err := loadExtraRPC(filepath.Join(dir, "missing.json"), logf); err == nil {
		t.Error("missing file accepted")
	}
	bad := write("bad.json", `{"Example.get": {"admin": true}}`)
	if _, err := loadExtraRPC(bad, logf); err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("bad file: %v", err)
	}
}
