package setup

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"maru/backend/config"
)

// 애드온 백엔드의 root 동기화(--addon-sync). maru가 원하는 상태(<state>/config.AddonBackendsFile)를 쓰고 요청
// (<state>/config.AddonSyncRequest)을 남기면 maru-addons.path가 maru-addons.service를 시작하고, 그 서비스가 root 소유
// 설치기 복사본으로 이것을 실행한다. 설계는 .claude/docs/addon-packages.md의 "root 동기화"다.
//
//   - maru가 쓴 요청과 원하는 상태는 믿지 않는 입력이다. O_NOFOLLOW·크기 상한으로 읽고 config.ParseAddonBackends로 다시
//     검사한다. 패키지 파일은 읽지 않는다
//   - 고정 템플릿 인스턴스(<config.AddonUnitPrefix>@<검사한 id>)에 정해진 systemctl 동사(enable --now, stop, disable,
//     try-restart, reset-failed, start, clean --what=state)와 읽기 전용 is-active만 실행한다. 유닛 파일·드롭인은 쓰지
//     않는다
//   - 무엇을 켜 두었고 어느 설치의 데이터를 남겼는지는 자기 기록(config.AddonSyncRecordFile, addon_record.go)으로 알고,
//     결과는 config.AddonSyncStatusFile(0644)에 쓴다. 기록을 읽거나 쓰는 동안 config.AddonSyncLockFile을 쥔다
//
// 작업 실패(원하는 상태 검사 실패, systemctl 실패)는 결과 파일에 남기고 0으로 끝낸다(경로 유닛이 계속 지켜보게).
// root가 아님, 잠금 실패, 결과 파일을 쓰지 못함만 오류다(--upgrade-service와 같다).

const (
	// addonSyncLockWait는 동기화가 설치기(설치·제거 중)의 잠금을 기다리는 시간이다. 설치기는 systemctl을 부르는 동안만
	// 잠금을 쥔다. 유닛의 시간 상한(addonSyncTimeout 10분) 안에 이 대기(1분)와 상한 개수를 하나씩 끄는 시간(32 ×
	// 백엔드 유닛의 TimeoutStopSec 15초 = 8분)을 두고 1분을 남긴다. 그래도 넘어 끊기면 기록은 바꿀 때마다 썼으므로 다음
	// 동기화가 이어 간다.
	addonSyncLockWait = time.Minute
	// addonSyncRounds는 한 번 실행할 때 원하는 상태를 다시 읽어 맞추는 최대 횟수다. 그 사이에 maru가 파일을 또 바꾸면
	// maru가 남긴 다음 요청으로 다시 실행된다.
	addonSyncRounds = 3
	// addonSyncErrorMax는 결과 파일에 남기는 오류 문구 하나의 최대 바이트 수다.
	addonSyncErrorMax = 256
)

// runAddonSync는 --addon-sync다.
func runAddonSync(c *Context) error {
	if !c.IsRoot {
		return fail(c.L("--addon-sync must run as root; "+config.AddonSyncName+".service runs it",
			"--addon-sync는 root로만 실행할 수 있습니다. "+config.AddonSyncName+".service가 실행합니다"))
	}
	return syncAddonBackends(c, addonSyncLockWait)
}

