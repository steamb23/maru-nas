package addonpkg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"maru/backend/config"
)

// 운영 백엔드의 root 동기화(.claude/docs/addon-packages.md의 "root 동기화"). maru는 원하는 상태
// (<state>/config.AddonBackendsFile)와 동기화 요청(<state>/config.AddonSyncRequest)을 쓰고, root 동기화(설치기
// --addon-sync)가 쓴 결과(config.AddonSyncStatusFile)를 읽기만 한다. maru는 systemctl을 부르지 않는다.

// SyncStall이 지나도 동기화 요청이 남아 있으면 root 동기화가 가져가지 않는 것으로 본다(경로 유닛은 바로 서비스를
// 시작하고, 동기화는 가장 먼저 요청을 지운다). 요청을 가져간 뒤 결과를 쓰기까지도 이 시간 안에 끝날 것으로 본다.
const SyncStall = time.Minute

// syncHistory는 지금 파일 말고 기억하는 지난 원하는 상태 파일의 수다. 결과가 지난 파일을 맞춘 것이어도 그 파일에서
// 바뀌지 않은 백엔드는 켜진 것으로 볼 수 있게 한다.
const syncHistory = 8

// 백엔드 패키지 하나의 동기화 상태(SyncState.Backend).
const (
	// BackendRunning은 마지막 결과가 지금 rev·설치 인스턴스의 유닛을 켰다는 뜻이다(소켓 활성화라 첫 요청 때 시작한다).
	BackendRunning = "running"
	// BackendFailed는 마지막 결과가 지금 rev·설치 인스턴스의 유닛을 켜지 못했거나, maru가 그 백엔드를 원하는 상태에
	// 넣지 못했다는 뜻이다(capability backend_unavailable).
	BackendFailed = "failed"
	// BackendPending은 아직 지금 rev·설치 인스턴스가 반영된 것을 확인하지 못했다는 뜻이다. 그동안 maru는 그 백엔드로
	// 요청을 보내지 않는다(이전 설치·rev의 유닛이 아직 그 소켓을 쥐고 있을 수 있다).
	BackendPending = "pending"
)

// ErrPurgeFull은 지울 백엔드 데이터 목록이 원하는 상태에 들어갈 수 있는 만큼(config.AddonBackendsMax) 차 있다는 뜻이다.
var ErrPurgeFull = errors.New("too many backend data removals are waiting for the sync")

// SyncPaths는 root 동기화 파일의 위치다.
type SyncPaths struct {
	// Desired는 원하는 상태 파일(<state>/config.AddonBackendsFile)이다. maru가 쓴다.
	Desired string
	// Request는 동기화 요청 파일(<state>/config.AddonSyncRequest)이다. maru가 쓰고 동기화가 지운다.
	Request string
	// Status는 동기화 결과 파일(config.AddonSyncStatusFile)이다. root가 쓰고 maru는 읽기만 한다.
	Status string
}

// InstalledSyncPaths는 설치본의 경로다. stateDir는 maru의 상태 디렉터리(config.InstalledStateDir)다. 경로 유닛은
// 설치한 상태 디렉터리의 요청만 지켜본다.
func InstalledSyncPaths(stateDir string) SyncPaths {
	return SyncPaths{
		Desired: filepath.Join(stateDir, config.AddonBackendsFile),
		Request: filepath.Join(stateDir, config.AddonSyncRequest),
		Status:  config.AddonSyncStatusFile,
	}
}

