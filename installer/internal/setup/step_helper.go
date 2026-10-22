package setup

// stepHelper는 도우미를 설치하기로 했으면 설치하고, 아니면 이전에 설치한 도우미를 지운다.
func stepHelper(c *Context) error {
	if helperWanted(c) {
		return installHelper(c)
	}
	return removeHelper(c)
}

// installHelper는 권한 도우미 실행 파일, 설정, 소켓·서비스 유닛을 쓴다. 도우미는 root로 돌지만 서비스 사용자의
// 연결만 받고, 사용자 권한으로 강등하는 데 필요한 권한만 남긴다.
func installHelper(c *Context) error {
	group := c.Values.Get(nameElevateGroup)
	ok, err := c.Sys.GroupExists(group)
	if err != nil {
		return err
	}
	if !ok {
		c.Warn(L("group '"+group+"' does not exist. Nobody can elevate to root until it is created.",
			"그룹 '"+group+"'이(가) 없습니다. 그룹을 만들기 전에는 아무도 root 권한으로 상승할 수 없습니다."))
	}
	if fi, err := c.Sys.Stat(chkpwdPath); err != nil || fi.Mode()&0o111 == 0 {
		c.Warn(L(chkpwdPath+" not found (package libpam-modules-bin); the helper will not start.",
			chkpwdPath+"가 없습니다(libpam-modules-bin 패키지). 도우미가 시작하지 않습니다."))
	}
	c.Info(L("Installing "+helperBinDst, helperBinDst+" 설치"))
	if err := c.Sys.InstallFile(c.helperBinSrc(), helperBinDst, 0o755); err != nil {
		return err
	}
	if err := c.writeConfig(helperEnvFile, c.Reg.renderEnv(c, HelperEnv, c.Installed.HelperEnv), true); err != nil {
		return err
	}
	data := c.unitData()
	for _, u := range []struct{ tmpl, path string }{
		{"maru-helper.socket", helperSocketUnit},
		{"maru-helper.service", helperServiceUnit},
	} {
		b, err := renderUnit(u.tmpl, data)
		if err != nil {
			return err
		}
		if err := c.writeConfig(u.path, b, false); err != nil {
			return err
		}
	}
	return nil
}

// removeHelper는 권한 도우미의 소켓·서비스를 멈추고 유닛, 실행 파일, 설정을 지운다. 없으면 아무것도 하지 않는다.
func removeHelper(c *Context) error {
	files := []string{helperSocketUnit, helperServiceUnit, helperBinDst, helperEnvFile}
	found := false
	for _, f := range files {
		found = found || exists(c.Sys, f)
	}
	if !found {
		return nil
	}
	c.Info(L("Removing the privileged helper", "권한 도우미 제거"))
	// 이미 멈췄거나 유닛이 없어도 계속한다.
	_ = c.Sys.RunSilent("systemctl", "disable", "--now", helperName+".socket", helperName+".service")
	for _, f := range files {
		if err := c.Sys.Remove(f); err != nil {
			return err
		}
	}
	return c.Sys.Run("systemctl", "daemon-reload")
}

// stepHelperSocket은 도우미 소켓을 켠다. 이미 돌던 도우미는 새 실행 파일로 다시 시작한다(티켓이 사라지므로
// 사용자는 다시 로그인한다. maru도 다시 시작하므로 어차피 다시 로그인한다).
func stepHelperSocket(c *Context) error {
	if err := c.Sys.RunQuiet("systemctl", "enable", "--now", helperName+".socket"); err != nil {
		return err
	}
	if c.Sys.Check("systemctl", "is-active", "--quiet", helperName+".service") == nil {
		return c.Sys.Run("systemctl", "restart", helperName+".service")
	}
	return nil
}

// stepHelperCheck는 시작한 뒤 도우미 소켓이 켜져 있는지 본다.
func stepHelperCheck(c *Context) error {
	if c.Sys.DryRun() {
		return nil
	}
	if c.Sys.Check("systemctl", "is-active", "--quiet", helperName+".socket") != nil {
		c.Warn(L(helperName+".socket is not active. See: systemctl status "+helperName+".socket",
			helperName+".socket 이(가) 켜져 있지 않습니다. 확인: systemctl status "+helperName+".socket"))
	}
	return nil
}