// syncAddonBackends는 요청을 지우고, 동기화 잠금을 쥔 채 원하는 상태를 맞춘다. 맞춘 뒤 원하는 상태 파일이 바뀌었으면
// addonSyncRounds번까지 다시 맞춘다. lockWait는 설치기의 잠금을 기다리는 시간이다.
func syncAddonBackends(c *Context, lockWait time.Duration) error {
	// 요청은 내용과 상관없이 가장 먼저 지운다. maru는 요청이 남아 있는 시간으로 동기화가 멈췄는지 보고, 잠금을 기다리기
	// 전에 지워야 잠금을 얻지 못하고 끝나도 경로 유닛이 같은 요청으로 곧바로 다시 시작하지 않는다.
	switch _, err := c.Sys.TakeFile(addonSyncRequest, config.AddonSyncRequestMaxBytes); {
	case err == nil, errors.Is(err, fs.ErrNotExist):
	case errors.Is(err, errRequestKept):
		// 동기화는 그대로 한다. 요청이 남아 있으므로 경로 유닛이 곧바로 다시 시작하고, maru에는 동기화 멈춤으로 보인다.
		c.Warn(L(addonSyncRequest+": "+err.Error()+"; "+config.AddonSyncName+".path starts the sync again until it is removed",
			addonSyncRequest+": "+err.Error()+". 지울 때까지 "+config.AddonSyncName+".path가 동기화를 다시 시작합니다"))
	default:
		c.Warn(L(addonSyncRequest+": "+err.Error()+" (removed; only its presence matters)",
			addonSyncRequest+": "+err.Error()+" (지웠습니다. 요청은 있는지만 봅니다)"))
	}
	// 동기화 디렉터리는 켜짐의 기준이므로 만들지 않는다(잠금 파일도 디렉터리가 없으면 만들지 못한다).
	if !dirExists(c.Sys, config.AddonSyncDir) {
		c.Info(L("Addon backends are not enabled ("+config.AddonSyncDir+" is missing); nothing to do",
			"애드온 백엔드가 켜져 있지 않습니다("+config.AddonSyncDir+" 없음). 할 일이 없습니다"))
		return nil
	}
	unlock, err := c.Sys.Lock(config.AddonSyncLockFile, 0)
	if err != nil && dirExists(c.Sys, config.AddonSyncDir) {
		c.Info(L("Waiting for the installer to finish (up to "+lockWait.String()+")",
			"설치기가 끝나기를 기다리는 중(최대 "+lockWait.String()+")"))
		unlock, err = c.Sys.Lock(config.AddonSyncLockFile, lockWait)
	}
	if err != nil {
		if !dirExists(c.Sys, config.AddonSyncDir) {
			// 기다리는 동안 설치기가 애드온 백엔드를 껐다(동기화 디렉터리를 지웠다).
			c.Info(L("Addon backends were turned off while waiting; nothing to do",
				"기다리는 동안 애드온 백엔드가 꺼졌습니다. 할 일이 없습니다"))
			return nil
		}
		return fail(c.L("cannot lock the addon backend sync: "+err.Error(), "애드온 백엔드 동기화를 잠글 수 없습니다: "+err.Error()))
	}
	defer unlock()
	desired := readAddonDesired(c.Sys)
	for round := 1; ; round++ {
		applied, err := syncAddonRound(c, desired)
		if err != nil {
			return err
		}
		if round == addonSyncRounds {
			return nil
		}
		desired = readAddonDesired(c.Sys)
		if desired.digest() == applied {
			return nil
		}
		c.Info(L(config.AddonBackendsFile+" changed during the sync; syncing again", "동기화하는 동안 "+config.AddonBackendsFile+" 이(가) 바뀌어 다시 맞춥니다"))
	}
}

// addonDesired는 원하는 상태 파일을 한 번 읽은 것이다.
type addonDesired struct {
	data []byte
	// present는 파일을 읽었는지다. 파일이 없으면(빈 원하는 상태) 거짓이고 err도 없다.
	present bool
	// err는 파일을 읽지 못한 이유다(심볼릭 링크, 일반 파일 아님, 상한 넘음).
	err error
}

// readAddonDesired는 maru가 쓴 원하는 상태 파일을 심볼릭 링크를 따라가지 않고 상한까지 읽는다.
func readAddonDesired(sys System) addonDesired {
	data, err := sys.ReadNoFollow(addonBackendsFile, config.AddonBackendsMaxBytes)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return addonDesired{}
	case err != nil:
		return addonDesired{err: err}
	}
	return addonDesired{data: data, present: true}
}

// digest는 결과에 적을 원하는 상태 파일의 digest(config.AddonBackendsDigest)다. 읽은 내용이 없으면(파일이 없거나 읽지
// 못함) 빈 값이다.
func (d addonDesired) digest() string {
	if !d.present {
		return ""
	}
	return config.AddonBackendsDigest(d.data)
}

// parse는 원하는 상태다. 파일이 없으면 빈 상태(모두 끈다)다.
func (d addonDesired) parse() (config.AddonBackends, error) {
	switch {
	case d.err != nil:
		return config.AddonBackends{}, d.err
	case !d.present:
		return config.AddonBackends{Format: config.AddonBackendsFormat, Backends: []config.AddonBackend{}, Purge: []string{}}, nil
	case !json.Valid(d.data):
		// 믿지 않는 입력이므로 ParseAddonBackends(뒤에 남은 값도 거부한다)에 앞서 내용 전체가 JSON 값 하나인지 따로 본다.
		return config.AddonBackends{}, errors.New("not a single JSON value")
	}
	return config.ParseAddonBackends(d.data)
}

