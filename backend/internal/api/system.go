package api

// HealthResponse는 GET /api/health의 응답이다.
type HealthResponse struct {
	Status string `json:"status"`
}

// SystemResponse는 GET /api/system의 응답이다.
type SystemResponse struct {
	CPU           CPUInfo    `json:"cpu"`
	Memory        MemoryInfo `json:"memory"`
	UptimeSeconds float64    `json:"uptimeSeconds"`
}

// CPUInfo는 전체 코어를 합산한 CPU 사용률이다.
type CPUInfo struct {
	// UsagePercent는 두 /proc/stat 샘플 사이에서 측정한 0~100 값이다.
	UsagePercent float64 `json:"usagePercent"`
}

// MemoryInfo는 바이트 단위 RAM 수치다. Used = Total - Available.
type MemoryInfo struct {
	TotalBytes     uint64 `json:"totalBytes"`
	UsedBytes      uint64 `json:"usedBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
}

// SystemStatsResponse는 GET /api/addons/system-info의 응답이다(시스템 정보 창).
// 사용률·속도는 누적 카운터로 주고, 프론트가 같은 백엔드의 두 응답 차이로 계산한다.
// 그래서 창과 다른 세션의 요청이 서로의 측정 구간에 영향을 주지 않는다(/api/system과 다른 점).
type SystemStatsResponse struct {
	AddonSources `tstype:",extends"`
	// SampledAtMs는 값을 읽은 시각이다. 백엔드 프로세스 시작 기준의 단조 시각(밀리초)이며,
	// 같은 백엔드의 두 응답 차이를 구하는 데만 쓴다(벽시계 시각이 아니다).
	SampledAtMs int64       `json:"sampledAtMs"`
	CPU         CPUCounters `json:"cpu"`
	Memory      MemoryInfo  `json:"memory"`
	// MemoryDetail은 메모리 구성과 스왑이다(성능 페이지의 메모리 구성 막대).
	MemoryDetail  MemoryDetail `json:"memoryDetail"`
	UptimeSeconds float64      `json:"uptimeSeconds"`
	// Load는 부하 평균이다. 읽지 못하면 없다.
	Load *LoadAverage `json:"load,omitempty"`
	// Network는 물리 인터페이스의 누적 바이트 합계다. 읽지 못하면 없다.
	Network *NetTotals `json:"network,omitempty"`
	// Disk는 물리 디스크의 누적 바이트 합계다. 읽지 못하면 없다.
	Disk *DiskTotals `json:"disk,omitempty"`
	// Tasks는 프로세스·스레드 수다. 읽지 못하면 없다. 수만 담으므로 누구에게나 준다.
	Tasks *TaskCounts `json:"tasks,omitempty"`
	// Processes는 프로세스 목록이다(PID 순서). 관리자가 ?processes=1로 요청할 때만 준다(provider proc-processes).
	Processes []Process `json:"processes,omitempty"`
	// Power는 OMV 전원 관리의 절전 방식이다. 관리자가 ?power=1로 요청할 때만 준다(provider omv-power).
	Power *PowerInfo `json:"power,omitempty"`
}

// PowerInfo는 OMV 전원 관리의 대기 방식이다(PowerMgmt.get, PowerMgmt.enumerateStandbyModes).
// 전원 모달은 이 값으로 절전(System.standby)을 쓸 수 있는지 정한다.
type PowerInfo struct {
	// StandbyMode는 설정된 대기 방식이다(poweroff, suspend, hibernate, suspendhybrid).
	StandbyMode string `json:"standbyMode"`
	// StandbyModes는 이 기기가 지원하는 대기 방식이다. poweroff는 늘 들어 있다.
	StandbyModes []string `json:"standbyModes"`
}

// TaskCounts는 프로세스와 스레드(커널 스레드 포함) 수다.
type TaskCounts struct {
	// Processes는 /proc의 PID 디렉터리 수다.
	Processes int `json:"processes"`
	// Threads는 /proc/loadavg의 스케줄링 단위 수다.
	Threads int `json:"threads"`
}

// CPUCounters는 /proc/stat의 누적 CPU 시간이다. 단위는 clock tick(USER_HZ)이다.
type CPUCounters struct {
	// TotalTicks는 모든 코어를 합한 시간이다(guest 제외). Process.CPUTicks와 단위가 같다.
	TotalTicks uint64 `json:"totalTicks"`
	// IdleTicks는 idle + iowait다.
	IdleTicks uint64 `json:"idleTicks"`
	// KernelTicks는 커널 시간(system + irq + softirq)이다. steal은 넣지 않는다.
	KernelTicks uint64 `json:"kernelTicks"`
	// Cores는 켜져 있는 코어별 값이다(cpuN 순서). 꺼진 코어는 빠지므로 ID로 맞춘다.
	Cores []CoreTicks `json:"cores"`
}

// CoreTicks는 코어 하나의 누적 CPU 시간이다.
type CoreTicks struct {
	// ID는 cpuN의 N이다.
	ID          int    `json:"id"`
	TotalTicks  uint64 `json:"totalTicks"`
	IdleTicks   uint64 `json:"idleTicks"`
	KernelTicks uint64 `json:"kernelTicks"`
}

// MemoryDetail은 RAM 구성(htop 방식)과 스왑이다. 단위는 바이트다.
// Apps + Shared + Compressed + Buffers + Cache + Free = MemoryInfo.TotalBytes.
type MemoryDetail struct {
	// AppsBytes는 나머지 항목을 뺀 메모리다. 프로세스와 회수할 수 없는 커널 메모리다.
	AppsBytes uint64 `json:"appsBytes"`
	// SharedBytes는 공유 메모리(Shmem, tmpfs 포함)다.
	SharedBytes uint64 `json:"sharedBytes"`
	// CompressedBytes는 zswap 풀과 zram 장치가 쓰는 메모리다.
	CompressedBytes uint64 `json:"compressedBytes"`
	// BuffersBytes는 블록 장치 버퍼(Buffers)다.
	BuffersBytes uint64 `json:"buffersBytes"`
	// CacheBytes는 페이지 캐시와 회수할 수 있는 slab(Cached + SReclaimable - Shmem)이다.
	CacheBytes uint64 `json:"cacheBytes"`
	// FreeBytes는 쓰지 않는 메모리(MemFree)다.
	FreeBytes      uint64 `json:"freeBytes"`
	SwapTotalBytes uint64 `json:"swapTotalBytes"`
	SwapUsedBytes  uint64 `json:"swapUsedBytes"`
}

// LoadAverage는 /proc/loadavg의 부하 평균이다.
type LoadAverage struct {
	// One, Five, Fifteen은 1·5·15분 부하 평균이다.
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
	// Running은 지금 실행할 수 있는 스케줄링 단위(스레드) 수다.
	Running int `json:"running"`
}

// NetTotals는 물리 네트워크 인터페이스의 누적 바이트 합계다.
type NetTotals struct {
	RxBytes uint64 `json:"rxBytes"`
	TxBytes uint64 `json:"txBytes"`
}

// DiskTotals는 물리 디스크의 누적 바이트 합계다. 파티션·RAID·LVM 장치는 빼서 이중으로 세지 않는다.
type DiskTotals struct {
	ReadBytes  uint64 `json:"readBytes"`
	WriteBytes uint64 `json:"writeBytes"`
}

// Process는 프로세스 하나다.
type Process struct {
	PID int `json:"pid"`
	// StartTicks는 부팅 후 시작 시각(clock tick)이다. PID가 다시 쓰였는지 구분하는 데 쓴다.
	StartTicks uint64 `json:"startTicks"`
	// Name은 커널이 아는 이름(comm)이다. 15바이트로 잘려 있을 수 있다.
	Name string `json:"name"`
	// Command는 명령줄이다(인자를 공백으로 이음, 길면 잘림). 커널 스레드와 좀비는 없다.
	Command string `json:"command,omitempty"`
	// User는 실제 사용자 이름이다. 이름을 모르면 uid 숫자다.
	User string `json:"user"`
	// State는 커널 상태 글자다(R, S, D, Z, T, I 등).
	State string `json:"state"`
	// CPUTicks는 누적 CPU 시간(utime + stime, clock tick)이다.
	CPUTicks uint64 `json:"cpuTicks"`
	// RSSBytes는 실제 메모리 사용량(VmRSS)이다.
	RSSBytes uint64 `json:"rssBytes"`
	Threads  int    `json:"threads"`
	// Kernel은 커널 스레드인지다.
	Kernel bool `json:"kernel,omitempty"`
}

// AboutResponse는 GET /api/about의 응답이다. 설정 앱의 정보 페이지가 보여 준다.
type AboutResponse struct {
	// Version은 마루 NAS 버전이다.
	Version string `json:"version"`
	// Revision은 빌드한 git 커밋(전체 해시)이다. go run이나 저장소 밖 빌드에는 없다.
	Revision string `json:"revision,omitempty"`
	// RevisionTime은 그 커밋의 시각(RFC 3339)이다.
	RevisionTime string `json:"revisionTime,omitempty"`
	// Modified는 커밋하지 않은 변경이 있는 작업 트리에서 빌드했는지다.
	Modified bool `json:"modified,omitempty"`
	// GoVersion은 빌드한 Go 버전이다.
	GoVersion string `json:"goVersion"`
	// OMVVersion은 설치된 openmediavault 패키지 버전이다(예: 8.0.0-1). 관리자에게만 주며, 읽지 못하면 없다.
	OMVVersion string `json:"omvVersion,omitempty"`
	// OMVVersionName은 OMV 릴리스 코드명이다(예: Synchrony). 관리자에게만 주며, 읽지 못하면 없다.
	OMVVersionName string `json:"omvVersionName,omitempty"`
}
