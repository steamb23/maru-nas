package addonpkg

import (
	"maru/backend/internal/addonpkg/format"
	"maru/backend/internal/addons"
	"maru/backend/internal/api"
)

// ProviderName은 패키지 애드온의 provider 이름이다. capability의 sources에 담긴다.
const ProviderName = "package"

// pkgProvider는 패키지 애드온의 합성 provider다. 조건은 manifest의 requires로 이미 보므로 따로 없다.
type pkgProvider struct{}

func (pkgProvider) Name() string               { return ProviderName }
func (pkgProvider) Requires() api.Requirements { return api.Requirements{} }

// ToAddon은 패키지 manifest를 레지스트리용 애드온으로 바꾼다. 이름·아이콘은 번역 키·아이콘 이름이 아니므로
// api.Manifest에 넣지 않고(addonPackages가 따로 준다), 데이터·조작의 Go 구현도 없다(패키지 백엔드가 맡는다).
func ToAddon(m *format.Manifest) addons.Addon {
	am := api.Manifest{
		ID: m.ID,
		Requires: api.Requirements{
			Admin:   m.Requires.Admin,
			Plugins: []string{},
		},
		RPC: api.RPCMethods{Read: nonNil(m.RPC.Read), Write: nonNil(m.RPC.Write)},
	}
	if w := m.Views.Window; w != nil {
		am.Views.Window = &api.WindowView{DefaultSize: w.DefaultSize, MinSize: w.MinSize}
	}
	if w := m.Views.Widget; w != nil {
		am.Views.Widget = &api.WidgetView{DefaultSize: w.DefaultSize, MinSize: w.MinSize, MaxSize: w.MaxSize}
	}
	plugins := map[string]string{}
	for _, p := range m.Requires.Plugins {
		am.Requires.Plugins = append(am.Requires.Plugins, p.Package)
		plugins[p.Package] = p.RPCFile
	}
	for _, a := range m.Actions {
		am.Actions = append(am.Actions, api.ActionDecl{Name: a.Name, Admin: a.Admin, Confirm: a.Confirm})
	}
	return addons.Addon{
		Kind:      addons.KindPackage,
		Manifest:  am,
		Plugins:   plugins,
		Providers: []addons.Provider{pkgProvider{}},
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
