package setup

import "errors"

// settingAddonBackends는 애드온 패키지의 백엔드 프로세스를 이 설치에서 실행할지다(.claude/docs/addon-packages.md의
// "백엔드 프로세스"). 켜면 백엔드 유닛 템플릿(네트워크 단계 4개), root 동기화(maru-addons.path·.service), 동기화
// 디렉터리(config.AddonSyncDir)와 빈 동기화 결과를 쓰고, maru 유닛에 root가 만드는 호스트 소켓 디렉터리를 연다
// (step_addon_backends.go, templates/maru.service.tmpl). 끄면 켜진 인스턴스를 끄고 이것들을 지운다(백엔드 데이터는 남긴다).
//
// 묻지 않는다(대시보드에서 업그레이드의 계획에도 나오지 않는다). 켜려면 --addon-backends를 준다. 기존 설치는 동기화
// 디렉터리가 있는지로 읽으므로, 한 번 켜면 이후 업그레이드와 --reinstall에서도 그대로다. 권한 도우미와 상관없다.
var settingAddonBackends = &Setting{
	Name: nameAddonBackends,
	Flag: FlagToggle,
	Usage: L("run, or do not run, the backends of addon packages as sandboxed systemd units (default: off)",
		"애드온 패키지의 백엔드를 샌드박스된 systemd 유닛으로 실행하거나 실행하지 않습니다(기본값: 끔)"),
	// 기존 설치(일부만 있는 설치 포함)는 켜 두었는지(동기화 디렉터리가 있는지), 새 설치는 끔이다.
	Default: func(c *Context) string {
		if c.Installed.UnitExists || c.Installed.Env != nil {
			return yesNo(c.Installed.AddonBackends)
		}
		return "no"
	},
	// 이 설정이 없던 버전의 설치도 동기화 디렉터리가 없으므로 꺼진 것으로 읽는다. 늘 정한 값을 돌려줘 묻지 않는다.
	Load: func(c *Context) (string, bool) { return yesNo(c.Installed.AddonBackends), true },
	Validate: func(c *Context, v string) error {
		if v != "yes" && v != "no" {
			return errors.New(c.L("choose --addon-backends or --no-addon-backends", "--addon-backends 또는 --no-addon-backends를 고르세요"))
		}
		return nil
	},
	Summary: &SummarySpec{
		Label: L("addon backends", "애드온 백엔드"),
		Value: func(c *Context, v string) string {
			if v == "yes" {
				return c.L("run (sandboxed systemd units)", "실행(샌드박스 systemd 유닛)")
			}
			return c.L("do not run", "실행 안 함")
		},
	},
}

// addonBackendsWanted는 애드온 백엔드를 켜기로 했는지다.
func addonBackendsWanted(c *Context) bool { return c.Values.Get(nameAddonBackends) == "yes" }
