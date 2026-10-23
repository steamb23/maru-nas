package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/notify"
)

// staticSource는 정해 둔 알림을 돌려주는 알림 출처다.
type staticSource []api.Notice

func (staticSource) Name() string                                    { return "fake" }
func (staticSource) Admin() bool                                     { return false }
func (s staticSource) Collect(context.Context) ([]api.Notice, error) { return s, nil }

// TestFakeNoticesPassValidation은 시나리오의 알림이 maru의 검사를 모두 통과하는지 확인한다.
func TestFakeNoticesPassValidation(t *testing.T) {
	since := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for _, scenario := range []string{noticesSample, noticesNone, noticesMany} {
		for group, list := range fakeNotices(scenario, since, 3) {
			for i := range list {
				list[i].Revision = notify.Revision(list[i].ID) // maru가 채우는 값 대신
			}
			got := notify.New(staticSource(list)).Notices(context.Background(), true)
			if len(got) != len(list) {
				t.Errorf("%s/%s: %d of %d notices accepted", scenario, group, len(got), len(list))
			}
		}
	}
}

func TestWriteNotices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notices.json")
	if err := startNotices(path, noticesSample, time.Hour); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f map[string][]api.Notice
	if err := json.Unmarshal(b, &f); err != nil || len(f["admin"]) != 2 || len(f["everyone"]) != 1 {
		t.Errorf("file = %s, %v", b, err)
	}
	if err := startNotices(path, "bogus", time.Hour); err == nil {
		t.Errorf("unknown scenario accepted")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temp files left: %v", entries)
	}
}
