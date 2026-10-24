package prefs

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"maru/backend/internal/jobs"
)

const testJobID = "20261024-101500-1a2b3c4d"

func TestJobsCurrent(t *testing.T) {
	s, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cur, err := s.LoadCurrent(); cur != nil || err != nil {
		t.Fatalf("empty = %+v, %v", cur, err)
	}
	rec := &jobs.Record{ID: testJobID, Kind: jobs.KindAptUpgrade, Owner: "admin", Runner: "omv-session",
		Handle: json.RawMessage(`{"filename":"/tmp/bgstatusAbc","pos":12}`), StartedAt: time.Date(2026, 10, 24, 10, 15, 0, 0, time.UTC),
		Output: jobs.OutputRange{End: 12}, Before: []string{"libc6@2.41-99"}}
	if err := s.SaveCurrent(rec); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.root, "jobs", "current.json")
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("current.json mode = %v, %v", st, err)
	}
	if st, err := os.Stat(filepath.Dir(path)); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("jobs dir mode = %v, %v", st, err)
	}
	got, err := s.LoadCurrent()
	var handle bytes.Buffer
	if err == nil {
		json.Compact(&handle, got.Handle)
	}
	if err != nil || got.ID != rec.ID || handle.String() != string(rec.Handle) || got.Output.End != 12 || len(got.Before) != 1 {
		t.Fatalf("loaded = %+v, %v", got, err)
	}
	if err := s.SaveCurrent(nil); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.LoadCurrent(); cur != nil {
		t.Error("current not deleted")
	}
	if err := s.SaveCurrent(nil); err != nil {
		t.Errorf("deleting twice: %v", err)
	}

	// 읽을 수 없는 기록은 옮겨 두고 없는 것으로 본다(새 작업을 막지 않게).
	os.WriteFile(path, []byte("{"), 0o600)
	if cur, err := s.LoadCurrent(); cur != nil || err != nil {
		t.Errorf("corrupt = %+v, %v", cur, err)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Error("corrupt file not kept")
	}
	os.WriteFile(path, []byte(`{"version":99,"id":"x"}`), 0o600)
	if cur, _ := s.LoadCurrent(); cur != nil {
		t.Error("unknown version loaded")
	}
}

func TestJobsResults(t *testing.T) {
	s, _ := NewFileStore(t.TempDir())
	if rs, err := s.LoadResults("admin"); len(rs) != 0 || err != nil {
		t.Fatalf("empty = %v, %v", rs, err)
	}
	count := 2
	rs := []jobs.Result{{ID: testJobID, Kind: jobs.KindAptUpdate, Owner: "a/../b", State: "succeeded", Count: &count, Items: []string{"x", "y"}}}
	if err := s.SaveResults("a/../b", rs); err != nil {
		t.Fatal(err)
	}
	// 사용자 이름은 파일 이름으로 인코딩해 디렉터리를 벗어나지 못한다.
	if _, err := os.Stat(filepath.Join(s.root, "jobs", "results", "a%2F%2E%2E%2Fb.json")); err != nil {
		t.Errorf("results file: %v", err)
	}
	got, err := s.LoadResults("a/../b")
	if err != nil || len(got) != 1 || *got[0].Count != 2 || got[0].Items[1] != "y" {
		t.Fatalf("loaded = %+v, %v", got, err)
	}
	if rs, _ := s.LoadResults("other"); len(rs) != 0 {
		t.Error("results shared between users")
	}
	if err := s.SaveResults("a/../b", nil); err != nil {
		t.Fatal(err)
	}
	if rs, _ := s.LoadResults("a/../b"); len(rs) != 0 {
		t.Error("results not deleted")
	}
	if _, err := s.LoadResults(""); err == nil {
		t.Error("empty user accepted")
	}
}

func TestJobsOutput(t *testing.T) {
	s, _ := NewFileStore(t.TempDir())
	if err := s.AppendOutput("../x", []byte("a")); err == nil {
		t.Error("invalid job id accepted")
	}
	if b, err := s.ReadOutput(testJobID, 0, 10); len(b) != 0 || err != nil {
		t.Errorf("missing output = %q, %v", b, err)
	}
	s.AppendOutput(testJobID, []byte("line1\nline2\n"))
	s.AppendOutput(testJobID, []byte("line3\n"))
	if b, _ := s.ReadOutput(testJobID, 6, 100); string(b) != "line2\nline3\n" {
		t.Errorf("read = %q", b)
	}
	if err := s.TruncateOutput(testJobID, 12); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.ReadOutput(testJobID, 0, 100); string(b) != "line1\nline2\n" {
		t.Errorf("after truncate = %q", b)
	}
	// 앞을 버리면 줄 경계부터 남긴다.
	dropped, err := s.DropOutputHead(testJobID, 8)
	if err != nil || dropped != 6 {
		t.Fatalf("dropped = %d, %v", dropped, err)
	}
	if b, _ := s.ReadOutput(testJobID, 0, 100); string(b) != "line2\n" {
		t.Errorf("after drop = %q", b)
	}
	if st, _ := os.Stat(filepath.Join(s.root, "jobs", "output", testJobID+".log")); st.Mode().Perm() != 0o600 {
		t.Errorf("output mode = %v", st.Mode())
	}

	other := "20261001-000000-00000000"
	s.AppendOutput(other, []byte("old"))
	old := time.Now().Add(-40 * 24 * time.Hour)
	os.Chtimes(filepath.Join(s.root, "jobs", "output", other+".log"), old, old)
	os.Chtimes(filepath.Join(s.root, "jobs", "output", testJobID+".log"), old, old)
	if err := s.PruneOutputs(time.Now().Add(-31*24*time.Hour), testJobID); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(s.root, "jobs", "output"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != testJobID+".log" {
		t.Errorf("after prune = %v", names)
	}
	if err := s.DeleteOutput(testJobID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteOutput(testJobID); err != nil {
		t.Errorf("deleting twice: %v", err)
	}
}
