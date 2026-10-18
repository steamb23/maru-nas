package setup

import "onmaru/backend/config"

// stepProxyUnchanged는 업그레이드에서 수신 주소와 서비스 사용자가 그대로인지 본다. 그러면 리버스 프록시 설정도
// 그대로 맞으므로 끝난 뒤의 안내를 줄인다. 파일을 다시 쓰기 전의 기존 설치와 비교한다.
func stepProxyUnchanged(c *Context) error {
	inst := c.Installed
	c.proxyUnchanged = c.Opts.Upgrade && inst.UnitExists &&
		inst.Env.Value(config.EnvListen) == c.Values.Get(nameListen) && inst.UnitUser == c.Values.Get(nameUser)
	return nil
}

// stepInstallBinary는 onmaru 실행 파일을 설치한다.
func stepInstallBinary(c *Context) error {
	c.Info(L("Installing "+binDst, binDst+" 설치"))
	return c.Sys.InstallFile(c.binSrc(), binDst, 0o755)
}

// stepEnv는 /etc/default/onmaru를 쓴다. 바뀌면 기존 파일을 백업한다.
func stepEnv(c *Context) error {
	return c.writeConfig(envFile, c.Reg.renderEnv(c, OnmaruEnv, c.Installed.Env), true)
}

// stepUnit은 onmaru 유닛을 쓴다. 설정으로 언제든 다시 만들 수 있으므로 백업하지 않는다.
func stepUnit(c *Context) error {
	data, err := renderUnit("onmaru.service", c.unitData())
	if err != nil {
		return err
	}
	return c.writeConfig(unitFile, data, false)
}