// Sync는 운영 백엔드의 원하는 상태를 쓰고 동기화 결과를 읽는다. 레지스트리가 바뀔 때마다(Manager.OnChange) Update를
// 부른다. 여러 고루틴에서 써도 된다.
type Sync struct {
	paths SyncPaths
	now   func() time.Time
	// active는 지금 레지스트리에 들어간 패키지다(Manager.Active, 잠금 없이 읽는다). 원하는 상태를 만들 때마다 mu를 쥔
	// 채로 부르므로, 레지스트리가 바뀐 뒤의 쓰기가 그 전에 읽은 목록으로 덮어쓰이지 않는다.
	active func() []*Installed

	// mu는 원하는 상태와 요청을 쓰는 동안 잡는다. Manager.OnChange(Manager의 잠금)에서 불리므로 Manager의 잠그는
	// 메서드를 부르지 않는다. 결과 캐시(statusMu)는 mu를 쥔 채로 잡을 수 있고, 그 반대로는 잡지 않는다.
	mu sync.Mutex
	// purge는 지울 백엔드 데이터의 id 전체다. 파일에는 앞의 config.AddonBackendsMax개만 들어간다. 시작할 때 지난 파일에서
	// 읽어 이어 간다.
	purge []string
	// written은 지금 디스크에 있는 원하는 상태 파일의 내용이다(없으면 nil).
	written []byte
	// writeErr는 마지막으로 파일을 쓰지 못한 이유다(쓰면 지운다).
	writeErr error
	// files는 maru가 쓴 원하는 상태 파일의 백엔드 항목이다(digest → id → rev·설치 인스턴스). 결과에는 설치 인스턴스가
	// 없으므로 결과의 digest로 그 파일을 찾아 켜진 유닛이 어느 설치의 것인지 안다. order는 오래된 것부터의 digest다.
	files map[string]map[string]syncEntry
	order []string
	// leftOut은 원하는 상태에 넣지 못한 백엔드 패키지와 그 이유다(검사 실패, 개수 상한). 바꿀 때 통째로 바꾼다.
	leftOut map[string]string
	// requestedAt은 마지막으로 요청을 쓴 시각이고 requestedStatus는 그때의 결과 파일이다. 동기화는 요청을 먼저 지우고
	// 일하므로, 요청 파일이 없어도 SyncStall 안이고 결과 파일이 그대로면 아직 결과를 기다리는 중이다.
	requestedAt     time.Time
	requestedStatus fileKey

	// statusMu는 결과 파일 캐시를 지킨다. capability가 요청마다 읽으므로 mu와 따로 둔다.
	statusMu sync.Mutex
	status   statusCache
}

// syncEntry는 원하는 상태 파일의 백엔드 항목 하나다.
type syncEntry struct{ rev, instance string }

// fileKey는 파일 하나를 가리키는 값이다(inode·크기·수정 시각). 없으면 ok가 false다.
type fileKey struct {
	ok   bool
	ino  uint64
	size int64
	mod  time.Time
}

func (k fileKey) same(o fileKey) bool {
	return k.ok == o.ok && k.ino == o.ino && k.size == o.size && k.mod.Equal(o.mod)
}

// statusCache는 결과 파일을 한 번 읽은 값이다. 파일의 inode·크기·수정 시각이 같으면 다시 읽지 않는다.
type statusCache struct {
	valid bool
	key   fileKey
	st    *config.AddonSyncStatus
	err   error
}

