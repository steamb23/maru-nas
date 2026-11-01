package aptindex

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"maru/backend/internal/api"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseIndex(t *testing.T) {
	pkgs, err := ParseIndex(strings.NewReader(string(readFixture(t, "upgradeindex.json"))))
	if err != nil {
		t.Fatal(err)
	}
	// 이름 없는 항목은 버리고 이름순이다.
	var names []string
	for _, p := range pkgs {
		names = append(names, p.Name)
	}
	want := []string{"libc6", "linux-image-6.12.99+deb13-amd64", "openmediavault"}
	if !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	omv := pkgs[2]
	if omv.Version != "8.5.12-1" || omv.OldVersion != "8.5.11-1" || omv.Repository != "openmediavault/synchrony" ||
		omv.Section != "admin" || omv.Size != 4096000 || omv.Filename != "pool/main/o/openmediavault/openmediavault_8.5.12-1_all.deb" {
		t.Errorf("openmediavault = %+v", omv)
	}
	if pkgs[1].OldVersion != "" {
		t.Errorf("new package oldversion = %q", pkgs[1].OldVersion)
	}
}

func TestParseIndexEmptyAndBroken(t *testing.T) {
	pkgs, err := ParseIndex(strings.NewReader("[]"))
	if err != nil || len(pkgs) != 0 {
		t.Errorf("empty = %v, %v", pkgs, err)
	}
	// 쓰는 도중의 파일처럼 잘린 JSON은 오류다(알림 Center가 직전 결과를 유지한다).
	if _, err := ParseIndex(strings.NewReader(`[{"name":"libc6"`)); err == nil {
		t.Error("truncated index parsed")
	}
	if _, err := ParseIndex(strings.NewReader("")); err == nil {
		t.Error("empty file parsed")
	}
}

func TestParseIndexLongField(t *testing.T) {
	long := strings.Repeat("가", 400) // 1200바이트
	pkgs, err := ParseIndex(strings.NewReader(`[{"name":"x","summary":"` + long + `"}]`))
	if err != nil {
		t.Fatal(err)
	}
	s := pkgs[0].Summary
	if len(s) > maxFieldBytes || !strings.HasPrefix(long, s) {
		t.Errorf("summary not cut on a rune boundary: %d bytes", len(s))
	}
}

func TestParseRebootPkgs(t *testing.T) {
	got, err := ParseRebootPkgs(strings.NewReader(string(readFixture(t, "reboot-required.pkgs"))))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"linux-image-6.12.99+deb13-amd64", "dbus"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func testRoot(t *testing.T, withIndex, withReboot bool) fstest.MapFS {
	t.Helper()
	at := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	m := fstest.MapFS{}
	if withIndex {
		m[IndexPath] = &fstest.MapFile{Data: readFixture(t, "upgradeindex.json"), ModTime: at}
	}
	if withReboot {
		m[RebootPath] = &fstest.MapFile{Data: []byte("*** System restart required ***\n"), ModTime: at}
		m[RebootPkgsPath] = &fstest.MapFile{Data: readFixture(t, "reboot-required.pkgs"), ModTime: at}
	}
	return m
}

func TestReader(t *testing.T) {
	r := NewReaderFS(testRoot(t, true, true))
	idx, err := r.Index()
	if err != nil || len(idx.Packages) != 3 || idx.ModTime.IsZero() {
		t.Fatalf("Index = %+v, %v", idx, err)
	}
	rb, err := r.Reboot()
	if err != nil || !rb.Required || len(rb.Packages) != 2 || rb.Since.IsZero() {
		t.Fatalf("Reboot = %+v, %v", rb, err)
	}

	empty := NewReaderFS(fstest.MapFS{})
	if _, err := empty.Index(); !errors.Is(err, ErrNoIndex) {
		t.Errorf("missing index err = %v", err)
	}
	if _, err := empty.IndexModTime(); !errors.Is(err, ErrNoIndex) {
		t.Errorf("missing index modtime err = %v", err)
	}
	if rb, err := empty.Reboot(); err != nil || rb.Required {
		t.Errorf("no reboot file = %+v, %v", rb, err)
	}
}

func TestUpdatesSource(t *testing.T) {
	ctx := context.Background()
	got, err := UpdatesSource{NewReaderFS(testRoot(t, true, false))}.Collect(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("Collect = %v, %v", got, err)
	}
	n := got[0]
	if n.ID != UpdatesNoticeID || n.Code != api.NoticeAptUpdates || n.Level != api.NoticeInfo || *n.Count != 3 ||
		len(n.Items) != 3 || n.Action == nil || n.Action.App != UpdatesApp {
		t.Errorf("notice = %+v", n)
	}

	// 내용이 같으면 파일을 다시 써도(수정 시각이 바뀌어도) 리비전이 같다.
	root := testRoot(t, true, false)
	root[IndexPath].ModTime = time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)
	again, _ := UpdatesSource{NewReaderFS(root)}.Collect(ctx)
	if again[0].Revision != n.Revision {
		t.Error("revision changed with the same content")
	}
	// 버전이 바뀌면 리비전도 바뀐다.
	root[IndexPath].Data = []byte(strings.Replace(string(root[IndexPath].Data), `"version": "8.5.12-1"`, `"version": "8.5.13-1"`, 1))
	changed, _ := UpdatesSource{NewReaderFS(root)}.Collect(ctx)
	if changed[0].Revision == n.Revision {
		t.Error("revision did not change with a new version")
	}

	// 목록이 없거나 비어 있으면 알리지 않는다.
	if got, err := (UpdatesSource{NewReaderFS(fstest.MapFS{})}).Collect(ctx); err != nil || len(got) != 0 {
		t.Errorf("no index = %v, %v", got, err)
	}
	emptyRoot := fstest.MapFS{IndexPath: &fstest.MapFile{Data: []byte("[]")}}
	if got, err := (UpdatesSource{NewReaderFS(emptyRoot)}).Collect(ctx); err != nil || len(got) != 0 {
		t.Errorf("empty index = %v, %v", got, err)
	}
	// 깨진 파일은 오류다(Center가 직전 결과를 유지한다).
	broken := fstest.MapFS{IndexPath: &fstest.MapFile{Data: []byte("[{")}}
	if _, err := (UpdatesSource{NewReaderFS(broken)}).Collect(ctx); err == nil {
		t.Error("broken index collected")
	}
}

