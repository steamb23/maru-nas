package addonpkg

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func readPkg(t *testing.T, data []byte, lim Limits) (*Package, string, error) {
	t.Helper()
	dir, root := stagingRoot(t)
	p, err := Read(bytes.NewReader(data), lim, root, "amd64")
	return p, dir, err
}

func TestReadValid(t *testing.T) {
	data := pack(t, append(basic(manifestJSON(t, nil)), file("i18n/ko.json", `{"greeting":"안녕"}`), file("LICENSE", "MIT")))
	p, dir, err := readPkg(t, data, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if p.SHA256 != hex.EncodeToString(sum[:]) || p.Manifest.ID != "hello" || p.Rev() != "1.0.0-"+p.SHA256[:8] {
		t.Errorf("package = %+v", p)
	}
	if !slices.Equal(p.Languages, []string{"en", "ko"}) || len(p.Backends) != 0 {
		t.Errorf("languages %v, backends %v", p.Languages, p.Backends)
	}
	// 받은 권한(0600)이 아니라 정한 권한으로 둔다.
	for name, want := range map[string]fs.FileMode{"ui": 0o755, "ui/assets": 0o755, "ui/window.html": 0o644, "icon.svg": 0o644} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s: mode %v, want %v", name, got, want)
		}
	}
}

func TestReadBackend(t *testing.T) {
	m := manifestJSON(t, func(m map[string]any) { m["backend"] = map[string]any{"network": []string{}} })
	data := pack(t, append(basic(m),
		entry{name: "backend/linux-amd64", typ: tar.TypeReg, body: fakeELF(elf.EM_X86_64)},
		entry{name: "backend/linux-arm64", typ: tar.TypeReg, body: fakeELF(elf.EM_AARCH64)},
	))
	p, _, err := readPkg(t, data, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Backends, []string{"amd64", "arm64"}) || !p.Manifest.HasBackend() {
		t.Errorf("backends = %v", p.Backends)
	}
}

