package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"onmaru/backend/internal/api"
)

// fileVersion은 사용자 설정 파일 형식의 버전이다. 형식을 바꾸면 올리고 읽을 때 변환한다.
const fileVersion = 1

// fileDoc은 사용자 설정 파일 한 개의 내용이다.
type fileDoc struct {
	Version int `json:"version"`
	api.Preferences
}

// FileStore는 상태 디렉터리에 저장하는 Store, SiteStore, ImageStore다.
//
//	<dir>/users/<인코딩한 사용자 이름>.json  사용자 설정
//	<dir>/site.json                          전역 설정(site.go)
//	<dir>/images/...                         배경 이미지(images.go)
//
// 쓰기는 임시 파일에 쓴 뒤 rename으로 바꿔 넣으므로 중간에 멈춰도 이전 파일이나 새 파일 중 하나가 남는다.
// 같은 디렉터리를 쓰는 프로세스는 하나라고 보고, 프로세스 안에서만 잠근다.
type FileStore struct {
	root string
	dir  string
	// mu는 사용자·전역 설정 파일을, imgMu는 이미지 파일을 지킨다. 큰 이미지를 쓰는 동안 설정 저장이 기다리지 않게 나눈다.
	mu    sync.Mutex
	imgMu sync.Mutex
}

// NewFileStore는 dir 아래에 설정을 저장하는 FileStore를 만든다. users 디렉터리가 없으면 만든다.
// 전역 설정과 이미지 디렉터리는 처음 쓸 때 만든다.
func NewFileStore(dir string) (*FileStore, error) {
	users := filepath.Join(dir, "users")
	if err := os.MkdirAll(users, 0o700); err != nil {
		return nil, err
	}
	return &FileStore{root: dir, dir: users}, nil
}

func (s *FileStore) Get(username string) (api.Preferences, error) {
	path, err := s.path(username)
	if err != nil {
		return api.Preferences{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read(path)
	return doc.Preferences, err
}

func (s *FileStore) SetWidgets(username string, widgets []api.WidgetPlacement) error {
	return s.update(username, func(p *api.Preferences) { p.Widgets = cloneList(widgets) })
}

func (s *FileStore) SetShortcuts(username string, shortcuts []api.Shortcut) error {
	return s.update(username, func(p *api.Preferences) { p.Shortcuts = cloneList(shortcuts) })
}

func (s *FileStore) SetDrawer(username string, drawer api.DrawerPrefs) error {
	return s.update(username, func(p *api.Preferences) { p.Drawer = &drawer })
}

func (s *FileStore) SetSystemInfo(username string, sysinfo api.SystemInfoPrefs) error {
	return s.update(username, func(p *api.Preferences) { p.SystemInfo = &sysinfo })
}

func (s *FileStore) UpdateAppearance(username string, apply func(*api.AppearancePrefs)) error {
	return s.update(username, func(p *api.Preferences) { p.Appearance = applyAppearance(p.Appearance, apply) })
}

// update는 username의 설정 파일을 읽어 설정 하나를 바꾼 뒤 다시 쓴다. 다른 설정은 그대로 둔다.
// 읽기부터 쓰기까지 잠그므로 서로 다른 설정을 동시에 저장해도 한쪽이 지워지지 않는다.
func (s *FileStore) update(username string, apply func(*api.Preferences)) error {
	path, err := s.path(username)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.read(path)
	if err != nil {
		return err
	}
	doc.Version = fileVersion
	apply(&doc.Preferences)
	// 응답할 때 붙이는 값은 저장하지 않는다.
	doc.WallpaperImage, doc.AvatarImage = nil, nil
	return writeAtomic(path, doc)
}

// read는 사용자 설정 파일을 읽는다(readJSON).
func (s *FileStore) read(path string) (fileDoc, error) {
	doc, err := readJSON[fileDoc](path)
	doc.WallpaperImage, doc.AvatarImage = nil, nil
	return doc, err
}

// readJSON은 JSON 파일을 읽는다. 없으면 빈 값이다. 읽을 수 없는 형식이면 파일을 .corrupt로 옮겨
// 두고(손으로 되살릴 수 있게) 빈 값으로 시작한다. 그대로 두면 사용자가 설정을 다시 저장할 수 없다.
func readJSON[T any](path string) (T, error) {
	var zero T
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return zero, nil
	}
	if err != nil {
		return zero, err
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		log.Printf("preferences: %s is not valid JSON, moving it to .corrupt: %v", filepath.Base(path), err)
		return zero, os.Rename(path, path+".corrupt")
	}
	return v, nil
}

// path는 사용자 설정 파일 경로다. 사용자 이름은 fileName으로 인코딩해 디렉터리를 벗어나지 못하게 한다.
func (s *FileStore) path(username string) (string, error) {
	if username == "" {
		return "", errors.New("preferences: empty username")
	}
	return filepath.Join(s.dir, fileName(username)+".json"), nil
}

// fileName은 사용자 이름을 파일 이름으로 바꾼다. 영문자·숫자·'_'·'-'는 그대로 두고, 나머지 바이트('.', '/',
// '%' 포함)는 %XX로 바꾼다. 보통 이름은 그대로 읽히고, 서로 다른 이름이 같은 파일 이름이 되지 않는다.
func fileName(username string) string {
	var b strings.Builder
	for i := 0; i < len(username); i++ {
		c := username[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// writeAtomic은 v를 JSON으로 path에 바꿔 넣는다(writeFileAtomic).
func writeAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'))
}

// writeFileAtomic은 data를 같은 디렉터리의 임시 파일에 쓰고 fsync한 뒤 path로 바꿔 넣는다.
func writeFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	// CreateTemp는 0600으로 만든다. 설정에는 비밀 값이 없지만 서비스 계정만 읽으면 된다.
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	// rename 자체가 디스크에 남도록 디렉터리도 fsync한다.
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
