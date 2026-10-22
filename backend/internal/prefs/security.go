package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"maru/backend/internal/security"
)

// 보안 강화 모드의 수단 기록(security.Store 구현): <dir>/security/<인코딩한 사용자 이름>.json.
// 사용자 설정과 파일을 나눈다. TOTP 비밀과 복구 코드 해시가 들어 있으므로 디렉터리는 0700, 파일은 0600이다.
//
// 다른 설정과 달리 읽을 수 없는 파일을 .corrupt로 옮기고 빈 값으로 시작하지 않는다. 빈 기록은 "2차 인증
// 없음"이라 조용히 보안이 낮아지기 때문이다. 오류를 돌려주고 파일은 그대로 둔다(root가 지워 되살린다).

// securityVersion은 수단 기록 파일 형식의 버전이다. 모르는 버전은 읽지 않는다.
const securityVersion = 1

type securityDoc struct {
	Version int `json:"version"`
	security.Record
}

// ErrSecurityRecord는 수단 기록 파일을 읽을 수 없다는 뜻이다(형식 오류, 모르는 버전).
var ErrSecurityRecord = errors.New("preferences: unreadable security record")

func (s *FileStore) securityDir() string { return filepath.Join(s.root, "security") }

func (s *FileStore) securityPath(username string) (string, error) {
	if username == "" {
		return "", errors.New("preferences: empty username")
	}
	return filepath.Join(s.securityDir(), fileName(username)+".json"), nil
}

// readSecurity는 수단 기록 파일을 읽는다. 없으면 빈 기록이다.
func readSecurity(path string) (security.Record, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return security.Record{}, nil
	}
	if err != nil {
		return security.Record{}, err
	}
	var doc securityDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		log.Printf("preferences: %s is not valid JSON: %v", filepath.Base(path), err)
		return security.Record{}, fmt.Errorf("%w: %s", ErrSecurityRecord, filepath.Base(path))
	}
	if doc.Version != securityVersion {
		log.Printf("preferences: %s has unsupported version %d", filepath.Base(path), doc.Version)
		return security.Record{}, fmt.Errorf("%w: %s", ErrSecurityRecord, filepath.Base(path))
	}
	return doc.Record, nil
}

func (s *FileStore) GetSecurity(username string) (security.Record, error) {
	path, err := s.securityPath(username)
	if err != nil {
		return security.Record{}, err
	}
	s.secMu.Lock()
	defer s.secMu.Unlock()
	return readSecurity(path)
}

func (s *FileStore) UpdateSecurity(username string, apply func(*security.Record) error) error {
	path, err := s.securityPath(username)
	if err != nil {
		return err
	}
	s.secMu.Lock()
	defer s.secMu.Unlock()
	rec, err := readSecurity(path)
	if err != nil {
		return err
	}
	if err := apply(&rec); err != nil {
		return err
	}
	if security.Normalize(&rec) {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(s.securityDir(), 0o700); err != nil {
		return err
	}
	return writeAtomic(path, securityDoc{Version: securityVersion, Record: rec})
}

func (s *FileStore) DeleteSecurity(username string) (bool, error) {
	path, err := s.securityPath(username)
	if err != nil {
		return false, err
	}
	s.secMu.Lock()
	defer s.secMu.Unlock()
	err = os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s *FileStore) ListSecurity() ([]string, error) {
	s.secMu.Lock()
	defer s.secMu.Unlock()
	entries, err := os.ReadDir(s.securityDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		base, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !e.Type().IsRegular() || strings.HasPrefix(base, ".") {
			continue
		}
		// fileName은 %XX로만 인코딩하므로 PathUnescape로 되돌린다.
		if name, err := url.PathUnescape(base); err == nil && fileName(name) == base {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

func (m *MemoryStore) GetSecurity(username string) (security.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.records[username].Clone(), nil
}

func (m *MemoryStore) UpdateSecurity(username string, apply func(*security.Record) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.records[username].Clone()
	if err := apply(&rec); err != nil {
		return err
	}
	if security.Normalize(&rec) {
		delete(m.records, username)
		return nil
	}
	m.records[username] = rec.Clone()
	return nil
}

func (m *MemoryStore) DeleteSecurity(username string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.records[username]
	delete(m.records, username)
	return ok, nil
}

func (m *MemoryStore) ListSecurity() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var names []string
	for name := range m.records {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}