func TestReadRejects(t *testing.T) {
	small := DefaultLimits
	small.Total = 100
	cases := map[string]struct {
		entries []entry
		lim     Limits
		reason  string
	}{
		"symlink":       {append(basic(manifestJSON(t, nil)), entry{name: "ui/x", typ: tar.TypeSymlink, link: "/etc/passwd"}), DefaultLimits, ReasonArchive},
		"hardlink":      {append(basic(manifestJSON(t, nil)), entry{name: "ui/x", typ: tar.TypeLink, link: "icon.svg"}), DefaultLimits, ReasonArchive},
		"fifo":          {append(basic(manifestJSON(t, nil)), entry{name: "ui/x", typ: tar.TypeFifo}), DefaultLimits, ReasonArchive},
		"dot dot":       {append(basic(manifestJSON(t, nil)), file("ui/../../etc/x", "x")), DefaultLimits, ReasonArchive},
		"absolute":      {append(basic(manifestJSON(t, nil)), file("/ui/x.js", "x")), DefaultLimits, ReasonArchive},
		"backslash":     {append(basic(manifestJSON(t, nil)), file(`ui\x.js`, "x")), DefaultLimits, ReasonArchive},
		"duplicate":     {append(basic(manifestJSON(t, nil)), file("ui/window.html", "again")), DefaultLimits, ReasonArchive},
		"hidden":        {append(basic(manifestJSON(t, nil)), file("ui/.env", "x")), DefaultLimits, ReasonArchive},
		"reserved":      {append(basic(manifestJSON(t, nil)), file("ui/_maru/sdk.js", "x")), DefaultLimits, ReasonArchive},
		"top level":     {append(basic(manifestJSON(t, nil)), file("run.sh", "x")), DefaultLimits, ReasonArchive},
		"top dir":       {append(basic(manifestJSON(t, nil)), entry{name: "etc/", typ: tar.TypeDir}), DefaultLimits, ReasonArchive},
		"i18n subdir":   {append(basic(manifestJSON(t, nil)), file("i18n/x/en.json", "{}")), DefaultLimits, ReasonArchive},
		"i18n language": {append(basic(manifestJSON(t, nil)), file("i18n/xx.json", "{}")), DefaultLimits, ReasonArchive},
		"backend name":  {append(basic(manifestJSON(t, nil)), file("backend/run", "x")), DefaultLimits, ReasonArchive},
		"total":         {basic(manifestJSON(t, nil)), small, ReasonTooLarge},
		"entries":       {basic(manifestJSON(t, nil)), Limits{Compressed: 1 << 20, Total: 1 << 20, Entries: 3, UIFile: 1 << 20, I18n: 1 << 20, Icon: 1 << 20}, ReasonTooLarge},
		"compressed":    {basic(manifestJSON(t, nil)), Limits{Compressed: 50, Total: 1 << 20, Entries: 100, UIFile: 1 << 20, I18n: 1 << 20, Icon: 1 << 20}, ReasonTooLarge},
		"no manifest":   {basic(manifestJSON(t, nil))[1:], DefaultLimits, ReasonManifest},
		"unknown field": {basic(manifestJSON(t, func(m map[string]any) { m["extra"] = 1 })), DefaultLimits, ReasonManifest},
		"sdk too new":   {basic(manifestJSON(t, func(m map[string]any) { m["sdk"] = 99 })), DefaultLimits, ReasonIncompatible},
		"no entry file": {basic(manifestJSON(t, nil))[:4], DefaultLimits, ReasonManifest},
		"no en":         {slices.Delete(basic(manifestJSON(t, nil)), 3, 4), DefaultLimits, ReasonManifest},
		"icon not svg":  {append(slices.Delete(basic(manifestJSON(t, nil)), 1, 2), file("icon.svg", "\x89PNG")), DefaultLimits, ReasonManifest},
		"i18n not flat": {append(slices.Delete(basic(manifestJSON(t, nil)), 3, 4), file("i18n/en.json", `{"a":{"b":"c"}}`)), DefaultLimits, ReasonManifest},
		"i18n bad key":  {append(slices.Delete(basic(manifestJSON(t, nil)), 3, 4), file("i18n/en.json", `{"a b":"c"}`)), DefaultLimits, ReasonManifest},
		"stray backend": {append(basic(manifestJSON(t, nil)), entry{name: "backend/linux-amd64", typ: tar.TypeReg, body: fakeELF(elf.EM_X86_64)}), DefaultLimits, ReasonManifest},
		"arch missing": {append(basic(manifestJSON(t, func(m map[string]any) { m["backend"] = map[string]any{"network": []string{}} })),
			entry{name: "backend/linux-arm64", typ: tar.TypeReg, body: fakeELF(elf.EM_AARCH64)}), DefaultLimits, ReasonArchMissing},
		"wrong machine": {append(basic(manifestJSON(t, func(m map[string]any) { m["backend"] = map[string]any{"network": []string{}} })),
			entry{name: "backend/linux-amd64", typ: tar.TypeReg, body: fakeELF(elf.EM_AARCH64)}), DefaultLimits, ReasonBackend},
		"script backend": {append(basic(manifestJSON(t, func(m map[string]any) { m["backend"] = map[string]any{"network": []string{}} })),
			file("backend/linux-amd64", "#!/bin/sh\nid\n")), DefaultLimits, ReasonBackend},
		"file and dir": {append(basic(manifestJSON(t, nil)), file("ui/assets", "x")), DefaultLimits, ReasonArchive},
	}
	for name, c := range cases {
		_, _, err := readPkg(t, pack(t, c.entries), c.lim)
		var e *Error
		if !errors.As(err, &e) || e.Reason != c.reason {
			t.Errorf("%s: err = %v, want reason %s", name, err, c.reason)
		}
	}
}

func TestReadNotGzip(t *testing.T) {
	_, _, err := readPkg(t, []byte("PK\x03\x04 not a tarball"), DefaultLimits)
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonArchive {
		t.Errorf("err = %v", err)
	}
}

// gzip 폭탄: 압축은 작아도 풀린 크기가 상한을 넘으면 멈춘다.
func TestReadBomb(t *testing.T) {
	big := strings.Repeat("A", 9<<20)
	_, _, err := readPkg(t, pack(t, append(basic(manifestJSON(t, nil)), file("ui/big.js", big))), DefaultLimits)
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonTooLarge {
		t.Errorf("err = %v", err)
	}
}
