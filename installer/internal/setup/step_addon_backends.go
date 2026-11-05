package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"time"

	"maru/backend/config"
	"maru/installer/internal/prompt"
)

// addonSyncTimeout은 root 동기화(maru-addons.service) 한 번의 시간 상한(유닛의 TimeoutStartSec)이다. 동기화는 백엔드를
// 하나씩 멈추므로(백엔드 유닛의 TimeoutStopSec 15초) 상한 개수(config.AddonBackendsMax)를 모두 끄는 경우도 들어가게 둔다.
const addonSyncTimeout = 10 * time.Minute

// addonTier는 백엔드 유닛 템플릿의 네트워크 단계 하나다. 같은 템플릿 원본을 단계마다 그린다.
type addonTier struct {
	// Prefix는 유닛 템플릿 이름이다(config.AddonUnitPrefix). 인스턴스는 <Prefix>@<id>.socket·.service다.
	Prefix string
	// Label은 유닛 설명에 넣는 네트워크 권한이다.
	Label string
	// None은 네트워크가 없는 단계, LAN·Loopback은 그 네트워크 권한이 있는 단계다.
	None, LAN, Loopback bool
}

// addonTiers는 네트워크 단계 4개다(없음, lan, loopback, lan + loopback).
func addonTiers() []addonTier {
	var out []addonTier
	for _, network := range [][]string{nil, {config.AddonNetworkLAN}, {config.AddonNetworkLoopback}, {config.AddonNetworkLAN, config.AddonNetworkLoopback}} {
		t := addonTier{
			Prefix:   config.AddonUnitPrefix(network),
			Label:    "no network",
			None:     len(network) == 0,
			LAN:      slices.Contains(network, config.AddonNetworkLAN),
			Loopback: slices.Contains(network, config.AddonNetworkLoopback),
		}
		if !t.None {
			t.Label = "network: " + strings.Join(network, ", ")
		}
		out = append(out, t)
	}
	return out
}

// validAddonUnitPrefix는 name이 백엔드 유닛 템플릿 이름 중 하나인지다.
func validAddonUnitPrefix(name string) bool {
	return slices.ContainsFunc(addonTiers(), func(t addonTier) bool { return t.Prefix == name })
}

func addonSocketUnit(prefix string) string  { return "/etc/systemd/system/" + prefix + "@.socket" }
func addonServiceUnit(prefix string) string { return "/etc/systemd/system/" + prefix + "@.service" }

// addonUnitFiles는 애드온 백엔드를 켜면 쓰는 유닛 파일이다(단계마다 소켓·서비스 템플릿, 동기화의 경로·서비스 유닛).
func addonUnitFiles() []string {
	var files []string
	for _, t := range addonTiers() {
		files = append(files, addonSocketUnit(t.Prefix), addonServiceUnit(t.Prefix))
	}
	return append(files, addonSyncPathUnit, addonSyncServiceUnit)
}

// addonTemplateChange는 한 단계의 템플릿을 새로 쓰거나 바꿨는지다.
type addonTemplateChange struct{ Socket, Service bool }

// addonInstance는 켜진 백엔드 인스턴스 하나다(<Unit>@<ID>).
type addonInstance struct{ Unit, ID string }

func (i addonInstance) socket() string  { return i.Unit + "@" + i.ID + ".socket" }
func (i addonInstance) service() string { return i.Unit + "@" + i.ID + ".service" }

// stepAddonBackends는 애드온 백엔드를 켜기로 했으면 유닛과 동기화 디렉터리를 쓰고, 아니면 이전에 켠 것을 지운다.
func stepAddonBackends(c *Context) error {
	if addonBackendsWanted(c) {
		return installAddonBackends(c)
	}
	return removeAddonBackends(c)
}

