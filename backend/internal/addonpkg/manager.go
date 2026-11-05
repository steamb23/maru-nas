package addonpkg

import (
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
	"sync/atomic"
	"time"

	"maru/backend/config"
	"maru/backend/internal/addonpkg/format"
	"maru/backend/internal/addons"
)

// 관리자 목록의 상태(Status.State).
const (
	// StateActive는 켜져 있고 레지스트리에 들어간 패키지다.
	StateActive = "active"
	// StateDisabled는 관리자가 끈 패키지다.
	StateDisabled = "disabled"
	// StateRejected는 켜져 있지만 레지스트리가 받지 않은 패키지다(충돌, 금지 표 등). Reason이 거부 코드다.
	StateRejected = "rejected"
	// StateInvalid는 파일을 읽을 수 없거나 검사에 걸린 패키지다. Reason이 addonpkg 이유 코드다.
	StateInvalid = "invalid"
)

// PluginTable은 패키지가 밝힌 플러그인 표를 받는 감지기다(*addons.Detector).
type PluginTable interface {
	SetExtra(map[string]string)
}

// Options는 NewManager의 설정이다.
type Options struct {
	// Store가 nil이면(상태 디렉터리 없음) 설치할 수 없고 개발 패키지만 쓴다.
	Store *Store
	// DevDirs는 개발 패키지 디렉터리다(MARU_DEV_ADDONS). 설치·끄기·제거를 할 수 없다.
	DevDirs []string
	// Arch는 실행 중인 아키텍처(runtime.GOARCH)다.
	Arch string
	// Plugins가 있으면 받은 패키지가 밝힌 플러그인 표를 넘긴다.
	Plugins PluginTable
	// BackendsEnabled는 이 설치에서 패키지 백엔드가 켜져 있는지다. 꺼져 있어도 백엔드 패키지를 설치할 수 있고,
	// 검토 대기에 경고(WarnBackendsDisabled)를 붙인다. 서버의 Config.AddonHost와 같은 값이어야 한다.
	BackendsEnabled bool
	Now             func() time.Time
}

// Installed는 레지스트리에 들어간 패키지 하나다.
type Installed struct {
	ID  string
	Rev string
	Dev bool
	// Dir는 패키지 파일이 있는 디렉터리다(<state>/addons/<id>/<rev>, 개발 패키지는 그 디렉터리).
	Dir       string
	Manifest  *format.Manifest
	Languages []string
	Summary   Summary
	// InstallUID는 사용자별 저장값을 묶는 값이다. 개발 패키지는 "dev"다.
	InstallUID string
}

// Status는 관리자 목록의 패키지 하나다.
type Status struct {
	ID       string
	Version  string
	Rev      string
	Dev      bool
	Enabled  bool
	State    string
	Reason   string
	Detail   string
	Record   *Record
	Manifest *format.Manifest
	Summary  *Summary
	Dir      string
}

// state는 한 번 불러온 결과다. 바꾸지 않고 통째로 교체한다.
type state struct {
	reg      *addons.Registry
	active   map[string]*Installed
	list     []Status
	indexErr error
	devRevs  map[string]string
}

// Manager는 설치한 패키지와 개발 패키지를 불러와 내장 레지스트리에 더한다. 읽기(Registry, Get, List)는 잠금 없이
// 지금 상태를 쓰고, 바꾸기(설치, 켜기·끄기, 제거, 다시 불러오기)는 하나씩 한다.
type Manager struct {
	base *addons.Registry
	opt  Options

	mu    sync.Mutex
	state atomic.Pointer[state]
	// onChange는 레지스트리를 바꾼 뒤 부르는 함수다(OnChange, m.mu를 쥔 채로 부른다).
	onChange func()
}

// NewManager는 패키지를 불러온 Manager를 만든다. 불러오다 실패한 패키지는 목록에 이유와 함께 남고
// 내장 애드온은 그대로다.
func NewManager(base *addons.Registry, opt Options) *Manager {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	m := &Manager{base: base, opt: opt}
	if opt.Store != nil {
		if err := opt.Store.Init(); err != nil {
			log.Printf("addon packages: %v", err)
		}
	}
	m.mu.Lock()
	m.reloadLocked()
	m.mu.Unlock()
	return m
}

