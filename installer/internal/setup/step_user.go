package setup

// stepUser는 서비스 사용자가 없으면 시스템 사용자로 만들고 기본 그룹을 찾는다.
func stepUser(c *Context) error {
	user := c.Values.Get(nameUser)
	_, ok, err := c.Sys.LookupUser(user)
	if err != nil {
		return err
	}
	if !ok {
		c.Info(L("Creating system user "+user, "시스템 사용자 "+user+" 생성"))
		if err := c.Sys.Run("useradd", "--system", "--user-group", "--no-create-home", "--home-dir", "/nonexistent",
			"--shell", "/usr/sbin/nologin", user); err != nil {
			return err
		}
	}
	// 기존 계정은 이름과 같은 그룹이 없을 수 있다(예: adduser --system의 기본 그룹 nogroup).
	group, err := c.Sys.PrimaryGroup(user)
	if err != nil || group == "" {
		return fail(c.L("cannot find the primary group of "+user, user+" 의 기본 그룹을 찾을 수 없습니다"))
	}
	c.group = group
	return nil
}