// installAddonBackends는 네트워크 단계마다 백엔드 소켓·서비스 템플릿, 동기화의 경로·서비스 유닛을 쓰고, 동기화
// 디렉터리(config.AddonSyncDir, root 0755)에 빈 동기화 결과가 없으면 둔다. maru는 시작할 때 그 결과 파일이 있는지로
// 백엔드가 켜졌는지 알므로 maru를 다시 시작하기 전에 둔다. 템플릿이 바뀌었는지 남겨 두고, 켜진 인스턴스는
// stepAddonBackendsPath가 다시 시작한다. 동기화 기록은 건드리지 않으므로 잠그지 않는다.
func installAddonBackends(c *Context) error {
	base := c.unitData()
	c.addonChanged = map[string]addonTemplateChange{}
	for _, tier := range addonTiers() {
		data := base
		data.Tier = tier
		var ch addonTemplateChange
		var err error
		if ch.Socket, err = writeUnitTracked(c, "maru-addon.socket", addonSocketUnit(tier.Prefix), data); err != nil {
			return err
		}
		if ch.Service, err = writeUnitTracked(c, "maru-addon.service", addonServiceUnit(tier.Prefix), data); err != nil {
			return err
		}
		c.addonChanged[tier.Prefix] = ch
	}
	if _, err := writeUnitTracked(c, "maru-addons.service", addonSyncServiceUnit, base); err != nil {
		return err
	}
	changed, err := writeUnitTracked(c, "maru-addons.path", addonSyncPathUnit, base)
	if err != nil {
		return err
	}
	c.addonPathChanged = changed
	return seedAddonSyncStatus(c)
}

// writeUnitTracked는 유닛 템플릿을 그려 path에 쓰고, 새로 썼거나 내용이 바뀌었는지 돌려준다.
func writeUnitTracked(c *Context, tmpl, path string, data unitData) (bool, error) {
	b, err := renderUnit(tmpl, data)
	if err != nil {
		return false, err
	}
	old, err := c.Sys.ReadFile(path)
	changed := err != nil || !bytes.Equal(old, b)
	return changed, c.writeConfig(path, b, false)
}