// syncAddonRound는 원하는 상태 하나를 맞추고 결과를 쓴다. 맞춘 파일의 digest를 돌려준다. 결과 파일을 쓰지 못하면
// 오류다. 원하는 상태나 기록을 읽지 못하면 아무것도 바꾸지 않고 결과의 error에 남긴다.
func syncAddonRound(c *Context, d addonDesired) (string, error) {
	st := config.AddonSyncStatus{
		Format:   config.AddonSyncStatusFormat,
		Digest:   d.digest(),
		Backends: []config.AddonBackendStatus{},
		Purged:   []string{},
	}
	if err := applyAddonDesired(c, d, &st); err != nil {
		c.Warn(L("addon backend sync failed: "+err.Error(), "애드온 백엔드 동기화 실패: "+err.Error()))
		// 전체가 실패한 결과는 maru가 항목을 보지 않는다(모두 대기로 본다).
		st.Error = boundedSyncError(err)
		st.Backends, st.Purged = []config.AddonBackendStatus{}, []string{}
	}
	st.UpdatedAt = c.Sys.Now().UTC()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return "", err
	}
	if err := c.Sys.WriteFile(config.AddonSyncStatusFile, append(data, '\n'), 0o644); err != nil {
		return "", fail(c.L("cannot write "+config.AddonSyncStatusFile+": "+err.Error(), config.AddonSyncStatusFile+" 을(를) 쓸 수 없습니다: "+err.Error()))
	}
	failed := 0
	for _, b := range st.Backends {
		if b.State == config.AddonBackendFailed {
			failed++
		}
	}
	c.Info(L(fmt.Sprintf("Addon backend sync: %d backends, %d failed, %d purged", len(st.Backends), failed, len(st.Purged)),
		fmt.Sprintf("애드온 백엔드 동기화: 백엔드 %d개, 실패 %d개, 데이터 삭제 %d개", len(st.Backends), failed, len(st.Purged))))
	return st.Digest, nil
}

// applyAddonDesired는 원하는 상태를 검사하고 기록과 비교해 맞춘다. 순서는 기록과 켜진 소켓 링크 맞추기, 원하는 상태에
// 없는 백엔드 끄기, 원하는 백엔드 켜기(id 순), purge다(데이터는 늘 그 id의 모든 단계를 멈춘 뒤 지운다). 항목별 결과는
// st에 담는다. 오류는 전체 실패(원하는 상태·기록을 읽지 못함, 기록을 쓰지 못함)다.
func applyAddonDesired(c *Context, d addonDesired, st *config.AddonSyncStatus) error {
	want, err := d.parse()
	if err != nil {
		return fmt.Errorf("%s: %w", config.AddonBackendsFile, err)
	}
	rec, err := readAddonSyncRecord(c.Sys)
	if err != nil {
		// 기록 없이는 남긴 데이터가 어느 설치의 것인지 모르므로 아무것도 바꾸지 않는다(빈 기록으로 보면 모든 데이터를
		// 지우게 된다). 손으로 고치거나 지우면 다음 동기화가 이어 간다.
		return err
	}
	backends := slices.Clone(want.Backends)
	slices.SortFunc(backends, func(a, b config.AddonBackend) int { return cmp.Compare(a.ID, b.ID) })
	wanted := map[string]bool{}
	for _, e := range backends {
		wanted[e.ID] = true
	}
	s := &addonSyncer{c: c, rec: rec, wanted: wanted, cleanErr: map[string]error{}, blocked: map[string]error{}}
	s.reconcile()
	if s.saveErr == nil {
		s.removeUnwanted(wanted)
	}
	for _, e := range backends {
		if s.saveErr != nil {
			break
		}
		in := addonInstance{Unit: config.AddonUnitPrefix(e.Network), ID: e.ID}
		bs := config.AddonBackendStatus{ID: e.ID, Rev: e.Rev, Unit: in.service(), State: config.AddonBackendEnabled}
		if err := s.apply(e, in); err != nil {
			c.Warn(L("addon backend "+e.ID+": "+err.Error(), "애드온 백엔드 "+e.ID+": "+err.Error()))
			bs.State, bs.Error = config.AddonBackendFailed, boundedSyncError(err)
		}
		st.Backends = append(st.Backends, bs)
	}
	if s.saveErr == nil {
		st.Purged = s.purge(want.Purge, wanted)
	}
	return s.saveErr
}

