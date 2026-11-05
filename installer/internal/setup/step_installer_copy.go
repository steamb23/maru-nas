package setup

// installerCopyWanted는 root 소유 설치기 복사본(upgradeInstallerDst)이 필요한지다. 대시보드에서 업그레이드의 업그레이드
// 서비스와 애드온 백엔드의 root 동기화가 저장소 안의 실행 파일 대신 이 복사본을 root로 실행한다.
func installerCopyWanted(c *Context) bool { return webUpgradeWanted(c) || addonBackendsWanted(c) }

// stepInstallerCopy는 복사본이 필요하면 지금 설치기(bin/maru-install)로 바꾸고, 아니면 지운다. 두 기능의 설치·제거
// 단계 뒤에 둔다(제거할 때 복사본을 실행하는 유닛을 먼저 멈추게).
func stepInstallerCopy(c *Context) error {
	if !installerCopyWanted(c) {
		return removeInstallerCopy(c)
	}
	c.Info(L("Installing "+upgradeInstallerDst+" (run by the root services)", upgradeInstallerDst+" 설치(root 서비스가 실행)"))
	return c.Sys.InstallFile(c.installerBinSrc(), upgradeInstallerDst, 0o755)
}

// removeInstallerCopy는 복사본을 지운다. 복사본 디렉터리는 비었을 때만 지운다. 없으면 아무것도 하지 않는다(명령도 없다).
func removeInstallerCopy(c *Context) error {
	if err := c.Sys.Remove(upgradeInstallerDst); err != nil {
		return err
	}
	if entries, err := c.Sys.ReadDir(upgradeInstallerDir); err == nil && len(entries) == 0 {
		return c.Sys.Remove(upgradeInstallerDir)
	}
	return nil
}
