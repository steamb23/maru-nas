package api

// 서버 백그라운드 작업 종류. Job.Kind에 들어간다.
const (
	// JobAptUpdate는 패키지 목록을 다시 받는 작업이다(OMV Apt.update, apt-get update).
	JobAptUpdate = "apt-update"
	// JobAptUpgrade는 모든 업데이트를 설치하는 작업이다(OMV Apt.upgrade, apt-get dist-upgrade).
	JobAptUpgrade = "apt-upgrade"
)

// 작업 상태. Job.State에 들어간다.
const (
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	// JobUnknown은 작업이 어떻게 끝났는지 알 수 없다는 뜻이다(Code에 이유).
	JobUnknown = "unknown"
)

// 작업 결과 코드. Job.Code에 들어간다.
const (
	// JobCodeAptFailed는 apt가 오류로 끝났다는 뜻이다(출력에 이유가 있다).
	JobCodeAptFailed = "apt_failed"
	// JobCodeAptCoreRemoval은 openmediavault 패키지를 지우게 되어 OMV가 설치를 멈췄다는 뜻이다(OMV 8.5.10부터).
	JobCodeAptCoreRemoval = "apt_core_removal"
	// JobCodeFailed는 그 밖의 이유로 작업이 실패했다는 뜻이다.
	JobCodeFailed = "job_failed"
	// JobCodeLost는 결과를 읽지 못했다는 뜻이다(다른 곳에서 먼저 읽음, 상태 파일이 사라짐).
	JobCodeLost = "job_lost"
	// JobCodeReboot는 작업 중에 시스템이 다시 시작되었다는 뜻이다.
	JobCodeReboot = "job_reboot"
	// JobCodeTimeout은 작업이 시간 상한을 넘었다는 뜻이다.
	JobCodeTimeout = "job_timeout"
)

// Job은 서버 백그라운드 작업 하나다(진행 중이거나 끝난 결과).
type Job struct {
	ID    string `json:"id"`
	Kind  string `json:"kind" tstype:"'apt-update' | 'apt-upgrade'"`
	State string `json:"state" tstype:"'running' | 'succeeded' | 'failed' | 'unknown'"`
	Code  string `json:"code,omitempty" tstype:"'apt_failed' | 'apt_core_removal' | 'job_failed' | 'job_lost' | 'job_reboot' | 'job_timeout'"`
	// Owner는 작업을 시작한 사용자다. Mine은 그 사용자가 이 세션의 사용자인지다.
	Owner string `json:"owner"`
	Mine  bool   `json:"mine"`
	// StartedAt·EndedAt은 RFC 3339다. 진행 중이면 EndedAt이 없다.
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt,omitempty"`
	// Waiting이면 OMV에서는 작업이 계속 돌지만 대시보드가 출력을 읽을 관리자 세션이 없어 기다리는 중이다.
	Waiting bool `json:"waiting,omitempty"`
	// LastLine은 진행 중 작업의 마지막 출력 한 줄이다(알림 센터의 백그라운드 작업). 끝난 작업은 없다.
	LastLine string `json:"lastLine,omitempty"`
	// OutputEnd는 지금까지 받은 출력의 끝 위치다(JobOutput.Next와 같은 단위).
	OutputEnd int64 `json:"outputEnd"`
	// OutputIncomplete면 출력 일부가 없다(앞부분을 상한 때문에 버렸거나 끝부분을 읽지 못했다).
	OutputIncomplete bool `json:"outputIncomplete,omitempty"`
	// Count·Items는 끝난 작업의 요약이다(확인: 찾은 업데이트, 설치: 업데이트한 패키지).
	Count *int     `json:"count,omitempty"`
	Items []string `json:"items,omitempty"`
	// Dismissed면 사용자가 결과 알림을 지웠다.
	Dismissed bool `json:"dismissed,omitempty"`
}

// JobsResponse는 GET /api/jobs의 응답이다.
type JobsResponse struct {
	// Current는 진행 중인 작업이다(누가 시작했든). 없으면 빠진다.
	Current *Job `json:"current,omitempty"`
	// Recent는 이 사용자가 시작한 작업의 결과다(최신 순).
	Recent []Job `json:"recent"`
	// Last는 마지막으로 끝난 작업이다(누가 시작했든, 서버 메모리에만). 다른 관리자가 시작한 작업이 끝난 뒤에도
	// 그 결과를 보이는 데 쓴다. 다시 시작하면 없다.
	Last *Job `json:"last,omitempty"`
}

// JobOutput은 GET /api/jobs/{id}/output?from=N의 응답이다.
type JobOutput struct {
	// From은 Text가 시작하는 위치다. 요청한 위치보다 크면 그 사이는 상한 때문에 버린 출력이다.
	From int64 `json:"from"`
	// Next는 다음 요청의 from이다.
	Next int64  `json:"next"`
	Text string `json:"text"`
	// More면 이미 받은 출력이 더 있다(바로 다시 요청한다).
	More    bool   `json:"more"`
	State   string `json:"state" tstype:"'running' | 'succeeded' | 'failed' | 'unknown'"`
	Waiting bool   `json:"waiting,omitempty"`
}