// seedAddonSyncStatus는 동기화 결과가 없으면 빈 결과(digest 없음, 0644)를 쓴다. 디렉터리가 없으면 WriteFile이 0755로
// 만든다. 이미 있으면 동기화가 쓴 결과이므로 그대로 둔다.
func seedAddonSyncStatus(c *Context) error {
	if exists(c.Sys, config.AddonSyncStatusFile) {
		return nil
	}
	st := config.AddonSyncStatus{
		Format:    config.AddonSyncStatusFormat,
		UpdatedAt: c.Sys.Now().UTC(),
		Backends:  []config.AddonBackendStatus{},
		Purged:    []string{},
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := c.Sys.WriteFile(config.AddonSyncStatusFile, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if !c.Sys.DryRun() {
		c.Info(L("Wrote "+config.AddonSyncStatusFile, config.AddonSyncStatusFile+" 작성"))
	}
	return nil
}

// stepAddonBackendsPath는 동기화 요청을 지켜보는 경로 유닛을 켜고(유닛이 바뀌었으면 다시 시작한다), 백엔드 템플릿이
// 바뀌었으면 켜진 인스턴스를 다시 시작한다. 동기화와 겹치지 않게 동기화 잠금을 쥔다. 동기화 서비스는 시작하지 않는다
// (요청이 있으면 경로 유닛이 시작하고, 그 서비스는 이 잠금을 기다린다).
func stepAddonBackendsPath(c *Context) error {
	unlock, err := lockAddonSync(c)
	if err != nil {
		return err
	}
	defer unlock()
	unit := config.AddonSyncName + ".path"
	if !c.addonPathChanged {
		if err := c.Sys.RunQuiet("systemctl", "enable", "--now", unit); err != nil {
			return err
		}
	} else {
		if err := c.Sys.RunQuiet("systemctl", "enable", unit); err != nil {
			return err
		}
		if err := c.Sys.Run("systemctl", "restart", unit); err != nil {
			return err
		}
	}
	restartAddonInstances(c)
	return nil
}

// restartAddonInstances는 템플릿이 바뀐 단계의 켜진 인스턴스(knownAddonInstances)를 다시 시작한다. 유닛 설정은 시작할
// 때만 적용되므로 돌고 있는 백엔드가 이전 샌드박스에 남지 않게 한다. 소켓 템플릿이 바뀌었으면 소켓을 다시 시작한다
// (서비스는 Requires=라 함께 다시 시작하고 새 소켓을 받는다). 서비스 템플릿만 바뀌었으면 서비스만 다시 시작한다.
// try-restart라 멈춰 있는 유닛은 시작하지 않는다. 실패는 경고만 한다(백엔드 하나 때문에 설치가 멈추지 않게).
func restartAddonInstances(c *Context) {
	changed := false
	for _, ch := range c.addonChanged {
		changed = changed || ch.Socket || ch.Service
	}
	if !changed {
		// 바뀐 템플릿이 없으면 기록도 읽지 않는다.
		return
	}
	for _, in := range knownAddonInstances(c) {
		var unit string
		switch ch := c.addonChanged[in.Unit]; {
		case ch.Socket:
			unit = in.socket()
		case ch.Service:
			unit = in.service()
		default:
			continue
		}
		if err := c.Sys.RunQuiet("systemctl", "try-restart", unit); err != nil {
			c.Warn(L("cannot restart "+unit+": "+err.Error(), unit+" 을(를) 다시 시작할 수 없습니다: "+err.Error()))
		}
	}
}

// removeAddonBackends는 애드온 백엔드를 끈다. 동기화 경로 유닛을 멈추고, 동기화 잠금을 쥔 채 켜진 인스턴스
// (knownAddonInstances)와 아직 올라와 있는 인스턴스를 모두 멈추고 끈 뒤, 유닛 파일, 동기화 디렉터리, 백엔드·호스트 소켓
// 디렉터리, 남은 요청(일반 파일이 아니어도)을 지운다. 아무것도 없으면 아무것도 하지 않는다(명령도 없다). 백엔드 데이터는 남긴다(제거할 때 따로
// 묻는다). 동기화 기록을 지우므로 남긴 데이터는 그 id를 다시 켤 때 다른 인스턴스의 것으로 보고 지운다(addon_record.go).
func removeAddonBackends(c *Context) error {
	units := addonUnitFiles()
	syncDir := dirExists(c.Sys, config.AddonSyncDir)
	found := syncDir
	for _, f := range units {
		found = found || exists(c.Sys, f)
	}
	if !found {
		return nil
	}
	c.Info(L("Removing addon backends", "애드온 백엔드 제거"))
	// 새 동기화가 시작하지 않게 경로 유닛부터 멈춘다.
	stopAndDisable(c, config.AddonSyncName+".path")
	if syncDir {
		// 돌고 있는 동기화가 있으면 끝날 때까지 기다린다. 디렉터리를 지운 뒤 잠금을 잡은 동기화는 잠금 파일이 바뀐 것을
		// 보고 끝난다.
		unlock, err := lockAddonSync(c)
		if err != nil {
			return err
		}
		defer unlock()
	}
	for _, in := range knownAddonInstances(c) {
		stopAndDisable(c, in.socket(), in.service())
		// disable이 지우지 못한 링크(템플릿이 이미 없는 경우)를 지운다.
		if err := c.Sys.Remove(addonWantsDir + "/" + in.socket()); err != nil {
			return err
		}
	}
	// 기록에도 링크에도 없지만 아직 올라와 있는 인스턴스(유닛 파일 없이 메모리에 남아 도는 것 포함)를 고정된 이름
	// 패턴으로 멈춘다. 패턴은 올라와 있는 유닛에만 맞으며 maru가 정한 값이 들어가지 않는다.
	_ = c.Sys.RunSilent("systemctl", append([]string{"stop"}, addonUnitPatterns()...)...)
	for _, f := range units {
		if err := c.Sys.Remove(f); err != nil {
			return err
		}
	}
	// 요청은 maru가 쓰는 자리라 일반 파일이 아닐 수 있다. 동기화의 takeFile처럼 따라가지 않고 디렉터리여도 통째로
	// 지운다(Remove는 비어 있지 않은 디렉터리에서 실패해 끄기를 끝내지 못하게 된다).
	if err := c.Sys.RemoveAll(addonSyncRequest); err != nil {
		return err
	}
	if err := c.Sys.Run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	for _, d := range []string{config.AddonSyncDir, config.AddonSocketDir, config.AddonHostDir} {
		if err := c.Sys.RemoveAll(d); err != nil {
			return err
		}
	}
	return nil
}

// stopAndDisable은 유닛을 멈춘 뒤 [Install]이 있는 것(소켓·경로 유닛)을 끈다. systemctl disable --now는 disable이
// 실패하면(유닛 파일이 이미 없으면) 멈추지 않으므로 따로 부른다. 실패해도 계속한다(이미 멈췄거나 꺼져 있다).
func stopAndDisable(c *Context, units ...string) {
	_ = c.Sys.RunSilent("systemctl", append([]string{"stop"}, units...)...)
	var installable []string
	for _, u := range units {
		if !strings.HasSuffix(u, ".service") {
			installable = append(installable, u)
		}
	}
	_ = c.Sys.RunSilent("systemctl", append([]string{"disable"}, installable...)...)
}

// addonUnitPatterns는 모든 백엔드 인스턴스에 맞는 systemctl 이름 패턴이다(단계 접두사는 config.AddonUnitPrefix).
func addonUnitPatterns() []string {
	base := config.AddonUnitPrefix(nil)
	var out []string
	for _, suffix := range []string{".socket", ".service"} {
		out = append(out, base+"@*"+suffix, base+"-*@*"+suffix)
	}
	return out
}

// knownAddonInstances는 켜져 있을 수 있는 인스턴스다. 기록에 있는 것(id 순) 다음에 sockets.target.wants의 링크에만
// 있는 것(이름 순)을 둔다. 기록이 없거나 깨져도 켜진 인스턴스를 놓치지 않게 링크도 본다. 기록을 읽지 못하면 경고하고
// 링크로 찾은 것만 쓴다. 일반 사용자의 dry-run은 root 0600 기록을 읽지 못하므로 경고하지 않는다.
func knownAddonInstances(c *Context) []addonInstance {
	var out []addonInstance
	rec, err := readAddonSyncRecord(c.Sys)
	switch {
	case err == nil:
		for _, b := range rec.Backends {
			out = append(out, addonInstance{Unit: b.Unit, ID: b.ID})
		}
	case c.Sys.DryRun() && errors.Is(err, fs.ErrPermission):
		// 일반 사용자의 dry-run: 경고 없이 링크로 찾은 것만 쓴다.
	default:
		c.Warn(L("cannot read "+config.AddonSyncRecordFile+" ("+err.Error()+"); using only the addon backends enabled in "+addonWantsDir,
			config.AddonSyncRecordFile+" 을(를) 읽을 수 없어("+err.Error()+") "+addonWantsDir+" 에서 켜진 애드온 백엔드만 다룹니다"))
	}
	for _, in := range enabledAddonInstances(c.Sys) {
		if !slices.Contains(out, in) {
			out = append(out, in)
		}
	}
	return out
}

// enabledAddonInstances는 sockets.target.wants에 링크가 있는 백엔드 소켓 인스턴스다(<단계>@<id>.socket, 이름 순). 이름이
// 단계와 id 형식에 맞는 것만 받는다(id가 systemctl 인자로 가므로).
func enabledAddonInstances(sys System) []addonInstance {
	entries, err := sys.ReadDir(addonWantsDir)
	if err != nil {
		return nil
	}
	var out []addonInstance
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".socket")
		if !ok {
			continue
		}
		unit, id, ok := strings.Cut(name, "@")
		if ok && validAddonUnitPrefix(unit) && config.ValidAddonID(id) {
			out = append(out, addonInstance{Unit: unit, ID: id})
		}
	}
	return out
}

