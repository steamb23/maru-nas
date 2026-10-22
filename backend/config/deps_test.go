package config

import (
	"os/exec"
	"strings"
	"testing"
)

// 설치기(별도 모듈)가 이 패키지를 쓰므로 표준 라이브러리 밖의 의존성이 딸려 가지 않게 한다.
// backend/internal도 import하지 않는다(설치기에서는 import할 수 없는 경로이기도 하다).
func TestStandardLibraryOnly(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	out, err := exec.Command(gobin, "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep != "maru/backend/config" {
			t.Errorf("backend/config depends on non-standard package %s", dep)
		}
	}
}
