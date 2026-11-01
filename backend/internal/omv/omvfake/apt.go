package omvfake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strings"
)

// APT 업데이트. engined/rpc/apt.inc(OMV 8.0~8.5에서 메서드와 스키마가 같다).
//
//   - Apt.update (admin, 파라미터 없음): apt 모듈이 적용 대기면 ConfigDirtyException("Please apply the configuration
//     changes first."), dpkg·apt 잠금이 잡혀 있으면 AssertException("…is held by another process. Please try again
//     later."). 아니면 apt-get update를 백그라운드 작업으로 돌리고 상태 파일 이름을 돌려준다
//   - Apt.upgrade (admin, 파라미터 없음): 잠금만 확인하고 apt-get dist-upgrade를 백그라운드 작업으로 돌린다. 8.5.10부터는
//     작업 안에서 먼저 시뮬레이션을 돌려 openmediavault를 지우게 되면 작업을 실패시킨다
//   - Apt.getChangeLog (admin, {filename}): 파일 이름의 기본 이름을 "_"로 나눈 첫 조각을 패키지 이름으로 보고,
//     .deb가 apt 캐시에 없으면 apt-get --download-only로 받은 뒤(출력에 넣지 않음) 변경 기록을 풀어 출력으로
//     준다. 받지 못하면 "Package file '<filename>' not found.", 변경 기록이 없으면 "No changelog found."로 실패한다
//
// 작업이 실패하면 OMV ExecException 문구("Failed to execute command '…' with exit code '100': …")로 끝난다.

// 잠금·적용 대기·작업 실패 문구(OMV 소스와 같다).
const (
	AptLockedMessage      = "The package database is held by another process. Please try again later."
	AptDirtyMessage       = "Please apply the configuration changes first."
	AptCoreRemovalMessage = "Aborting, the operation would remove the 'openmediavault' package itself."
	AptExecFailedMessage  = "Failed to execute command 'export LC_ALL=C.UTF-8; apt-get --yes dist-upgrade 2>&1' with exit code '100': E: Sub-process /usr/bin/dpkg returned an error code (1)"
)

// AptJob은 가짜 Apt.update·upgrade 작업 하나의 설정이다.
type AptJob struct {
	// Chunks는 폴링마다 하나씩 출력에 붙는 조각이다.
	Chunks []string
	// Fail이 비어 있지 않으면 조각을 다 붙인 뒤 이 메시지로 실패한다.
	Fail string
	// OnDone은 성공해 끝날 때 부른다(가짜 OMV의 잠금을 쥔 채로). 개발 서버가 가짜 업데이트 목록을 고칠 때 쓴다.
	OnDone func()
}

// AptChangelog은 가짜 Apt.getChangeLog의 패키지 하나다.
type AptChangelog struct {
	// Text는 변경 기록이다. 비어 있으면 "No changelog found."로 실패한다.
	Text string
	// Download는 내려받기를 흉내 내는 폴링 수다. 그동안 출력 없이 진행 중이다.
	Download int
	// Fail이 비어 있지 않으면 내려받은 뒤 이 메시지로 실패한다.
	Fail string
}

// 변경 기록 실패 문구(OMV 소스와 같다).
const (
	AptNoChangelogMessage = "No changelog found."
	// AptPackageFileMessage는 내려받지 못한 패키지 파일이다(%s는 요청한 파일 이름).
	AptPackageFileMessage = "Package file '%s' not found."
)

// AptConfig는 가짜 Apt 서비스의 설정이다. SetAptConfig로 바꾼다.
type AptConfig struct {
	// Locked면 update·upgrade 모두 잠금 오류로 거절한다.
	Locked  bool
	Update  AptJob
	Upgrade AptJob
	// Changelogs는 패키지 이름마다의 변경 기록이다. 없는 패키지는 내려받기 실패다.
	Changelogs map[string]AptChangelog
}

// DefaultAptConfig는 개발 서버와 테스트가 쓰는 합성 출력이다.
func DefaultAptConfig() AptConfig {
	return AptConfig{
		Update: AptJob{Chunks: []string{
			"Hit:1 http://deb.example.com/debian trixie InRelease\n",
			"Get:2 http://security.example.com/debian-security trixie-security InRelease [43.4 kB]\n",
			"Get:3 http://packages.example.com/public synchrony InRelease [6,342 B]\nFetched 49.7 kB in 1s (52.3 kB/s)\n",
			"Reading package lists...\n",
		}},
		Upgrade: AptJob{Chunks: []string{
			"Reading package lists...\nBuilding dependency tree...\nReading state information...\nCalculating upgrade...\n",
			"The following packages will be upgraded:\n  libc6 openmediavault\n2 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.\n",
			"Get:1 http://deb.example.com/debian trixie/main amd64 libc6 amd64 2.41-99 [2,867 kB]\n",
			"Get:2 http://packages.example.com/public synchrony/main all openmediavault all 8.5.12-1 [4,096 kB]\n",
			"Preparing to unpack .../libc6_2.41-99_amd64.deb ...\nUnpacking libc6:amd64 (2.41-99) over (2.41-12) ...\n",
			"Setting up libc6:amd64 (2.41-99) ...\nSetting up openmediavault (8.5.12-1) ...\n",
			"Processing triggers for libc-bin (2.41-99) ...\n",
		}},
	}
}

// SetAptConfig는 가짜 Apt 서비스의 설정을 바꾼다.
func (s *Server) SetAptConfig(c AptConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apt = c
}

// aptJob은 Apt.update·upgrade를 처리한다. s.mu를 쥔 채로 부른다.
func (s *Server) aptJob(method string) (any, *rpcError) {
	job := s.apt.Upgrade
	if method == "update" {
		if slices.Contains(s.plugin.Dirty, "apt") {
			return nil, errStatus(http.StatusInternalServerError, AptDirtyMessage)
		}
		job = s.apt.Update
	}
	if s.apt.Locked {
		return nil, errStatus(http.StatusInternalServerError, AptLockedMessage)
	}
	return s.startJob(&bgJob{chunks: slices.Clone(job.Chunks), fail: job.Fail, onDone: job.OnDone}), nil
}

// aptChangelog은 Apt.getChangeLog를 처리한다. s.mu를 쥔 채로 부른다.
func (s *Server) aptChangelog(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Filename *string `json:"filename"`
	}
	if json.Unmarshal(params, &p) != nil || p.Filename == nil {
		return nil, errStatus(http.StatusInternalServerError, "Invalid RPC parameters.")
	}
	base := path.Base(*p.Filename)
	name, _, _ := strings.Cut(strings.TrimSuffix(base, ".deb"), "_")
	c, ok := s.apt.Changelogs[name]
	job := &bgJob{chunks: make([]string, c.Download)}
	switch {
	case !ok:
		job.fail = fmt.Sprintf(AptPackageFileMessage, *p.Filename)
	case c.Fail != "":
		job.fail = c.Fail
	case c.Text == "":
		job.fail = AptNoChangelogMessage
	default:
		job.chunks = append(job.chunks, c.Text)
	}
	return s.startJob(job), nil
}
