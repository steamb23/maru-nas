// Package jobs는 서버 백그라운드 작업(시스템 업데이트의 확인·설치)을 기록하고 끝까지 지켜본다.
//
// 브라우저 없이 서버에서 끝까지 도는 작업만 다룬다. 작업을 실제로 시작하고 진행을 읽는 일은 Runner가 한다(지금은
// OMV 세션으로 Exec.getOutput을 읽는 omvjob). 이 패키지는 실행 방식과 무관하게 진행 기록(재시작해도 이어 간다),
// 출력, 사용자별 결과와 결과 알림을 다룬다. 기록에 Runner 이름과 불투명한 handle을 두므로, 나중에 root 서비스가
// 작업을 실행하고 상태 파일을 쓰는 방식은 Runner 하나를 더하는 일이다.
//
// 작업은 한 번에 하나다(종류와 사용자에 상관없이). 작업은 apt·dpkg 잠금을 함께 쓰기 때문이다.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
)

// Kind는 작업 종류다(api.JobAptUpdate 등).
type Kind string

const (
	KindAptUpdate  Kind = api.JobAptUpdate
	KindAptUpgrade Kind = api.JobAptUpgrade
)

// OMVMethod는 작업을 시작하는 OMV RPC다.
type OMVMethod struct{ Service, Method string }

// KindSpec은 작업 종류 하나의 설정이다. 새 종류는 Kinds에 한 줄 더한다(문구는 프론트).
type KindSpec struct {
	// Addon은 작업을 시작하는 애드온이다. 감시자의 OMV 호출은 그 애드온의 RPC 허용 목록을 따른다.
	Addon string
	// OMV는 OMV 세션으로 시작할 때 부르는 메서드다.
	OMV OMVMethod
	// MaxRun은 OMV가 계속 실행 중이라고 답해도 결과를 모름으로 보는 시간이다(멈춘 작업 대비).
	MaxRun time.Duration
}

// Kinds는 작업 종류 표다.
var Kinds = map[Kind]KindSpec{
	KindAptUpdate:  {Addon: "packages", OMV: OMVMethod{"Apt", "update"}, MaxRun: 30 * time.Minute},
	KindAptUpgrade: {Addon: "packages", OMV: OMVMethod{"Apt", "upgrade"}, MaxRun: 4 * time.Hour},
}

// OutputRange는 저장한 출력의 논리 위치다. 출력 파일에는 [Start, End) 바이트가 있다. 상한을 넘어 앞을 버리면
// Start가 커진다. API 위치(JobOutput.From·Next)는 이 논리 위치라 버려도 브라우저의 커서가 유지된다.
type OutputRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	// Truncated면 상한 때문에 앞부분을 버렸다.
	Truncated bool `json:"truncated,omitempty"`
	// Incomplete면 끝부분을 읽지 못했다(다른 곳에서 먼저 읽음).
	Incomplete bool `json:"incomplete,omitempty"`
}

// Record는 진행 중인 작업의 기록이다(상태 디렉터리의 jobs/current.json). 세션 ID는 넣지 않는다.
type Record struct {
	ID    string `json:"id"`
	Kind  Kind   `json:"kind"`
	Owner string `json:"owner"`
	// Runner는 작업을 시작한 Runner의 이름이다. 다시 시작한 뒤 같은 Runner가 이어 지켜본다.
	Runner string `json:"runner"`
	// Handle은 Runner가 이어 읽을 자리(예: OMV 상태 파일 이름과 위치)다. 이 패키지는 내용을 모른다.
	Handle json.RawMessage `json:"handle"`
	// BootID는 시작할 때의 부트 id다. 다르면 작업 중에 다시 시작된 것이다.
	BootID    string      `json:"bootId,omitempty"`
	StartedAt time.Time   `json:"startedAt"`
	Output    OutputRange `json:"output"`
	// Before는 시작할 때 업데이트 목록의 "이름@버전"이다. 설치 결과 요약에 쓴다.
	Before []string `json:"before,omitempty"`
}

// Result는 끝난 작업이다(상태 디렉터리의 jobs/results/<사용자>.json, 시작한 사용자별).
type Result struct {
	ID        string      `json:"id"`
	Kind      Kind        `json:"kind"`
	Owner     string      `json:"owner"`
	Runner    string      `json:"runner"`
	State     string      `json:"state"`
	Code      string      `json:"code,omitempty"`
	StartedAt time.Time   `json:"startedAt"`
	EndedAt   time.Time   `json:"endedAt"`
	Output    OutputRange `json:"output"`
	Count     *int        `json:"count,omitempty"`
	Items     []string    `json:"items,omitempty"`
	// Dismissed면 사용자가 결과 알림을 지웠다. 결과 목록에는 정리될 때까지 남는다.
	Dismissed bool `json:"dismissed,omitempty"`
}

// StartRequest는 작업을 시작하는 요청이다.
type StartRequest struct {
	// Owner는 시작한 사용자다. 결과를 이 사용자에게 남긴다.
	Owner string
	// SessionKey는 시작한 대시보드 세션이다(메모리에만, 감시자가 먼저 쓸 세션). 저장하지 않는다.
	SessionKey string
	// RPC는 시작한 세션의 OMV 호출기다(그 애드온의 쓰기를 부를 수 있다).
	RPC addons.RPCCaller
}

