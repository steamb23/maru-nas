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