// stepRemoveAddonData는 제거할 때 확인을 받은 뒤에만 백엔드 데이터(systemd가 둔 StateDirectory와 그 링크)를 지운다.
// 유닛을 모두 지운 뒤라 systemctl clean을 쓰지 않고 디렉터리를 지운다. 없으면 묻지 않는다.
func stepRemoveAddonData(c *Context) error {
	dirs := []string{addonPrivateDataDir, addonDataDir}
	if !slices.ContainsFunc(dirs, func(d string) bool { return dirExists(c.Sys, d) }) {
		return nil
	}
	ok, err := c.confirm(prompt.Confirm{
		ID: "uninstall-addon-data",
		Title: c.L("Also delete the data that addon backends kept ("+addonPrivateDataDir+")?",
			"애드온 백엔드가 남긴 데이터("+addonPrivateDataDir+")도 지울까요?"),
		// 동기화 기록을 지웠으므로 남긴 데이터는 그 패키지의 백엔드를 다시 켤 때 지워진다(removeAddonBackends, addon_sync.go).
		Help: c.L("If addon backends are turned on again, a package's kept data is deleted before that package's backend starts again; keep it only to copy it by hand.",
			"남겨 두어도 애드온 백엔드를 다시 켜면 패키지의 백엔드를 다시 켜기 전에 그 패키지의 데이터를 지웁니다. 손으로 옮겨 둘 때만 남겨 두세요."),
	})
	if err != nil || !ok {
		return err
	}
	for _, d := range dirs {
		if err := c.Sys.RemoveAll(d); err != nil {
			return err
		}
	}
	c.Info(L("Removed the data of addon backends", "애드온 백엔드 데이터 삭제"))
	return nil
}