// addonSyncer는 동기화 한 차례의 상태다. 기록(rec)은 바꿀 때마다 바로 쓴다(도중에 끊겨도 켠 인스턴스와 남긴 데이터의
// 설치 인스턴스를 잃지 않게).
type addonSyncer struct {
	c   *Context
	rec addonSyncRecord
	// wanted는 원하는 상태의 id다.
	wanted map[string]bool
	// cleanErr는 이번 차례에 데이터를 지우지 못한 id와 그 이유다(같은 id를 다시 시도하지 않는다).
	cleanErr map[string]error
	// blocked는 기록과 맞지 않는 인스턴스(기록에 없는 링크, 링크가 없는 기록 항목)를 멈추지 못한 id와 그 이유다. 이번
	// 차례에는 그 id를 켜거나 데이터를 지우지 않는다(멈추지 못한 프로세스가 그 데이터를 쓰거나 새 설치로 기록될 수 있다).
	blocked map[string]error
	// saveErr는 기록을 쓰지 못한 이유다. 생기면 더 바꾸지 않고 전체 실패로 끝낸다.
	saveErr error
}

// enabled는 켜 둔 것으로 적힌 id의 항목이다.
func (s *addonSyncer) enabled(id string) (addonRecordBackend, bool) {
	i := slices.IndexFunc(s.rec.Backends, func(b addonRecordBackend) bool { return b.ID == id })
	if i < 0 {
		return addonRecordBackend{}, false
	}
	return s.rec.Backends[i], true
}

// retained는 데이터를 남긴 것으로 적힌 id의 항목이다.
func (s *addonSyncer) retained(id string) (addonRecordData, bool) {
	i := slices.IndexFunc(s.rec.Data, func(d addonRecordData) bool { return d.ID == id })
	if i < 0 {
		return addonRecordData{}, false
	}
	return s.rec.Data[i], true
}

// setEnabled는 id를 켜 둔 것으로 적는다(남긴 데이터 항목은 지운다).
func (s *addonSyncer) setEnabled(b addonRecordBackend) {
	s.drop(b.ID)
	s.rec.Backends = append(s.rec.Backends, b)
	s.save()
}

// retain은 id를 끄고 그 설치 인스턴스의 데이터를 남긴 것으로 적는다. 남긴 데이터 항목이 상한이면, 원하는 상태에 없는
// id는 적지 않고(기록에 없는 데이터는 그 id를 다시 켤 때 다른 설치의 것으로 보고 지운다), 원하는 상태에 있는 id(단계를
// 바꾸거나 다시 켜려고 잠깐 끈 것)는 원하는 상태에 없는 항목 하나(id 순으로 첫 번째)를 밀어내고 적는다. 그래야 켜기가
// 실패해도 다음 동기화가 같은 설치의 데이터를 지우지 않는다. 원하는 id는 config.AddonBackendsMax개까지라 상한
// (addonRecordMaxData)보다 적으므로 밀어낼 항목은 늘 있다.
func (s *addonSyncer) retain(id, instance string) {
	s.drop(id)
	if len(s.rec.Data) >= addonRecordMaxData {
		i := slices.IndexFunc(s.rec.Data, func(d addonRecordData) bool { return !s.wanted[d.ID] })
		if !s.wanted[id] || i < 0 {
			s.c.Warn(L(fmt.Sprintf("more than %d addon backends have kept data; the data of %s is no longer recorded and is deleted if that backend is enabled again", addonRecordMaxData, id),
				fmt.Sprintf("데이터를 남긴 애드온 백엔드가 %d개를 넘어 %s 의 데이터는 기록하지 않습니다. 그 백엔드를 다시 켜면 지웁니다", addonRecordMaxData, id)))
			s.save()
			return
		}
		old := s.rec.Data[i].ID
		s.c.Warn(L(fmt.Sprintf("more than %d addon backends have kept data; the data of %s is no longer recorded (to keep the record of %s) and is deleted if that backend is enabled again", addonRecordMaxData, old, id),
			fmt.Sprintf("데이터를 남긴 애드온 백엔드가 %d개를 넘어 %s 의 기록을 남기려고 %s 의 데이터는 기록하지 않습니다. 그 백엔드를 다시 켜면 지웁니다", addonRecordMaxData, id, old)))
		s.rec.Data = slices.Delete(s.rec.Data, i, i+1)
	}
	s.rec.Data = append(s.rec.Data, addonRecordData{ID: id, Instance: instance})
	s.save()
}

