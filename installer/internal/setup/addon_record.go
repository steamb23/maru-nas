package setup

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"time"

	"maru/backend/config"
)

// root 동기화의 자기 기록(config.AddonSyncRecordFile, root 0600). 동기화(--addon-sync)가 켜고 끈 결과와 남긴 백엔드
// 데이터를 적고, 설치기의 --service-install·--uninstall이 켜진 인스턴스를 다시 시작하거나 끌 때 읽는다. maru는 읽지
// 않는다. 기록을 쓰는 root 쪽은 모두 config.AddonSyncLockFile을 잡는다(lockAddonSync).
//
// 백엔드 데이터의 인스턴스 규칙(.claude/docs/addon-packages.md의 "root 동기화"): 같은 id의 데이터가 기록상 다른 설치
// 인스턴스의 것이면 켜기 전에 지운다. 데이터의 인스턴스는 이 기록으로만 증명하므로, 디스크에 데이터가 있는데 기록에 그 id가
// 없으면(기록을 잃었거나 --no-addon-backends로 끄면서 동기화 디렉터리를 지웠다) 다른 인스턴스의 데이터로 보고 그 id를 켜기
// 전에 지운다. 켜지 않는 id의 데이터는 지우지 않는다(purge 제외, addon_sync.go).

// addonRecordFormat은 addonSyncRecord.Format의 현재 값이다.
const addonRecordFormat = 1

// addonRecordMaxData는 기록에 남길 수 있는 "데이터만 남은 id"의 수다. 넘치면 동기화가 원하는 상태에 없는 id는 새로
// 적지 않고(기록에 없는 데이터는 위 규칙대로 다른 인스턴스의 것으로 보므로, 그 id를 다시 켤 때 지워질 뿐이다), 원하는
// 상태에 있는 id는 원하는 상태에 없는 항목 하나를 밀어내고 적는다(addonSyncer.retain).
const addonRecordMaxData = 128

// addonRecordMaxBytes는 기록 파일의 크기 상한이다.
const addonRecordMaxBytes = 64 << 10

// addonLockWait는 설치기가 동기화 잠금을 기다리는 시간이다. 돌고 있는 동기화가 시간 상한까지 갈 수 있으므로 그보다 길다.
const addonLockWait = addonSyncTimeout + 30*time.Second

// addonSyncRecord는 root 동기화의 자기 기록이다.
type addonSyncRecord struct {
	Format int `json:"format"`
	// Backends는 켜 둔 인스턴스다(id 순).
	Backends []addonRecordBackend `json:"backends"`
	// Data는 백엔드는 껐지만 데이터(StateDirectory)를 남긴 id와 그 데이터의 설치 인스턴스다(id 순). Backends의 id는
	// 여기에 두지 않는다(켜 둔 id의 데이터는 그 인스턴스의 것이다).
	Data []addonRecordData `json:"data"`
}

// addonRecordBackend는 켜 둔 인스턴스 하나다. 유닛은 <Unit>@<ID>.socket·.service다.
type addonRecordBackend struct {
	ID string `json:"id"`
	// Unit은 네트워크 단계의 유닛 템플릿 이름(config.AddonUnitPrefix)이다.
	Unit     string `json:"unit"`
	Rev      string `json:"rev"`
	Instance string `json:"instance"`
}

// addonRecordData는 데이터만 남은 id 하나다.
type addonRecordData struct {
	ID       string `json:"id"`
	Instance string `json:"instance"`
}

// emptyAddonRecord는 아무것도 켜지 않은 기록이다(기록 파일이 없을 때).
func emptyAddonRecord() addonSyncRecord {
	return addonSyncRecord{Format: addonRecordFormat, Backends: []addonRecordBackend{}, Data: []addonRecordData{}}
}

// validate는 기록의 값을 검사한다. id는 systemctl 인자로 가므로 root가 쓴 기록도 읽을 때마다 검사한다.
func (r addonSyncRecord) validate() error {
	if r.Format != addonRecordFormat {
		return fmt.Errorf("addon sync record: unsupported format %d", r.Format)
	}
	if len(r.Backends) > config.AddonBackendsMax || len(r.Data) > addonRecordMaxData {
		return errors.New("addon sync record: too many entries")
	}
	seen := map[string]bool{}
	for _, b := range r.Backends {
		switch {
		case !config.ValidAddonID(b.ID) || seen[b.ID]:
			return fmt.Errorf("addon sync record: invalid or duplicate id %q", b.ID)
		case !validAddonUnitPrefix(b.Unit):
			return fmt.Errorf("addon sync record: %s: invalid unit %q", b.ID, b.Unit)
		case !config.ValidAddonRev(b.Rev):
			return fmt.Errorf("addon sync record: %s: invalid rev %q", b.ID, b.Rev)
		case !config.ValidAddonInstance(b.Instance):
			return fmt.Errorf("addon sync record: %s: invalid instance", b.ID)
		}
		seen[b.ID] = true
	}
	for _, d := range r.Data {
		switch {
		case !config.ValidAddonID(d.ID) || seen[d.ID]:
			return fmt.Errorf("addon sync record: invalid or duplicate data id %q", d.ID)
		case !config.ValidAddonInstance(d.Instance):
			return fmt.Errorf("addon sync record: %s: invalid data instance", d.ID)
		}
		seen[d.ID] = true
	}
	return nil
}

