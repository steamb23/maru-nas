package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"onmaru/backend/internal/helper/proto"
	"onmaru/backend/internal/helper/worker"
)

// Credential은 worker를 실행할 사용자 권한이다. nil이면 root다.
type Credential struct {
	UID    int
	GID    int
	Groups []int
}

// Spawner는 worker를 실행한다.
type Spawner interface {
	// Spawn은 worker를 cred 권한으로 실행하고 conn을 넘긴 뒤 끝날 때까지 기다린다.
	// started가 true면 worker가 응답을 썼을 수 있으므로 부모는 conn에 더 쓰지 않는다.
	// code는 연산 결과 코드이고 성공이면 빈 문자열이다. err는 로그용 세부다.
	Spawn(ctx context.Context, cred *Credential, job worker.Job, conn *net.UnixConn) (code string, started bool, err error)
}

// ExecSpawner는 자기 실행 파일을 `worker` 하위 명령으로 실행한다. root로 돌아야 사용자 권한으로 강등할 수 있다.
type ExecSpawner struct {
	// Path는 실행 파일 경로다. 비우면 /proc/self/exe(업그레이드로 파일이 바뀌어도 지금 실행 중인 것).
	Path string
	// noSetGroups는 보조 그룹을 바꾸지 않는다. root 없이 실행 경로를 시험할 때만 쓴다.
	noSetGroups bool
}

// workerStderr는 worker stderr를 받아 두는 최대 바이트다.
const workerStderr = 4096

func (s ExecSpawner) Spawn(ctx context.Context, cred *Credential, job worker.Job, conn *net.UnixConn) (string, bool, error) {
	path := s.Path
	if path == "" {
		path = "/proc/self/exe"
	}
	body, err := json.Marshal(job)
	if err != nil {
		return "", false, err
	}
	// 연결의 복제본을 fd 3으로 넘긴다.
	f, err := conn.File()
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	cmd := exec.CommandContext(ctx, path, "worker")
	cmd.Env = []string{}
	cmd.Dir = "/"
	cmd.Stdin = bytes.NewReader(body)
	stderr := &limitedBuffer{max: workerStderr}
	cmd.Stderr = stderr
	cmd.ExtraFiles = []*os.File{f}
	cmd.WaitDelay = 5 * time.Second
	attr := &syscall.SysProcAttr{Setsid: true, Pdeathsig: syscall.SIGKILL}
	if cred != nil {
		groups := make([]uint32, len(cred.Groups))
		for i, g := range cred.Groups {
			groups[i] = uint32(g)
		}
		attr.Credential = &syscall.Credential{Uid: uint32(cred.UID), Gid: uint32(cred.GID), Groups: groups, NoSetGroups: s.noSetGroups}
	} else {
		// root 범위도 도우미가 가진 보조 그룹은 넘기지 않는다.
		attr.Credential = &syscall.Credential{Uid: 0, Gid: 0, Groups: []uint32{}}
	}
	cmd.SysProcAttr = attr
	if err := cmd.Start(); err != nil {
		return "", false, err
	}
	f.Close()
	err = cmd.Wait()
	if err == nil {
		return "", true, nil
	}
	if ctx.Err() != nil {
		return proto.CodeTimeout, true, fmt.Errorf("worker killed: %w", ctx.Err())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 2 {
		if code := parseCode(stderr.String()); code != "" {
			return code, true, nil
		}
	}
	return proto.CodeInternal, true, fmt.Errorf("worker: %v: %s", err, strings.TrimSpace(stderr.String()))
}

// parseCode는 worker stderr의 `code=<코드>` 줄을 읽는다.
func parseCode(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if c, ok := strings.CutPrefix(line, "code="); ok {
			return strings.TrimSpace(c)
		}
	}
	return ""
}

// limitedBuffer는 max 바이트까지만 받는 버퍼다. 나머지는 버린다.
type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.max - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