// OnChange는 레지스트리를 바꿀 때마다(설치, 켜기·끄기, 제거, 다시 불러오기) fn을 부르게 한다. 등록할 때도 한 번
// 부른다. fn은 Manager의 잠금을 쥔 채로 불리므로 차례대로 불리고, 잠금 없이 읽는 메서드(Registry, Get, Active)만
// 써야 한다(잠그는 메서드를 부르면 멈춘다). 서버가 백엔드 패키지마다 호스트 소켓을 맞추는 데 쓴다.
func (m *Manager) OnChange(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
	if fn != nil {
		fn()
	}
}

// Registry는 내장 애드온과 받은 패키지를 모두 담은 지금 레지스트리다.
func (m *Manager) Registry() *addons.Registry { return m.state.Load().reg }

// Get은 레지스트리에 들어간 패키지 id를 찾는다.
func (m *Manager) Get(id string) (*Installed, bool) {
	p, ok := m.state.Load().active[id]
	return p, ok
}

// Active는 레지스트리에 들어간 패키지 목록이다(등록 순서).
func (m *Manager) Active() []*Installed {
	st := m.state.Load()
	var out []*Installed
	for _, a := range st.reg.All() {
		if p, ok := st.active[a.Manifest.ID]; ok {
			out = append(out, p)
		}
	}
	return out
}

// List는 관리자 목록이다. IndexError가 있으면 설치 기록을 읽지 못한 것이다(개발 패키지만 보인다).
func (m *Manager) List() ([]Status, error) {
	st := m.state.Load()
	return slices.Clone(st.list), st.indexErr
}

// CanInstall은 설치할 수 있는지(상태 디렉터리가 있는지)다.
func (m *Manager) CanInstall() bool { return m.opt.Store != nil }

// HasDev는 개발 패키지 디렉터리가 있는지다.
func (m *Manager) HasDev() bool { return len(m.opt.DevDirs) > 0 }

// Reload는 설치 기록과 개발 디렉터리를 다시 읽는다.
func (m *Manager) Reload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadLocked()
}

// RescanDev는 개발 패키지 파일이 바뀌었으면 다시 불러온다. capabilities를 받을 때 부른다.
func (m *Manager) RescanDev() {
	if len(m.opt.DevDirs) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.state.Load()
	for _, dir := range m.opt.DevDirs {
		rev := ""
		if p, err := ReadDir(dir, DefaultLimits, m.opt.Arch); err == nil {
			rev = p.DevRev()
		}
		if st.devRevs[dir] != rev {
			m.reloadLocked()
			return
		}
	}
}

// candidate는 레지스트리에 더할 후보 하나다.
type candidate struct {
	inst   *Installed
	status *Status
}

func (m *Manager) reloadLocked() {
	st := &state{active: map[string]*Installed{}, devRevs: map[string]string{}}
	var cands []candidate

	// 개발 패키지를 먼저 둔다. 같은 id의 설치본이 있으면 개발본이 이긴다(개발자가 고치는 중이다).
	for _, dir := range m.opt.DevDirs {
		s := Status{Dev: true, Enabled: true, Dir: dir, ID: filepath.Base(dir)}
		p, err := ReadDir(dir, DefaultLimits, m.opt.Arch)
		if err != nil {
			s.State, s.Reason, s.Detail = StateInvalid, reasonOf(err), err.Error()
			st.list = append(st.list, s)
			st.devRevs[dir] = ""
			continue
		}
		st.devRevs[dir] = p.DevRev()
		sum := Summarize(p.Manifest)
		s.ID, s.Version, s.Rev, s.Manifest, s.Summary = p.Manifest.ID, p.Manifest.Version, p.DevRev(), p.Manifest, &sum
		cands = append(cands, candidate{
			inst:   &Installed{ID: p.Manifest.ID, Rev: p.DevRev(), Dev: true, Dir: dir, Manifest: p.Manifest, Languages: p.Languages, Summary: sum, InstallUID: "dev"},
			status: &s,
		})
	}

	if store := m.opt.Store; store != nil {
		records, err := store.Load()
		if err != nil {
			log.Printf("addon packages: %v", err)
			st.indexErr = err
		}
		slices.SortStableFunc(records, func(a, b Record) int {
			if c := a.InstalledAt.Compare(b.InstalledAt); c != 0 {
				return c
			}
			return strings.Compare(a.ID, b.ID)
		})
		for _, r := range records {
			rec := r
			s := Status{ID: r.ID, Version: r.Version, Rev: r.Rev, Enabled: r.Enabled, Record: &rec, Dir: store.Path(r.ID, r.Rev)}
			mf, langs, err := loadInstalled(s.Dir)
			if err == nil && mf.ID != r.ID {
				err = fail(ReasonManifest, "manifest id %q does not match the record %q", mf.ID, r.ID)
			}
			if err != nil {
				s.State, s.Reason, s.Detail = StateInvalid, reasonOf(err), err.Error()
				st.list = append(st.list, s)
				continue
			}
			sum := Summarize(mf)
			s.Manifest, s.Summary = mf, &sum
			if !r.Enabled {
				s.State = StateDisabled
				st.list = append(st.list, s)
				continue
			}
			cands = append(cands, candidate{
				inst:   &Installed{ID: r.ID, Rev: r.Rev, Dir: s.Dir, Manifest: mf, Languages: langs, Summary: sum, InstallUID: r.InstallUID},
				status: &s,
			})
		}
	}

	addonList := make([]addons.Addon, len(cands))
	for i, c := range cands {
		addonList[i] = ToAddon(c.inst.Manifest)
	}
	reg, rejected := addons.Extend(m.base, addonList)
	rejectedAt := map[int]addons.Rejection{}
	for _, rj := range rejected {
		rejectedAt[rj.Index] = rj
	}
	plugins := map[string]string{}
	for i, c := range cands {
		if rj, ok := rejectedAt[i]; ok {
			c.status.State, c.status.Reason, c.status.Detail = StateRejected, rj.Code, rj.Detail
			st.list = append(st.list, *c.status)
			continue
		}
		c.status.State = StateActive
		st.active[c.inst.ID] = c.inst
		st.list = append(st.list, *c.status)
		for _, p := range c.inst.Manifest.Requires.Plugins {
			plugins[p.Package] = p.RPCFile
		}
	}
	st.reg = reg
	slices.SortStableFunc(st.list, func(a, b Status) int { return strings.Compare(a.ID, b.ID) })
	if m.opt.Plugins != nil {
		m.opt.Plugins.SetExtra(plugins)
	}
	m.state.Store(st)
	if m.onChange != nil {
		m.onChange()
	}
}