func TestRebootSource(t *testing.T) {
	ctx := context.Background()
	got, err := RebootSource{NewReaderFS(testRoot(t, false, true))}.Collect(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("Collect = %v, %v", got, err)
	}
	n := got[0]
	if n.ID != RebootNoticeID || n.Code != api.NoticeRebootRequired || n.Level != api.NoticeWarning ||
		n.Since != "2026-10-20T03:00:00Z" || len(n.Items) != 2 || n.Action == nil || n.Action.Kind != api.NoticeActionPower {
		t.Errorf("notice = %+v", n)
	}
	if got, err := (RebootSource{NewReaderFS(fstest.MapFS{})}).Collect(ctx); err != nil || len(got) != 0 {
		t.Errorf("no reboot = %v, %v", got, err)
	}
}

func TestIndexDescription(t *testing.T) {
	pkgs, err := ParseIndex(strings.NewReader(string(readFixture(t, "upgradeindex.json"))))
	if err != nil {
		t.Fatal(err)
	}
	// omv-mkaptidx가 만든 문단 사이의 빈 줄 둘은 하나로 줄인다.
	want := "Example NAS solution based on Debian Linux. It contains services\nlike SSH, SMB/CIFS and rsync.\n\nFeatures:\n* web administration\n* plugin system"
	if got := pkgs[2].Description; got != want {
		t.Errorf("description = %q", got)
	}
	if pkgs[0].Description != "" {
		t.Errorf("libc6 description = %q", pkgs[0].Description)
	}
	if got := IndexDescription("\n\n  a  \n\n\n\nb\n\n"); got != "  a\n\nb" {
		t.Errorf("tidy = %q", got)
	}
}

func TestParseInstalledDescription(t *testing.T) {
	status := string(readFixture(t, "dpkg-status"))
	cases := []struct {
		name, want string
		found      bool
	}{
		// 요약(첫 줄)은 빼고, " ."은 빈 줄, 두 칸 이상 들여쓰기는 남긴다.
		{"libc6", "Contains the standard libraries that are used by nearly all programs on\n" +
			"the system. This package includes shared versions of the standard C library\n" +
			"and the standard math library.\n\nExample list:\n * first item that is long enough to\n" +
			"   wrap onto a second line\n * second item", true},
		{"libc6:i386", "The i386 copy.", true},
		{"held-pkg", "A held package is still installed.", true},
		{"summary-only", "", true},
		{"removed-pkg", "", false},
		{"libc", "", false},
		{"libc6:arm64", "", false},
	}
	for _, c := range cases {
		got, found, err := ParseInstalledDescription(strings.NewReader(status), c.name)
		if err != nil || found != c.found || got != c.want {
			t.Errorf("%s = %q, %v, %v", c.name, got, found, err)
		}
	}
}

func TestDescriptionLimit(t *testing.T) {
	long := strings.Repeat("word ", 30) + "\n"
	got := IndexDescription(strings.Repeat(long, 1000))
	if len(got) > maxDescriptionBytes || !strings.HasSuffix(got, "word") {
		t.Errorf("len = %d, tail %q", len(got), got[max(len(got)-10, 0):])
	}
}

func TestReaderInstalledDescription(t *testing.T) {
	r := NewReaderFS(fstest.MapFS{DpkgStatusPath: {Data: readFixture(t, "dpkg-status")}})
	if got, err := r.InstalledDescription("libc6:i386"); err != nil || got != "The i386 copy." {
		t.Errorf("libc6:i386 = %q, %v", got, err)
	}
	if got, err := r.InstalledDescription("nope"); err != nil || got != "" {
		t.Errorf("nope = %q, %v", got, err)
	}
	// 파일이 없으면(개발 환경) 설명이 없을 뿐 오류가 아니다.
	if got, err := NewReaderFS(fstest.MapFS{}).InstalledDescription("libc6"); err != nil || got != "" {
		t.Errorf("no status = %q, %v", got, err)
	}
	dir := NewReaderFS(fstest.MapFS{DpkgStatusPath + "/x": {Data: []byte("x")}})
	if _, err := dir.InstalledDescription("libc6"); err == nil {
		t.Error("directory status parsed")
	}
}
