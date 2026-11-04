package builtin

import (
	"bytes"
	"os"
	"testing"
)

func TestBuiltinRegistryIsValid(t *testing.T) {
	r := New(HostDeps())
	if len(r.All()) == 0 {
		t.Fatal("no built-in addons")
	}
}

// 쓰기 RPC는 조작(actions)을 가진 애드온만 선언한다. 조작이 없는 애드온은 쓰기를 부를 경로가 없으므로
// 쓰기를 선언했다면 실수다.
func TestBuiltinWritesOnlyWithActions(t *testing.T) {
	for _, a := range New(HostDeps()).All() {
		if len(a.Manifest.RPC.Write) > 0 && len(a.Manifest.Actions) == 0 {
			t.Errorf("addon %q declares write rpc %v without actions", a.Manifest.ID, a.Manifest.RPC.Write)
		}
	}
}

// 내장 조작은 상태를 바꾸므로 쓰기 RPC를 하나 이상 선언한다(addons.validateActions는 패키지 조작 때문에
// 이 조건을 보지 않는다. 패키지 조작은 OMV 쓰기 없이 백엔드의 상태만 바꿀 수 있다).
func TestBuiltinActionsHaveWrites(t *testing.T) {
	for _, a := range New(HostDeps()).All() {
		if len(a.Manifest.Actions) > 0 && len(a.Manifest.RPC.Write) == 0 {
			t.Errorf("addon %q declares actions without write rpc", a.Manifest.ID)
		}
	}
}

// 프론트 목록(manifests.gen.ts)이 백엔드 manifest와 같은지 확인한다. 백엔드와 프론트의
// 애드온 id 목록이 어긋나지 않게 하는 검사다. 실패하면 `make types`를 실행한다.
func TestFrontendManifestsUpToDate(t *testing.T) {
	want, err := New(HostDeps()).GenerateTS()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../../../frontend/src/addons/manifests.gen.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("frontend/src/addons/manifests.gen.ts is out of date; run `make types`")
	}
}