// BackendsEnabled는 이 설치에서 패키지 백엔드가 켜져 있는지다(Options.BackendsEnabled).
func (m *Manager) BackendsEnabled() bool { return m.opt.BackendsEnabled }

// loadInstalled는 설치한 rev 디렉터리의 manifest와 번역 언어를 읽는다(설치할 때 모두 검사했다).
func loadInstalled(dir string) (*format.Manifest, []string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, fail(ReasonManifest, "package files are missing: %v", err)
	}
	defer root.Close()
	b, err := readSmall(root, "manifest.json", format.MaxManifestBytes)
	if err != nil {
		return nil, nil, err
	}
	mf, err := format.Decode(b)
	if err != nil {
		return nil, nil, fail(ReasonManifest, "%v", err)
	}
	if err := format.Validate(mf); err != nil {
		var p *format.Problem
		if errors.As(err, &p) && p.Incompatible {
			return nil, nil, fail(ReasonIncompatible, "%v", err)
		}
		return nil, nil, fail(ReasonManifest, "%v", err)
	}
	entries, err := fs.ReadDir(root.FS(), "i18n")
	if err != nil {
		return nil, nil, fail(ReasonManifest, "read i18n: %v", err)
	}
	var langs []string
	for _, e := range entries {
		if lang, ok := strings.CutSuffix(e.Name(), ".json"); ok && slices.Contains(format.Languages, lang) {
			langs = append(langs, lang)
		}
	}
	slices.Sort(langs)
	return mf, langs, nil
}

// reasonOf는 오류의 이유 코드다.
func reasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ReasonArchive
}

// ErrNotFound는 그런 패키지가 없다는 뜻이다.
var ErrNotFound = errors.New("addon package not found")

// ErrDev는 개발 패키지는 바꿀 수 없다는 뜻이다.
var ErrDev = errors.New("development packages cannot be changed")

// ErrNoStore는 상태 디렉터리가 없어 설치할 수 없다는 뜻이다.
var ErrNoStore = errors.New("addon packages need a state directory")

// Pending은 올렸지만 아직 설치하지 않은 패키지다. 스테이징 디렉터리에 풀려 있다.
type Pending struct {
	Package   *Package
	Summary   Summary
	Previous  *Status
	Diff      *SummaryDiff
	Warnings  []string
	CreatedAt time.Time
	staging   *Staging
}

