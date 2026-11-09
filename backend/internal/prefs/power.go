package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"maru/backend/internal/power"
)

// 다시 시작 예약의 기록(power.Store 구현). <dir>/power.json 하나이며 예약이 없으면 파일도 없다.

// powerVersion은 예약 기록 파일 형식의 버전이다. 모르는 버전은 기록이 없는 것으로 본다.
const powerVersion = 1

// maxPowerBytes는 예약 기록 파일을 읽는 크기 상한이다.
const maxPowerBytes = 4 << 10

type powerDoc struct {
	Version int `json:"version"`
	power.Record
}

func (s *FileStore) powerPath() string { return filepath.Join(s.root, "power.json") }

func (s *FileStore) PowerRecord() (power.Record, bool, error) {
	s.powerMu.Lock()
	defer s.powerMu.Unlock()
	b, err := readCapped(s.powerPath(), maxPowerBytes)
	if err != nil || b == nil {
		return power.Record{}, false, err
	}
	var doc powerDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return power.Record{}, false, fmt.Errorf("preferences: power.json: %w", err)
	}
	if doc.Version != powerVersion {
		return power.Record{}, false, nil
	}
	return doc.Record, true, nil
}

func (s *FileStore) SetPowerRecord(r power.Record) error {
	s.powerMu.Lock()
	defer s.powerMu.Unlock()
	return writeAtomic(s.powerPath(), powerDoc{Version: powerVersion, Record: r})
}

func (s *FileStore) ClearPowerRecord() error {
	s.powerMu.Lock()
	defer s.powerMu.Unlock()
	if err := os.Remove(s.powerPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