// NewSync는 Sync를 만든다. active는 지금 레지스트리의 패키지다(Manager.Active). 지난 원하는 상태 파일이 있으면 그
// purge 목록을 이어 가고 그 파일의 항목을 기억한다(읽지 못하면 로그만 남기고 빈 목록에서 시작한다). now가 nil이면
// time.Now다.
func NewSync(paths SyncPaths, now func() time.Time, active func() []*Installed) *Sync {
	if now == nil {
		now = time.Now
	}
	s := &Sync{paths: paths, now: now, active: active, files: map[string]map[string]syncEntry{}, leftOut: map[string]string{}}
	data, err := readNoFollow(paths.Desired, config.AddonBackendsMaxBytes)
	switch {
	case err == nil:
		s.written = data
		if b, err := config.ParseAddonBackends(data); err == nil {
			s.purge = b.Purge
			s.rememberLocked(data, b)
		} else {
			log.Printf("addon backends: %s: %v; starting with an empty purge list", paths.Desired, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		log.Printf("addon backends: %s: %v", paths.Desired, err)
	}
	return s
}

// Update는 지금 레지스트리로 원하는 상태를 다시 만들고, 내용이 바뀌었으면 쓴다. 결과가 지금 원하는 상태를 반영하지
// 않았고 대기 중인 요청이 없으면 동기화를 요청한다.
func (s *Sync) Update() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.updateLocked("", false)
	return err
}

// Purge는 id의 백엔드 데이터를 지우게 한다(패키지를 제거하면서 데이터 지우기를 고른 경우, 제거가 끝난 뒤). id가 다시
// 레지스트리에 있으면(다시 설치) 넣지 않는다. 다시 설치한 패키지는 설치 인스턴스가 바뀌어 동기화가 켜기 전에 데이터를
// 지운다. 목록이 파일에 다 들어가지 않으면 나머지는 앞의 것이 확인될 때까지 메모리에서 기다린다(PurgeFull로 미리 막는다).
func (s *Sync) Purge(id string) error {
	if !config.ValidAddonID(id) {
		return fmt.Errorf("invalid addon id %q", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.updateLocked(id, false)
	return err
}

// PurgeFull은 id를 더하면 지울 데이터 목록이 원하는 상태 파일에 다 들어가지 않는지다(이미 목록에 있으면 false).
// 제거하기 전에 본다.
func (s *Sync) PurgeFull(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, _ := s.readStatus()
	on := map[string]bool{}
	for _, p := range s.activeList() {
		if p.ID != id && !p.Dev && p.Manifest != nil && p.Manifest.HasBackend() {
			on[p.ID] = true
		}
	}
	list := s.purgeList(on, st, "")
	return !slices.Contains(list, id) && len(list) >= config.AddonBackendsMax
}

// Request는 원하는 상태를 맞춘 뒤 결과와 상관없이 동기화를 요청한다(관리자 페이지의 "다시 동기화", 실패한 백엔드를
// 다시 켜 본다). 아직 가져가지 않은 요청이 있으면 새로 쓰지 않고, 멈춘 요청(SyncStall)은 덮어쓴다. 요청을 썼으면 true다.
func (s *Sync) Request() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateLocked("", true)
}

func (s *Sync) activeList() []*Installed {
	if s.active == nil {
		return nil
	}
	return s.active()
}

// updateLocked는 원하는 상태를 쓰고 필요하면 요청을 쓴다. purgeID가 있으면 purge 목록에 더한다(지난 결과가 지운 것을
// 확인한 id라도 다시 넣는다). force면 결과가 같아도 요청한다.
func (s *Sync) updateLocked(purgeID string, force bool) (bool, error) {
	st, _ := s.readStatus()
	want, leftOut := s.desired(s.activeList(), st, purgeID)
	// 쓰지 못해도 purge 목록은 남겨 다음 변경 때 다시 쓴다. 파일에는 앞의 상한만큼만 넣고 나머지는 메모리에서 기다린다.
	s.purge, s.leftOut = want.Purge, leftOut
	file := want
	if n := len(file.Purge); n > config.AddonBackendsMax {
		log.Printf("addon backends: %d backend data directories to purge; the file carries %d, the rest wait until those are confirmed",
			n, config.AddonBackendsMax)
		file.Purge = file.Purge[:config.AddonBackendsMax]
	}
	data, err := marshalBackends(file)
	if err != nil {
		s.writeErr = err
		return false, err
	}
	if !bytes.Equal(data, s.written) {
		if err := writeFileAtomic(s.paths.Desired, data, 0o600); err != nil {
			s.writeErr = fmt.Errorf("write %s: %w", filepath.Base(s.paths.Desired), err)
			return false, s.writeErr
		}
		s.written = data
		s.rememberLocked(data, file)
	}
	pending, stalled := s.pendingLocked()
	applied := st != nil && st.Error == "" && st.Digest == config.AddonBackendsDigest(data)
	// 아직 가져가지 않은 요청이 있으면 동기화가 지금 파일을 읽는다(멈춘 요청은 강제로 다시 요청할 때만 덮어쓴다).
	// 결과가 지금 파일을 맞췄으면 강제할 때만 요청한다.
	if (pending && !(force && stalled)) || (applied && !force) {
		s.writeErr = nil
		return false, nil
	}
	if err := writeFileAtomic(s.paths.Request, []byte(config.AddonSyncRequestBody), 0o600); err != nil {
		s.writeErr = fmt.Errorf("write %s: %w", filepath.Base(s.paths.Request), err)
		return false, s.writeErr
	}
	s.requestedAt, s.requestedStatus = s.now(), s.statusKey()
	s.writeErr = nil
	return true, nil
}

// rememberLocked는 maru가 쓴(또는 시작할 때 디스크에 있던) 원하는 상태 파일의 항목을 기억한다. 지금 파일과 그 전
// syncHistory개만 둔다.
func (s *Sync) rememberLocked(data []byte, b config.AddonBackends) {
	digest := config.AddonBackendsDigest(data)
	entries := make(map[string]syncEntry, len(b.Backends))
	for _, e := range b.Backends {
		entries[e.ID] = syncEntry{rev: e.Rev, instance: e.Instance}
	}
	s.files[digest] = entries
	s.order = append(slices.DeleteFunc(s.order, func(d string) bool { return d == digest }), digest)
	for len(s.order) > syncHistory+1 {
		delete(s.files, s.order[0])
		s.order = s.order[1:]
	}
}

// desired는 원하는 상태와, 넣지 못한 백엔드 패키지(id → 이유)다. 백엔드는 레지스트리에 들어간 설치본(개발본 제외)의
// 백엔드 패키지를 id 순으로 둔다(설치·켜기가 config.AddonBackendsMax개를 넘지 않게 막는다). purge는 purgeList다.
func (s *Sync) desired(active []*Installed, st *config.AddonSyncStatus, purgeID string) (config.AddonBackends, map[string]string) {
	want := config.AddonBackends{Format: config.AddonBackendsFormat, Backends: []config.AddonBackend{}, Purge: []string{}}
	leftOut := map[string]string{}
	on := map[string]bool{}
	for _, p := range active {
		if p.Dev || p.Manifest == nil || !p.Manifest.HasBackend() {
			continue
		}
		// 넣지 못한 id도 설치되어 있으므로 on에 둬 purge에 넣지 않는다(root는 backends에 없는 purge id의 데이터를 지운다).
		on[p.ID] = true
		network := slices.Clone(p.Manifest.Backend.Network)
		if network == nil {
			network = []string{}
		}
		slices.Sort(network)
		e := config.AddonBackend{ID: p.ID, Version: p.Manifest.Version, Rev: p.Rev, Instance: p.InstallUID, Network: network}
		one := config.AddonBackends{Format: config.AddonBackendsFormat, Backends: []config.AddonBackend{e}, Purge: []string{}}
		if err := one.Validate(); err != nil {
			// 설치할 때 모두 검사했으므로 일어나지 않아야 한다. root가 파일 전체를 거부하지 않게 이 항목만 뺀다.
			log.Printf("addon %s: backend left out of the sync: %v", p.ID, err)
			leftOut[p.ID] = "left out of the backend sync: " + err.Error()
			continue
		}
		want.Backends = append(want.Backends, e)
	}
	slices.SortFunc(want.Backends, func(a, b config.AddonBackend) int { return strings.Compare(a.ID, b.ID) })
	if n := len(want.Backends); n > config.AddonBackendsMax {
		// 설치·켜기가 막으므로 손으로 고친 설치 기록에서만 일어난다.
		log.Printf("addon backends: %d backend packages; only the first %d (by id) are kept running", n, config.AddonBackendsMax)
		for _, e := range want.Backends[config.AddonBackendsMax:] {
			leftOut[e.ID] = fmt.Sprintf("more than %d backend packages are enabled", config.AddonBackendsMax)
		}
		want.Backends = want.Backends[:config.AddonBackendsMax]
	}
	want.Purge = s.purgeList(on, st, purgeID)
	return want, leftOut
}

// purgeList는 지울 데이터 목록이다. 지난 목록에서 지난 결과가 지운 것을 확인한 id(그 결과가 지금 디스크의 파일을 맞춘
// 경우만)와 다시 켜진 id(on)를 빼고 purgeID를 더한다. 상한으로 자르지 않는다.
func (s *Sync) purgeList(on map[string]bool, st *config.AddonSyncStatus, purgeID string) []string {
	confirmed := map[string]bool{}
	if st != nil && s.written != nil && st.Error == "" && st.Digest == config.AddonBackendsDigest(s.written) {
		for _, id := range st.Purged {
			confirmed[id] = true
		}
	}
	list := []string{}
	for _, id := range s.purge {
		if !confirmed[id] && !on[id] && config.ValidAddonID(id) {
			list = append(list, id)
		}
	}
	if purgeID != "" && !on[purgeID] {
		list = append(list, purgeID)
	}
	slices.Sort(list)
	return slices.Compact(list)
}

// marshalBackends는 원하는 상태의 고정된 꼴이다(내용이 같으면 바이트도 같다). root가 같은 검사를 다시 하므로 쓰기 전에
// 검사한다.
func marshalBackends(b config.AddonBackends) ([]byte, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// pendingLocked는 요청 파일이 남아 있는지와, 그것이 SyncStall보다 오래되었는지다.
func (s *Sync) pendingLocked() (pending, stalled bool) {
	fi, err := os.Lstat(s.paths.Request)
	if err != nil {
		return false, false
	}
	return true, s.now().Sub(fi.ModTime()) > SyncStall
}

// waitingLocked는 maru가 쓴 요청을 동기화가 가져갔지만 아직 새 결과를 쓰지 않았는지다(SyncStall 안이고 결과 파일이
// 요청을 쓴 때 그대로).
func (s *Sync) waitingLocked() bool {
	return !s.requestedAt.IsZero() && s.now().Sub(s.requestedAt) < SyncStall && s.statusKey().same(s.requestedStatus)
}

// statusKey는 지금 결과 파일을 가리키는 값이다(심볼릭 링크는 따라가지 않는다).
func (s *Sync) statusKey() fileKey {
	fi, err := os.Lstat(s.paths.Status)
	if err != nil {
		return fileKey{}
	}
	return keyOf(fi)
}

func keyOf(fi fs.FileInfo) fileKey {
	k := fileKey{ok: true, size: fi.Size(), mod: fi.ModTime()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		k.ino = sys.Ino
	}
	return k
}

// SyncState는 한 번 읽은 동기화 상태다.
type SyncState struct {
	// Status는 마지막 동기화 결과다. 없거나 읽을 수 없으면 nil이고 StatusErr가 이유다.
	Status    *config.AddonSyncStatus
	StatusErr error
	// WriteErr는 maru가 원하는 상태나 요청을 쓰지 못한 이유다.
	WriteErr error
	// Pending은 요청을 동기화가 아직 가져가지 않았거나, 가져갔지만 아직 결과를 쓰지 않았다는 뜻이다(SyncStall 안).
	// Stalled는 요청 파일이 SyncStall보다 오래 남아 있다는 뜻이다.
	Pending, Stalled bool
	// Digest는 지금 디스크에 있는 원하는 상태 파일의 digest다. 아직 쓰지 않았으면 비어 있다.
	Digest string

	// applied는 결과가 맞춘 원하는 상태 파일의 항목이다(maru가 그 파일을 기억할 때만, 아니면 nil).
	applied map[string]syncEntry
	leftOut map[string]string
}

// Applied는 마지막 결과가 지금 원하는 상태를 맞췄는지다(동기화 전체가 실패한 결과는 아니다).
func (s SyncState) Applied() bool {
	return s.Status != nil && s.Status.Error == "" && s.Digest != "" && s.Status.Digest == s.Digest
}

// Synced는 동기화가 한 번이라도 결과를 냈는지다(설치기가 써 둔 빈 결과는 digest가 없다).
func (s SyncState) Synced() bool { return s.Status != nil && s.Status.Digest != "" }

// Backend는 백엔드 패키지 하나(지금 rev와 설치 인스턴스)의 상태와, 실패했으면 그 이유다.
//   - maru가 원하는 상태에 넣지 못했으면 failed다.
//   - 결과가 맞춘 파일(digest)을 maru가 기억하고, 그 파일의 항목이 같은 rev·설치 인스턴스이고, 결과가 그 id를 같은 rev로
//     켰으면 running, 켜지 못했으면 failed다. 결과가 지난 파일의 것이어도 그 파일에서 바뀌지 않은 백엔드는 따른다.
//   - 그 밖(결과 없음·읽지 못함, 동기화 전체 실패, 모르는 파일, 다른 rev·설치 인스턴스, 결과에 없음)은 pending이다.
func (s SyncState) Backend(id, rev, instance string) (state, reason string) {
	if why, ok := s.leftOut[id]; ok {
		return BackendFailed, why
	}
	if s.Status == nil || s.Status.Error != "" {
		return BackendPending, ""
	}
	if e, ok := s.applied[id]; !ok || e.rev != rev || e.instance != instance {
		return BackendPending, ""
	}
	for _, b := range s.Status.Backends {
		switch {
		case b.ID != id:
			continue
		case b.Rev == rev && b.State == config.AddonBackendEnabled:
			return BackendRunning, ""
		case b.Rev == rev && b.State == config.AddonBackendFailed:
			return BackendFailed, b.Error
		}
		return BackendPending, ""
	}
	return BackendPending, ""
}

// State는 지금 동기화 상태다.
func (s *Sync) State() SyncState {
	st, err := s.readStatus()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := SyncState{Status: st, StatusErr: err, WriteErr: s.writeErr, leftOut: s.leftOut}
	pending, stalled := s.pendingLocked()
	out.Pending, out.Stalled = pending || s.waitingLocked(), stalled
	if s.written != nil {
		out.Digest = config.AddonBackendsDigest(s.written)
	}
	if st != nil {
		out.applied = s.files[st.Digest]
	}
	return out
}

// Backend는 백엔드 패키지 하나의 지금 상태다(SyncState.Backend). 데이터·조작 요청과 capability가 요청마다 부른다.
func (s *Sync) Backend(id, rev, instance string) (state, reason string) {
	st, _ := s.readStatus()
	s.mu.Lock()
	view := SyncState{Status: st, leftOut: s.leftOut}
	if st != nil {
		view.applied = s.files[st.Digest]
	}
	s.mu.Unlock()
	return view.Backend(id, rev, instance)
}

// readStatus는 결과 파일을 심볼릭 링크를 따라가지 않고 크기를 묶어 읽는다. 파일이 바뀌지 않았으면 지난 값(읽지 못한
// 이유 포함)을 쓴다. 읽지 못한 이유는 바뀔 때 한 번 로그에 남긴다.
func (s *Sync) readStatus() (*config.AddonSyncStatus, error) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	prev := s.status.err
	st, err := s.readStatusLocked()
	if err != nil && (prev == nil || prev.Error() != err.Error()) {
		log.Printf("addon backends: sync status: %v", err)
	}
	return st, err
}

// readStatusLocked는 결과 파일을 읽고 캐시(s.status)를 바꾼다.
func (s *Sync) readStatusLocked() (*config.AddonSyncStatus, error) {
	fail := func(err error) (*config.AddonSyncStatus, error) {
		s.status = statusCache{err: err}
		return nil, err
	}
	f, err := os.OpenFile(s.paths.Status, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fail(err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	if !fi.Mode().IsRegular() {
		return fail(fmt.Errorf("%s is not a regular file", s.paths.Status))
	}
	key := keyOf(fi)
	if c := s.status; c.valid && c.key.same(key) {
		return c.st, c.err
	}
	if fi.Size() > config.AddonSyncStatusMaxBytes {
		return fail(fmt.Errorf("%s is larger than %d bytes", s.paths.Status, config.AddonSyncStatusMaxBytes))
	}
	data, err := io.ReadAll(io.LimitReader(f, config.AddonSyncStatusMaxBytes+1))
	if err != nil {
		return fail(err)
	}
	c := statusCache{valid: true, key: key}
	if parsed, err := config.ParseAddonSyncStatus(data); err != nil {
		c.err = err
	} else {
		c.st = &parsed
	}
	s.status = c
	return c.st, c.err
}

// readNoFollow는 일반 파일을 심볼릭 링크를 따라가지 않고 max 바이트까지 읽는다.
func readNoFollow(path string, max int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if fi.Size() > max {
		return nil, fmt.Errorf("larger than %d bytes", max)
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}
