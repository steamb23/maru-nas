package packages

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/aptindex"
)

func TestRead(t *testing.T) {
	at := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	root := fstest.MapFS{
		aptindex.IndexPath: &fstest.MapFile{ModTime: at, Data: []byte(`[
			{"name": "openmediavault", "version": "8.5.12-1", "oldversion": "8.5.11-1", "repository": "openmediavault/synchrony",
			 "section": "admin", "summary": "openmediavault", "filename": "pool/main/o/openmediavault/openmediavault_8.5.12-1_all.deb", "size": 4096000},
			{"name": "linux-image-6.12.99+deb13-amd64", "version": "6.12.99-1", "oldversion": "", "size": 1}
		]`)},
		aptindex.RebootPath:     &fstest.MapFile{ModTime: at},
		aptindex.RebootPkgsPath: &fstest.MapFile{Data: []byte("linux-image-6.12.99+deb13-amd64\n")},
	}
	res, err := Read(aptindex.NewReaderFS(root))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Indexed || res.IndexedAt != "2026-10-20T03:00:00Z" || len(res.Packages) != 2 || res.Sources[0] != Source {
		t.Fatalf("res = %+v", res)
	}
	omv := res.Packages[1]
	if omv.Name != "openmediavault" || omv.OldVersion != "8.5.11-1" || omv.Size != 4096000 || omv.Filename == "" {
		t.Errorf("package = %+v", omv)
	}
	if res.Packages[0].OldVersion != "" {
		t.Errorf("new package = %+v", res.Packages[0])
	}
	if !res.Reboot.Required || res.Reboot.Since != "2026-10-20T03:00:00Z" || len(res.Reboot.Packages) != 1 {
		t.Errorf("reboot = %+v", res.Reboot)
	}

	empty, err := Read(aptindex.NewReaderFS(fstest.MapFS{}))
	if err != nil || empty.Indexed || empty.Packages == nil || len(empty.Packages) != 0 || empty.Reboot.Required {
		t.Errorf("empty = %+v, %v", empty, err)
	}
	if _, err := Read(aptindex.NewReaderFS(fstest.MapFS{aptindex.IndexPath: &fstest.MapFile{Data: []byte("[")}})); err == nil {
		t.Error("broken index read")
	}
}

type starter struct {
	kinds []string
	err   error
}

func (s *starter) Start(_ context.Context, kind string) (string, error) {
	s.kinds = append(s.kinds, kind)
	return "id", s.err
}

func TestActions(t *testing.T) {
	a := New(aptindex.NewReaderFS(fstest.MapFS{}))
	s := &starter{}
	for name, kind := range map[string]string{"check": "apt-update", "upgrade": "apt-upgrade"} {
		s.kinds = nil
		if err := a.Actions[name].Run(context.Background(), addons.ActionRequest{Admin: true, Jobs: s}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(s.kinds) != 1 || s.kinds[0] != kind {
			t.Errorf("%s started %v", name, s.kinds)
		}
	}
	// 대상은 받지 않는다(브라우저가 보낸 값을 작업에 넘기지 않는다).
	if err := a.Actions["upgrade"].Run(context.Background(), addons.ActionRequest{Target: "libc6", Jobs: s}); !errors.Is(err, addons.ErrInvalidTarget) {
		t.Errorf("target err = %v", err)
	}
	refused := &addons.ActionError{Code: "job_busy"}
	s.err = refused
	if err := a.Actions["check"].Run(context.Background(), addons.ActionRequest{Jobs: s}); !errors.Is(err, refused) {
		t.Errorf("refusal err = %v", err)
	}
	if err := a.Actions["check"].Run(context.Background(), addons.ActionRequest{}); err == nil {
		t.Error("ran without a job starter")
	}
}
