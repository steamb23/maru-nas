// Package prefs는 대시보드 설정을 저장한다: 사용자별 설정(위젯 배치, 바로가기, 화면 등), 모든 사용자에게
// 적용하는 전역 설정(site.go), 배경 이미지(images.go), 보안 강화 모드의 수단 기록(security.go).
//
// 대시보드 자신의 상태만 다룬다. OMV 설정이나 시스템 파일은 건드리지 않는다.
// 운영에서는 상태 디렉터리(systemd StateDirectory) 아래 파일(FileStore)에,
// 상태 디렉터리가 없으면 메모리(MemoryStore)에 둔다.
package prefs

import (
	"slices"
	"sync"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/security"
)

// Store는 사용자 설정 저장소다. 여러 고루틴에서 함께 써도 된다.
type Store interface {
	// Get은 username의 설정이다. 저장한 적이 없으면 빈 설정이다.
	Get(username string) (api.Preferences, error)
	// SetWidgets는 위젯 배치만 바꾼다. 다른 설정은 그대로 둔다. 검사는 호출하는 쪽이 한다.
	SetWidgets(username string, widgets []api.WidgetPlacement) error
	// SetShortcuts는 바로가기 목록만 바꾼다. 다른 설정은 그대로 둔다. 검사는 호출하는 쪽이 한다.
	SetShortcuts(username string, shortcuts []api.Shortcut) error
	// SetDrawer는 "모든 앱" 화면 설정만 바꾼다. 다른 설정은 그대로 둔다. 검사는 호출하는 쪽이 한다.
	SetDrawer(username string, drawer api.DrawerPrefs) error
	// SetSystemInfo는 시스템 정보 창 설정만 바꾼다. 다른 설정은 그대로 둔다. 검사는 호출하는 쪽이 한다.
	SetSystemInfo(username string, sysinfo api.SystemInfoPrefs) error
	// SetFiles는 파일 탐색기 보기 설정만 바꾼다. 다른 설정은 그대로 둔다. 검사는 호출하는 쪽이 한다.
	SetFiles(username string, files api.FilesPrefs) error
	// UpdateAppearance는 화면 설정만 바꾼다. apply는 지금 값(저장한 적 없으면 빈 값)을 받아 고친다.
	// 읽기부터 쓰기까지 잠그므로 조건부 변경(올린 이미지를 지울 때 그 이미지를 가리키던 배경 지우기 등)에도 쓴다.
	UpdateAppearance(username string, apply func(*api.AppearancePrefs)) error
}

// MemoryStore는 프로세스 메모리에 두는 Store, SiteStore, ImageStore, security.Store다. 백엔드를 재시작하면 사라진다.
type MemoryStore struct {
	mu      sync.Mutex
	users   map[string]api.Preferences
	site    api.SiteSettings
	images  map[ImageKey]memImage
	records map[string]security.Record
	// rev는 이미지 리비전을 만드는 카운터다.
	rev int
}

// NewMemoryStore는 빈 MemoryStore를 반환한다.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:   make(map[string]api.Preferences),
		images:  make(map[ImageKey]memImage),
		records: make(map[string]security.Record),
	}
}

func (m *MemoryStore) Get(username string) (api.Preferences, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return clonePrefs(m.users[username]), nil
}

func (m *MemoryStore) SetWidgets(username string, widgets []api.WidgetPlacement) error {
	m.update(username, func(p *api.Preferences) { p.Widgets = cloneList(widgets) })
	return nil
}

func (m *MemoryStore) SetShortcuts(username string, shortcuts []api.Shortcut) error {
	m.update(username, func(p *api.Preferences) { p.Shortcuts = cloneList(shortcuts) })
	return nil
}

func (m *MemoryStore) SetDrawer(username string, drawer api.DrawerPrefs) error {
	m.update(username, func(p *api.Preferences) { p.Drawer = &drawer })
	return nil
}

func (m *MemoryStore) SetSystemInfo(username string, sysinfo api.SystemInfoPrefs) error {
	m.update(username, func(p *api.Preferences) { p.SystemInfo = &sysinfo })
	return nil
}

func (m *MemoryStore) SetFiles(username string, files api.FilesPrefs) error {
	m.update(username, func(p *api.Preferences) { p.Files = &files })
	return nil
}

func (m *MemoryStore) UpdateAppearance(username string, apply func(*api.AppearancePrefs)) error {
	m.update(username, func(p *api.Preferences) { p.Appearance = applyAppearance(p.Appearance, apply) })
	return nil
}

// update는 username의 설정 하나를 바꾼다. 다른 설정은 그대로 둔다.
func (m *MemoryStore) update(username string, apply func(*api.Preferences)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.users[username]
	apply(&p)
	m.users[username] = p
}

// clonePrefs는 호출하는 쪽이 바꿔도 저장값에 영향이 없도록 복사한다.
func clonePrefs(p api.Preferences) api.Preferences {
	if p.Widgets != nil {
		p.Widgets = cloneList(*p.Widgets)
	}
	if p.Shortcuts != nil {
		p.Shortcuts = cloneList(*p.Shortcuts)
	}
	if p.Drawer != nil {
		d := *p.Drawer
		p.Drawer = &d
	}
	if p.SystemInfo != nil {
		s := *p.SystemInfo
		p.SystemInfo = &s
	}
	if p.Files != nil {
		f := *p.Files
		p.Files = &f
	}
	p.Appearance = cloneAppearance(p.Appearance)
	return p
}

// applyAppearance는 지금 화면 설정(없으면 빈 값)의 복사본에 apply를 적용한 새 값이다.
func applyAppearance(cur *api.AppearancePrefs, apply func(*api.AppearancePrefs)) *api.AppearancePrefs {
	next := cloneAppearance(cur)
	if next == nil {
		next = &api.AppearancePrefs{}
	}
	apply(next)
	return cloneAppearance(next)
}

func cloneAppearance(a *api.AppearancePrefs) *api.AppearancePrefs {
	if a == nil {
		return nil
	}
	c := *a
	c.Wallpaper = cloneWallpaper(a.Wallpaper)
	if a.Animations != nil {
		v := *a.Animations
		c.Animations = &v
	}
	return &c
}

func cloneWallpaper(w *api.Wallpaper) *api.Wallpaper {
	if w == nil {
		return nil
	}
	c := *w
	return &c
}

// cloneList는 nil도 빈 목록으로 복사한다. 빈 목록은 "모두 지움"이고, 없음(nil 포인터)은 "저장한 적 없음"이다.
func cloneList[T any](s []T) *[]T {
	c := slices.Clone(s)
	if c == nil {
		c = []T{}
	}
	return &c
}
