package setup

import (
	"bytes"
	"path/filepath"
	"strings"

	"maru/backend/config"
)

// stepWebUpgrade는 대시보드에서 업그레이드를 허용하기로 했으면 업그레이드 서비스를 설치하고, 아니면 이전에 설치한 것을
// 지운다(도우미를 설치하지 않으면 함께 지운다).
func stepWebUpgrade(c *Context) error {
	if webUpgradeWanted(c) {
		return installWebUpgrade(c)
	}
	return removeWebUpgrade(c)
}

// installWebUpgrade는 root 소유 설치기 복사본, /etc/default/maru-upgrade, 경로·서비스 유닛을 쓴다. 업그레이드 서비스는
// 저장소 안의 실행 파일이 아니라 이 복사본을 root로 실행한다. 설치할 때마다(대시보드에서 업그레이드한 설치 포함) 지금
// 설치기로 바꾸므로, 다음 업그레이드는 이번에 설치한 버전의 업그레이드 서비스가 진행한다.
func installWebUpgrade(c *Context) error {
	path := c.Values.Get(nameBuildPath)
	var missing []string
	for _, tool := range buildTools {
		if !inPath(c.Sys, path, tool) {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		c.Warn(L("not found in the build PATH for upgrading from the dashboard: "+strings.Join(missing, ", ")+" (PATH="+path+"). Checks will ask for a manual upgrade until they are installed there; to use another PATH, run "+c.entry()+" again as the repository owner or pass --build-path.",
			"대시보드에서 업그레이드의 빌드 PATH에서 찾을 수 없습니다: "+strings.Join(missing, ", ")+" (PATH="+path+"). 설치하기 전에는 확인할 때 수동 업그레이드로 안내합니다. 다른 PATH를 쓰려면 저장소 소유자 계정으로 "+c.entry()+" 를 다시 실행하거나 --build-path를 주세요."))
	}
	c.Info(L("Installing "+upgradeInstallerDst+" (dashboard upgrade)", upgradeInstallerDst+" 설치(대시보드 업그레이드)"))
	if err := c.Sys.InstallFile(c.installerBinSrc(), upgradeInstallerDst, 0o755); err != nil {
		return err
	}
	if err := c.writeConfig(config.UpgradeEnvFile, c.Reg.renderEnv(c, UpgradeEnv, c.Installed.UpgradeEnv), true); err != nil {
		return err
	}
	data := c.unitData()
	for _, u := range []struct{ tmpl, path string }{
		{"maru-upgrade.service", upgradeServiceUnit},
		{"maru-upgrade.path", upgradePathUnit},
	} {
		b, err := renderUnit(u.tmpl, data)
		if err != nil {
			return err
		}
		if u.path == upgradePathUnit {
			old, err := c.Sys.ReadFile(u.path)
			c.upgradePathChanged = err != nil || !bytes.Equal(old, b)
		}
		if err := c.writeConfig(u.path, b, false); err != nil {
			return err
		}
	}
	return nil
}

// inPath는 PATH 목록(dirs) 안에 실행할 수 있는 name이 있는지다.
func inPath(sys System, dirs, name string) bool {
	for _, d := range strings.Split(dirs, ":") {
		if fi, err := sys.Stat(filepath.Join(d, name)); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

// stepWebUpgradePath는 업그레이드 요청을 지켜보는 경로 유닛을 켠다. 유닛이 바뀌었으면(지켜보는 경로가 바뀌었을 수 있다)
// 다시 시작한다. 업그레이드 서비스(maru-upgrade.service)는 건드리지 않는다. 대시보드에서 업그레이드할 때 그 서비스
// 안에서 이 설치기가 돌기 때문이다(경로 유닛을 다시 시작해도 서비스는 멈추지 않는다).
func stepWebUpgradePath(c *Context) error {
	unit := upgradeName + ".path"
	if !c.upgradePathChanged {
		return c.Sys.RunQuiet("systemctl", "enable", "--now", unit)
	}
	if err := c.Sys.RunQuiet("systemctl", "enable", unit); err != nil {
		return err
	}
	return c.Sys.Run("systemctl", "restart", unit)
}

// removeWebUpgrade는 대시보드에서 업그레이드를 끈다. 경로 유닛을 멈추고 유닛, 설치기 복사본, 설정 파일, 남은 요청과
// 상태 파일을 지운다. 없으면 아무것도 하지 않는다. 돌고 있는 업그레이드 서비스는 멈추지 않는다(설치 중에 끊지 않게).
func removeWebUpgrade(c *Context) error {
	files := []string{upgradePathUnit, upgradeServiceUnit, upgradeInstallerDst, config.UpgradeEnvFile}
	found := false
	for _, f := range files {
		found = found || exists(c.Sys, f)
	}
	if !found {
		return nil
	}
	c.Info(L("Removing the dashboard upgrade service", "대시보드 업그레이드 서비스 제거"))
	// 이미 멈췄거나 유닛이 없어도 계속한다.
	_ = c.Sys.RunSilent("systemctl", "disable", "--now", upgradeName+".path")
	for _, f := range append(files, upgradeCheckRequest, config.UpgradeStatusFile) {
		if err := c.Sys.Remove(f); err != nil {
			return err
		}
	}
	// 복사본 디렉터리는 비었을 때만 지운다.
	if entries, err := c.Sys.ReadDir(upgradeInstallerDir); err == nil && len(entries) == 0 {
		if err := c.Sys.Remove(upgradeInstallerDir); err != nil {
			return err
		}
	}
	if err := c.Sys.RemoveAll(config.UpgradeRequestDir); err != nil {
		return err
	}
	return c.Sys.Run("systemctl", "daemon-reload")
}
