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

	"omv-desktop-dashboard/backend/internal/api"
)

// fileVersion은 사용자 설정 파일 형식의 버전이다. 형식을 바꾸면 올리고 읽을 때 변환한다.
const fileVersion = 1

// fileDoc은 사용자 설정 파일 한 개의 내용이다.
type fileDoc struct {
	Version int `json:"version"`
	api.Preferences
}

// FileStore는 <dir>/users/<인코딩한 사용자 이름>.json에 사용자별로 저장하는 Store다.
// 쓰기는 임시 파일에 쓴 뒤 rename으로 바꿔 넣으므로 중간에 멈춰도 이전 파일이나 새 파일 중 하나가 남는다.
// 같은 디렉터리를 쓰는 프로세스는 하나라고 보고, 프로세스 안에서만 잠근다.
type FileStore struct {
	dir string
	mu  sync.Mutex
}

// NewFileStore는 dir 아래에 사용자 설정을 저장하는 FileStore를 만든다. users 디렉터리가 없으면 만든다.
func NewFileStore(dir string) (*FileStore, error) {
	users := filepath.Join(dir, "users")
	if err := os.MkdirAll(users, 0o700); err != nil {
		return nil, err
	}
	return &FileStore{dir: users}, nil
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
	doc.Widgets = cloneWidgets(widgets)
	return writeAtomic(path, doc)
}

// read는 설정 파일을 읽는다. 없으면 빈 설정이다. 읽을 수 없는 형식이면 파일을 .corrupt로 옮겨
// 두고(손으로 되살릴 수 있게) 빈 설정으로 시작한다. 그대로 두면 사용자가 배치를 다시 저장할 수 없다.
func (s *FileStore) read(path string) (fileDoc, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fileDoc{}, nil
	}
	if err != nil {
		return fileDoc{}, err
	}
	var doc fileDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		log.Printf("preferences: %s is not valid JSON, moving it to .corrupt: %v", filepath.Base(path), err)
		if err := os.Rename(path, path+".corrupt"); err != nil {
			return fileDoc{}, err
		}
		return fileDoc{}, nil
	}
	return doc, nil
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

// writeAtomic은 v를 JSON으로 같은 디렉터리의 임시 파일에 쓰고 fsync한 뒤 path로 바꿔 넣는다.
func writeAtomic(path string, v any) (err error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
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
	if _, err = f.Write(append(b, '\n')); err != nil {
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
