package builtin

import (
	"bytes"
	"os"
	"testing"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/addons/sysinfo"
)

func TestBuiltinRegistryIsValid(t *testing.T) {
	r := New(Deps{System: sysinfo.NewReader()})
	if len(r.All()) == 0 {
		t.Fatal("no built-in addons")
	}
}

// 3단계(읽기 전용)에서는 어떤 내장 애드온도 쓰기 RPC를 선언하지 않는다. 4단계에서 바꾼다.
func TestBuiltinAddonsDeclareNoWrites(t *testing.T) {
	if addons.WritesEnabled {
		t.Error("addons.WritesEnabled must be false before stage 4")
	}
	for _, a := range New(Deps{System: sysinfo.NewReader()}).All() {
		if len(a.Manifest.RPC.Write) > 0 {
			t.Errorf("addon %q declares write rpc %v", a.Manifest.ID, a.Manifest.RPC.Write)
		}
	}
}

// 프론트 목록(manifests.gen.ts)이 백엔드 manifest와 같은지 확인한다. 백엔드와 프론트의
// 애드온 id 목록이 어긋나지 않게 하는 검사다. 실패하면 `make types`를 실행한다.
func TestFrontendManifestsUpToDate(t *testing.T) {
	want, err := New(Deps{System: sysinfo.NewReader()}).GenerateTS()
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