// forget은 id를 기록에서 지운다.
func (s *addonSyncer) forget(id string) {
	if s.drop(id) {
		s.save()
	}
}

// drop은 id의 항목을 기록(메모리)에서 지우고, 지운 것이 있었는지 돌려준다.
func (s *addonSyncer) drop(id string) bool {
	n := len(s.rec.Backends) + len(s.rec.Data)
	s.rec.Backends = slices.DeleteFunc(s.rec.Backends, func(b addonRecordBackend) bool { return b.ID == id })
	s.rec.Data = slices.DeleteFunc(s.rec.Data, func(d addonRecordData) bool { return d.ID == id })
	return len(s.rec.Backends)+len(s.rec.Data) != n
}

// save는 기록을 쓴다. 한 번 실패하면 더 쓰지 않는다.
func (s *addonSyncer) save() {
	if s.saveErr != nil {
		return
	}
	if err := writeAddonSyncRecord(s.c.Sys, s.rec); err != nil {
		s.saveErr = fmt.Errorf("write %s: %w", config.AddonSyncRecordFile, err)
	}
}

// systemctl은 systemctl 동사 하나를 실행한다(인자는 정해진 동사와 검사한 id로 만든 인스턴스 이름뿐이다).
func (s *addonSyncer) systemctl(args ...string) error {
	line := commandLine("systemctl", args)
	s.c.Info(L(line, line))
	return s.c.Sys.Run("systemctl", args...)
}

// stop은 인스턴스를 멈추고 소켓을 끈다. 서비스에는 [Install]이 없으므로 disable은 소켓만 받는다. disable --now는
// disable이 실패하면 멈추지 않으므로 stop을 먼저 따로 부른다(stopAndDisable과 같은 순서지만 실패를 돌려준다).
func (s *addonSyncer) stop(in addonInstance) error {
	if err := s.systemctl("stop", in.socket(), in.service()); err != nil {
		return err
	}
	return s.systemctl("disable", in.socket())
}

// start는 인스턴스의 소켓을 켜고 시작한다(서비스는 첫 연결 때 시작한다). 실패하면 반쯤 켜진 것(링크만 생겼거나 소켓만
// 떴다)을 되돌린다. 기록에 없는 소켓이 다음 부팅에 혼자 뜨지 않게 하려는 것이다.
func (s *addonSyncer) start(in addonInstance) error {
	err := s.systemctl("enable", "--now", in.socket())
	if err != nil {
		_ = s.c.Sys.RunSilent("systemctl", "stop", in.socket(), in.service())
		_ = s.c.Sys.RunSilent("systemctl", "disable", in.socket())
	}
	return err
}

// ensureSocket은 켜 둔 인스턴스의 소켓이 떠 있는지 보고, 아니면(계속 죽는 백엔드가 시작 한도에 걸려 소켓까지 실패했거나
// 누가 멈췄다) 실패 상태와 한도를 지우고 소켓을 다시 시작한다. 다시 시작했는지 돌려준다. 서비스는 보지 않는다(연결이
// 없으면 멈춰 있는 것이 정상이다). 링크는 남아 있으므로(reconcile) 다시 켜지 않는다.
func (s *addonSyncer) ensureSocket(in addonInstance) (bool, error) {
	if s.c.Sys.Check("systemctl", "is-active", "--quiet", in.socket()) == nil {
		return false, nil
	}
	s.c.Warn(L(in.socket()+" is enabled but not active (its backend may have kept exiting); starting it again",
		in.socket()+" 이(가) 켜져 있지만 떠 있지 않아(백엔드가 계속 끝났을 수 있습니다) 다시 시작합니다"))
	if err := s.systemctl("reset-failed", in.socket(), in.service()); err != nil {
		// 실패 상태가 아니었거나 올라와 있지 않은 유닛이다. 다시 시작할 수 있는지는 start가 정한다.
		s.c.Warn(L("cannot reset "+in.socket()+": "+err.Error(), in.socket()+" 의 실패 상태를 지울 수 없습니다: "+err.Error()))
	}
	return true, s.systemctl("start", in.socket())
}

// addonIDUnits는 id의 모든 네트워크 단계 인스턴스 이름이다(단계마다 소켓·서비스).
func addonIDUnits(id string) []string {
	var out []string
	for _, t := range addonTiers() {
		in := addonInstance{Unit: t.Prefix, ID: id}
		out = append(out, in.socket(), in.service())
	}
	return out
}