// 검토 화면의 경고(Pending.Warnings).
const (
	WarnAuthorChanged = "author_changed"
	WarnDowngrade     = "downgrade"
	WarnReinstall     = "reinstall"
	WarnSameDigest    = "same_digest"
	// WarnBackendsDisabled는 백엔드 패키지인데 이 설치에서 백엔드가 꺼져 있다는 뜻이다. 설치는 되지만 켜기 전까지
	// 데이터·조작을 쓸 수 없다(capability backend_unavailable).
	WarnBackendsDisabled = "backends_disabled"
)

// Discard는 대기를 버린다(스테이징 디렉터리를 지운다).
func (p *Pending) Discard() { p.staging.Discard() }

// Prepare는 압축 파일 r을 스테이징에 풀고 검사해 대기를 만든다. 레지스트리에 들어갈 수 있는지(id·메서드 충돌,
// 금지 표)도 지금 상태로 미리 본다. 실패하면 *Error 또는 *RejectError다.
func (m *Manager) Prepare(r io.Reader) (*Pending, error) {
	store := m.opt.Store
	if store == nil {
		return nil, ErrNoStore
	}
	st, err := store.NewStaging()
	if err != nil {
		return nil, err
	}
	p, err := Read(r, DefaultLimits, st.Root, m.opt.Arch)
	if err != nil {
		st.Discard()
		return nil, err
	}
	if err := m.wouldAccept(p.Manifest); err != nil {
		st.Discard()
		return nil, err
	}
	pend := &Pending{Package: p, Summary: Summarize(p.Manifest), CreatedAt: m.opt.Now(), staging: st}
	if p.Manifest.HasBackend() && !m.opt.BackendsEnabled {
		pend.Warnings = append(pend.Warnings, WarnBackendsDisabled)
	}
	for _, s := range m.state.Load().list {
		if s.ID != p.Manifest.ID || s.Dev {
			continue
		}
		prev := s
		pend.Previous = &prev
		if prev.Summary != nil {
			d := Diff(*prev.Summary, pend.Summary)
			pend.Diff = &d
		}
		if prev.Manifest != nil && prev.Manifest.Author.Name != p.Manifest.Author.Name {
			pend.Warnings = append(pend.Warnings, WarnAuthorChanged)
		}
		switch c := format.CompareVersions(p.Manifest.Version, prev.Version); {
		case prev.Rev == p.Rev():
			pend.Warnings = append(pend.Warnings, WarnSameDigest)
		case c < 0:
			pend.Warnings = append(pend.Warnings, WarnDowngrade)
		case c == 0:
			pend.Warnings = append(pend.Warnings, WarnReinstall)
		}
	}
	return pend, nil
}

// RejectError는 레지스트리가 패키지를 받지 않을 것이라는 뜻이다.
type RejectError struct{ addons.Rejection }

func (e *RejectError) Error() string { return e.Code + ": " + e.Detail }

// ReasonTooManyBackends는 백엔드가 있는 설치본을 config.AddonBackendsMax개보다 많이 켤 수 없다는 거부 코드다(root
// 동기화의 원하는 상태가 그만큼만 받는다). 개발 패키지는 세지 않는다.
const ReasonTooManyBackends = "too_many_backends"

// wouldAccept는 지금 받은 패키지(같은 id는 빼고)에 mf를 더했을 때 레지스트리가 받는지 본다. 백엔드 패키지는 켜 둔
// 백엔드 설치본의 수도 본다.
func (m *Manager) wouldAccept(mf *format.Manifest) error {
	var list []addons.Addon
	backends := 0
	for _, p := range m.Active() {
		if p.ID != mf.ID {
			list = append(list, ToAddon(p.Manifest))
			if !p.Dev && p.Manifest.HasBackend() {
				backends++
			}
		}
	}
	if mf.HasBackend() && backends >= config.AddonBackendsMax {
		return &RejectError{addons.Rejection{ID: mf.ID, Code: ReasonTooManyBackends,
			Detail: fmt.Sprintf("at most %d packages with a backend can be enabled", config.AddonBackendsMax)}}
	}
	list = append(list, ToAddon(mf))
	_, rejected := addons.Extend(m.base, list)
	for _, rj := range rejected {
		if rj.ID == mf.ID {
			return &RejectError{rj}
		}
	}
	return nil
}

