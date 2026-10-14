package builtin

import (
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
