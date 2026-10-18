package setup

import (
	"onmaru/backend/config"
	"onmaru/installer/internal/prompt"
)

// stepUninstallConfirm은 제거를 확인한다. 아니요면 아무것도 하지 않고 끝낸다.
func stepUninstallConfirm(c *Context) error {
	c.Info(L("Uninstalling "+serviceName, serviceName+" 제거"))
	ok, err := c.confirm(prompt.Confirm{
		ID: "uninstall",
		Title: c.L("Stop the service and remove "+unitFile+", "+binDst+" and "+envFile+" (and the privileged helper if installed)?",
			"서비스를 멈추고 "+unitFile+", "+binDst+", "+envFile+" 파일(설치했다면 권한 도우미도)을 지울까요?"),
	})
	if err != nil {
		return err
	}
	if !ok {
		return quit(0)
	}
	return nil
}

// stepStop은 onmaru를 멈추고 끈다.
func stepStop(c *Context) error {
	if c.Sys.Check("systemctl", "list-unit-files", serviceName+".service") == nil {
		_ = c.Sys.RunSilent("systemctl", "disable", "--now", serviceName+".service")
	}
	return nil
}

// stepRemoveFiles는 유닛, 실행 파일, 설정, 설치 기록 디렉터리를 지운다. 지우기 전에 유닛의 서비스 사용자를 읽어 둔다.
func stepRemoveFiles(c *Context) error {
	if data, err := c.Sys.ReadFile(unitFile); err == nil {
		c.uninstallUser = unitUser(data)
	}
	for _, f := range []string{unitFile, binDst, envFile} {
		if err := c.Sys.Remove(f); err != nil {
			return err
		}
	}
	if err := c.Sys.RemoveAll(config.UpgradeStateDir); err != nil {
		return err
	}
	return c.Sys.Run("systemctl", "daemon-reload")
}

// stepRemoveRuntimeDir은 /run/onmaru를 지운다. RuntimeDirectoryPreserve=yes라 서비스를 멈춰도 남는다
// (재부팅하면 어차피 사라진다).
func stepRemoveRuntimeDir(c *Context) error {
	if err := c.Sys.RemoveAll(runtimeDir); err != nil {
		return err
	}
	c.Info(L("Removed service, binary and settings (backups "+envFile+".bak-* are kept)",
		"서비스, 실행 파일, 설정을 지웠습니다(백업 "+envFile+".bak-* 은 남겨 둡니다)"))
	return nil
}

// stepRemoveStateDir은 확인을 받은 뒤에만 상태 디렉터리를 지운다.
func stepRemoveStateDir(c *Context) error {
	if fi, err := c.Sys.Stat(stateDir); err != nil || !fi.IsDir() {
		return nil
	}
	ok, err := c.confirm(prompt.Confirm{
		ID:    "uninstall-state-dir",
		Title: c.L("Also delete "+stateDir+" (dashboard settings and uploaded backgrounds)?", stateDir+"(대시보드 설정과 올린 배경 이미지)도 지울까요?"),
	})
	if err != nil || !ok {
		return err
	}
	if err := c.Sys.RemoveAll(stateDir); err != nil {
		return err
	}
	c.Info(L("Removed "+stateDir, stateDir+" 삭제"))
	return nil
}

// stepRemoveUser는 설치기가 만든 형태의 시스템 사용자만 확인을 받은 뒤 지운다.
func stepRemoveUser(c *Context) error {
	user := c.uninstallUser
	if user == "" {
		return nil
	}
	own, err := isOwnSystemUser(c, user)
	if err != nil || !own {
		return err
	}
	ok, err := c.confirm(prompt.Confirm{
		ID:    "uninstall-user",
		Title: c.L("Also delete the system user '"+user+"'?", "시스템 사용자 '"+user+"'도 지울까요?"),
	})
	if err != nil || !ok {
		return err
	}
	if err := c.Sys.Run("userdel", user); err != nil {
		return err
	}
	c.Info(L("Removed user "+user, "사용자 "+user+" 삭제"))
	return nil
}

// isOwnSystemUser는 설치기가 만든 형태의 계정인지 본다. 기존 공용 계정을 지정했을 수 있으므로 시스템 UID 범위이고
// 홈이 /nonexistent일 때만 제거를 제안한다.
func isOwnSystemUser(c *Context, name string) (bool, error) {
	a, ok, err := c.Sys.LookupUser(name)
	if err != nil || !ok {
		return false, err
	}
	return a.UID > 0 && a.UID < 1000 && a.Home == "/nonexistent", nil
}

func stepProxyNote(c *Context) error {
	c.Info(L("If a reverse proxy pointed at onmaru, remove that configuration as well.",
		"리버스 프록시에 onmaru 설정이 있다면 그것도 지워 주세요."))
	return nil
}
