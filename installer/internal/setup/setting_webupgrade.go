package setup

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"maru/backend/config"
)

// buildTools는 대시보드에서 업그레이드의 확인·빌드에 필요한 명령이다. 기본 빌드 PATH는 이 명령을 찾은 디렉터리다.
var buildTools = []string{"git", "go", "node", "npm", "make"}

// standardPath는 기본 빌드 PATH 끝에 붙이는 시스템 디렉터리다(make가 부르는 sh, tar 등).
var standardPath = []string{"/usr/local/bin", "/usr/bin", "/bin"}

// settingWebUpgrade는 대시보드에서 업그레이드(설정 앱의 업데이트 페이지)를 허용할지다. 권한 도우미를 설치할 때만 쓴다.
// 허용하면 업그레이드 서비스(maru-upgrade.path·.service), root 소유 설치기 복사본, root 쪽 설정 파일을 쓰고 도우미
// 유닛에 요청 디렉터리를 더한다(step_webupgrade.go). 허용하지 않으면 이것들을 지운다.
//
// 이 설정이 없던 버전의 설치는 꺼진 것으로 읽어 업그레이드할 때 묻지 않는다. 켜려면 --web-upgrade를 주거나 --reinstall로 묻는다.
var settingWebUpgrade = &Setting{
	Name: nameWebUpgrade,
	Flag: FlagToggle,
	Usage: L("allow, or do not allow, upgrading from the dashboard (needs --helper; builds as the repository owner)",
		"대시보드에서 업그레이드를 허용하거나 허용하지 않습니다(--helper 필요, 저장소 소유자 계정으로 빌드)"),
	Env: []EnvLine{
		{
			File:    UpgradeEnv,
			Comment: "Repository to build from (the checkout install.sh ran from).",
			Key:     config.EnvUpgradeRepo,
			Value:   func(c *Context) string { return c.RepoDir },
		},
		{
			File:    UpgradeEnv,
			Comment: "Account that runs git and the build: the repository owner, never root.",
			Key:     config.EnvUpgradeUser,
			Value: func(c *Context) string {
				_, owner := repoOwner(c)
				return owner
			},
		},
	},
	When: func(v Values) bool { return v.Get(nameHelper) == "yes" },
	// 기존 설치(일부만 있는 설치 포함)는 켜 두었는지(설정 파일이 있는지), 새 설치는 저장소 소유자가 일반 사용자이면 허용이
	// 기본이다.
	Default: func(c *Context) string {
		if c.Installed.UnitExists || c.Installed.Env != nil {
			return yesNo(c.Installed.UpgradeEnv != nil)
		}
		uid, _ := repoOwner(c)
		return yesNo(uid > 0)
	},
	Load: func(c *Context) (string, bool) { return yesNo(c.Installed.UpgradeEnv != nil), true },
	Prompt: &PromptSpec{
		Kind:  PromptConfirm,
		Title: L("Allow upgrading from the dashboard?", "대시보드에서 업그레이드를 허용할까요?"),
		Help: L(`Administrators can then upgrade maru from the settings app (Update page) instead of running git pull and
./install.sh over SSH. A root service (maru-upgrade) runs git and the build as the owner of this repository and
then installs the new version as root. Upgrading needs root elevation (the password again).
As with ./install.sh, whoever can change this repository can get code run as root through an upgrade.
See README.md, "Upgrading from the dashboard".`,
			`허용하면 관리자가 SSH로 git pull과 ./install.sh를 실행하는 대신 설정 앱(업데이트 페이지)에서 마루 NAS를 업그레이드할 수
있습니다. root로 도는 서비스(maru-upgrade)가 이 저장소의 소유자 계정으로 git과 빌드를 실행한 뒤 새 버전을 root로
설치합니다. 업그레이드에는 root 권한 상승(비밀번호 재입력)이 필요합니다.
./install.sh와 마찬가지로 이 저장소를 바꿀 수 있는 계정은 업그레이드를 통해 root로 코드를 실행할 수 있습니다.
README.md의 "대시보드에서 업그레이드"를 참고하세요.`),
	},
	Validate: func(c *Context, v string) error {
		switch v {
		case "no":
			return nil
		case "yes":
			uid, _ := repoOwner(c)
			switch {
			case uid < 0:
				return errors.New(c.L("cannot find the owner of "+c.RepoDir+"; upgrading from the dashboard builds as the repository owner",
					c.RepoDir+" 의 소유자를 알 수 없습니다. 대시보드에서 업그레이드는 저장소 소유자 계정으로 빌드합니다"))
			case uid == 0:
				return errors.New(c.L("the repository "+c.RepoDir+" is owned by root, but upgrading from the dashboard builds as the repository owner and never as root. Clone it as a regular user, or choose --no-web-upgrade",
					"저장소 "+c.RepoDir+" 의 소유자가 root입니다. 대시보드에서 업그레이드는 저장소 소유자 계정으로 빌드하며 root로는 빌드하지 않습니다. 일반 사용자로 클론하거나 --no-web-upgrade를 고르세요"))
			}
			return nil
		}
		return errors.New(c.L("choose --web-upgrade or --no-web-upgrade", "--web-upgrade 또는 --no-web-upgrade를 고르세요"))
	},
	Summary: &SummarySpec{
		Label: L("web upgrade", "웹 업그레이드"),
		Value: func(c *Context, v string) string {
			if v != "yes" {
				return c.L("do not allow", "허용 안 함")
			}
			_, owner := repoOwner(c)
			return c.L("allow (build as "+owner+")", "허용("+owner+" 계정으로 빌드)")
		},
	},
}

