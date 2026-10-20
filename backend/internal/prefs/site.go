package prefs

import (
	"path/filepath"
	"slices"

	"onmaru/backend/internal/api"
)

// SiteStore는 모든 사용자에게 적용하는 전역 설정 저장소다. 관리자만 바꾼다(검사는 호출하는 쪽이 한다).
type SiteStore interface {
	// GetSite는 전역 설정이다. 저장한 적이 없으면 빈 설정이다.
	GetSite() (api.SiteSettings, error)
	// UpdateSite는 전역 설정을 읽어 apply로 고친 뒤 저장한다. 읽기부터 쓰기까지 잠근다.
	UpdateSite(apply func(*api.SiteSettings)) error
}

// siteVersion은 전역 설정 파일 형식의 버전이다.
const siteVersion = 1

// siteDoc은 site.json의 내용이다.
type siteDoc struct {
	Version int `json:"version"`
	api.SiteSettings
}

func (m *MemoryStore) GetSite() (api.SiteSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneSite(m.site), nil
}

func (m *MemoryStore) UpdateSite(apply func(*api.SiteSettings)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := cloneSite(m.site)
	apply(&next)
	m.site = cloneSite(next)
	return nil
}

func (s *FileStore) GetSite() (api.SiteSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := readJSON[siteDoc](s.sitePath())
	return doc.SiteSettings, err
}

func (s *FileStore) UpdateSite(apply func(*api.SiteSettings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := readJSON[siteDoc](s.sitePath())
	if err != nil {
		return err
	}
	doc.Version = siteVersion
	apply(&doc.SiteSettings)
	return writeAtomic(s.sitePath(), doc)
}

func (s *FileStore) sitePath() string {
	return filepath.Join(s.root, "site.json")
}

// cloneSite는 호출하는 쪽이 바꿔도 저장값에 영향이 없도록 복사한다.
func cloneSite(v api.SiteSettings) api.SiteSettings {
	if v.Login != nil {
		l := *v.Login
		l.Background = cloneWallpaper(l.Background)
		v.Login = &l
	}
	if v.Defaults != nil {
		d := *v.Defaults
		d.Wallpaper = cloneWallpaper(d.Wallpaper)
		v.Defaults = &d
	}
	if v.Server != nil {
		o := *v.Server
		o.WorkbenchURL = cloneString(o.WorkbenchURL)
		o.SmartRefreshAt = cloneString(o.SmartRefreshAt)
		v.Server = &o
	}
	if v.Security != nil {
		p := *v.Security
		v.Security = &p
	}
	if v.Files != nil {
		f := *v.Files
		f.Shares = slices.Clone(f.Shares)
		v.Files = &f
	}
	return v
}

func cloneString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
