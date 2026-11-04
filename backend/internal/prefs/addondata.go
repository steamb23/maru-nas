package prefs

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
)

// 애드온 패키지의 사용자별 저장값: <dir>/addon-data/<인코딩한 사용자 이름>.json. 사용자 설정 파일과 따로 두고
// 따로 잠근다(위젯 배치를 저장할 때마다 큰 값을 다시 쓰지 않게). 읽을 수 없는 파일은 다른 설정처럼 .corrupt로
// 옮기고 빈 값으로 시작한다(패키지가 다시 저장할 수 있게).

// addonDataVersion은 저장값 파일 형식의 버전이다.
const addonDataVersion = 1

const (
	// MaxAddonData는 패키지 하나의 저장값 크기 상한(JSON 바이트)이다.
	MaxAddonData = 64 << 10
	// MaxAddonDataPerUser는 사용자 하나의 저장값 합의 상한이다.
	MaxAddonDataPerUser = 1 << 20
)

// AddonData는 패키지 하나의 저장값이다. InstallUID는 저장할 때의 설치 ID이며, 다시 설치한 패키지(다른 설치 ID)는
// 이 값을 읽지 못한다.
type AddonData struct {
	InstallUID string          `json:"installUID"`
	Data       json.RawMessage `json:"data"`
}

// AddonDataStore는 애드온 패키지의 사용자별 저장값이다.
type AddonDataStore interface {
	// GetAddonData는 username의 id 저장값이다. 없으면 false다.
	GetAddonData(username, id string) (AddonData, bool, error)
	// SetAddonData는 id 저장값을 바꾼다. 같이 저장된 다른 패키지의 값 중 keep이 false인 것(지웠거나 다시 설치한
	// 패키지)은 지운다. 합이 MaxAddonDataPerUser를 넘으면 ErrAddonDataFull이다.
	SetAddonData(username, id string, d AddonData, keep func(id, installUID string) bool) error
}

// ErrAddonDataFull은 사용자의 저장값 합이 상한을 넘는다는 뜻이다.
var ErrAddonDataFull = errors.New("preferences: addon data is full")

type addonDataDoc struct {
	Version int                  `json:"version"`
	Addons  map[string]AddonData `json:"addons"`
}

func (s *FileStore) addonDataPath(username string) (string, error) {
	if username == "" {
		return "", errors.New("preferences: empty username")
	}
	return filepath.Join(s.root, "addon-data", fileName(username)+".json"), nil
}

func (s *FileStore) GetAddonData(username, id string) (AddonData, bool, error) {
	path, err := s.addonDataPath(username)
	if err != nil {
		return AddonData{}, false, err
	}
	s.addonMu.Lock()
	defer s.addonMu.Unlock()
	doc, err := readJSON[addonDataDoc](path)
	if err != nil {
		return AddonData{}, false, err
	}
	d, ok := doc.Addons[id]
	return d, ok, nil
}

func (s *FileStore) SetAddonData(username, id string, d AddonData, keep func(id, installUID string) bool) error {
	path, err := s.addonDataPath(username)
	if err != nil {
		return err
	}
	s.addonMu.Lock()
	defer s.addonMu.Unlock()
	doc, err := readJSON[addonDataDoc](path)
	if err != nil {
		return err
	}
	next, err := mergeAddonData(doc.Addons, id, d, keep)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeAtomic(path, addonDataDoc{Version: addonDataVersion, Addons: next})
}

// mergeAddonData는 저장값 표에 id 값을 넣고 keep이 false인 값을 뺀 새 표다.
func mergeAddonData(cur map[string]AddonData, id string, d AddonData, keep func(string, string) bool) (map[string]AddonData, error) {
	next := map[string]AddonData{}
	total := 0
	for k, v := range cur {
		if k == id || (keep != nil && !keep(k, v.InstallUID)) {
			continue
		}
		next[k] = v
		total += len(v.Data)
	}
	next[id] = d
	if total+len(d.Data) > MaxAddonDataPerUser {
		return nil, ErrAddonDataFull
	}
	return next, nil
}

func (m *MemoryStore) GetAddonData(username, id string) (AddonData, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.addonData[username][id]
	return d, ok, nil
}

func (m *MemoryStore) SetAddonData(username, id string, d AddonData, keep func(id, installUID string) bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next, err := mergeAddonData(maps.Clone(m.addonData[username]), id, d, keep)
	if err != nil {
		return err
	}
	if m.addonData == nil {
		m.addonData = map[string]map[string]AddonData{}
	}
	m.addonData[username] = next
	return nil
}
