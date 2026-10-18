package setup

import (
	"path/filepath"
)

// stepBuild는 1단계의 빌드다. 실행한 계정으로 빌드한다. 처음 클론한 상태면 서브모듈과 npm 의존성부터 준비한다.
func stepBuild(c *Context) error {
	who := c.Sys.CurrentUser()
	c.Info(L("Building as "+who, who+" 계정으로 빌드"))
	for _, tool := range []string{"make", "go", "npm"} {
		if !c.Sys.LookPath(tool) {
			return fail(c.L("'"+tool+"' not found in PATH. Install it, or build elsewhere and use --no-build.",
				"PATH에서 '"+tool+"'을 찾을 수 없습니다. 설치하거나, 다른 곳에서 빌드한 뒤 --no-build를 사용하세요."))
		}
	}
	repo := c.RepoDir
	if entries, err := c.Sys.ReadDir(filepath.Join(repo, iconsRel)); err != nil || len(entries) == 0 {
		if !c.Sys.LookPath("git") {
			return fail(c.L("'git' is required to fetch the icon submodule", "아이콘 서브모듈을 받으려면 'git'이 필요합니다"))
		}
		c.Info(L("Fetching the icon submodule", "아이콘 서브모듈 받는 중"))
		if err := c.Sys.RunUser(repo, "git", "-C", repo, "submodule", "update", "--init", "--depth", "1"); err != nil {
			return err
		}
	}
	if npmCINeeded(c) {
		c.Info(L("Installing frontend dependencies", "프론트엔드 의존성 설치 중"))
		if err := c.Sys.RunUser(repo, "npm", "--prefix", filepath.Join(repo, "frontend"), "ci"); err != nil {
			return err
		}
	}
	if err := c.Sys.RunUser(repo, "make", "-C", repo, "build"); err != nil {
		return err
	}
	c.Info(L("Build finished: "+c.binSrc(), "빌드 완료: "+c.binSrc()))
	return nil
}

// npmCINeeded는 frontend/node_modules가 없거나 package-lock.json이 설치된 의존성보다 새로운지다.
func npmCINeeded(c *Context) bool {
	front := filepath.Join(c.RepoDir, "frontend")
	if fi, err := c.Sys.Stat(filepath.Join(front, "node_modules")); err != nil || !fi.IsDir() {
		return true
	}
	lock, err := c.Sys.Stat(filepath.Join(front, "package-lock.json"))
	if err != nil {
		return false
	}
	installed, err := c.Sys.Stat(filepath.Join(front, "node_modules", ".package-lock.json"))
	return err != nil || lock.ModTime().After(installed.ModTime())
}

// requireBinary는 설치할 실행 파일이 있는지 확인한다. 프론트엔드 빌드가 없으면 경고만 한다(한 번).
func requireBinary(c *Context) error {
	check := func(path string) error {
		if fi, err := c.Sys.Stat(path); err != nil || !fi.Mode().IsRegular() || fi.Mode()&0o111 == 0 {
			return fail(c.L(path+" not found. Run "+c.entry()+" without --no-build to build it first.",
				path+" 파일이 없습니다. --no-build 없이 "+c.entry()+" 를 실행해 먼저 빌드하세요."))
		}
		return nil
	}
	if err := check(c.binSrc()); err != nil {
		return err
	}
	if helperWanted(c) {
		if err := check(c.helperBinSrc()); err != nil {
			return err
		}
	}
	if dist := filepath.Join(c.RepoDir, distIndexRel); !c.distWarned && !exists(c.Sys, dist) {
		c.distWarned = true
		c.Warn(L("frontend build not found ("+dist+"). The binary will serve a 'frontend not built' page.",
			"프론트엔드 빌드가 없습니다("+dist+"). 화면 대신 'frontend not built' 안내만 나옵니다."))
	}
	return nil
}
