package setup

import (
	"bytes"
	"embed"
	"fmt"
	"path"
	"text/template"
	"time"

	"maru/backend/config"
)

// templates는 systemd 유닛 템플릿이다. 유닛의 하드닝 줄을 바꾸는 일은 이 템플릿 파일을 고치는 것이다
// (.claude/docs/helper.md를 함께 보고, 사용자에게 보이는 영향(쓸 수 있는 경로 등)이 바뀌면 docs/en/helper.md도 고친다).
//
//go:embed templates/*.tmpl
var templateFS embed.FS

var unitTemplates = template.Must(template.New("").Option("missingkey=error").ParseFS(templateFS, "templates/*.tmpl"))

// unitData는 유닛 템플릿에 넣는 값이다.
type unitData struct {
	User, Group      string
	AllowNonLoopback bool
	Helper           bool
	WebUpgrade       bool

	Service, EnvFile, BinDst, RuntimeDir                  string
	HelperName, HelperSocket, HelperEnvFile, HelperBinDst string

	UpgradeName, UpgradeInstallerDst, UpgradeEnvFile, UpgradeCheckRequest, UpgradeApplyRequest string
	UpgradeRequestDirName, UpgradeTimeout                                                      string

	// 애드온 백엔드(--addon-backends). AddonBackends면 maru 유닛에 호스트 소켓 디렉터리(root가 만들고 maru가 그 안의
	// <id>/에만 쓴다)를 더한다.
	AddonBackends                                            bool
	AddonHostDir, AddonUnitHostDir, AddonSocketDir           string
	AddonExec, AddonsDir, AddonUserPrefix, AddonStateDirName string
	StateDir, UpgradeStateDir, AddonSyncDir, AddonSyncName   string
	AddonSyncRequest, AddonBackendsFile, AddonSyncTimeout    string
	// Tier는 백엔드 템플릿을 그리는 네트워크 단계다(addonTierData가 채운다. 다른 유닛에서는 비어 있다).
	Tier addonTier
}

func (c *Context) unitData() unitData {
	return unitData{
		User:             c.Values.Get(nameUser),
		Group:            c.group,
		AllowNonLoopback: c.Values.Bool(nameAllowNonLoopback),
		Helper:           helperWanted(c),
		WebUpgrade:       webUpgradeWanted(c),
		Service:          serviceName,
		EnvFile:          envFile,
		BinDst:           binDst,
		RuntimeDir:       runtimeDir,
		HelperName:       helperName,
		HelperSocket:     helperSocket,
		HelperEnvFile:    helperEnvFile,
		HelperBinDst:     helperBinDst,

		UpgradeName:           upgradeName,
		UpgradeInstallerDst:   upgradeInstallerDst,
		UpgradeEnvFile:        config.UpgradeEnvFile,
		UpgradeCheckRequest:   upgradeCheckRequest,
		UpgradeApplyRequest:   upgradeApplyRequest,
		UpgradeRequestDirName: upgradeRequestDirName,
		UpgradeTimeout:        systemdSpan(config.UpgradeServiceTimeout),

		AddonBackends:     addonBackendsWanted(c),
		AddonHostDir:      config.AddonHostDir,
		AddonUnitHostDir:  path.Dir(config.AddonUnitHostSocket),
		AddonSocketDir:    config.AddonSocketDir,
		AddonExec:         path.Join(config.InstalledStateDir, config.AddonsDirName, "%i", config.AddonCurrentLink, config.AddonBackendExec),
		AddonsDir:         path.Join(config.InstalledStateDir, config.AddonsDirName),
		AddonUserPrefix:   config.AddonUnitUserPrefix,
		AddonStateDirName: config.AddonStateDirName,
		StateDir:          stateDir,
		UpgradeStateDir:   config.UpgradeStateDir,
		AddonSyncDir:      config.AddonSyncDir,
		AddonSyncName:     config.AddonSyncName,
		AddonSyncRequest:  addonSyncRequest,
		AddonBackendsFile: path.Join(stateDir, config.AddonBackendsFile),
		AddonSyncTimeout:  systemdSpan(addonSyncTimeout),
	}
}

// systemdSpan은 유닛의 시간 값(TimeoutStartSec= 등)이다. 시·분으로 나누어떨어지면 그 단위로 쓴다.
func systemdSpan(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dmin", d/time.Minute)
	}
	return fmt.Sprintf("%ds", (d+time.Second-1)/time.Second)
}

// renderUnit은 유닛 템플릿(templates/<name>.tmpl)을 채운다.
func renderUnit(name string, data unitData) ([]byte, error) {
	var b bytes.Buffer
	if err := unitTemplates.ExecuteTemplate(&b, name+".tmpl", data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
