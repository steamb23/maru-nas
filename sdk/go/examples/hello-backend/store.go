// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 백엔드가 스스로 두는 값. 데이터 디렉터리(maruaddon.StateDir: 설치본은 systemd의 StateDirectory, 개발은
// MARU_ADDON_STATE)가 있으면 그 안의 state.json에, 없으면 메모리에만 둔다. 이 디렉터리는 모든 대시보드 사용자가 함께
// 쓰고, 관리자가 패키지를 지울 때 함께 지울 수 있다.

// stateFile은 데이터 디렉터리 안의 파일 이름이다.
const stateFile = "state.json"

// change는 성능 통계를 바꾼 기록 하나다.
type change struct {
	User   string    `json:"user"`
	At     time.Time `json:"at"`
	Enable bool      `json:"enable"`
}

// savedState는 state.json의 내용이다.
type savedState struct {
	DataRequests int64   `json:"dataRequests"`
	LastChange   *change `json:"lastChange,omitempty"`
}

type store struct {
	mu   sync.Mutex
	path string
	st   savedState
}

// openStore는 dir의 state.json을 읽는다. dir이 비면 메모리에만 둔다. 읽을 수 없는 내용(손상)은 처음부터 다시 센다.
// 예제가 두는 값이라 잃어도 되기 때문이다. 잃으면 안 되는 값이라면 오류로 멈추는 편이 낫다.
func openStore(dir string) (*store, error) {
	s := &store{}
	if dir == "" {
		return s, nil
	}
	s.path = filepath.Join(dir, stateFile)
	data, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, &s.st); err != nil {
			log.Printf("hello-backend: %s: %v; starting over", s.path, err)
			s.st = savedState{}
		}
	}
	return s, nil
}

func (s *store) persistent() bool { return s.path != "" }

// update는 fn으로 값을 바꾸고 바뀐 값을 돌려준다. 파일에 쓰지 못하면 로그만 남기고 메모리의 값을 쓴다(데이터 요청이
// 수를 세지 못했다고 실패하지 않게, 조작이 이미 OMV를 바꾼 뒤에 실패로 보이지 않게).
func (s *store) update(fn func(*savedState)) savedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.st)
	if s.path != "" {
		if err := writeJSON(s.path, s.st); err != nil {
			log.Printf("hello-backend: save %s: %v", s.path, err)
		}
	}
	return s.st
}

// writeJSON은 같은 디렉터리의 임시 파일에 쓴 뒤 이름을 바꾼다. 쓰는 도중에 멈춰도 이전 파일이 남는다.
func writeJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
