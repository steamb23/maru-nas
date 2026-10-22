package setup

// installSteps는 설치 단계다(순서대로). User 단계는 실행한 계정으로(1단계 끝), Root 단계는 root로(2단계) 실행한다.
// 설정 질문·검증·요약은 단계 앞에서 끝난다. 새 단계는 step_<이름>.go에 구현하고 여기에 한 줄 더한다.
var installSteps = []Step{
	{Name: "build", Phase: PhaseUser, When: func(c *Context) bool { return c.build }, Run: stepBuild},
	{Name: "binary", Phase: PhaseUser, Run: requireBinary},

	// 설정을 물은 뒤에야 도우미를 설치할지 알 수 있으므로 실행 파일을 다시 확인한다.
	{Name: "binary", Phase: PhaseRoot, Run: requireBinary},
	{Name: "user", Phase: PhaseRoot, Run: stepUser},
	{Name: "port", Phase: PhaseRoot, Run: stepPort},
	// 파일을 다시 쓰기 전에 기존 값과 비교한다.
	{Name: "proxy-unchanged", Phase: PhaseRoot, Run: stepProxyUnchanged},
	{Name: "maru-binary", Phase: PhaseRoot, Run: stepInstallBinary},
	{Name: "helper", Phase: PhaseRoot, Run: stepHelper},
	{Name: "web-upgrade", Phase: PhaseRoot, Run: stepWebUpgrade},
	{Name: "env", Phase: PhaseRoot, Run: stepEnv},
	{Name: "unit", Phase: PhaseRoot, Run: stepUnit},
	{Name: "daemon-reload", Phase: PhaseRoot, Run: stepDaemonReload},
	{Name: "web-upgrade-path", Phase: PhaseRoot, When: webUpgradeWanted, Run: stepWebUpgradePath},
	// maru가 시작할 때 도우미에 연결하므로 도우미 소켓을 먼저 연다.
	{Name: "helper-socket", Phase: PhaseRoot, When: helperWanted, Run: stepHelperSocket},
	{Name: "start", Phase: PhaseRoot, Run: stepStart},
	// 서비스를 시작한 뒤에 기록한다(시작하지 못한 설치를 기록하지 않게).
	{Name: "record", Phase: PhaseRoot, Run: stepRecord},
	{Name: "health", Phase: PhaseRoot, Run: stepHealth},
	{Name: "helper-check", Phase: PhaseRoot, When: helperWanted, Run: stepHelperCheck},
	{Name: "next-steps", Phase: PhaseRoot, Run: stepNextSteps},
}

// uninstallSteps는 제거 단계다(순서대로, root).
var uninstallSteps = []Step{
	{Name: "confirm", Phase: PhaseRoot, Run: stepUninstallConfirm},
	{Name: "stop", Phase: PhaseRoot, Run: stepStop},
	{Name: "web-upgrade", Phase: PhaseRoot, Run: removeWebUpgrade},
	{Name: "helper", Phase: PhaseRoot, Run: removeHelper},
	{Name: "files", Phase: PhaseRoot, Run: stepRemoveFiles},
	{Name: "runtime-dir", Phase: PhaseRoot, Run: stepRemoveRuntimeDir},
	{Name: "state-dir", Phase: PhaseRoot, Run: stepRemoveStateDir},
	{Name: "user", Phase: PhaseRoot, Run: stepRemoveUser},
	{Name: "proxy-note", Phase: PhaseRoot, Run: stepProxyNote},
}