// Runner는 작업을 실제로 시작하고 끝까지 읽는다.
type Runner interface {
	// Name은 기록에 남는 이름이다. 바꾸지 않는다.
	Name() string
	// Start는 작업을 시작하고 이어 읽을 자리를 돌려준다. 응답을 잃어도 다시 시도하지 않는다(작업이 둘 생길 수 있다).
	// 시작하지 않고 거절했으면 ErrLocked·ErrConfigDirty 또는 그 밖의 오류다.
	Start(ctx context.Context, kind Kind, req StartRequest) (json.RawMessage, error)
	// Watch는 작업이 끝나거나 ctx가 끝날 때까지 출력을 읽어 sink에 넘긴다.
	Watch(ctx context.Context, job WatchJob, sink Sink) Outcome
}

// WatchJob은 Runner가 지켜볼 작업이다.
type WatchJob struct {
	ID        string
	Kind      Kind
	Handle    json.RawMessage
	StartedAt time.Time
	// Prefer는 먼저 쓸 대시보드 세션이다(시작한 세션, 다시 시작한 뒤에는 비어 있다).
	Prefer string
}

// Sink는 Runner가 진행을 알리는 곳이다.
type Sink interface {
	// Output은 새 출력과 그 뒤에 이어 읽을 자리를 함께 받는다(둘이 늘 같이 저장된다). chunk는 비어 있을 수 있다.
	Output(chunk []byte, handle json.RawMessage)
	// Waiting은 출력을 읽을 수단(관리자 세션)이 없어 기다리는지 알린다.
	Waiting(waiting bool)
	// Offers는 읽을 수단이 새로 생겼을 수 있을 때(관리자 요청) 신호를 받는 채널이다.
	Offers() <-chan struct{}
}

// Outcome은 Watch의 결과다. State가 비어 있으면 끝나지 않고 멈췄다(ctx 끝남).
type Outcome struct {
	State string
	Code  string
	// Incomplete면 끝부분 출력을 읽지 못했다.
	Incomplete bool
}

// Store는 작업 기록·결과·출력을 저장한다. prefs.FileStore(상태 디렉터리)와 MemoryStore가 구현한다.
type Store interface {
	// LoadCurrent는 진행 중 기록이다. 없으면 nil이다.
	LoadCurrent() (*Record, error)
	// SaveCurrent는 진행 중 기록을 바꾼다. nil이면 지운다.
	SaveCurrent(r *Record) error
	// LoadResults는 user의 결과다(최신 순). 없으면 빈 목록이다.
	LoadResults(user string) ([]Result, error)
	SaveResults(user string, rs []Result) error
	// AppendOutput은 작업 id의 출력 파일 끝에 b를 붙인다.
	AppendOutput(id string, b []byte) error
	// ReadOutput은 출력 파일의 off(파일 안 위치)부터 n바이트까지 읽는다.
	ReadOutput(id string, off int64, n int) ([]byte, error)
	// TruncateOutput은 출력 파일을 size바이트로 자른다(기록보다 긴 부분, 다시 받을 출력).
	TruncateOutput(id string, size int64) error
	// DropOutputHead는 출력 파일의 뒤 keep바이트 정도(줄 경계부터)만 남기고 버린 바이트 수를 돌려준다.
	DropOutputHead(id string, keep int64) (int64, error)
	DeleteOutput(id string) error
	// PruneOutputs는 before보다 오래된 출력 파일을 지운다(keep은 남긴다). 결과보다 오래 남은 파일을 정리한다.
	PruneOutputs(before time.Time, keep string) error
}

var (
	// ErrBusy는 작업이 이미 진행 중이라는 뜻이다.
	ErrBusy = errors.New("jobs: a job is already running")
	// ErrLocked는 다른 프로세스가 apt·dpkg 잠금을 쥐고 있어 OMV가 시작하지 않았다는 뜻이다.
	ErrLocked = errors.New("jobs: package database is locked")
	// ErrConfigDirty는 OMV에 적용하지 않은 apt 설정 변경이 있어 시작하지 않았다는 뜻이다.
	ErrConfigDirty = errors.New("jobs: configuration changes are pending")
	// ErrUnknownKind는 모르는 작업 종류다.
	ErrUnknownKind = errors.New("jobs: unknown job kind")
	// ErrNotFound는 그 작업(또는 지울 수 있는 결과 알림)이 없다는 뜻이다.
	ErrNotFound = errors.New("jobs: not found")
	// ErrNoRunner는 등록된 Runner가 없다는 뜻이다.
	ErrNoRunner = errors.New("jobs: no runner")
)

// idPattern은 작업 id 형식이다(UTC 시작 시각 + 무작위). 파일 이름과 알림 id에 그대로 쓴다.
var idPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{8}$`)

// ValidID는 작업 id 형식이 맞는지다.
func ValidID(id string) bool { return idPattern.MatchString(id) }

func newID(now time.Time) string {
	b := make([]byte, 4)
	rand.Read(b)
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// NoticeID는 작업 결과 알림의 id다.
func NoticeID(jobID string) string { return "job-" + jobID }