// parseAddonSyncRecord는 기록 파일 내용을 엄격하게 읽는다(모르는 필드, 뒤에 남은 값, 크기, 형식을 거부한다).
func parseAddonSyncRecord(data []byte) (addonSyncRecord, error) {
	if len(data) > addonRecordMaxBytes {
		return addonSyncRecord{}, fmt.Errorf("addon sync record: larger than %d bytes", addonRecordMaxBytes)
	}
	var r addonSyncRecord
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return addonSyncRecord{}, fmt.Errorf("addon sync record: %w", err)
	}
	// More()는 뒤에 붙은 ]·}를 놓치므로 다음 토큰이 끝(io.EOF)인지 본다.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return addonSyncRecord{}, errors.New("addon sync record: trailing data")
	}
	if r.Backends == nil {
		r.Backends = []addonRecordBackend{}
	}
	if r.Data == nil {
		r.Data = []addonRecordData{}
	}
	if err := r.validate(); err != nil {
		return addonSyncRecord{}, err
	}
	return r, nil
}

// readAddonSyncRecord는 기록을 읽는다. 파일이 없으면 빈 기록이다. 일반 파일이 아니거나 상한을 넘거나 형식이 틀리면
// 오류다(부르는 쪽은 빈 기록으로 보고 경고하거나 멈춘다).
func readAddonSyncRecord(sys System) (addonSyncRecord, error) {
	fi, err := sys.Stat(config.AddonSyncRecordFile)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyAddonRecord(), nil
	}
	if err != nil {
		return addonSyncRecord{}, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > addonRecordMaxBytes {
		return addonSyncRecord{}, fmt.Errorf("addon sync record: %s is not a regular file within %d bytes", config.AddonSyncRecordFile, addonRecordMaxBytes)
	}
	data, err := sys.ReadFile(config.AddonSyncRecordFile)
	if err != nil {
		return addonSyncRecord{}, err
	}
	return parseAddonSyncRecord(data)
}

// writeAddonSyncRecord는 기록을 id 순으로 정리하고 검사한 뒤 root 0600으로 원자적으로 쓴다. 동기화 잠금을 쥔 채로 부른다.
func writeAddonSyncRecord(sys System, r addonSyncRecord) error {
	r.Format = addonRecordFormat
	r.Backends = slices.Clone(r.Backends)
	r.Data = slices.Clone(r.Data)
	if r.Backends == nil {
		r.Backends = []addonRecordBackend{}
	}
	if r.Data == nil {
		r.Data = []addonRecordData{}
	}
	slices.SortFunc(r.Backends, func(a, b addonRecordBackend) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(r.Data, func(a, b addonRecordData) int { return cmp.Compare(a.ID, b.ID) })
	if err := r.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return sys.WriteFile(config.AddonSyncRecordFile, append(data, '\n'), 0o600)
}

// lockAddonSync는 동기화 잠금(config.AddonSyncLockFile)을 잡는다. 동기화 디렉터리가 있어야 한다(없는 디렉터리를 만들지
// 않는다. 설치기는 디렉터리가 있는지로 켜졌는지 읽는다). 바로 잡지 못하면(돌고 있는 동기화가 있으면) 기다린다고 알린 뒤
// 기다린다.
func lockAddonSync(c *Context) (func(), error) {
	unlock, err := c.Sys.Lock(config.AddonSyncLockFile, 0)
	if err != nil && dirExists(c.Sys, config.AddonSyncDir) {
		c.Info(L("Waiting for "+config.AddonSyncName+".service to finish (up to "+addonLockWait.String()+")",
			config.AddonSyncName+".service가 끝나기를 기다리는 중(최대 "+addonLockWait.String()+")"))
		unlock, err = c.Sys.Lock(config.AddonSyncLockFile, addonLockWait)
	}
	if err != nil {
		return nil, fail(c.L("cannot lock the addon backend sync ("+err.Error()+"); is "+config.AddonSyncName+".service stuck? See: systemctl status "+config.AddonSyncName+".service",
			"애드온 백엔드 동기화를 잠글 수 없습니다("+err.Error()+"). "+config.AddonSyncName+".service가 멈춰 있지 않은지 보세요: systemctl status "+config.AddonSyncName+".service"))
	}
	return unlock, nil
}
