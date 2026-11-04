package addonpkg

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// repoRoot는 이 패키지 기준 저장소 루트다.
const repoRoot = "../../.."

// TestSchemaFileUpToDate는 커밋한 스키마 파일이 지금 형식과 같은지 본다. 다르면 `make types`를 실행한다.
func TestSchemaFileUpToDate(t *testing.T) {
	want, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(repoRoot, SchemaFile))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is out of date; run `make types`", SchemaFile)
	}
	var v map[string]any
	if err := json.Unmarshal(want, &v); err != nil {
		t.Fatal(err)
	}
}
