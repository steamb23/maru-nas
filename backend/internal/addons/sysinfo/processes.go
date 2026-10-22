package sysinfo

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"os/user"
	"slices"
	"strconv"

	"maru/backend/internal/api"
)

// SourceProcesses는 프로세스 목록 provider 이름이다.
const SourceProcesses = "proc-processes"

// Processes는 /proc/<pid>를 읽어 프로세스 목록을 만드는 provider다(관리자 전용).
//
// /proc는 권한 없이 모든 PID가 보이지만(hidepid 없음), 명령줄에 비밀이 들어 있을 수 있으므로
// OMV(System.getTopInfo)와 같이 관리자에게만 준다.
type Processes struct {
	proc fs.FS
	// lookup은 uid를 사용자 이름으로 바꾼다. 모르면 uid를 그대로 돌려준다.
	lookup func(uid string) string
}

// NewProcesses는 호스트의 /proc를 읽고 사용자 이름을 os/user로 찾는 provider를 반환한다.
func NewProcesses() *Processes {
	return NewProcessesFS(os.DirFS("/proc"), lookupUser)
}

// NewProcessesFS는 proc의 "<pid>/stat", "<pid>/status", "<pid>/cmdline"을 읽는 provider를 반환한다.
func NewProcessesFS(proc fs.FS, lookup func(uid string) string) *Processes {
	return &Processes{proc: proc, lookup: lookup}
}

func lookupUser(uid string) string {
	u, err := user.LookupId(uid)
	if err != nil {
		return uid
	}
	return u.Username
}

// Name은 provider 이름이다.
func (*Processes) Name() string { return SourceProcesses }

// Requires는 provider 조건이다.
func (*Processes) Requires() api.Requirements { return api.Requirements{Admin: true} }

// List는 프로세스 목록을 PID 순서로 반환한다. 읽는 사이에 끝난 프로세스는 빼고,
// 하나도 읽지 못했을 때만 오류로 본다.
func (p *Processes) List() ([]api.Process, error) {
	entries, err := fs.ReadDir(p.proc, ".")
	if err != nil {
		return nil, err
	}
	users := make(map[string]string)
	list := make([]api.Process, 0, len(entries))
	var firstErr error
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 || !e.IsDir() {
			continue
		}
		proc, err := p.read(e.Name(), pid, users)
		if err != nil {
			// 끝난 프로세스는 파일이 사라지거나(ENOENT) 읽기가 실패한다(ESRCH). 어느 쪽이든 건너뛴다.
			if firstErr == nil && !errors.Is(err, fs.ErrNotExist) {
				firstErr = err
			}
			continue
		}
		list = append(list, proc)
	}
	if len(list) == 0 && firstErr != nil {
		return nil, firstErr
	}
	slices.SortFunc(list, func(a, b api.Process) int { return cmp.Compare(a.PID, b.PID) })
	return list, nil
}

func (p *Processes) read(dir string, pid int, users map[string]string) (api.Process, error) {
	st, err := readFile(p.proc, dir+"/stat", ParsePIDStat)
	if err != nil {
		return api.Process{}, err
	}
	status, err := readFile(p.proc, dir+"/status", ParsePIDStatus)
	if err != nil {
		return api.Process{}, err
	}
	var command string
	if !st.Kernel() {
		// 명령줄은 보조 정보다. 읽지 못해도 프로세스는 보여 준다.
		command, _ = readFile(p.proc, dir+"/cmdline", ParseCmdline)
	}
	name, ok := users[status.UID]
	if !ok {
		name = p.lookup(status.UID)
		users[status.UID] = name
	}
	return api.Process{
		PID:        pid,
		StartTicks: st.StartTicks,
		Name:       st.Comm,
		Command:    command,
		User:       name,
		State:      st.State,
		CPUTicks:   st.CPUTicks,
		RSSBytes:   status.RSSBytes,
		Threads:    st.Threads,
		Kernel:     st.Kernel(),
	}, nil
}