// clean은 id의 백엔드 데이터를 지운다(멈춘 유닛에서 systemctl clean --what=state). StateDirectory는 네트워크 단계와
// 상관없이 같으므로 네트워크 없는 단계의 서비스 이름으로 지운다. systemctl clean은 이름을 준 유닛이 멈췄는지만 보므로,
// 기록·링크와 상관없이 그 id의 모든 단계 인스턴스를 먼저 멈춘다(손으로 시작한 다른 단계가 그 데이터를 쓰는 채로 지우지
// 않게). 데이터가 없으면 아무것도 하지 않는다(돌고 있는 서비스는 시작할 때 StateDirectory를 만들었으므로 데이터가 있다.
// 테스트 VM에서 private 아래의 디렉터리와 바깥 링크가 모두 생기고 clean이 둘 다 지웠다). 지운 뒤에도
// 남아 있으면 실패다(다른 설치의 데이터가 남은 채로 켜지 않게). 기록과 맞지 않는 인스턴스를 멈추지 못한 id는 지우지 않는다.
func (s *addonSyncer) clean(id string) error {
	if err, ok := s.cleanErr[id]; ok {
		return err
	}
	if err := s.blocked[id]; err != nil {
		return err
	}
	if !hasAddonData(s.c.Sys, id) {
		return nil
	}
	err := s.systemctl(append([]string{"stop"}, addonIDUnits(id)...)...)
	if err == nil {
		err = s.systemctl("clean", "--what=state", addonInstance{Unit: config.AddonUnitPrefix(nil), ID: id}.service())
	}
	if err == nil && hasAddonData(s.c.Sys, id) {
		err = errors.New("backend data is still present after systemctl clean")
	}
	if err != nil {
		s.cleanErr[id] = fmt.Errorf("delete the data of another installation: %w", err)
		return s.cleanErr[id]
	}
	return nil
}

// reconcile은 기록과 sockets.target.wants의 소켓 링크를 맞춘다. 기록에 없는 켜진 인스턴스(기록을 잃었거나 손으로
// 켰다)는 멈추고 끈다. 그 데이터는 기록에 없으므로 그 id를 켤 때 다른 설치의 것으로 보고 지운다. 링크가 없는 기록
// 항목(손으로 껐다. 링크만 지우고 유닛은 돌 수 있다)은 멈춘 뒤 데이터를 남긴 것으로 옮겨 아래에서 다시 켠다. 멈추지
// 못하면 그 id를 이번 차례에 막는다(blocked, 기록은 그대로).
func (s *addonSyncer) reconcile() {
	links := enabledAddonInstances(s.c.Sys)
	for _, in := range links {
		if b, ok := s.enabled(in.ID); ok && b.Unit == in.Unit {
			continue
		}
		s.c.Warn(L(in.socket()+" is enabled but not recorded in "+config.AddonSyncRecordFile+"; stopping it",
			in.socket()+" 이(가) 켜져 있지만 "+config.AddonSyncRecordFile+" 에 없어 멈춥니다"))
		if err := s.stop(in); err != nil {
			s.c.Warn(L("cannot stop "+in.socket()+": "+err.Error(), in.socket()+" 을(를) 멈출 수 없습니다: "+err.Error()))
			s.blocked[in.ID] = fmt.Errorf("cannot stop %s, which is enabled but not recorded: %w", in.socket(), err)
		}
	}
	for _, b := range sortedRecordBackends(s.rec.Backends) {
		in := addonInstance{Unit: b.Unit, ID: b.ID}
		if slices.Contains(links, in) {
			continue
		}
		s.c.Warn(L(in.socket()+" is recorded but not enabled; stopping it, and enabling it again if it is wanted",
			in.socket()+" 이(가) 기록에 있지만 켜져 있지 않아 멈추고, 원하는 상태에 있으면 다시 켭니다"))
		if err := s.stop(in); err != nil {
			s.c.Warn(L("cannot stop "+in.socket()+": "+err.Error(), in.socket()+" 을(를) 멈출 수 없습니다: "+err.Error()))
			s.blocked[b.ID] = fmt.Errorf("cannot stop %s, which is recorded but not enabled: %w", in.socket(), err)
			continue
		}
		s.retain(b.ID, b.Instance)
		if s.saveErr != nil {
			return
		}
	}
}

