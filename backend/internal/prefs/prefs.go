// Package prefs는 대시보드 사용자 설정(위젯 배치, 바로가기 등)을 사용자별로 저장한다.
//
// 대시보드 자신의 상태만 다룬다. OMV 설정이나 시스템 파일은 건드리지 않는다.
// 운영에서는 상태 디렉터리(systemd StateDirectory) 아래 사용자별 JSON 파일(FileStore)에,
// 상태 디렉터리가 없으면 메모리(MemoryStore)에 둔다.
package prefs

import (
	"slices"
	"sync"

	"onmaru/backend/internal/api"
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
}

// MemoryStore는 프로세스 메모리에 두는 Store다. 백엔드를 재시작하면 사라진다.
type MemoryStore struct {
	mu    sync.Mutex
	users map[string]api.Preferences
}

// NewMemoryStore는 빈 MemoryStore를 반환한다.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{users: make(map[string]api.Preferences)}
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
	return p
}

// cloneList는 nil도 빈 목록으로 복사한다. 빈 목록은 "모두 지움"이고, 없음(nil 포인터)은 "저장한 적 없음"이다.
func cloneList[T any](s []T) *[]T {
	c := slices.Clone(s)
	if c == nil {
		c = []T{}
	}
	return &c
}
