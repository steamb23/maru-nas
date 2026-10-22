package setup

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"strings"

	"maru/backend/config"
)

// runUpgradePlan은 --upgrade-plan이다. 이 버전으로 업그레이드할 때 물을 설정, 수동 재설치 사유, 업그레이드 뒤 할 일을
// JSON(config.UpgradePlan)으로 표준 출력에 쓴다. 기존 설치를 읽기만 하고 파일을 쓰거나 시스템을 바꾸는 명령을 실행하지
// 않는다(실행하는 명령은 node --version뿐이다). 표준 출력에는 JSON만 쓰고 경고는 오류 출력에 쓴다. 수동 재설치가
// 필요해도 계획을 냈으면 성공(0)이다.
//
// 대시보드에서 업그레이드는 대상 커밋의 설치기를 저장소 소유자 계정으로 임시 빌드해 이것을 실행하므로, 저장소 파일을
// 읽지 않는다(실행 파일 위치가 저장소가 아닐 수 있다). 설정 옵션을 함께 주면 그 값을 쓰고 검사하므로 입력한 값을
// 확인하는 데에도 쓴다.
func runUpgradePlan(c *Context) error {
	plan, err := buildUpgradePlan(c)
	if err != nil {
		return err
	}
	// 사람이 읽어도 되게 들여 쓰고, 버전 범위의 >=를 >로 바꾸지 않는다.
	enc := json.NewEncoder(c.Out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(plan)
}

// buildUpgradePlan은 업그레이드 계획을 만든다. 사유를 모두 모은다(첫 사유에서 멈추지 않는다). 설치되어 있지 않으면
// 그 사유만 낸다.
func buildUpgradePlan(c *Context) (config.UpgradePlan, error) {
	self, _, _ := vcsInfo(c.SelfBuild)
	p := config.UpgradePlan{
		Format:    config.UpgradePlanFormat,
		Version:   config.Version,
		Revision:  self,
		Manual:    []config.UpgradeReason{},
		Questions: []config.UpgradeQuestion{},
		Notes:     []config.LocalizedText{},
		Requires:  config.UpgradeRequires{Go: requireGo, Node: requireNode},
	}
	addManual := func(id string, t Text, command string) {
		p.Manual = append(p.Manual, config.UpgradeReason{ID: id, Text: t.localized(), Command: command})
	}
	if inst := c.Installed; !inst.UnitExists || inst.Env == nil {
		addManual(config.UpgradeReasonNotInstalled, L(
			"maru is not installed ("+unitFile+" and "+envFile+" are both needed)",
			"maru가 설치되어 있지 않습니다("+unitFile+"와 "+envFile+"가 모두 있어야 합니다)"), config.UpgradeCommandInstall)
		finishPlan(&p)
		return p, nil
	}
	c.mode = modeUpgrade

	p.Installed = readInstallRecord(c)
	if installed, ok := recordVersion(p.Installed); !ok {
		addManual(config.UpgradeReasonNoRecord, L(
			"there is no usable install record ("+config.InstallRecordFile+"), so the installed version is unknown. After one install with install.sh, later versions can be upgraded from the dashboard",
			"쓸 수 있는 설치 기록("+config.InstallRecordFile+")이 없어 설치된 버전을 알 수 없습니다. install.sh로 한 번 설치하면 그다음 버전부터 대시보드에서 업그레이드할 수 있습니다"),
			config.UpgradeCommandInstall)
	} else {
		this, _ := parseVersion(config.Version)
		if installed.cmp(this) > 0 {
			addManual(config.UpgradeReasonDowngrade, L(
				"the installed version "+p.Installed.Version+" is newer than this version "+config.Version,
				"설치된 버전 "+p.Installed.Version+"이(가) 이 버전 "+config.Version+"보다 높습니다"), config.UpgradeCommandInstall)
		}
		for _, m := range manualUpgrades {
			if below, ok := parseVersion(m.Below); ok && installed.cmp(below) < 0 {
				addManual(config.UpgradeReasonManualList, m.Reason, m.Command)
			}
		}
		for _, n := range upgradeNotes {
			if since, ok := parseVersion(n.Since); ok && installed.cmp(since) < 0 {
				p.Notes = append(p.Notes, n.Text.localized())
			}
		}
	}

	planSettings(c, &p)
	if err := planNode(c, &p); err != nil {
		return p, err
	}
	finishPlan(&p)
	return p, nil
}

// finishPlan은 사유로 Web과 Command를 정한다.
func finishPlan(p *config.UpgradePlan) {
	p.Web = len(p.Manual) == 0
	if p.Web {
		return
	}
	p.Command = config.UpgradeCommandInstall
	for _, r := range p.Manual {
		if r.Command == config.UpgradeCommandReinstall {
			p.Command = config.UpgradeCommandReinstall
		}
	}
}

// readInstallRecord는 설치 기록을 읽는다. 없거나 읽을 수 없거나 모르는 형식이면 nil이다(없는 것 말고는 경고한다).
func readInstallRecord(c *Context) *config.InstallRecord {
	data, err := c.Sys.ReadFile(config.InstallRecordFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	var rec config.InstallRecord
	if err == nil {
		err = json.Unmarshal(data, &rec)
	}
	if err == nil && rec.Format != config.InstallRecordFormat {
		err = errors.New("unknown format")
	}
	if err != nil {
		c.Warn(L("cannot read the install record "+config.InstallRecordFile+": "+err.Error(),
			"설치 기록 "+config.InstallRecordFile+" 을(를) 읽을 수 없습니다: "+err.Error()))
		return nil
	}
	return &rec
}

// recordVersion은 기록의 버전이다. 커밋과 버전이 모두 있어야 쓸 수 있는 기록이다(대시보드에서 업그레이드는 커밋으로
// 대상이 설치된 커밋에서 이어지는지 확인한다).
func recordVersion(rec *config.InstallRecord) (version, bool) {
	if rec == nil || rec.Revision == "" {
		return version{}, false
	}
	return parseVersion(rec.Version)
}

// planSettings는 업그레이드처럼 기존 설치의 설정을 읽어(옵션이 덮어쓴다) 이 버전의 검사를 통과하지 못한 값을 버리고,
// 남은 빠진 설정을 질문으로 만든다. 일반 질문(Prompt)은 웹에서 물을 수 있고, 특수한 질문(Ask, 예: 수신 주소)은
// 수동 재설치 사유가 된다.
func planSettings(c *Context, p *config.UpgradePlan) {
	var check []*Setting
	for _, s := range c.Reg {
		if c.Values.Has(s.Name) {
			check = append(check, s)
		}
	}
	check = append(check, c.Reg.load(c)...)
	before := maps.Clone(c.Values)
	c.invalid = c.Reg.dropInvalid(c, check)
	for _, s := range c.Reg.missing(c) {
		var errText *config.LocalizedText
		if _, bad := c.invalid[s.Name]; bad {
			t := bothLangs(c, before, func(cc *Context) string {
				if err := s.Validate(cc, before.Get(s.Name)); err != nil {
					return err.Error()
				}
				return ""
			})
			errText = &t
		}
		if s.Prompt == nil {
			t := L("the new setting --"+s.Name+" cannot be asked in the dashboard",
				"새 설정 --"+s.Name+" 은(는) 대시보드에서 물을 수 없습니다")
			if errText != nil {
				t = L("--"+s.Name+" in the existing installation is not valid for this version ("+errText.En+") and cannot be asked in the dashboard",
					"기존 설치의 --"+s.Name+" 값이 이 버전의 검사를 통과하지 못했고("+errText.Ko+") 대시보드에서 물을 수 없습니다")
			}
			p.Manual = append(p.Manual, config.UpgradeReason{
				ID: config.UpgradeReasonSetting, Setting: s.Name, Text: t.localized(), Command: config.UpgradeCommandInstall,
			})
			continue
		}
		q := config.UpgradeQuestion{
			Name:    s.Name,
			Kind:    config.UpgradeQuestionInput,
			Title:   s.Prompt.Title.localized(),
			Help:    s.Prompt.Help.localized(),
			Default: s.defaultValue(c),
			Dash:    s.Prompt.Dash,
			EmptyOK: s.Flag == FlagValueEmptyOK,
			Reason:  config.UpgradeQuestionNew,
		}
		if s.Prompt.Kind == PromptConfirm {
			q.Kind = config.UpgradeQuestionConfirm
		}
		if errText != nil {
			q.Reason, q.Error = config.UpgradeQuestionInvalid, errText
		}
		p.Questions = append(p.Questions, q)
	}
}

// bothLangs는 언어마다 Context 사본(설정 값은 values)으로 문구를 만든다. 설정 정의의 문구는 현재 언어로만 만들어지므로
// 계획에 두 언어를 담을 때 쓴다.
func bothLangs(c *Context, values Values, f func(cc *Context) string) config.LocalizedText {
	in := func(lang string) string {
		cc := *c
		cc.Lang = lang
		cc.Values = maps.Clone(values)
		return f(&cc)
	}
	return config.LocalizedText{En: in("en"), Ko: in("ko")}
}

// planNode는 PATH의 node가 이 버전이 요구하는 범위 안인지 본다. Go는 이 설치기를 빌드할 수 있었다면 충분하므로 보지
// 않는다(installer/go.mod의 go 지시어는 backend/go.mod와 같다).
func planNode(c *Context, p *config.UpgradePlan) error {
	r, err := parseVersionRange(requireNode)
	if err != nil {
		return err
	}
	out, err := c.Sys.Output("node", "--version")
	if err != nil {
		p.Manual = append(p.Manual, config.UpgradeReason{ID: config.UpgradeReasonNode, Text: L(
			"Node.js (node) was not found in PATH; this version needs Node.js "+requireNode,
			"PATH에서 Node.js(node)를 찾을 수 없습니다. 이 버전에는 Node.js "+requireNode+"이(가) 필요합니다").localized(),
			Command: config.UpgradeCommandInstall})
		return nil
	}
	found := strings.TrimSpace(string(out))
	p.Requires.NodeFound = strings.TrimPrefix(found, "v")
	if v, ok := parseVersion(found); ok && r.contains(v) {
		return nil
	}
	p.Manual = append(p.Manual, config.UpgradeReason{ID: config.UpgradeReasonNode, Text: L(
		"Node.js "+found+" is installed but this version needs "+requireNode+"; update Node.js first (see docs/install.md)",
		"Node.js "+found+"이(가) 설치되어 있지만 이 버전에는 "+requireNode+"이(가) 필요합니다. 먼저 Node.js를 업데이트하세요(docs/install.md 참고)").localized(),
		Command: config.UpgradeCommandInstall})
	return nil
}