// removeUnwanted는 원하는 상태에 없는 켜진 id를 멈추고 끈다. 데이터는 남기고 그 설치 인스턴스를 적는다(제거한 패키지를
// 다시 설치하거나 purge할 때까지). 끄지 못하면 기록에 그대로 두고 다음 동기화가 다시 끈다.
func (s *addonSyncer) removeUnwanted(wanted map[string]bool) {
	for _, b := range sortedRecordBackends(s.rec.Backends) {
		if wanted[b.ID] {
			continue
		}
		if err := s.stop(addonInstance{Unit: b.Unit, ID: b.ID}); err != nil {
			s.c.Warn(L("cannot stop the removed addon backend "+b.ID+": "+err.Error(), "제거한 애드온 백엔드 "+b.ID+" 을(를) 멈출 수 없습니다: "+err.Error()))
			continue
		}
		s.retain(b.ID, b.Instance)
		if s.saveErr != nil {
			return
		}
	}
}

// apply는 원하는 백엔드 하나를 맞춘다. in은 그 네트워크 단계의 인스턴스다. 켜 두었으면 nil, 아니면 결과에 적을 이유다.
//   - 기록과 맞지 않는 인스턴스를 멈추지 못한 id(blocked): 아무것도 하지 않고 실패다
//   - 설치 인스턴스가 다름(다시 설치했거나 같은 id의 다른 패키지): 이전 단계를 끄고 데이터를 지운 뒤 켠다
//   - 네트워크 단계가 다름: 이전 단계를 끄고 새 단계를 켠다(데이터는 그대로)
//   - rev가 다름(업데이트): 돌고 있는 서비스만 다시 시작한다(멈춰 있으면 다음 연결 때 새 실행 파일로 시작한다)
//   - 바뀐 것 없음: 소켓이 떠 있는지 본다(rev가 다를 때도 먼저 본다. ensureSocket)
//   - 꺼져 있음: 남긴 데이터가 같은 설치의 것이면 그대로, 다른 설치의 것이거나 기록에 없으면 지운 뒤 켠다
func (s *addonSyncer) apply(e config.AddonBackend, in addonInstance) error {
	if err := s.blocked[e.ID]; err != nil {
		return err
	}
	cur, on := s.enabled(e.ID)
	curIn := addonInstance{Unit: cur.Unit, ID: cur.ID}
	if err := checkAddonAccount(s.c.Sys, e.ID); err != nil {
		// 같은 이름의 고정 계정이 있으면 systemd는 동적 사용자 대신 그 계정으로 실행한다. 켜져 있었으면 끈다.
		if on && s.stop(curIn) == nil {
			s.retain(e.ID, cur.Instance)
		}
		return err
	}
	switch {
	case on && cur.Instance != e.Instance:
		if err := s.stop(curIn); err != nil {
			return err
		}
		s.retain(e.ID, cur.Instance)
		if err := s.clean(e.ID); err != nil {
			return err
		}
		s.forget(e.ID)
		return s.enable(e, in)
	case on && cur.Unit != in.Unit:
		if err := s.stop(curIn); err != nil {
			return err
		}
		s.retain(e.ID, cur.Instance)
		return s.enable(e, in)
	case on && cur.Rev != e.Rev:
		// 실패하면 기록의 rev를 그대로 두어 다음 동기화가 다시 시작해 본다. 소켓을 다시 시작했으면 서비스도 멈춰 있었을
		// 것이므로(Requires=) 다음 연결 때 새 실행 파일로 시작한다.
		restarted, err := s.ensureSocket(in)
		if err != nil {
			return err
		}
		if !restarted {
			if err := s.systemctl("try-restart", in.service()); err != nil {
				return err
			}
		}
		cur.Rev = e.Rev
		s.setEnabled(cur)
		return nil
	case on:
		_, err := s.ensureSocket(in)
		return err
	}
	if d, ok := s.retained(e.ID); !ok || d.Instance != e.Instance {
		if err := s.clean(e.ID); err != nil {
			return err
		}
		s.forget(e.ID)
	}
	return s.enable(e, in)
}

// enable은 꺼져 있는 id의 인스턴스를 켜고 기록한다.
func (s *addonSyncer) enable(e config.AddonBackend, in addonInstance) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	if len(s.rec.Backends) >= config.AddonBackendsMax {
		return fmt.Errorf("more than %d backends would be enabled (a removed backend could not be stopped)", config.AddonBackendsMax)
	}
	if err := s.start(in); err != nil {
		return err
	}
	s.setEnabled(addonRecordBackend{ID: e.ID, Unit: in.Unit, Rev: e.Rev, Instance: e.Instance})
	return nil
}

