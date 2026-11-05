// SPDX-License-Identifier: MIT

package maruaddon

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/steamb23/maru-nas/sdk/go"

// TestStandardLibraryOnly는 SDK 모듈(시험 포함)이 표준 라이브러리 밖의 패키지에 기대지 않는지 본다.
// 패키지 작성자가 SDK를 쓸 때 다른 모듈을 끌어오지 않게 한다(.claude/rules/addon-packages.md).
func TestStandardLibraryOnly(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go command not found")
	}
	cmd := exec.Command(goBin, "list", "-deps", "-test", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "./...")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	n := 0
	for sc.Scan() {
		p := strings.TrimSpace(sc.Text())
		if p == "" {
			continue
		}
		n++
		if p != modulePath && !strings.HasPrefix(p, modulePath+"/") {
			t.Errorf("depends on a package outside the standard library: %s", p)
		}
	}
	if n == 0 {
		t.Error("go list found no packages of this module")
	}

	mod, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(mod)), "\n")
	if lines[0] != "module "+modulePath {
		t.Errorf("go.mod module line = %q", lines[0])
	}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "require") || strings.HasPrefix(l, "replace") || strings.HasPrefix(l, "toolchain") {
			t.Errorf("go.mod has %q; the SDK must stay dependency-free and buildable with Go 1.23", l)
		}
	}
	if !strings.Contains(string(mod), "\ngo 1.23\n") {
		t.Errorf("go.mod does not require go 1.23:\n%s", mod)
	}
	if _, err := os.Stat("../go.sum"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("go.sum exists (%v); the SDK has no dependencies", err)
	}
}

const spdxLine = "// SPDX-License-Identifier: MIT"

// TestLicense는 모듈이 MIT 라이선스를 제 것으로 갖는지 본다. 하위 폴더 모듈에 LICENSE가 없으면 go 명령이 모듈
// 압축 파일에 저장소 루트의 LICENSE(GPL-3.0)를 넣는다. 모듈 압축 파일은 심볼릭 링크를 담지 않으므로 일반 파일이어야
// 한다(.claude/docs/addon-packages.md).
func TestLicense(t *testing.T) {
	fi, err := os.Lstat("../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("sdk/go/LICENSE is %v, want a regular file", fi.Mode())
	}
	own, err := os.ReadFile("../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(own, []byte("MIT License\n")) {
		t.Errorf("sdk/go/LICENSE is not the MIT license:\n%.200s", own)
	}
	// 저장소 체크아웃에서는 sdk/LICENSE와 같아야 한다. 모듈 캐시에서 돌 때는 sdk/LICENSE가 없다
	if sdk, err := os.ReadFile("../../LICENSE"); err == nil {
		if !bytes.Equal(own, sdk) {
			t.Error("sdk/go/LICENSE differs from sdk/LICENSE; copy sdk/LICENSE again")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}

	// 모듈 압축 파일에 들어가는 Go 소스(예제 포함)는 모두 첫 줄에 SPDX 표시를 둔다. go 명령이 무시하는 testdata와
	// "."·"_"로 시작하는 디렉터리는 보지 않는다
	n := 0
	err = filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != ".." && (name == "testdata" || strings.HasPrefix(name, ".") ||
				strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		n++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if first, _, _ := bytes.Cut(data, []byte("\n")); string(first) != spdxLine {
			t.Errorf("%s: first line is %q, want %q", path, first, spdxLine)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no Go files found")
	}
}
