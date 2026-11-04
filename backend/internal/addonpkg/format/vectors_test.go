package format

import (
	"os"
	"path/filepath"
	"testing"
)

// vectorsDir는 SDK CLI(maru-addon validate)와 함께 쓰는 manifest 시험 벡터다.
//   - valid: 스키마와 Validate를 모두 통과한다
//   - invalid: 스키마와 Validate가 모두 거절한다
//   - server-only: 스키마는 통과하지만 Validate가 거절한다(필드 사이의 조건, 문구의 제어 문자 등)
//
// CLI 쪽 검사는 frontend/src/addonpkg/cli.node.test.ts가 같은 파일로 한다.
const vectorsDir = "../../../../sdk/testdata/manifests"

func readVectors(t *testing.T, kind string) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(vectorsDir, kind, "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no vectors in %s/%s: %v", vectorsDir, kind, err)
	}
	out := map[string][]byte{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(p)] = b
	}
	return out
}

func check(b []byte) error {
	m, err := Decode(b)
	if err != nil {
		return err
	}
	return Validate(m)
}

func TestVectorsValid(t *testing.T) {
	for name, b := range readVectors(t, "valid") {
		if err := check(b); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestVectorsInvalid(t *testing.T) {
	for _, kind := range []string{"invalid", "server-only"} {
		for name, b := range readVectors(t, kind) {
			if err := check(b); err == nil {
				t.Errorf("%s/%s: accepted", kind, name)
			}
		}
	}
}