// purge는 원하는 상태의 purge 가운데 backends에 없는 id의 데이터를 지운다. 남긴 데이터 항목이 없어도 디스크에 있으면
// 지운다. backends에 있는 id는 무시한다. 아직 켜져 있는 id(끄지 못함)는 다음 동기화로 미룬다. 지운 것을(데이터가
// 없었던 것도) 확인한 id를 돌려준다.
func (s *addonSyncer) purge(ids []string, wanted map[string]bool) []string {
	purged := []string{}
	ids = slices.Clone(ids)
	slices.Sort(ids)
	for _, id := range ids {
		if wanted[id] {
			continue
		}
		if _, on := s.enabled(id); on {
			s.c.Warn(L("not deleting the data of "+id+": its backend is still enabled", id+" 의 데이터를 지우지 않습니다: 백엔드가 아직 켜져 있습니다"))
			continue
		}
		if err := s.clean(id); err != nil {
			s.c.Warn(L("cannot delete the data of "+id+": "+err.Error(), id+" 의 데이터를 지울 수 없습니다: "+err.Error()))
			continue
		}
		s.forget(id)
		if s.saveErr != nil {
			break
		}
		purged = append(purged, id)
	}
	return purged
}

// sortedRecordBackends는 켜 둔 항목을 id 순으로 정리한 사본이다.
func sortedRecordBackends(list []addonRecordBackend) []addonRecordBackend {
	out := slices.Clone(list)
	slices.SortFunc(out, func(a, b addonRecordBackend) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// hasAddonData는 id의 백엔드 데이터가 디스크에 있는지다(DynamicUser의 StateDirectory: private 아래의 디렉터리나 바깥의
// 링크). 디렉터리 목록을 읽지 못하면(없음 제외) 있다고 본다(지울지 판단할 때 남은 데이터를 놓치지 않게).
func hasAddonData(sys System, id string) bool {
	for _, dir := range []string{addonPrivateDataDir, addonDataDir} {
		entries, err := sys.ReadDir(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return true
		case slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return e.Name() == id }):
			return true
		}
	}
	return false
}

// checkAddonAccount는 백엔드의 동적 사용자 이름(config.AddonUnitUserPrefix + id)으로 고정 계정·그룹이 있는지 NSS로
// 본다. systemd는 같은 이름의 계정이 이미 있으면 동적 사용자를 만들지 않고 그 계정으로 실행한다(그 계정의 파일에
// 닿는다). 도는 동안의 동적 사용자는 동적 범위(config.AddonDynamicUIDMin~Max)로 보이므로 범위 밖일 때만 거절한다(테스트
// VM에서 도는 백엔드가 있는 동기화가 이 확인에 걸리지 않았다).
func checkAddonAccount(sys System, id string) error {
	name := config.AddonUnitUserPrefix + id
	a, ok, err := sys.LookupUser(name)
	if err != nil {
		return fmt.Errorf("look up the account %s: %w", name, err)
	}
	if ok && !dynamicAddonID(a.UID) {
		return fmt.Errorf("an account named %s already exists (uid %d); remove it so that the backend runs as a dynamic user", name, a.UID)
	}
	gid, ok, err := sys.GroupID(name)
	if err != nil {
		return fmt.Errorf("look up the group %s: %w", name, err)
	}
	if ok && !dynamicAddonID(gid) {
		return fmt.Errorf("a group named %s already exists (gid %d); remove it so that the backend runs as a dynamic user", name, gid)
	}
	return nil
}

// dynamicAddonID는 uid·gid가 systemd의 동적 사용자 범위인지다.
func dynamicAddonID(id int) bool {
	return id >= config.AddonDynamicUIDMin && id <= config.AddonDynamicUIDMax
}

// boundedSyncError는 결과 파일에 남길 한 줄짜리 오류 문구다(addonSyncErrorMax 바이트까지, UTF-8 경계에서 자른다).
func boundedSyncError(err error) string {
	msg := strings.ToValidUTF8(strings.Join(strings.Fields(err.Error()), " "), "?")
	if len(msg) <= addonSyncErrorMax {
		return msg
	}
	msg = msg[:addonSyncErrorMax-3]
	for !utf8.ValidString(msg) {
		msg = msg[:len(msg)-1]
	}
	return msg + "..."
}
