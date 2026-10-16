package main

import (
	"os/exec"
	"strings"
	"testing"
)

// 도우미는 root로 돈다. onmaru 웹 코드와 HTTP 스택이 이 실행 파일에 들어오지 않게 한다.
func TestNoWebDependencies(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	out, err := exec.Command(gobin, "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "net/http" || (strings.HasPrefix(dep, "onmaru/backend/internal/") && !strings.HasPrefix(dep, "onmaru/backend/internal/helper/")) {
			t.Errorf("onmaru-helper depends on %s", dep)
		}
		if dep == "onmaru/backend/internal/helper/helperfake" || dep == "onmaru/backend/internal/helper/client" {
			t.Errorf("onmaru-helper depends on %s", dep)
		}
	}
}

func TestCheckChkpwd(t *testing.T) {
	if err := checkChkpwd("/nonexistent/unix_chkpwd"); err == nil {
		t.Fatal("missing checker accepted")
	}
	// 일반 사용자가 쓸 수 있는 파일은 받지 않는다.
	f := t.TempDir() + "/unix_chkpwd"
	if err := exec.Command("sh", "-c", "touch "+f+" && chmod 0777 "+f).Run(); err != nil {
		t.Fatal(err)
	}
	if err := checkChkpwd(f); err == nil {
		t.Fatal("writable checker accepted")
	}
}
