package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"maru/backend/internal/jobs"
)

// 서버 백그라운드 작업의 기록(jobs.Store 구현). 사용자 설정과 따로 둔다.
//
//	<dir>/jobs/current.json                     진행 중 작업(없으면 파일이 없다)
//	<dir>/jobs/results/<인코딩한 사용자 이름>.json 시작한 사용자별 결과
//	<dir>/jobs/output/<작업 id>.log             작업 출력
//
// 출력은 조각마다 덧붙이고 fsync하지 않는다. 진행 기록의 출력 범위가 기준이며, 다시 시작하면 그 길이로 자르고
// 기록한 자리부터 다시 받는다.

// jobsVersion은 작업 기록 파일 형식의 버전이다. 모르는 버전은 읽지 않는다.
const jobsVersion = 1

// 읽는 크기의 상한.
const (
	maxCurrentBytes = 256 << 10
	maxResultsBytes = 256 << 10
)

type currentDoc struct {
	Version int `json:"version"`
	jobs.Record
}

type resultsDoc struct {
	Version int           `json:"version"`
	Results []jobs.Result `json:"results"`
}

func (s *FileStore) jobsDir() string { return filepath.Join(s.root, "jobs") }

func (s *FileStore) jobsPath(parts ...string) (string, error) {
	return filepath.Join(append([]string{s.jobsDir()}, parts...)...), nil
}

// ensureDir는 쓰기 전에 path의 디렉터리를 만든다(읽기만 하는 요청이 디렉터리를 만들지 않게).
func ensureDir(path string) error { return os.MkdirAll(filepath.Dir(path), 0o700) }

func (s *FileStore) outputPath(id string) (string, error) {
	if !jobs.ValidID(id) {
		return "", fmt.Errorf("preferences: invalid job id %q", id)
	}
	return s.jobsPath("output", id+".log")
}

// readCapped는 path를 limit바이트까지 읽는다. 없으면 nil, nil이다. 넘으면 오류다.
func readCapped(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("preferences: %s is too large", filepath.Base(path))
	}
	return b, nil
}

func (s *FileStore) LoadCurrent() (*jobs.Record, error) {
	path, err := s.jobsPath("current.json")
	if err != nil {
		return nil, err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	b, err := readCapped(path, maxCurrentBytes)
	if err != nil || b == nil {
		return nil, err
	}
	var doc currentDoc
	if err := json.Unmarshal(b, &doc); err != nil || doc.Version != jobsVersion {
		// 읽을 수 없는 진행 기록은 손으로 살펴볼 수 있게 옮겨 두고 없는 것으로 본다(새 작업을 막지 않게).
		log.Printf("preferences: %s is unreadable, moving it to .corrupt", filepath.Base(path))
		return nil, os.Rename(path, path+".corrupt")
	}
	return &doc.Record, nil
}

func (s *FileStore) SaveCurrent(r *jobs.Record) error {
	path, err := s.jobsPath("current.json")
	if err != nil {
		return err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if r == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := ensureDir(path); err != nil {
		return err
	}
	return writeAtomic(path, currentDoc{Version: jobsVersion, Record: *r})
}

func (s *FileStore) resultsPath(user string) (string, error) {
	if user == "" {
		return "", errors.New("preferences: empty username")
	}
	return s.jobsPath("results", fileName(user)+".json")
}

func (s *FileStore) LoadResults(user string) ([]jobs.Result, error) {
	path, err := s.resultsPath(user)
	if err != nil {
		return nil, err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	b, err := readCapped(path, maxResultsBytes)
	if err != nil || b == nil {
		return nil, err
	}
	var doc resultsDoc
	if err := json.Unmarshal(b, &doc); err != nil || doc.Version != jobsVersion {
		log.Printf("preferences: %s is unreadable, moving it to .corrupt", filepath.Base(path))
		return nil, os.Rename(path, path+".corrupt")
	}
	return doc.Results, nil
}

func (s *FileStore) SaveResults(user string, rs []jobs.Result) error {
	path, err := s.resultsPath(user)
	if err != nil {
		return err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if len(rs) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := ensureDir(path); err != nil {
		return err
	}
	return writeAtomic(path, resultsDoc{Version: jobsVersion, Results: rs})
}

func (s *FileStore) AppendOutput(id string, b []byte) error {
	path, err := s.outputPath(id)
	if err != nil {
		return err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if err := ensureDir(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func (s *FileStore) ReadOutput(id string, off int64, n int) ([]byte, error) {
	path, err := s.outputPath(id)
	if err != nil {
		return nil, err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	got, err := f.ReadAt(buf, off)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return buf[:got], err
}

func (s *FileStore) TruncateOutput(id string, size int64) error {
	path, err := s.outputPath(id)
	if err != nil {
		return err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || st.Size() <= size {
		return err
	}
	return os.Truncate(path, size)
}

// DropOutputHead는 출력 파일의 뒤쪽만 새 파일로 옮겨 바꿔 넣는다. 출력 상한(수 MiB)이라 메모리로 읽어도 된다.
func (s *FileStore) DropOutputHead(id string, keep int64) (int64, error) {
	path, err := s.outputPath(id)
	if err != nil {
		return 0, err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	drop := jobs.HeadToDrop(data, keep)
	if drop == 0 {
		return 0, nil
	}
	if err := writeFileAtomic(path, data[drop:]); err != nil {
		return 0, err
	}
	return drop, nil
}

func (s *FileStore) DeleteOutput(id string) error {
	path, err := s.outputPath(id)
	if err != nil {
		return err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *FileStore) PruneOutputs(before time.Time, keep string) error {
	dir := filepath.Join(s.jobsDir(), "output")
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".log")
		if !ok || id == keep || !e.Type().IsRegular() {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(before) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}