// settingBuildPath는 대시보드에서 업그레이드가 저장소 소유자로 git과 빌드를 실행할 때의 PATH다. 묻지 않는다.
// sudo는 PATH를 secure_path로 바꾸므로 1단계(빌드하는 계정)가 매번 그 계정의 PATH에서 빌드 도구를 다시 찾아 2단계에 옵션으로
// 넘긴다. 2단계를 직접 실행하거나 대시보드에서 업그레이드할 때는 기존 값을 쓴다.
var settingBuildPath = &Setting{
	Name: nameBuildPath,
	Flag: FlagValue,
	Arg:  "PATH",
	Usage: L("PATH for git and the build when upgrading from the dashboard (default: where install.sh finds "+strings.Join(buildTools, ", ")+")",
		"대시보드에서 업그레이드할 때 git과 빌드에 쓸 PATH(기본값: install.sh가 "+strings.Join(buildTools, ", ")+"을 찾은 곳)"),
	Env: []EnvLine{{
		File:    UpgradeEnv,
		Comment: "PATH for git, Go, Node.js and make when building as that account.",
		Key:     config.EnvUpgradePath,
	}},
	When:    func(v Values) bool { return v.Get(nameWebUpgrade) == "yes" },
	Default: defaultBuildPath,
	Load: func(c *Context) (string, bool) {
		if c.Opts.Action == ActionInstall {
			return "", false
		}
		v := c.Installed.UpgradeEnv.Value(config.EnvUpgradePath)
		return v, v != ""
	},
	Validate: func(c *Context, v string) error {
		if err := checkBuildPath(v); err != nil {
			return errors.New(c.L("--build-path must be absolute directories separated by ':' ("+err.Error()+"): "+v,
				"--build-path는 ':'로 나눈 절대 경로 디렉터리여야 합니다("+err.Error()+"): "+v))
		}
		return nil
	},
	Summary: &SummarySpec{Label: L("build PATH", "빌드 PATH")},
}

// defaultBuildPath는 지금 PATH에서 빌드 도구를 찾은 디렉터리(찾은 순서) 뒤에 시스템 디렉터리를 붙인 PATH다.
func defaultBuildPath(c *Context) string {
	var dirs []string
	for _, tool := range buildTools {
		if p, ok := c.Sys.Which(tool); ok && !slices.Contains(dirs, filepath.Dir(p)) {
			dirs = append(dirs, filepath.Dir(p))
		}
	}
	for _, d := range standardPath {
		if !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return strings.Join(dirs, ":")
}

// checkBuildPath는 빌드 PATH가 정리된 절대 경로 디렉터리 목록인지 본다. 빈 항목·상대 경로(. 포함)는 받지 않는다.
func checkBuildPath(v string) error {
	if v == "" {
		return errors.New("empty")
	}
	if len(v) > 4096 {
		return errors.New("too long")
	}
	for _, d := range strings.Split(v, ":") {
		if !filepath.IsAbs(d) || filepath.Clean(d) != d {
			return errors.New("not an absolute directory: " + strings.TrimSpace(d))
		}
		for _, r := range d {
			if r < ' ' || r == 0x7f {
				return errors.New("control character")
			}
		}
	}
	return nil
}

// webUpgradeWanted는 대시보드에서 업그레이드를 허용하기로 했는지다(도우미를 설치할 때만).
func webUpgradeWanted(c *Context) bool {
	return helperWanted(c) && c.Values.Get(nameWebUpgrade) == "yes"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
