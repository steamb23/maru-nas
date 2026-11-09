package prefs

import (
	"os"
	"path/filepath"
	"testing"

	"maru/backend/internal/power"
)

func TestPowerRecord(t *testing.T) {
	s, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.PowerRecord(); ok || err != nil {
		t.Fatalf("empty: %v %v", ok, err)
	}
	// 예약이 없으면 지울 것도 없다.
	if err := s.ClearPowerRecord(); err != nil {
		t.Fatal(err)
	}
	rec := power.Record{USEC: 1794231164000000, Mode: "reboot", By: "bob"}
	if err := s.SetPowerRecord(rec); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.root, "power.json")
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("power.json: %v %v", fi, err)
	}
	if got, ok, err := s.PowerRecord(); !ok || err != nil || got != rec {
		t.Fatalf("read back: %+v %v %v", got, ok, err)
	}
	if err := s.ClearPowerRecord(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("after clear: %v", err)
	}
	// 모르는 버전은 기록이 없는 것으로, 깨진 파일은 오류로 본다.
	os.WriteFile(path, []byte(`{"version":2,"usec":1,"mode":"reboot","by":"bob"}`), 0o600)
	if _, ok, err := s.PowerRecord(); ok || err != nil {
		t.Fatalf("unknown version: %v %v", ok, err)
	}
	os.WriteFile(path, []byte(`{`), 0o600)
	if _, _, err := s.PowerRecord(); err == nil {
		t.Fatal("corrupt file: no error")
	}
}
