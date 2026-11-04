package addonpkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"

	"maru/backend/internal/addonpkg/format"
)

// entry는 합성 압축 파일의 항목 하나다.
type entry struct {
	name string
	typ  byte
	body []byte
	link string
	mode int64
}

func file(name, body string) entry { return entry{name: name, typ: tar.TypeReg, body: []byte(body)} }

// pack은 항목을 gzip tar로 묶는다.
func pack(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o600
		}
		hdr := &tar.Header{Name: e.name, Typeflag: e.typ, Mode: mode, Size: int64(len(e.body)), Linkname: e.link}
		if e.typ != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Size > 0 {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// manifestJSON은 검사를 통과하는 최소 manifest에 edit을 적용한 JSON이다.
func manifestJSON(t *testing.T, edit func(map[string]any)) string {
	t.Helper()
	m := map[string]any{
		"format":  1,
		"id":      "hello",
		"version": "1.0.0",
		"sdk":     1,
		"name":    map[string]any{"en": "Hello", "ko": "안녕"},
		"author":  map[string]any{"name": "Example"},
		"icon":    "icon.svg",
		"views": map[string]any{
			"window": map[string]any{"entry": "window.html", "defaultSize": map[string]any{"width": 480, "height": 320}},
		},
	}
	if edit != nil {
		edit(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const iconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16"/></svg>`

// basic은 manifest를 뺀 최소 패키지 항목이다.
func basic(manifest string) []entry {
	return []entry{
		file("manifest.json", manifest),
		file("icon.svg", iconSVG),
		{name: "i18n/", typ: tar.TypeDir},
		file("i18n/en.json", `{"greeting":"Hello"}`),
		file("ui/window.html", `<!doctype html><script src="./_maru/sdk.js"></script>`),
		file("ui/assets/app.js", `console.log(1)`),
	}
}

// fakeELF는 machine용 64비트 리틀 엔디언 ELF 머리만 있는 실행 파일이다.
func fakeELF(machine elf.Machine) []byte {
	b := make([]byte, 64)
	copy(b, elf.ELFMAG)
	b[elf.EI_CLASS] = byte(elf.ELFCLASS64)
	b[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	b[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	le := binary.LittleEndian
	le.PutUint16(b[16:], uint16(elf.ET_EXEC))
	le.PutUint16(b[18:], uint16(machine))
	le.PutUint32(b[20:], uint32(elf.EV_CURRENT))
	le.PutUint16(b[52:], 64) // ehsize
	le.PutUint16(b[54:], 56) // phentsize
	le.PutUint16(b[58:], 64) // shentsize
	return b
}

// stagingRoot는 빈 임시 디렉터리를 os.Root로 연다.
func stagingRoot(t *testing.T) (string, *os.Root) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return dir, root
}

func mustManifest(t *testing.T, data string) *format.Manifest {
	t.Helper()
	m, err := format.Decode([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := format.Validate(m); err != nil {
		t.Fatal(err)
	}
	return m
}
