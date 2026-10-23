package setup

import (
	"bytes"
	"embed"
	"fmt"
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
