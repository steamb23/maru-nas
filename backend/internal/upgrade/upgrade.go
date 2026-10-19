// Package upgrade는 대시보드에서 업그레이드(.claude/docs/upgrade.md)의 onmaru 쪽 파일을 다룬다. 업그레이드 서비스가 쓰는 상태
// 파일과 root 쪽 설정 파일을 읽고, 확인 요청을 onmaru의 상태 디렉터리에 쓴다. 적용 요청은 권한 도우미의
// upgrade.apply가 쓰므로 여기에 없다. 형식은 backend/config(upgrade.go, upgrade_request.go)에 있다.
package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"onmaru/backend/config"
)

const (
	// pendingStall이 지나도 확인 요청이 남아 있으면 업그레이드 서비스가 가져가지 않는 것으로 본다(경로 유닛은 바로
	// 서비스를 시작하고, 서비스는 가장 먼저 요청을 지운다).
	pendingStall = time.Minute
	// checkInterval은 확인 요청 사이의 최소 간격이다. 확인마다 저장소 소유자의 git fetch와 임시 빌드가 돈다.
	checkInterval = 30 * time.Second
	// 읽는 파일의 크기 상한. 업그레이드 서비스의 상한(installer의 maxRootFile, maxStatusFile, maxRecordFile)과 같다.
	maxEnvFile    = 64 << 10
	maxStatusFile = 4 << 20
	maxRecordFile = 64 << 10
)

// Paths는 다루는 파일의 경로다.
type Paths struct {
	// EnvFile은 root 쪽 설정 파일(config.UpgradeEnvFile)이다. 있으면 대시보드에서 업그레이드가 켜져 있다.
	EnvFile string
	// StatusFile은 업그레이드 서비스의 상태 파일(config.UpgradeStatusFile)이다.
	StatusFile string
	// RecordFile은 설치기가 설치할 때마다 쓰는 설치 기록(config.InstallRecordFile)이다.
	RecordFile string
	// CheckRequest는 확인 요청 파일(<상태 디렉터리>/config.UpgradeCheckRequestName)이다. 비우면 확인을 요청할 수 없다.
	CheckRequest string
}

// NewPaths는 설치된 경로다. root는 경로 앞에 붙이는 가짜 루트(개발 서버, 비우면 실제 경로)이고, stateDir는 onmaru의
// 상태 디렉터리다(비우면 확인 요청을 쓸 곳이 없다).
func NewPaths(root, stateDir string) Paths {
	p := Paths{EnvFile: root + config.UpgradeEnvFile, StatusFile: root + config.UpgradeStatusFile, RecordFile: root + config.InstallRecordFile}
	if stateDir != "" {
		p.CheckRequest = filepath.Join(stateDir, config.UpgradeCheckRequestName)
	}
	return p
}

// Snapshot은 한 번 읽은 상태다.
type Snapshot struct {
	// Enabled는 root 쪽 설정 파일이 있다는 뜻이다.
	Enabled bool
	// Repo, User는 설정 파일의 저장소 경로와 빌드 계정이다(수동 재설치 안내용). 읽지 못하면 빈 값이다.
	Repo, User string
	// Status는 상태 파일이다. 없거나, 읽을 수 없거나, 모르는 형식이면 nil이다.
	Status *config.UpgradeStatus
	// Installed는 설치 기록이다. 대시보드에서 업그레이드가 꺼져 있어도 설치기가 쓴다. 없거나, 읽을 수 없거나, 모르는
	// 형식이면 nil이다.
	Installed *config.InstallRecord
	// Pending은 확인 요청을 업그레이드 서비스가 아직 가져가지 않았다는 뜻이다. PendingStalled는 pendingStall보다
	// 오래 남아 있다는 뜻이다(경로 유닛이 돌지 않거나 상태 디렉터리가 경로 유닛이 보는 곳이 아니다).
	Pending, PendingStalled bool
	// Interrupted는 상태 파일이 확인·적용 중인 채로 config.UpgradeServiceTimeout보다 오래 바뀌지 않았다는 뜻이다.
	Interrupted bool
}

// Running은 상태 파일이 확인·적용 중이라는 뜻이다(중단된 실행 포함).
func (s Snapshot) Running() bool {
	return s.Status != nil && (s.Status.State == config.UpgradeStateChecking || s.Status.State == config.UpgradeStateApplying)
}

// Busy는 지금 새 확인·적용을 받지 않는다는 뜻이다. 멈춘 요청과 중단된 실행은 바쁘지 않다(다시 요청할 수 있다).
func (s Snapshot) Busy() bool {
	return (s.Pending && !s.PendingStalled) || (s.Running() && !s.Interrupted)
}

