package addonpkg

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeDir는 entries의 일반 파일로 풀린 패키지 디렉터리를 만든다(디렉터리 항목은 건너뛴다).
func writeDir(t *testing.T, entries []entry) string {
	t.Helper()
	dir := t.TempDir()
	for _, e := range entries {
		if e.body == nil {
			continue
		}
		p := filepath.Join(dir, filepath.FromSlash(e.name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, e.body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestReadDirBackendWithoutExecutable은 새로 받은 체크아웃의 백엔드 예제처럼 실행 파일(backend/linux-*)이 없는 백엔드
// 패키지를 개발 디렉터리로는 받고, 같은 파일을 묶은 압축 파일은 설치하지 않는지 본다. 개발 디렉터리의 dev/(가짜 OMV
// 응답 등)는 검사하지도 rev에 넣지도 않는다.
func TestReadDirBackendWithoutExecutable(t *testing.T) {
	m := manifestJSON(t, func(m map[string]any) {
		m["backend"] = map[string]any{"network": []string{}}
		m["actions"] = []any{map[string]any{"name": "run"}}
	})
	dir := writeDir(t, basic(m))

	p, err := ReadDir(dir, DefaultLimits, "amd64")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if !p.Manifest.HasBackend() || len(p.Backends) != 0 {
		t.Errorf("backend %v, executables %v", p.Manifest.HasBackend(), p.Backends)
	}
	rev := p.DevRev()

	// dev/ 안의 파일은 패키지 규칙(최상위에 둘 수 있는 이름, 크기)에 맞지 않아도 되고 rev를 바꾸지 않는다.
	for name, body := range map[string]string{
		"dev/fake-rpc.json":      `{"Example.get": {"result": 1}}`,
		"dev/nested/notes.txt":   "not part of the package",
		"dev/.hidden-and-wrong~": "x",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err = ReadDir(dir, DefaultLimits, "amd64")
	if err != nil {
		t.Fatalf("ReadDir with dev/: %v", err)
	}
	if p.DevRev() != rev {
		t.Errorf("dev/ changed the rev: %s → %s", rev, p.DevRev())
	}

	// 하위 디렉터리의 dev는 건너뛰지 않는다(최상위 dev/만 개발용이다).
	os.MkdirAll(filepath.Join(dir, "ui", "dev"), 0o755)
	os.WriteFile(filepath.Join(dir, "ui", "dev", "x.js"), []byte("1"), 0o644)
	if p, err := ReadDir(dir, DefaultLimits, "amd64"); err != nil || p.DevRev() == rev {
		t.Errorf("ui/dev: rev %v, err %v", p, err)
	}

	// 압축 파일(설치)은 실행 중인 아키텍처의 실행 파일이 있어야 한다.
	_, _, err = readPkg(t, pack(t, basic(m)), DefaultLimits)
	var pe *Error
	if !errors.As(err, &pe) || pe.Reason != ReasonArchMissing {
		t.Errorf("archive without executable: %v", err)
	}
}