// Install은 대기를 설치하고 레지스트리를 바꾼다. 같은 id가 있으면 업데이트이며 켜짐 상태·설치 ID·처음 설치 시각을
// 유지한다. 성공하든 실패하든 대기는 쓸 수 없다.
func (m *Manager) Install(p *Pending, by string) (*Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	store := m.opt.Store
	if store == nil {
		p.Discard()
		return nil, ErrNoStore
	}
	mf := p.Package.Manifest
	if dev := m.devStatus(mf.ID); dev {
		p.Discard()
		return nil, ErrDev
	}
	// 올린 뒤에 다른 패키지가 설치되었을 수 있으므로 다시 본다.
	if err := m.wouldAccept(mf); err != nil {
		p.Discard()
		return nil, err
	}
	records, err := store.Load()
	if err != nil {
		p.Discard()
		return nil, err
	}
	rev := p.Package.Rev()
	if err := store.Commit(p.staging, mf.ID, rev, m.opt.Arch); err != nil {
		p.Discard()
		return nil, err
	}
	now := m.opt.Now()
	rec := Record{
		ID: mf.ID, Version: mf.Version, Rev: rev, SHA256: p.Package.SHA256, Author: mf.Author.Name,
		Enabled: true, InstalledBy: by, InstalledAt: now, UpdatedAt: now, InstallUID: randomHex(16),
		Grant: p.Summary.Digest(),
	}
	i := slices.IndexFunc(records, func(r Record) bool { return r.ID == mf.ID })
	if i >= 0 {
		old := records[i]
		rec.Enabled, rec.InstalledAt, rec.InstallUID = old.Enabled, old.InstalledAt, old.InstallUID
		records[i] = rec
	} else {
		records = append(records, rec)
	}
	if err := store.Save(records); err != nil {
		return nil, err
	}
	m.reloadLocked()
	if err := store.Prune(mf.ID, rev); err != nil {
		log.Printf("addon %s: prune old revisions: %v", mf.ID, err)
	}
	return m.statusLocked(mf.ID), nil
}

// SetEnabled는 패키지를 켜거나 끈다. 켤 때 레지스트리가 받지 않으면 바꾸지 않고 *RejectError를 돌려준다.
func (m *Manager) SetEnabled(id string, on bool) (*Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	store := m.opt.Store
	if store == nil {
		return nil, ErrNoStore
	}
	records, err := store.Load()
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(records, func(r Record) bool { return r.ID == id })
	if i < 0 {
		if m.devStatus(id) {
			return nil, ErrDev
		}
		return nil, ErrNotFound
	}
	if on && !records[i].Enabled {
		s := m.statusLocked(id)
		if s == nil || s.Manifest == nil {
			return nil, fmt.Errorf("%w: package files are unreadable", ErrNotFound)
		}
		if err := m.wouldAccept(s.Manifest); err != nil {
			return nil, err
		}
	}
	records[i].Enabled = on
	records[i].UpdatedAt = m.opt.Now()
	if err := store.Save(records); err != nil {
		return nil, err
	}
	m.reloadLocked()
	return m.statusLocked(id), nil
}

// Uninstall은 설치 기록과 패키지 파일을 지운다. 기록을 지우고 레지스트리를 바꾼 뒤에는 제거가 끝난 것이므로(부르는
// 쪽은 그 뒤의 일, 백엔드 데이터 지우기 등을 이어 한다) 파일을 지우지 못하면 로그만 남긴다. 기록이 없으므로 남은 파일은
// 목록·레지스트리에 나오지 않는다.
func (m *Manager) Uninstall(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	store := m.opt.Store
	if store == nil {
		return ErrNoStore
	}
	records, err := store.Load()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(records, func(r Record) bool { return r.ID == id })
	if i < 0 {
		if m.devStatus(id) {
			return ErrDev
		}
		return ErrNotFound
	}
	records = slices.Delete(records, i, i+1)
	if err := store.Save(records); err != nil {
		return err
	}
	m.reloadLocked()
	if err := store.Remove(id); err != nil {
		log.Printf("addon %s: remove package files: %v", id, err)
	}
	return nil
}

// Status는 관리자 목록의 id 항목이다.
func (m *Manager) Status(id string) (*Status, bool) {
	s := m.statusLocked(id)
	return s, s != nil
}

func (m *Manager) statusLocked(id string) *Status {
	for _, s := range m.state.Load().list {
		if s.ID == id && !s.Dev {
			return &s
		}
	}
	for _, s := range m.state.Load().list {
		if s.ID == id {
			return &s
		}
	}
	return nil
}

// devStatus는 id가 개발 패키지인지다.
func (m *Manager) devStatus(id string) bool {
	for _, s := range m.state.Load().list {
		if s.ID == id && s.Dev {
			return true
		}
	}
	return false
}
