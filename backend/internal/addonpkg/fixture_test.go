package addonpkg

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestReadSDKPackedFixture는 SDK CLI(maru-addon pack)가 묶은 고정 파일을 설치 경로(Read)가 받는지 본다. 고정 파일은
// sdk/testdata/dev-min을 묶은 것이고, 같은 디렉터리를 다시 묶어 같은 tar가 나오는지는 프론트 테스트
// (frontend/src/addonpkg/cli.node.test.ts)가 본다.
func TestReadSDKPackedFixture(t *testing.T) {
	f, err := os.Open(filepath.Join(repoRoot, "sdk/testdata/packed/dev-min.maru-addon"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	pkg, err := Read(f, DefaultLimits, root, "amd64")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if pkg.Manifest.ID != "dev-min" || pkg.Manifest.Version != "0.1.0" {
		t.Errorf("manifest = %s %s", pkg.Manifest.ID, pkg.Manifest.Version)
	}

	// 풀린 파일이 원본 디렉터리와 같다.
	src := filepath.Join(repoRoot, "sdk/testdata/dev-min")
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		want, _ := os.ReadFile(p)
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			return nil
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: contents differ", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
