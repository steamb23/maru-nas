package setup

import (
	"bytes"
	"embed"
	"text/template"

	"onmaru/backend/config"
)

// templates는 systemd 유닛 템플릿이다. 유닛의 하드닝 줄을 바꾸는 일은 이 템플릿 파일을 고치는 것이다
// (README.md의 보안 설계, .claude/docs/helper.md도 함께 본다).
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
	UpgradeRequestDirName                                                                      string
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
	}
}

// renderUnit은 유닛 템플릿(templates/<name>.tmpl)을 채운다.
func renderUnit(name string, data unitData) ([]byte, error) {
	var b bytes.Buffer
	if err := unitTemplates.ExecuteTemplate(&b, name+".tmpl", data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