var (
	// ErrDisabled는 대시보드에서 업그레이드가 꺼져 있거나 확인 요청을 쓸 곳이 없다는 뜻이다.
	ErrDisabled = errors.New("upgrading from the dashboard is not enabled")
	// ErrBusy는 확인 요청이 남아 있거나 확인·적용이 진행 중이라는 뜻이다.
	ErrBusy = errors.New("an upgrade check or upgrade is in progress")
)

// RateLimitedError는 확인 요청이 너무 잦다는 뜻이다.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("upgrade check requested too often, retry after %v", e.RetryAfter)
}

// Manager는 업그레이드 파일을 읽고 확인 요청을 쓴다. 여러 고루틴에서 써도 된다.
type Manager struct {
	paths Paths
	now   func() time.Time

	mu sync.Mutex
	// last는 마지막으로 확인을 요청한 시각이다(메모리에만 둔다).
	last time.Time
}

// New는 Manager를 만든다. now가 nil이면 time.Now다.
func New(paths Paths, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{paths: paths, now: now}
}

// CanRequest는 확인 요청을 쓸 곳이 있는지다.
func (m *Manager) CanRequest() bool { return m.paths.CheckRequest != "" }

// Read는 지금 상태를 읽는다.
func (m *Manager) Read() Snapshot {
	var s Snapshot
	now := m.now()
	if data, err := readNoFollow(m.paths.EnvFile, maxEnvFile); err == nil {
		s.Enabled = true
		s.Repo, s.User = envValue(data, config.EnvUpgradeRepo), envValue(data, config.EnvUpgradeUser)
	} else if !errors.Is(err, fs.ErrNotExist) {
		// 있지만 읽을 수 없는 파일(권한, 크기)도 켜진 것으로 본다. 진행할 수 있는지는 서비스가 판단한다.
		s.Enabled = true
	}
	if data, err := readNoFollow(m.paths.StatusFile, maxStatusFile); err == nil {
		var st config.UpgradeStatus
		if json.Unmarshal(data, &st) == nil && st.Format == config.UpgradeStatusFormat {
			s.Status = &st
		}
	}
	if data, err := readNoFollow(m.paths.RecordFile, maxRecordFile); err == nil {
		var rec config.InstallRecord
		if json.Unmarshal(data, &rec) == nil && rec.Format == config.InstallRecordFormat {
			s.Installed = &rec
		}
	}
	if s.Running() {
		at, err := time.Parse(time.RFC3339, s.Status.UpdatedAt)
		s.Interrupted = err != nil || now.Sub(at) > config.UpgradeServiceTimeout
	}
	if m.paths.CheckRequest != "" {
		if fi, err := os.Lstat(m.paths.CheckRequest); err == nil {
			s.Pending = true
			s.PendingStalled = now.Sub(fi.ModTime()) > pendingStall
		}
	}
	return s
}

// RequestCheck는 확인 요청(mode는 config.UpgradeModeRemote 또는 UpgradeModeLocal)을 쓴다. 경로 유닛이 업그레이드
// 서비스를 실행한다. 꺼져 있으면 ErrDisabled, 바쁘면 ErrBusy, 너무 잦으면 *RateLimitedError다.
func (m *Manager) RequestCheck(mode string) error {
	if mode != config.UpgradeModeRemote && mode != config.UpgradeModeLocal {
		return fmt.Errorf("unknown check mode %q", mode)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.CanRequest() {
		return ErrDisabled
	}
	s := m.Read()
	switch {
	case !s.Enabled:
		return ErrDisabled
	case s.Busy():
		return ErrBusy
	}
	now := m.now()
	if wait := checkInterval - now.Sub(m.last); !m.last.IsZero() && wait > 0 {
		return &RateLimitedError{RetryAfter: wait}
	}
	data, err := json.Marshal(config.UpgradeCheckRequest{Format: config.UpgradeRequestFormat, Mode: mode})
	if err != nil {
		return err
	}
	if err := writeAtomic(m.paths.CheckRequest, data); err != nil {
		return err
	}
	m.last = now
	return nil
}

// writeAtomic은 같은 디렉터리의 임시 파일에 쓴 뒤 이름을 바꾼다(경로 유닛은 다 쓴 파일만 본다). 업그레이드 서비스는
// root로 읽으므로 권한은 0644다(상태 디렉터리 자체가 onmaru 전용이다).
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upgrade-check-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// readNoFollow는 일반 파일을 심볼릭 링크를 따라가지 않고 max 바이트까지 읽는다.
func readNoFollow(path string, max int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if fi.Size() > max {
		return nil, errors.New("file too large")
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}

// envValue는 환경 파일에서 KEY= 로 시작하는 마지막 줄의 값이다. 값 전체를 감싼 큰따옴표 한 쌍은 벗긴다(설치기의
// EnvFile.Get과 같다). source하지 않는다.
func envValue(data []byte, key string) string {
	v := ""
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSuffix(line, "\r"), key+"="); ok {
			v = rest
		}
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	return v
}
