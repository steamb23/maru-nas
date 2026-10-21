// Package api는 HTTP API가 주고받는 JSON 형태를 정의한다.
// 이 패키지에서 tygo로 TypeScript 타입을 생성한다(tygo.yaml 참고).
package api

import (
	"encoding/json"

	"onmaru/backend/config"
)

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

// Manifest는 애드온 하나의 선언이다. 백엔드 정적 목록(internal/addons/builtin)이 원본이며,
// 프론트 목록(frontend/src/addons/manifests.gen.ts)은 여기서 생성한다.
type Manifest struct {
	// ID는 고유 이름이다. 창·위젯 종류 식별자와 같다. 소문자, 숫자, '-'만 쓴다.
	ID string `json:"id"`
	// Title은 창 제목·위젯 이름의 번역 키다(프론트 en.json의 키).
	Title string `json:"title"`
	// Icon은 freedesktop 아이콘 이름이다. 프론트 리졸버가 해석한다.
	Icon     string       `json:"icon"`
	Views    AddonViews   `json:"views"`
	Requires Requirements `json:"requires"`
	RPC      RPCMethods   `json:"rpc"`
	// Actions는 상태를 바꾸는 조작이다(POST /api/addons/{id}/actions/{name}). 구현은 addons.Addon.Actions에 있다.
	Actions []ActionDecl `json:"actions,omitempty"`
}

// ActionDecl은 애드온이 제공하는 조작 하나의 선언이다.
type ActionDecl struct {
	// Name은 조작 이름이다(start, stop 등). 소문자, 숫자, '-'만 쓴다.
	Name string `json:"name"`
	// Admin은 OMV 관리자만 실행할 수 있는지다. 애드온의 requires.admin과 따로 정한다
	// (목록은 누구나 보고 조작은 관리자만 하는 경우).
	Admin bool `json:"admin,omitempty"`
	// Confirm은 실행 전에 사용자 확인을 받아야 하는지다(정지, 끄기 등).
	Confirm bool `json:"confirm,omitempty"`
}

// AddonViews는 애드온의 보기 목록이다. 하나 이상 있어야 한다.
type AddonViews struct {
	Window *WindowView `json:"window,omitempty"`
	Widget *WidgetView `json:"widget,omitempty"`
}

// WindowView는 창 보기의 크기(픽셀)다.
type WindowView struct {
	DefaultSize Size `json:"defaultSize"`
	// MinSize는 크기 조절로 줄일 수 있는 최소 크기다. 없으면 프론트의 공통 하한을 쓴다.
	MinSize *Size `json:"minSize,omitempty"`
}

// Size는 픽셀 단위 크기다.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// WidgetView는 바탕화면 위젯 보기의 크기(그리드 칸)다.
type WidgetView struct {
	DefaultSize GridSize `json:"defaultSize"`
	// MinSize, MaxSize는 크기 조절 범위다. 없으면 기본 크기로 고정된다.
	MinSize *GridSize `json:"minSize,omitempty"`
	MaxSize *GridSize `json:"maxSize,omitempty"`
}

// GridSize는 바탕화면 위젯 그리드의 칸 단위 크기다.
type GridSize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// Requirements는 애드온이나 provider를 쓰는 데 필요한 조건이다.
type Requirements struct {
	// Plugins는 필요한 OMV 플러그인 패키지 이름이다(예: openmediavault-compose).
	Plugins []string `json:"plugins,omitempty"`
	// Admin은 OMV 관리자만 쓸 수 있는지다.
	Admin bool `json:"admin,omitempty"`
}

// RPCMethods는 애드온이 호출하는 OMV RPC 메서드(Service.method)다.
// 백엔드의 OMV RPC 중계는 등록된 애드온이 선언한 메서드만 통과시킨다.
type RPCMethods struct {
	Read []string `json:"read,omitempty"`
	// Write는 상태를 바꾸는 메서드다. 애드온의 조작(Actions)만 부를 수 있고 /api/rpc 중계는 거부한다.
	// 쓰기 허용 여부는 addons.WritesEnabled가 정한다.
	Write []string `json:"write,omitempty"`
}

// capability를 쓸 수 없는 이유. AddonCapability.Reason에 들어간다.
const (
	ReasonPluginMissing = "plugin_missing"
	ReasonAdminRequired = "admin_required"
)

// CapabilitiesResponse는 GET /api/capabilities의 응답이다. 플러그인 설치 상태와
// 로그인한 사용자의 역할로 계산한다.
type CapabilitiesResponse struct {
	// Addons는 등록된 모든 애드온의 capability다(등록 순서).
	Addons []AddonCapability `json:"addons"`
	// Plugins는 설치된 것으로 감지한, 대시보드가 아는 OMV 플러그인 패키지 이름이다(정렬).
	Plugins []string `json:"plugins"`
	// WorkbenchURL은 브라우저에서 OMV 웹 UI(workbench)를 열 주소다(-workbench-url). 비어 있으면
	// 설정하지 않은 것이다. http(s) URL이거나 대시보드 문서 기준의 경로 참조(/, ../)이며 해석은 프론트엔드가 한다.
	WorkbenchURL string `json:"workbenchUrl,omitempty"`
	// Helper는 권한 도우미를 설정했는지다(-helper-socket). 이 세션이 쓸 수 있는지는 GET /api/helper로 안다.
	Helper bool `json:"helper"`
	// Terminal은 웹 터미널을 쓸 수 있는지다(-terminal, 설정에서 덮어쓴 값). 실제로 들어갈 수 있는지는 sshd가 정한다.
	Terminal bool `json:"terminal"`
}

// AddonCapability는 애드온 하나를 이 사용자가 쓸 수 있는지다.
type AddonCapability struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
	// Reason은 쓸 수 없는 이유다. Available이면 없다.
	Reason string `json:"reason,omitempty" tstype:"'plugin_missing' | 'admin_required'"`
	// Sources는 쓸 수 있는 provider 이름이다(우선순위 순서).
	Sources []string `json:"sources"`
}

// RPCRequest는 POST /api/rpc의 요청 본문이다. 백엔드는 등록된 애드온이 선언한
// 메서드만 OMV rpc.php로 중계하고, 성공하면 OMV 응답의 response 값을 그대로 돌려준다.
type RPCRequest struct {
	Service string `json:"service"`
	Method  string `json:"method"`
	// Params는 OMV RPC 파라미터다. 없으면 빈 객체로 보낸다.
	Params json.RawMessage `json:"params,omitempty" tstype:"{ [key: string]: unknown }"`
}

// ActionRequest는 POST /api/addons/{id}/actions/{action}의 요청 본문이다. 성공하면 204다.
type ActionRequest struct {
	// Target은 조작 대상이다(컨테이너 이름, 서비스 이름 등). 형식은 애드온이 검사한다.
	Target string `json:"target"`
}

// AddonSources는 애드온 데이터 응답(GET /api/addons/{id})에 공통으로 담는 출처 정보다.
type AddonSources struct {
	// Sources는 이번 응답에 값을 보탠 provider 이름이다(우선순위 순서).
	Sources []string `json:"sources"`
	// FailedSources는 쓸 수 있었지만 이번에 실패한 provider 이름이다. 나머지 출처의 값은 그대로 담는다.
	FailedSources []string `json:"failedSources,omitempty"`
}

// StorageResponse는 GET /api/addons/storage의 응답이다.
type StorageResponse struct {
	AddonSources `tstype:",extends"`
	// Filesystems는 로컬 파일시스템이다(마운트 경로 순서).
	Filesystems []Filesystem `json:"filesystems"`
}

// Filesystem은 마운트된 파일시스템 하나의 용량이다. 바이트 단위다.
type Filesystem struct {
	// Device는 장치 경로(/dev/sda1 등)나 풀 이름이다.
	Device string `json:"device"`
	// Label은 파일시스템 라벨이다. 없으면 비어 있다.
	Label      string `json:"label,omitempty"`
	MountPoint string `json:"mountPoint"`
	Type       string `json:"type"`
	TotalBytes uint64 `json:"totalBytes"`
	// UsedBytes = 전체 - 남은 공간(root 예약분 포함).
	UsedBytes uint64 `json:"usedBytes"`
	// AvailableBytes는 일반 사용자가 쓸 수 있는 남은 공간이다(root 예약분 제외).
	AvailableBytes uint64 `json:"availableBytes"`
	ReadOnly       bool   `json:"readOnly,omitempty"`
}

// NetworkResponse는 GET /api/addons/network의 응답이다.
type NetworkResponse struct {
	AddonSources `tstype:",extends"`
	// SampledAtMs는 카운터를 읽은 시각이다. 백엔드 프로세스 시작 기준의 단조 시각(밀리초)이며,
	// 같은 백엔드의 두 응답 차이로 속도를 계산하는 데만 쓴다(벽시계 시각이 아니다).
	SampledAtMs int64 `json:"sampledAtMs"`
	// Interfaces는 물리 인터페이스를 먼저, 가상 인터페이스를 뒤에 둔다. 루프백과 veth는 뺀다.
	Interfaces []NetInterface `json:"interfaces"`
}

// NetInterface는 네트워크 인터페이스 하나의 상태와 누적 카운터다.
type NetInterface struct {
	Name string `json:"name"`
	// Virtual은 물리 장치가 없는 인터페이스(브리지, docker0, VPN 등)인지다.
	Virtual bool `json:"virtual,omitempty"`
	// State는 커널 operstate 값이다(up, down, unknown, dormant, lowerlayerdown 등).
	State string `json:"state"`
	// SpeedMbps는 링크 속도다. 모르거나 링크가 내려가 있으면 없다.
	SpeedMbps int    `json:"speedMbps,omitempty"`
	RxBytes   uint64 `json:"rxBytes"`
	TxBytes   uint64 `json:"txBytes"`
	RxPackets uint64 `json:"rxPackets"`
	TxPackets uint64 `json:"txPackets"`
	RxErrors  uint64 `json:"rxErrors"`
	TxErrors  uint64 `json:"txErrors"`
	RxDropped uint64 `json:"rxDropped"`
	TxDropped uint64 `json:"txDropped"`
}

// TemperatureResponse는 GET /api/addons/temperature의 응답이다.
type TemperatureResponse struct {
	AddonSources `tstype:",extends"`
	// Groups는 칩·장치별 센서 묶음이다. CPU, 디스크, 그 밖의 순서다.
	Groups []SensorGroup `json:"groups"`
}

// 센서 묶음 종류. SensorGroup.Kind에 들어간다.
const (
	SensorCPU   = "cpu"
	SensorDisk  = "disk"
	SensorOther = "other"
)

// SensorGroup은 한 칩·장치의 온도 센서 묶음이다.
type SensorGroup struct {
	// ID는 응답 안에서 고유한 식별자다(hwmon 이름과 장치, SMART 장치 이름 등).
	ID   string `json:"id"`
	Kind string `json:"kind" tstype:"'cpu' | 'disk' | 'other'"`
	// Name은 칩 이름(coretemp, nvme, acpitz 등)이나 디스크 모델명이다.
	Name string `json:"name"`
	// Device는 디스크 장치 이름(nvme0, sda 등)이다. 디스크가 아니면 없다.
	Device string `json:"device,omitempty"`
	// Source는 이 묶음을 가져온 provider 이름이다.
	Source  string   `json:"source"`
	Sensors []Sensor `json:"sensors"`
	// CheckedAt은 값을 읽은 시각(RFC 3339)이다. 캐시한 값(SMART)에만 있다.
	CheckedAt string `json:"checkedAt,omitempty"`
}

// Sensor는 온도 센서 하나다. 섭씨다.
type Sensor struct {
	// Label은 센서 이름(Package id 0, Composite 등)이다. 없으면 비어 있다.
	Label   string  `json:"label,omitempty"`
	Celsius float64 `json:"celsius"`
	// HighCelsius, CritCelsius는 칩이 알려 주는 경고·위험 온도다. 없으면 없다.
	HighCelsius *float64 `json:"highCelsius,omitempty"`
	CritCelsius *float64 `json:"critCelsius,omitempty"`
}

// ContainersResponse는 GET /api/addons/containers의 응답이다.
type ContainersResponse struct {
	AddonSources `tstype:",extends"`
	// Runtime은 compose 플러그인이 쓰는 컨테이너 런타임이다. 알 수 없으면 비어 있다.
	// Podman은 같은 RPC로 동작할 것으로 보이지만 검증하지 않았다.
	Runtime string `json:"runtime" tstype:"'docker' | 'podman' | ''"`
	// RuntimeStatus는 런타임 서비스 상태를 OMV가 번역한 문구다(running, stopped 등).
	// OMV 언어 설정에 따라 바뀌므로 비교하지 말고 표시에만 쓴다.
	RuntimeStatus string `json:"runtimeStatus"`
	// Containers는 이름 순서다.
	Containers []Container `json:"containers"`
}

// Container는 컨테이너 하나다(docker container ls --all의 한 줄).
type Container struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	// State는 Status에서 읽은 상태다.
	State string `json:"state" tstype:"'running' | 'paused' | 'restarting' | 'exited' | 'created' | 'dead' | 'removing' | 'unknown'"`
	// Health는 헬스체크 결과다. 헬스체크가 없으면 없다.
	Health string `json:"health,omitempty" tstype:"'healthy' | 'unhealthy' | 'starting'"`
	// Status는 docker가 준 상태 문구 원문이다(Up 3 days (healthy), Exited (0) 2 hours ago 등).
	Status string `json:"status"`
}

// ServicesResponse는 GET /api/addons/services의 응답이다.
type ServicesResponse struct {
	AddonSources `tstype:",extends"`
	// Services는 이름 순서다.
	Services []Service `json:"services"`
}

// Service는 OMV 서비스 하나의 상태다(Services.getStatus의 한 행).
type Service struct {
	// Name은 OMV 모듈 이름(ssh, samba, nfs 등)이다. 조작 대상으로 쓴다.
	Name string `json:"name"`
	// Title은 OMV가 번역한 이름이다. OMV 언어 설정에 따라 바뀌므로 표시에만 쓴다.
	Title string `json:"title"`
	// Enabled는 OMV 설정에서 서비스를 켰는지다.
	Enabled bool `json:"enabled"`
	// Running은 systemd 유닛이 실행 중인지다.
	Running bool `json:"running"`
	// Toggleable은 대시보드에서 켜고 끌 수 있는지다. 아니면 OMV에서 설정한다.
	Toggleable bool `json:"toggleable"`
}

// SmartResponse는 GET /api/addons/smart의 응답이다.
//
// OMV의 Smart.getList는 디스크마다 smartctl을 실행해 대기 중인 디스크를 깨우므로, 백엔드가 결과를
// 모든 세션에서 함께 쓰도록 보관한다. 하루 한 번(기준 시각 이후 첫 요청) 또는 새로고침 때만 다시 가져온다.
type SmartResponse struct {
	AddonSources `tstype:",extends"`
	// CheckedAt은 OMV에서 가져온 시각(RFC 3339)이다.
	CheckedAt string `json:"checkedAt"`
	// RefreshInSeconds는 새로고침(?refresh=1)으로 다시 가져올 수 있기까지 남은 초다. 0이면 지금 된다.
	// 시계 차이를 피하려고 절대 시각 대신 남은 시간으로 준다.
	RefreshInSeconds int         `json:"refreshInSeconds"`
	Disks            []SmartDisk `json:"disks"`
}

// SMART 상태 요약. SmartDisk.Status에 들어간다.
const (
	SmartGood    = "good"
	SmartWarning = "warning"
	SmartFailing = "failing"
	SmartUnknown = "unknown"
)

// SmartDisk는 SMART를 지원하는 디스크 하나다. 시리얼 번호 등 식별 정보는 넘기지 않는다.
type SmartDisk struct {
	// Device는 커널 장치 이름(sda, nvme0n1)이다.
	Device    string `json:"device"`
	Model     string `json:"model"`
	SizeBytes uint64 `json:"sizeBytes"`
	// Celsius는 SMART로 읽은 온도다. 모르면 없다.
	Celsius *float64 `json:"celsius,omitempty"`
	// Status는 OverallStatus의 요약이다(OMV workbench의 색 구분과 같다). good: 정상, warning: 불량 섹터가
	// 조금 있음, failing: 기준치를 지금 또는 과거에 넘었거나 불량 섹터가 많음, unknown: 상태 확인 실패 등.
	Status string `json:"status" tstype:"'good' | 'warning' | 'failing' | 'unknown'"`
	// OverallStatus는 OMV가 준 원래 값이다(GOOD, BAD_ATTRIBUTE_IN_THE_PAST 등).
	OverallStatus string `json:"overallStatus"`
	// Monitored는 OMV에서 이 디스크의 SMART 모니터링을 켰는지다.
	Monitored bool `json:"monitored"`
}

// Preferences는 GET /api/preferences의 응답이며 로그인한 사용자의 대시보드 설정이다.
// 백엔드 상태 디렉터리에 사용자별로 저장되어 로그아웃해도 남는다.
type Preferences struct {
	// Widgets는 바탕화면 위젯 배치다. 한 번도 저장하지 않았으면 없다(프론트가 기본 배치를 쓴다).
	Widgets *[]WidgetPlacement `json:"widgets,omitempty"`
	// Shortcuts는 바탕화면 바로가기 목록이며 순서대로 놓인다. 한 번도 저장하지 않았으면 없고
	// (프론트가 기본 목록을 쓴다), 모두 지웠으면 빈 목록이다.
	Shortcuts *[]Shortcut `json:"shortcuts,omitempty"`
	// Drawer는 "모든 앱" 화면 설정이다. 한 번도 저장하지 않았으면 없다(프론트가 기본값을 쓴다).
	Drawer *DrawerPrefs `json:"drawer,omitempty"`
	// SystemInfo는 시스템 정보 창의 보기 설정이다. 한 번도 저장하지 않았으면 없다(프론트가 기본값을 쓴다).
	SystemInfo *SystemInfoPrefs `json:"systemInfo,omitempty"`
	// Appearance는 화면 설정(테마, 바탕화면 등)이다. 없거나 비어 있는 값은 관리자가 정한 기본 화면을 따른다.
	Appearance *AppearancePrefs `json:"appearance,omitempty"`
	// WallpaperImage는 이 사용자가 올린 바탕화면 이미지의 정보다. 올리지 않았으면 없다.
	// 설정 파일에 저장하지 않고 응답할 때 이미지 저장소에서 붙인다.
	WallpaperImage *ImageInfo `json:"wallpaperImage,omitempty"`
	// AvatarImage는 이 사용자의 프로필 사진 정보다. 없으면 기본 아바타를 그린다. WallpaperImage처럼 응답할 때만 붙인다.
	AvatarImage *ImageInfo `json:"avatarImage,omitempty"`
}

// 테마. AppearancePrefs.Theme, SiteDefaults.Theme에 들어간다.
const (
	// ThemeAuto는 브라우저(OS)의 밝은·어두운 모드 설정을 따른다.
	ThemeAuto  = "auto"
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// 작업표시줄 정렬. AppearancePrefs.TaskbarAlign에 들어간다.
const (
	// TaskbarAlignCenter는 "모든 앱" 버튼과 창 버튼을 작업표시줄 가운데에 둔다(Windows 11처럼).
	TaskbarAlignCenter = "center"
	// TaskbarAlignLeft는 "모든 앱" 버튼과 창 버튼을 작업표시줄 왼쪽 끝부터 둔다.
	TaskbarAlignLeft = "left"
)

// 배경 종류. Wallpaper.Kind에 들어간다.
const (
	// WallpaperPreset은 프론트에 들어 있는 배경(그라데이션·단색)이다.
	WallpaperPreset = "preset"
	// WallpaperImage는 올린 이미지다. 어느 이미지인지는 배경을 쓰는 자리가 정한다
	// (사용자 바탕화면, 로그인 배경, 기본 바탕화면).
	WallpaperImage = "image"
)

// Wallpaper는 바탕화면·로그인 화면 배경 하나다. 다른 출처(파일 탐색기에서 고른 파일 등)는 Kind를 더해 담는다.
type Wallpaper struct {
	Kind string `json:"kind" tstype:"'preset' | 'image'"`
	// Preset은 Kind가 preset일 때 프론트 프리셋 id다. 백엔드는 형식만 검사하고, 모르는 id는 프론트가 기본 배경으로 그린다.
	Preset string `json:"preset,omitempty"`
}

// AppearancePrefs는 사용자 화면 설정이다. 설정 전체를 한 번에 바꾼다. 비어 있는 값은 관리자가 정한 기본 화면
// (SiteDefaults)을, 그것도 없으면 대시보드 기본값을 따른다.
type AppearancePrefs struct {
	// Theme은 Theme* 중 하나다. 비어 있으면 기본 화면을 따른다.
	Theme string `json:"theme,omitempty" tstype:"'auto' | 'light' | 'dark'"`
	// Wallpaper는 바탕화면이다. 없으면 기본 화면을 따른다. image는 이 사용자가 올린 이미지다.
	Wallpaper *Wallpaper `json:"wallpaper,omitempty"`
	// Animations는 창·메뉴 애니메이션을 켤지다. 없으면 켠다.
	Animations *bool `json:"animations,omitempty"`
	// TaskbarAlign은 TaskbarAlign* 중 하나다. 비어 있으면 가운데다(관리자 기본값은 없다).
	TaskbarAlign string `json:"taskbarAlign,omitempty" tstype:"'center' | 'left'"`
	// WidgetsLocked는 바탕화면 위젯을 옮기거나 크기를 바꾸거나 메뉴로 추가·제거하지 못하게 할지다.
	// 눌러서 창을 여는 것은 막지 않는다. 프론트만 지키는 편의 설정이라 위젯 배치 저장은 막지 않는다.
	// 관리자 기본값은 없다.
	WidgetsLocked bool `json:"widgetsLocked,omitempty"`
}

// ImageInfo는 저장된 배경 이미지의 정보다.
type ImageInfo struct {
	// Rev는 이미지가 바뀔 때마다 달라지는 값이다. 이미지 주소에 붙여(?v=) 브라우저가 이전 이미지를 쓰지 않게 한다.
	Rev string `json:"rev"`
}

// SiteSettings는 모든 사용자에게 적용하는 전역 설정이며 관리자만 바꾼다. 상태 디렉터리의 site.json에 저장한다.
// 부분마다 전체를 한 번에 바꾸고, 없는 부분은 기본값이다.
type SiteSettings struct {
	Login    *SiteLogin       `json:"login,omitempty"`
	Defaults *SiteDefaults    `json:"defaults,omitempty"`
	Server   *ServerOverrides `json:"server,omitempty"`
	// Security는 보안 강화 모드의 기능별 단계다. 없으면 모두 꺼짐이다.
	Security *SecurityPolicy `json:"security,omitempty"`
	// Files는 파일 탐색기의 전역 설정과 공유 폴더 스냅숏이다. 없으면 스냅숏이 없고 디스크 보기는 꺼짐이다.
	Files *SiteFiles `json:"files,omitempty"`
}

// SiteLogin은 로그인 화면 설정이다. 저장한 적 없으면 대시보드 기본 배경에 대기 연출을 켠다.
type SiteLogin struct {
	// Background는 로그인 화면 배경이다. 없으면 대시보드 기본 배경이다. image는 관리자가 올린 로그인 배경 이미지다.
	Background *Wallpaper `json:"background,omitempty"`
	// EasterEgg는 로그인 화면에서 오래 기다리면 나오는 연출을 켤지다.
	EasterEgg bool `json:"easterEgg"`
}

// SiteDefaults는 화면 설정을 정하지 않은 사용자에게 적용하는 기본 화면이다.
type SiteDefaults struct {
	// Theme은 Theme* 중 하나다. 비어 있으면 auto다.
	Theme string `json:"theme,omitempty" tstype:"'auto' | 'light' | 'dark'"`
	// Wallpaper는 기본 바탕화면이다. 없으면 대시보드 기본 배경이다. image는 관리자가 올린 기본 바탕화면 이미지다.
	Wallpaper *Wallpaper `json:"wallpaper,omitempty"`
}

// ServerOverrides는 재시작 없이 바꿀 수 있는 가동 설정을 웹에서 덮어쓴 값이다. 없는 값은 플래그·환경 변수
// (/etc/default/onmaru) 값을 따른다.
type ServerOverrides struct {
	// WorkbenchURL은 -workbench-url을 덮어쓴다. 빈 문자열이면 OMV 워크벤치 앱을 끈다.
	WorkbenchURL *string `json:"workbenchUrl,omitempty"`
	// SmartRefreshAt은 -smart-refresh-at(HH:MM)을 덮어쓴다.
	SmartRefreshAt *string `json:"smartRefreshAt,omitempty"`
	// Terminal은 -terminal을 덮어쓴다. "true" 또는 "false"다.
	Terminal *string `json:"terminal,omitempty"`
}

// AppearanceRequest는 PUT /api/preferences/appearance의 요청 본문이다. 화면 설정 전체를 바꾼다.
type AppearanceRequest struct {
	Appearance *AppearancePrefs `json:"appearance"`
}

// "모든 앱" 화면의 앱 정렬 방식. DrawerPrefs.Sort에 들어간다.
const (
	// DrawerSortRecommended는 대시보드가 정한 순서다(비슷한 기능의 앱끼리 모인다).
	DrawerSortRecommended = "recommended"
	// DrawerSortName은 화면 언어의 이름순이다.
	DrawerSortName = "name"
)

// DrawerPrefs는 "모든 앱" 화면 설정이다. 설정 전체를 한 번에 바꾼다.
type DrawerPrefs struct {
	// Sort는 앱 정렬 방식이다(DrawerSort*). 프론트는 모르는 값이면 추천순을 쓴다.
	Sort string `json:"sort"`
}

// DrawerRequest는 PUT /api/preferences/drawer의 요청 본문이다.
type DrawerRequest struct {
	Drawer *DrawerPrefs `json:"drawer"`
}

// 프로세스 CPU 사용률의 기준. SystemInfoPrefs.CPUScale에 들어간다.
const (
	// CPUScaleMachine은 전체 CPU 대비 0~100%다(작업 관리자 방식).
	CPUScaleMachine = "machine"
	// CPUScaleCore는 코어 하나를 100%로 본다(top 방식, 코어 수 × 100%까지).
	CPUScaleCore = "core"
)

// 개요의 CPU 그래프 모양. SystemInfoPrefs.CPUGraph에 들어간다.
const (
	// CPUGraphOverall은 전체 사용률 그래프 하나다.
	CPUGraphOverall = "overall"
	// CPUGraphCores는 코어마다 작은 그래프다.
	CPUGraphCores = "cores"
)

// SystemInfoPrefs는 시스템 정보 창의 보기 설정이다. 설정 전체를 한 번에 바꾼다.
type SystemInfoPrefs struct {
	// CPUScale은 CPUScale* 중 하나다. 프론트는 모르는 값이면 machine을 쓴다.
	CPUScale string `json:"cpuScale"`
	// CPUGraph는 CPUGraph* 중 하나다. 프론트는 모르는 값이면 overall을 쓴다.
	CPUGraph string `json:"cpuGraph"`
	// CPUKernel은 CPU 그래프에 커널 시간을 겹쳐 보일지다. 없으면 보인다.
	CPUKernel *bool `json:"cpuKernel,omitempty"`
}

// SystemInfoRequest는 PUT /api/preferences/system-info의 요청 본문이다.
type SystemInfoRequest struct {
	SystemInfo *SystemInfoPrefs `json:"systemInfo"`
}

// Shortcut은 바탕화면 바로가기 하나다. 지금은 앱만 가리킨다. 폴더나 OMV 페이지를 가리키는
// 바로가기가 생기면 필드를 더한다(저장 파일 형식을 바꾸지 않고 늘릴 수 있게 객체로 둔다).
type Shortcut struct {
	// App은 프론트 앱 id다(창 보기가 있는 애드온 또는 준비 중 앱). 백엔드는 형식만 검사한다.
	App string `json:"app"`
}

// ShortcutsRequest는 PUT /api/preferences/shortcuts의 요청 본문이다. 바로가기 목록 전체를 바꾼다.
type ShortcutsRequest struct {
	Shortcuts []Shortcut `json:"shortcuts"`
}

// WidgetPlacement는 바탕화면 위젯 하나의 자리다. 그리드는 오른쪽 위를 기준으로 하므로
// 화면 크기가 달라도 위젯끼리 겹치는지는 이 값만으로 정해진다.
type WidgetPlacement struct {
	// Addon은 위젯 보기가 있는 애드온 id다. 애드온마다 위젯은 하나다.
	Addon string `json:"addon"`
	// Right는 그리드 오른쪽 끝에서 위젯 오른쪽 끝까지의 칸 수다.
	Right int `json:"right"`
	// Top은 그리드 위쪽 끝에서 위젯 위쪽 끝까지의 칸 수다.
	Top  int `json:"top"`
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// WidgetsRequest는 PUT /api/preferences/widgets의 요청 본문이다. 위젯 배치 전체를 바꾼다.
type WidgetsRequest struct {
	Widgets []WidgetPlacement `json:"widgets"`
}

// SiteInfo는 GET /api/site의 응답이다. 로그인 화면이 로그인 전에 받으므로 인증 없이 준다.
// 화면 꾸밈에 쓰는 전역 설정만 담고 가동 설정(주소 등)은 넣지 않는다.
type SiteInfo struct {
	// LoginBackground는 로그인 화면 배경이다. 없으면 대시보드 기본 배경이다.
	LoginBackground *Wallpaper `json:"loginBackground,omitempty"`
	// LoginEasterEgg는 로그인 화면의 대기 연출을 켤지다.
	LoginEasterEgg bool `json:"loginEasterEgg"`
	// DefaultTheme, DefaultWallpaper는 화면 설정을 정하지 않은 사용자에게 적용하는 기본 화면이다.
	DefaultTheme     string     `json:"defaultTheme,omitempty" tstype:"'auto' | 'light' | 'dark'"`
	DefaultWallpaper *Wallpaper `json:"defaultWallpaper,omitempty"`
	Images           SiteImages `json:"images"`
}

// SiteImages는 관리자가 올린 전역 배경 이미지의 정보다. 올리지 않은 이미지는 없다.
type SiteImages struct {
	LoginBackground  *ImageInfo `json:"loginBackground,omitempty"`
	DefaultWallpaper *ImageInfo `json:"defaultWallpaper,omitempty"`
}

// SiteLoginRequest는 PUT /api/site/login의 요청 본문이다. 로그인 화면 설정 전체를 바꾼다.
type SiteLoginRequest struct {
	Login *SiteLogin `json:"login"`
}

// SiteDefaultsRequest는 PUT /api/site/defaults의 요청 본문이다. 기본 화면 전체를 바꾼다.
type SiteDefaultsRequest struct {
	Defaults *SiteDefaults `json:"defaults"`
}

// ServerSettingsRequest는 PUT /api/site/server의 요청 본문이다. 덮어쓰기 값 전체를 바꾼다(없는 값은 덮어쓰지 않음).
type ServerSettingsRequest struct {
	Server *ServerOverrides `json:"server"`
}

// ServerSettings는 GET /api/site/server의 응답이다(관리자).
type ServerSettings struct {
	WorkbenchURL   OverridableSetting `json:"workbenchUrl"`
	SmartRefreshAt OverridableSetting `json:"smartRefreshAt"`
	// Terminal은 웹 터미널 허용 여부("true" 또는 "false")다.
	Terminal OverridableSetting `json:"terminal"`
	// Runtime은 시작할 때 정해진 가동 설정 전체다(플래그 순서). 바꾸려면 /etc/default/onmaru를 고치고 다시 시작한다.
	Runtime []RuntimeSetting `json:"runtime"`
}

// OverridableSetting은 웹에서 덮어쓸 수 있는 가동 설정 하나다.
type OverridableSetting struct {
	// Value는 지금 쓰는 값이다.
	Value string `json:"value"`
	// Base는 플래그·환경 변수(없으면 기본값)로 정한 값이다. 덮어쓰기를 지우면 이 값으로 돌아간다.
	Base string `json:"base"`
	// Overridden은 웹에서 덮어썼는지다.
	Overridden bool `json:"overridden"`
}

// 가동 설정 값의 출처. RuntimeSetting.Source에 들어간다.
const (
	RuntimeSourceDefault = "default"
	RuntimeSourceEnv     = "env"
	RuntimeSourceFlag    = "flag"
	// RuntimeSourceSystemd는 systemd가 넘긴 값이다(StateDirectory=의 STATE_DIRECTORY).
	RuntimeSourceSystemd = "systemd"
)

// RuntimeSetting은 시작할 때 정해진 가동 설정 하나다.
type RuntimeSetting struct {
	// Flag는 명령줄 플래그 이름이다(listen, omv-url 등).
	Flag string `json:"flag"`
	// Env는 같은 값을 정하는 환경 변수 이름이다.
	Env    string `json:"env"`
	Value  string `json:"value"`
	Source string `json:"source" tstype:"'default' | 'env' | 'flag' | 'systemd'"`
}

// 프로필(전자 우편, 태그)을 바꿀 수 없는 이유. ProfileResponse.ReadOnlyReason에 들어간다.
const (
	// ProfileReadOnlySystem은 OMV 시스템 계정이다(OMV 기본 admin 계정 등). OMV 사용자 관리 목록에도 없다.
	ProfileReadOnlySystem = "system"
	// ProfileReadOnlyLocked는 OMV에서 이 사용자의 계정 수정을 막았다는 뜻이다(disallowusermod).
	ProfileReadOnlyLocked = "locked"
)

// ProfileResponse는 GET /api/profile의 응답이며 로그인한 사용자 자신의 OMV 계정 정보다.
// SSH 공개 키, uid, 홈 디렉터리 등은 넘기지 않는다.
type ProfileResponse struct {
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	// Tags는 계정 태그다. OMV 사용자 편집 화면의 "Tags"이며, 실제로는 Linux 계정의 설명 칸(GECOS)에
	// 쉼표로 이어 저장된다. 백엔드가 나누고 잇는다.
	Tags []string `json:"tags,omitempty"`
	// Groups는 속한 그룹이다. OMV가 관리자에게만 알려 주므로 일반 사용자는 없다.
	Groups []string `json:"groups,omitempty"`
	// SystemAccount는 OMV 사용자 관리 목록에 없는 시스템 계정인지다. 관리자만 알 수 있다.
	SystemAccount bool `json:"systemAccount,omitempty"`
	// ReadOnlyReason이 있으면 전자 우편·태그를 바꿀 수 없다.
	ReadOnlyReason string `json:"readOnlyReason,omitempty" tstype:"'system' | 'locked'"`
}

// ProfileRequest는 PUT /api/profile의 요청 본문이다. 사용자가 자기 계정에서 바꿀 수 있는 전자 우편과 태그를
// 바꾼다(OMV UserMgmt.setUserByContext와 같은 범위). 없는 값은 그대로 둔다. 비밀번호·그룹 등은 OMV에서 바꾼다.
type ProfileRequest struct {
	// Email이 빈 문자열이면 전자 우편을 지운다.
	Email *string   `json:"email,omitempty"`
	Tags  *[]string `json:"tags,omitempty"`
}

// ProfileSaveResponse는 PUT /api/profile의 응답이다.
type ProfileSaveResponse struct {
	// Applied는 OMV 적용까지 끝났는지다. 일반 사용자는 적용 권한이 없어 false이며, OMV에 적용 대기가 남는다
	// (관리자가 OMV 웹 UI에서 적용한다). 바꿀 것이 없었으면 true다.
	Applied bool `json:"applied"`
}

// AboutResponse는 GET /api/about의 응답이다. 설정 앱의 정보 페이지가 보여 준다.
type AboutResponse struct {
	// Version은 온마루 버전이다.
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

// LoginRequest는 POST /api/login의 요청 본문이다.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// VerifyRequest는 POST /api/login/verify의 요청 본문이다. 로그인 응답이
// challengeRequired일 때 2차 인증 값(예: TOTP 코드)을 보낸다.
type VerifyRequest struct {
	Response string `json:"response"`
}

// 로그인 응답 상태.
const (
	LoginAuthenticated     = "authenticated"
	LoginChallengeRequired = "challengeRequired"
)

// LoginResponse는 POST /api/login, POST /api/login/verify의 성공 응답이다.
// Status가 authenticated면 Session이, challengeRequired면 Challenge가 들어 있다.
type LoginResponse struct {
	Status    string         `json:"status" tstype:"'authenticated' | 'challengeRequired'"`
	Session   *SessionInfo   `json:"session,omitempty"`
	Challenge *ChallengeInfo `json:"challenge,omitempty"`
	// StepUp이 있으면 로그인을 마친 뒤 2차 인증을 권한다(보안 강화 모드). 세션은 이미 만들어졌으므로
	// 건너뛰면 일반 세션으로 쓴다. 로그인 응답에만 넣는다(세션 복원 때는 묻지 않는다).
	StepUp *StepUpInfo `json:"stepUp,omitempty"`
	// PasskeyLoginOffer가 있으면 이 사용자가 패스키로 로그인을 켤 수 있다는 뜻이다(공개 주소가 있고 패스키를 등록함).
	// 이 브라우저에 등록이 없으면 로그인 화면이 켜기를 권한다. 로그인 응답에만 넣는다.
	PasskeyLoginOffer *PasskeyLoginOffer `json:"passkeyLoginOffer,omitempty"`
}

// PasskeyLoginOffer는 로그인 직후 권하는 패스키로 로그인이다.
type PasskeyLoginOffer struct {
	// Days는 지금 켜면 쓸 수 있는 기간(일)이다(정책의 PasskeyLoginDays). 권유 문구에 보인다.
	Days int `json:"days"`
}

// StepUpInfo는 로그인 직후 권하는 2차 인증이다.
type StepUpInfo struct {
	// Methods는 이 사용자가 쓸 수 있는 수단이다(Factor*, password 제외).
	Methods []string `json:"methods" tstype:"Array<'passkey' | 'totp' | 'recovery'>"`
}

// SessionInfo는 GET /api/session, POST /api/session/activity의 응답이며 로그인한 사용자를 나타낸다.
type SessionInfo struct {
	Username string `json:"username"`
	// Admin은 OMV 관리자 역할인지다. 관리자가 아니면 설정 관련 창과 버튼을 숨긴다.
	Admin bool `json:"admin"`
	// ExpiresInSeconds는 OMV 세션이 만료되기까지 남은 초(예상)다. 사용자 조작이 없으면
	// 이 시간 뒤에 만료된다. OMV 자동 로그아웃이 꺼져 있거나 시간을 모르면 없다.
	// 브라우저와 서버의 시계 차이에 영향받지 않도록 절대 시각 대신 남은 시간으로 준다.
	ExpiresInSeconds *int `json:"expiresInSeconds,omitempty"`
	// Assurance는 이 세션이 거친 확인의 수준이다(Assurance*, 보안 강화 모드).
	Assurance string `json:"assurance" tstype:"'login' | 'password' | 'twoFactor'"`
}

// ChallengeInfo는 2차 인증 요구 정보다. 종류는 OMV 플러그인이 정한다.
type ChallengeInfo struct {
	Kind string `json:"kind"`
	// RedirectURL이 있으면 대시보드에서 처리할 수 없는 인증이다(OMV 화면에서 진행).
	RedirectURL string `json:"redirectUrl,omitempty"`
}

// 오류 코드. ErrorResponse.Code에 들어간다.
const (
	ErrInvalidCredentials = "invalid_credentials"
	ErrRateLimited        = "rate_limited"
	ErrChallengeFailed    = "challenge_failed"
	ErrChallengeExpired   = "challenge_expired"
	ErrOMVUnavailable     = "omv_unavailable"
	ErrUnauthorized       = "unauthorized"
	ErrBadRequest         = "bad_request"
	// ErrRPCForbidden는 허용 목록에 없거나 쓰기라서 중계하지 않는 OMV RPC다.
	ErrRPCForbidden = "rpc_forbidden"
	// ErrAdminRequired, ErrPluginMissing은 RPC를 선언한 애드온을 이 사용자가 쓸 수 없는 이유다.
	ErrAdminRequired = ReasonAdminRequired
	ErrPluginMissing = ReasonPluginMissing
	// ErrOMVError는 중계한 OMV RPC가 실패했다는 뜻이다(OMV의 오류 메시지는 넘기지 않는다).
	ErrOMVError = "omv_error"
	// ErrActionBusy는 같은 대상에 진행 중인 조작이 있다는 뜻이다.
	ErrActionBusy = "action_busy"
	// ErrActionFailed는 조작을 OMV가 실행했지만 실패했다는 뜻이다(docker 명령 실패, 설정 적용 실패 등).
	ErrActionFailed = "action_failed"
	// ErrActionTimeout은 조작이 시간 안에 끝나지 않았다는 뜻이다. OMV 쪽 작업은 계속될 수 있다.
	ErrActionTimeout = "action_timeout"
	// ErrConfigDirty는 OMV에 적용하지 않은 설정 변경이 있어 조작을 시작하지 않았다는 뜻이다.
	ErrConfigDirty = "config_dirty"
	// ErrImageTooLarge는 올린 배경 이미지가 크기 제한을 넘었다는 뜻이다.
	ErrImageTooLarge = "image_too_large"
	// ErrUnsupportedImage는 올린 파일이 받는 이미지 형식(PNG, JPEG, WebP)이 아니라는 뜻이다.
	ErrUnsupportedImage = "unsupported_image"
	// ErrProfileReadOnly는 이 계정의 정보를 바꿀 수 없다는 뜻이다(시스템 계정, OMV에서 수정을 막은 계정).
	ErrProfileReadOnly = "profile_readonly"
	// ErrHelperUnavailable은 권한 도우미를 설정하지 않았거나 지금 연결할 수 없다는 뜻이다.
	ErrHelperUnavailable = "helper_unavailable"
	// ErrHelperReauth는 이 세션에 도우미 티켓이 없거나 사라졌다는 뜻이다(도우미 재시작 등). 다시 로그인하면 된다.
	ErrHelperReauth = "helper_reauth"
	// ErrElevationFailed는 권한 상승 비밀번호가 틀렸다는 뜻이다.
	ErrElevationFailed = "elevation_failed"
	// ErrElevationNotAllowed는 이 계정이 root 권한으로 상승할 수 없다는 뜻이다(그룹 조건).
	ErrElevationNotAllowed = "elevation_not_allowed"
	// ErrElevationRequired는 root 권한이 필요한 조작이라 먼저 상승해야 한다는 뜻이다.
	ErrElevationRequired = "elevation_required"
	// ErrTerminalDisabled는 관리자가 웹 터미널을 허용하지 않았다는 뜻이다.
	ErrTerminalDisabled = "terminal_disabled"
	// ErrSecurityRequired는 보안 강화 모드의 확인(ErrorResponse.Required 단계)을 먼저 거쳐야 한다는 뜻이다.
	ErrSecurityRequired = "security_required"
	// ErrFactorFailed는 재확인한 비밀번호나 2차 인증 값이 틀렸다는 뜻이다.
	ErrFactorFailed = "factor_failed"
	// ErrManageRequired는 인증 수단을 바꾸기 전에 관리 확인(POST /api/security/manage)이 필요하다는 뜻이다.
	ErrManageRequired = "manage_required"
	// ErrPasskeyUnavailable은 공개 주소(-public-origin)를 설정하지 않아 패스키를 쓸 수 없다는 뜻이다.
	ErrPasskeyUnavailable = "passkey_unavailable"
	// ErrPasskeyUVUnsupported는 새 패스키의 시험 확인에서 인증기가 사용자 확인(UV)을 알리지 않아 등록하지 않았다는
	// 뜻이다(확인 응답에 UV를 켜 주지 않는 동기화 저장소 등). 다른 저장소(이 기기, 보안 키)로 다시 등록해야 한다.
	ErrPasskeyUVUnsupported = "passkey_uv_unsupported"
	// ErrSecurityUnavailable은 수단 기록을 읽을 수 없다는 뜻이다(손상된 파일 등). 관리자가 초기화해야 한다.
	ErrSecurityUnavailable = "security_unavailable"
	// ErrPasskeyLoginUnknown은 패스키로 로그인 등록이 없다는 뜻이다(없는 사용자, 해제했거나 패스키를 지운 등록). 브라우저
	// 항목을 지운다.
	ErrPasskeyLoginUnknown = "passkey_login_unknown"
	// ErrPasskeyLoginExpired는 패스키로 로그인 등록의 기간이 끝나 지웠다는 뜻이다. 비밀번호로 로그인한 뒤 다시 켠다.
	ErrPasskeyLoginExpired = "passkey_login_expired"
	// ErrPasskeyLoginStale은 풀어 낸 비밀번호로 로그인하지 못해(비밀번호를 바꿈) 그 등록을 지웠다는 뜻이다.
	ErrPasskeyLoginStale = "passkey_login_stale"
	// ErrUpgradeDisabled는 대시보드에서 업그레이드가 꺼져 있다는 뜻이다(UpgradeInfo.DisabledReason).
	ErrUpgradeDisabled = "upgrade_disabled"
	// ErrUpgradeBusy는 확인 요청이 남아 있거나 확인·업그레이드가 진행 중이라는 뜻이다.
	ErrUpgradeBusy = "upgrade_busy"
	// ErrUpgradeOutdated는 그 커밋으로 업그레이드할 수 있다고 끝난 확인이 없다는 뜻이다(다시 확인해야 한다).
	ErrUpgradeOutdated = "upgrade_outdated"
	// 파일 탐색기의 파일 연산 오류. 권한 도우미가 사용자 권한으로 실행한 결과다.
	ErrNotFound         = "not_found"
	ErrPermissionDenied = "permission_denied"
	// ErrOutsideRoots는 파일 탐색기가 보이는 폴더(홈, 공유 폴더, 켠 경우 디스크) 밖이거나 권한 도우미의 기준 디렉터리 밖이라는 뜻이다.
	ErrOutsideRoots = "outside_roots"
	ErrFileExists   = "file_exists"
	ErrNotDir       = "not_dir"
	ErrIsDir        = "is_dir"
	// ErrNotRegular는 일반 파일이 아니라(FIFO, 장치 등) 내려받을 수 없다는 뜻이다.
	ErrNotRegular = "not_regular"
	ErrNoSpace    = "no_space"
	ErrReadOnly   = "read_only"
	// ErrHelperBusy는 권한 도우미가 동시에 실행할 수 있는 연산 수(사용자당 4개)를 넘었다는 뜻이다.
	ErrHelperBusy = "helper_busy"
	// ErrTransferTimeout은 전송이 60초 동안 진행되지 않았거나 시간 상한(6시간)을 넘었다는 뜻이다.
	ErrTransferTimeout = "transfer_timeout"
	// ErrListingExpired는 목록(FilesListing.ID)이 사라졌다는 뜻이다(유휴, 상한, 로그아웃). 폴더를 다시 연다.
	ErrListingExpired = "listing_expired"
	// ErrSortPending은 날짜·크기순 정렬에 필요한 항목 정보를 아직 다 읽지 않았다는 뜻이다(prepare를 마저 부른다).
	ErrSortPending = "sort_pending"
	// ErrNotEmpty는 폴더 안에 항목이 있어 지우지 않았다는 뜻이다(폴더 안까지 지우라고 하지 않은 삭제).
	ErrNotEmpty = "not_empty"
	// ErrCrossDevice는 다른 디스크(권한 도우미의 다른 기준 디렉터리나 다른 파일시스템)로 옮기려 했다는 뜻이다.
	// 옮기기는 이름 바꾸기 하나로만 하므로 복사가 필요한 곳으로는 옮길 수 없다.
	ErrCrossDevice = "cross_device"
	// ErrRootFolder는 사이드바에 보이는 폴더(홈, 공유 폴더, 디스크)이거나 그것을 품은 폴더라서 이름을 바꾸거나
	// 옮기거나 지울 수 없다는 뜻이다.
	ErrRootFolder = "root_folder"
	// ErrNoTrash는 그 항목이 든 디스크에 쓸 수 있는 휴지통이 없다는 뜻이다(영구 삭제만 할 수 있다).
	ErrNoTrash = "no_trash"
)

// ErrorResponse는 2xx가 아닌 상태 코드와 함께 반환된다.
type ErrorResponse struct {
	Error string `json:"error"`
	// Code는 화면에서 분기할 때 쓰는 오류 종류다. 없을 수 있다.
	Code string `json:"code,omitempty" tstype:"'invalid_credentials' | 'rate_limited' | 'challenge_failed' | 'challenge_expired' | 'omv_unavailable' | 'unauthorized' | 'bad_request' | 'rpc_forbidden' | 'admin_required' | 'plugin_missing' | 'omv_error' | 'action_busy' | 'action_failed' | 'action_timeout' | 'config_dirty' | 'image_too_large' | 'unsupported_image' | 'profile_readonly' | 'helper_unavailable' | 'helper_reauth' | 'elevation_failed' | 'elevation_not_allowed' | 'elevation_required' | 'terminal_disabled' | 'security_required' | 'factor_failed' | 'manage_required' | 'passkey_unavailable' | 'passkey_uv_unsupported' | 'security_unavailable' | 'passkey_login_unknown' | 'passkey_login_expired' | 'passkey_login_stale' | 'upgrade_disabled' | 'upgrade_busy' | 'upgrade_outdated' | 'not_found' | 'permission_denied' | 'outside_roots' | 'file_exists' | 'not_dir' | 'is_dir' | 'not_regular' | 'no_space' | 'read_only' | 'helper_busy' | 'transfer_timeout' | 'listing_expired' | 'sort_pending' | 'not_empty' | 'cross_device' | 'root_folder' | 'no_trash'"`
	// RetryAfterSeconds는 rate_limited일 때 다시 시도할 수 있기까지 남은 초다.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`
	// Required는 security_required일 때 거쳐야 하는 단계다.
	Required string `json:"required,omitempty" tstype:"'password' | 'twoFactor'"`
}

// 권한 도우미 상태. HelperStatus.State에 들어간다.
const (
	// HelperDisabled는 도우미를 설정하지 않았다는 뜻이다(-helper-socket이 비어 있음).
	HelperDisabled = "disabled"
	// HelperUnavailable은 설정했지만 지금 도우미에 연결할 수 없다는 뜻이다.
	HelperUnavailable = "unavailable"
	// HelperNone은 이 세션에 도우미 티켓이 없다는 뜻이다(로그인할 때 발급받지 못함, 시스템 계정 등).
	HelperNone = "none"
	// HelperReauth는 티켓이 도우미에서 사라졌다는 뜻이다(도우미 재시작 등). 다시 로그인하면 된다.
	HelperReauth = "reauth"
	// HelperActive는 이 세션이 도우미를 쓸 수 있다는 뜻이다.
	HelperActive = "active"
)

// HelperStatus는 GET /api/helper, POST·DELETE /api/helper/elevation의 응답이다.
type HelperStatus struct {
	State string `json:"state" tstype:"'disabled' | 'unavailable' | 'none' | 'reauth' | 'active'"`
	// CanElevate는 이 사용자가 root 권한으로 상승할 수 있는지다(도우미가 그룹으로 판단).
	CanElevate bool `json:"canElevate"`
	// ElevatedSeconds는 상승이 끝날 때까지 남은 초다. 상승하지 않았으면 0이다.
	ElevatedSeconds int `json:"elevatedSeconds"`
}

// HelperGroup은 그룹 하나다.
type HelperGroup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// HelperIdentity는 도우미 worker가 실제로 가진 권한이다.
type HelperIdentity struct {
	UID    int           `json:"uid"`
	GID    int           `json:"gid"`
	User   string        `json:"user"`
	Groups []HelperGroup `json:"groups"`
}

// HelperIdentityResponse는 GET /api/helper/identity의 응답이다.
type HelperIdentityResponse struct {
	// User는 이 세션의 사용자 권한으로 실행한 worker의 권한이다.
	User HelperIdentity `json:"user"`
	// Root는 상승 중일 때 root 범위 worker의 권한이다.
	Root *HelperIdentity `json:"root,omitempty"`
}

// ElevationRequest는 POST /api/helper/elevation의 본문이다. 비밀번호는 도우미가 직접 확인한다.
type ElevationRequest struct {
	Password string `json:"password"`
}

// 웹 터미널(GET /api/terminal, WebSocket). 바이너리 프레임은 터미널 입출력 바이트이고, 텍스트 프레임은
// 아래 JSON 제어 메시지다. 쿼리 cols·rows는 처음 창 크기다.

// TerminalClientMessage는 브라우저가 보내는 제어 메시지다.
//   - resize: 창 크기(Cols, Rows)가 바뀌었다
//   - answer: 마지막 prompt의 답(Answers, 질문 수와 같다)
//   - cancel: 마지막 prompt를 취소한다(접속을 그만둔다)
type TerminalClientMessage struct {
	Type    string   `json:"type" tstype:"'resize' | 'answer' | 'cancel'"`
	Cols    int      `json:"cols,omitempty"`
	Rows    int      `json:"rows,omitempty"`
	Answers []string `json:"answers,omitempty"`
}

// TerminalServerMessage는 서버가 보내는 제어 메시지다.
//   - status: 접속 상태(State). closed면 Reason과 ExitCode·힌트가 붙는다
//   - prompt: sshd가 묻는 질문. 답은 answer로 보낸다
type TerminalServerMessage struct {
	Type  string `json:"type" tstype:"'status' | 'prompt'"`
	State string `json:"state,omitempty" tstype:"'connecting' | 'connected' | 'closed'"`
	// Reason은 닫힌 이유다. exit(셸이 끝남, ExitCode), 세션·설정 이유(session_ended, disabled,
	// connection_lost, security_required), 접속 실패(ssh_unavailable, ssh_host_key, ssh_auth_failed, ssh_failed,
	// canceled, too_many_windows, busy, rate_limited) 중 하나다.
	Reason   string `json:"reason,omitempty" tstype:"'exit' | 'session_ended' | 'disabled' | 'connection_lost' | 'security_required' | 'ssh_unavailable' | 'ssh_host_key' | 'ssh_auth_failed' | 'ssh_failed' | 'canceled' | 'too_many_windows' | 'busy' | 'rate_limited'"`
	ExitCode *int   `json:"exitCode,omitempty"`
	// KeyCode는 도우미 임시 키를 쓰지 못한 이유(도우미 오류 코드, 또는 sshd가 키를 받지 않았으면
	// key_rejected)다. ssh_auth_failed의 힌트다.
	KeyCode string `json:"keyCode,omitempty"`
	// NotAllowedGroups는 사용자가 sshd의 AllowGroups 중 어느 그룹에도 없을 때 그 그룹 목록이다.
	NotAllowedGroups []string `json:"notAllowedGroups,omitempty"`
	// RetryAfterSeconds는 rate_limited일 때 다시 시도할 수 있을 때까지의 초다.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`
	// Required는 security_required일 때 거쳐야 하는 단계다. 정책이 바뀌어 열린 터미널이 닫혔을 때는 없다.
	Required string `json:"required,omitempty" tstype:"'password' | 'twoFactor'"`
	// Kind는 prompt의 종류다. password면 질문 문구를 프론트가 현지화하고, keyboard-interactive면
	// sshd(PAM)가 준 Instruction과 Prompts의 문구를 그대로 보여 준다.
	Kind        string           `json:"kind,omitempty" tstype:"'password' | 'keyboard-interactive'"`
	Instruction string           `json:"instruction,omitempty"`
	Prompts     []TerminalPrompt `json:"prompts,omitempty"`
}

// TerminalPrompt는 질문 하나다. Echo가 false면 입력을 화면에 보이지 않는다(비밀번호).
type TerminalPrompt struct {
	Text string `json:"text"`
	Echo bool   `json:"echo"`
}

// 보안 강화 모드. 보안에 민감한 기능(웹 터미널, root 권한 상승)을 확인을 거친 세션에서만 쓰게 한다.
// 확인은 세션의 속성(Assurance)이라 따로 만료가 없다.

// 기능별 단계. SecurityPolicy의 값이다.
const (
	SecurityOff       = "off"
	SecurityPassword  = "password"
	SecurityTwoFactor = "twoFactor"
	// 파일 탐색기 정책의 적용 범위(SecurityPolicy.FilesScope)
	FilesScopeAll    = "all"
	FilesScopeWrites = "writes"
)

// 세션의 보증 수준. SessionInfo.Assurance, SecurityStatus.Assurance에 들어간다.
const (
	// AssuranceLogin은 로그인만 한 세션이다. 로그인할 때 넣은 비밀번호는 재확인으로 치지 않는다.
	AssuranceLogin     = "login"
	AssurancePassword  = "password"
	AssuranceTwoFactor = "twoFactor"
)

// 확인 수단. SecurityVerifyRequest.Method, StepUpInfo.Methods에 들어간다.
const (
	FactorPassword = "password"
	FactorTOTP     = "totp"
	FactorRecovery = "recovery"
	FactorPasskey  = "passkey"
)

// SecurityPolicy는 보안 강화 모드의 기능별 단계(Security*)다. 관리자 전역 설정(site.json)이며 빈 값은 off다.
type SecurityPolicy struct {
	Terminal  string `json:"terminal,omitempty" tstype:"'off' | 'password' | 'twoFactor'"`
	Elevation string `json:"elevation,omitempty" tstype:"'off' | 'password' | 'twoFactor'"`
	// Files는 파일 탐색기다. 기본으로 목록·내려받기 같은 읽기까지 모두 막는다(빼앗은 세션이 내려받기만 해도 정보가 샌다).
	Files string `json:"files,omitempty" tstype:"'off' | 'password' | 'twoFactor'"`
	// FilesScope는 Files를 어디에 적용하는지다. writes면 쓰기·삭제(올리기, 폴더 만들기, 이름 바꾸기·옮기기, 삭제,
	// 휴지통 연산)에만 적용하고 목록·내려받기는 연다. 없으면 all이다.
	FilesScope string `json:"filesScope,omitempty" tstype:"'all' | 'writes'"`
	// PasskeyLoginDays는 패스키로 로그인 등록의 만료 기간(일)이다. 없으면 PasskeyLoginDaysDefault다. 바꾸려면
	// 비밀번호를 다시 확인한 세션이어야 한다. 줄이면 기존 등록에도 바로 적용되고, 늘리면 새 등록부터 적용된다.
	PasskeyLoginDays int `json:"passkeyLoginDays,omitempty" tstype:"7 | 30 | 90 | 180 | 365"`
}

// PasskeyLoginDaysDefault는 패스키로 로그인 등록의 기본 만료 기간(일)이다.
const PasskeyLoginDaysDefault = 30

// SecurityPolicyRequest는 PUT /api/site/security의 본문이다. 정책 전체를 바꾼다. 바꾸는 기능마다 이전·새 단계 중
// 높은 쪽을 이 세션이 거쳤어야 한다(아니면 403 security_required).
type SecurityPolicyRequest struct {
	Security *SecurityPolicy `json:"security"`
}

// SecurityPolicyResponse는 GET /api/site/security의 응답이다(관리자).
type SecurityPolicyResponse struct {
	// Policy는 지금 정책이다. 모든 값이 채워져 있다.
	Policy SecurityPolicy `json:"policy"`
	// AutoLogoutMinutes는 OMV 자동 로그아웃 시간(분)이다. 0이면 꺼져 있어 강화 상태가 로그아웃할 때까지 남는다.
	// 모르면 없다.
	AutoLogoutMinutes *int `json:"autoLogoutMinutes,omitempty"`
	// PasskeyRPID는 패스키의 RP ID(공개 주소의 호스트 이름)다. 공개 주소를 설정하지 않아 패스키를 쓸 수 없으면 없다.
	PasskeyRPID string `json:"passkeyRpId,omitempty"`
}

// PasskeyInfo는 등록한 패스키 하나다.
type PasskeyInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Created, LastUsed는 RFC 3339 시각이다. 쓴 적이 없으면 LastUsed가 없다.
	Created  string `json:"created"`
	LastUsed string `json:"lastUsed,omitempty"`
	// BackupEligible은 동기화 저장소(iCloud 키체인, 비밀번호 관리자)의 패스키라는 뜻이다(등록할 때의 BE 플래그).
	BackupEligible bool `json:"backupEligible,omitempty"`
}

// PasskeyLoginInfo는 패스키로 로그인 등록 하나(브라우저 하나와 패스키 하나의 짝)다. 키 조각은 넣지 않는다.
type PasskeyLoginInfo struct {
	ID string `json:"id"`
	// Passkey는 이 등록에 쓰는 패스키의 ID(PasskeyInfo.ID)다.
	Passkey string `json:"passkey"`
	// Created, ExpiresAt, LastUsed는 RFC 3339 시각이다. ExpiresAt은 지금 만료 기간을 적용한 값이다. 쓴 적이 없으면
	// LastUsed가 없다.
	Created   string `json:"created"`
	ExpiresAt string `json:"expiresAt"`
	LastUsed  string `json:"lastUsed,omitempty"`
}

// TOTPInfo는 등록한 인증 앱(TOTP)이다.
type TOTPInfo struct {
	Created string `json:"created"`
}

// SecurityStatus는 GET /api/security와 확인·관리 요청의 응답이며, 로그인한 사용자 자신의 보안 상태다.
type SecurityStatus struct {
	Assurance string `json:"assurance" tstype:"'login' | 'password' | 'twoFactor'"`
	// ManageSeconds는 인증 수단을 바꿀 수 있는 관리 창이 닫히기까지 남은 초다. 닫혀 있으면 0이다.
	ManageSeconds int `json:"manageSeconds"`
	// PasskeysAvailable은 이 서버에서 패스키를 쓸 수 있는지다(공개 주소 설정). 브라우저 지원은 프론트가 따로 본다.
	PasskeysAvailable bool          `json:"passkeysAvailable"`
	Passkeys          []PasskeyInfo `json:"passkeys"`
	TOTP              *TOTPInfo     `json:"totp,omitempty"`
	// RecoveryCodes는 남은 복구 코드 수다.
	RecoveryCodes int `json:"recoveryCodes"`
	// Methods는 지금 확인에 쓸 수 있는 2차 수단이다(Factor*, password 제외).
	Methods []string       `json:"methods" tstype:"Array<'passkey' | 'totp' | 'recovery'>"`
	Policy  SecurityPolicy `json:"policy"`
	// PasskeyLogins는 만료되지 않은 패스키로 로그인 등록이다.
	PasskeyLogins []PasskeyLoginInfo `json:"passkeyLogins"`
}

// SecurityVerifyRequest는 POST /api/security/verify의 본문이다. 맞으면 이 세션의 보증 수준을 올린다
// (password는 password, 나머지는 twoFactor).
type SecurityVerifyRequest struct {
	Method   string `json:"method" tstype:"'password' | 'totp' | 'recovery' | 'passkey'"`
	Password string `json:"password,omitempty"`
	// Code는 TOTP 코드 또는 복구 코드다.
	Code    string            `json:"code,omitempty"`
	Passkey *PasskeyAssertion `json:"passkey,omitempty"`
}

// SecurityManageRequest는 POST /api/security/manage의 본문이다. 비밀번호와(등록한 수단이 있으면) 그 수단을 다시
// 확인하면 이 세션에 관리 창이 잠깐 열린다. 세션을 빼앗은 쪽이 자기 수단을 등록하지 못하게 한다.
type SecurityManageRequest struct {
	Password string `json:"password"`
	// Factor는 등록한 2차 수단의 확인이다. 수단이 없으면 비운다.
	Factor *SecurityVerifyRequest `json:"factor,omitempty"`
	// DeviceCode는 등록한 수단 대신 받는 기기 추가 코드다(POST /api/security/device-code). Factor와 함께 보내지
	// 않는다. 맞으면 관리 창을 열되 보증 수준은 password까지만 올린다.
	DeviceCode string `json:"deviceCode,omitempty"`
}

// DeviceCodeResponse는 POST /api/security/device-code의 응답이다. 수단이 있는 기기가 만들어, 그 패스키가 없는
// 새 기기의 관리 확인(SecurityManageRequest.DeviceCode)에 입력하게 한다. 사용자마다 하나이고 다시 만들면 이전
// 코드는 쓸 수 없다.
type DeviceCodeResponse struct {
	// Code는 한 번만 쓸 수 있는 코드(XXXX-XXXX)다. 이 응답에만 담긴다.
	Code string `json:"code"`
	// ExpiresInSeconds는 코드를 쓸 수 있는 남은 초다.
	ExpiresInSeconds int `json:"expiresInSeconds"`
}

// 패스키 challenge의 목적. PasskeyChallengeRequest.Purpose에 들어간다.
const (
	// PasskeyAssert는 확인(로그인 단계, 세션 중 확인, 관리 확인)이다.
	PasskeyAssert = "assert"
	// PasskeyRegister는 새 패스키 등록이다(관리 창 필요).
	PasskeyRegister = "register"
)

// PasskeyChallengeRequest는 POST /api/security/passkeys/challenge의 본문이다.
type PasskeyChallengeRequest struct {
	Purpose string `json:"purpose" tstype:"'assert' | 'register'"`
}

// PasskeyOptions는 navigator.credentials.get()·create()에 넘길 값이다. 이진 값은 base64url(패딩 없음)이다.
type PasskeyOptions struct {
	Challenge string `json:"challenge"`
	RPID      string `json:"rpId"`
	// TimeoutMs는 브라우저에 주는 제한 시간이다.
	TimeoutMs int `json:"timeoutMs"`
	// Credentials는 확인에서는 쓸 수 있는 자격 증명(allowCredentials), 등록에서는 이미 등록한 자격 증명
	// (excludeCredentials)이다.
	Credentials []PasskeyDescriptor `json:"credentials"`
	// 아래는 등록에만 있다.
	RPName     string       `json:"rpName,omitempty"`
	User       *PasskeyUser `json:"user,omitempty"`
	Algorithms []int        `json:"algorithms,omitempty"`
}

// PasskeyDescriptor는 자격 증명 하나를 가리킨다.
type PasskeyDescriptor struct {
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

// PasskeyUser는 등록할 때 인증기에 넣는 사용자 정보다. ID는 무작위 핸들이다(사용자 이름을 넣지 않는다).
type PasskeyUser struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// PasskeyAssertion은 navigator.credentials.get()의 결과다. 이진 값은 base64url이다.
type PasskeyAssertion struct {
	ID                string `json:"id"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle,omitempty"`
}

// PasskeyRegistration은 navigator.credentials.create()의 결과다. PublicKey는 getPublicKey()의 SPKI DER이며,
// 브라우저가 알고리즘을 모르면 비어 있다(등록하지 않는다).
type PasskeyRegistration struct {
	ID                 string   `json:"id"`
	ClientDataJSON     string   `json:"clientDataJSON"`
	AuthenticatorData  string   `json:"authenticatorData"`
	PublicKey          string   `json:"publicKey"`
	PublicKeyAlgorithm int      `json:"publicKeyAlgorithm"`
	Transports         []string `json:"transports,omitempty"`
}

// PasskeyRegisterRequest는 POST /api/security/passkeys의 본문이다(관리 창 필요). 등록 값이 맞으면 바로 저장하지
// 않고, 응답(PasskeyOptions)으로 그 패스키 하나만 허용하는 확인 challenge를 준다. 그 확인을
// POST /api/security/passkeys/confirm으로 보내 통과해야 저장한다.
type PasskeyRegisterRequest struct {
	Name       string               `json:"name"`
	Credential *PasskeyRegistration `json:"credential"`
}

// PasskeyConfirmRequest는 POST /api/security/passkeys/confirm의 본문이다(관리 창 필요). 등록 직후 새 패스키로 한
// 확인이며, 통과하면 패스키를 저장하고 SecurityChangeResponse를 준다.
type PasskeyConfirmRequest struct {
	Passkey *PasskeyAssertion `json:"passkey"`
}

// PasskeyLoginEnrollRequest는 POST /api/security/passkey-logins의 본문이다. 비밀번호와 그 패스키(PRF를 함께 요청한
// 확인)를 확인하면 이 브라우저의 패스키로 로그인을 켠다. 암호문은 브라우저에만 두므로 보내지 않는다.
type PasskeyLoginEnrollRequest struct {
	Password string            `json:"password"`
	Passkey  *PasskeyAssertion `json:"passkey"`
	// Replace는 이 브라우저가 전에 쓰던 등록 ID다. 있으면 함께 지운다(다시 켤 때).
	Replace string `json:"replace,omitempty"`
}

// PasskeyLoginEnrollResponse는 POST /api/security/passkey-logins의 응답이다. Share는 서버 쪽 키 조각(base64url)이며
// 브라우저가 PRF 결과와 함께 키를 만드는 데만 쓴다(저장하지 않는다).
type PasskeyLoginEnrollResponse struct {
	ID    string `json:"id"`
	Share string `json:"share"`
	// ExpiresAt은 RFC 3339 만료 시각이다.
	ExpiresAt string         `json:"expiresAt"`
	Status    SecurityStatus `json:"status"`
}

// PasskeyLoginOptionsRequest는 POST /api/login/passkey/options의 본문이다(인증 없음). 브라우저 항목의 사용자 이름과
// 등록 ID를 보내면 그 패스키 하나만 허용하는 확인 옵션(PasskeyOptions)을 준다.
type PasskeyLoginOptionsRequest struct {
	Username   string `json:"username"`
	Enrollment string `json:"enrollment"`
}

// PasskeyLoginRequest는 POST /api/login/passkey의 본문이다(인증 없음). 확인이 맞으면 PasskeyLoginUnlockResponse를 준다.
type PasskeyLoginRequest struct {
	Passkey *PasskeyAssertion `json:"passkey"`
}

// PasskeyLoginUnlockResponse는 POST /api/login/passkey의 응답이다. Share로 키를 만들어 비밀번호를 풀고(안 되면
// NextShare로 푼다) POST /api/login/passkey/finish로 보낸다. 둘 다 base64url 키 조각이다.
type PasskeyLoginUnlockResponse struct {
	Share string `json:"share"`
	// NextShare는 회전을 기다리는 다음 키 조각이다. 브라우저는 풀어 낸 비밀번호를 이것으로 다시 암호화해 항목을 바꾼다.
	// 확정(finish의 Rotated)할 때까지 같은 값을 준다.
	NextShare string `json:"nextShare"`
}

// PasskeyLoginFinishRequest는 POST /api/login/passkey/finish의 본문이다. 풀어 낸 비밀번호로 로그인하며 응답은
// LoginResponse다. 풀지 못했으면 Discard로 그 등록을 지운다(204).
type PasskeyLoginFinishRequest struct {
	Password string `json:"password,omitempty"`
	// Rotated는 브라우저가 항목을 NextShare용으로 바꿔 저장했다는 뜻이다. OMV가 비밀번호를 받으면 NextShare를 Share로
	// 확정한다.
	Rotated bool `json:"rotated,omitempty"`
	Discard bool `json:"discard,omitempty"`
}

// TOTPSetupResponse는 POST /api/security/totp/setup의 응답이다. 비밀 키는 확인(POST /api/security/totp)할 때까지
// 이 세션의 메모리에만 있다.
type TOTPSetupResponse struct {
	Secret string `json:"secret"`
	// URI는 인증 앱에 넣는 otpauth:// 주소다(QR 코드의 내용).
	URI string `json:"uri"`
}

// TOTPConfirmRequest는 POST /api/security/totp의 본문이다. 인증 앱이 보여 주는 코드로 설정을 마친다.
type TOTPConfirmRequest struct {
	Code string `json:"code"`
}

// SecurityChangeResponse는 수단을 등록·삭제하거나 복구 코드를 다시 만든 뒤의 응답이다.
type SecurityChangeResponse struct {
	Status SecurityStatus `json:"status"`
	// RecoveryCodes는 새로 만든 복구 코드다. 첫 수단을 등록했거나 다시 만들었을 때만 있고, 다시 볼 수 없다.
	RecoveryCodes []string `json:"recoveryCodes,omitempty"`
}

// SecurityUserInfo는 GET /api/security/users(관리자)의 한 줄이며, 인증 수단 기록이 있는 사용자다.
type SecurityUserInfo struct {
	Username      string `json:"username"`
	Passkeys      int    `json:"passkeys"`
	TOTP          bool   `json:"totp"`
	RecoveryCodes int    `json:"recoveryCodes"`
	// Unreadable은 기록을 읽을 수 없다는 뜻이다(손상된 파일). 초기화하면 지운다.
	Unreadable bool `json:"unreadable,omitempty"`
}

// 대시보드에서 업그레이드를 쓸 수 없는 이유(UpgradeInfo.DisabledReason).
const (
	// UpgradeNotEnabled: 설치기가 대시보드에서 업그레이드를 켜지 않았다(/etc/default/onmaru-upgrade가 없다).
	UpgradeNotEnabled = "not-enabled"
	// UpgradeNoHelper: 권한 도우미를 설정하지 않았다(-helper-socket). 업그레이드는 도우미의 root 연산으로 요청한다.
	UpgradeNoHelper = "no-helper"
	// UpgradeStateDir: 상태 디렉터리가 없거나 설치한 위치(/var/lib/onmaru)가 아니다. 업그레이드 서비스의 경로 유닛은
	// 그 안의 확인 요청만 지켜본다.
	UpgradeStateDir = "state-dir"
)

// UpgradeInfo는 GET /api/upgrade(관리자)의 응답이다. 설정 앱의 업데이트 페이지가 보여 준다.
// Status는 업그레이드 서비스가 쓴 상태 파일 그대로이고 형식은 backend/config(upgrade_request.go)에 있다.
type UpgradeInfo struct {
	// Enabled는 대시보드에서 업그레이드를 쓸 수 있다는 뜻이다. 아니면 DisabledReason에 이유가 있다.
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabledReason,omitempty" tstype:"'not-enabled' | 'no-helper' | 'state-dir'"`
	// Repo, User는 업그레이드할 저장소와 저장소 소유자(빌드 계정)다. 수동 재설치 안내에 쓴다. 모르면 없다.
	Repo string `json:"repo,omitempty"`
	User string `json:"user,omitempty"`
	// Pending은 확인 요청을 업그레이드 서비스가 아직 가져가지 않았다는 뜻이다. PendingStalled는 1분 넘게 남아 있다는
	// 뜻이다(경로 유닛 onmaru-upgrade.path가 돌지 않는다).
	Pending        bool `json:"pending"`
	PendingStalled bool `json:"pendingStalled"`
	// Interrupted는 상태 파일이 확인·업그레이드 중인 채로 업그레이드 서비스의 시간 상한(2시간)보다 오래 바뀌지 않았다는
	// 뜻이다. 서비스가 중단됐으므로 다시 확인할 수 있다.
	Interrupted bool `json:"interrupted"`
	// Version, Revision은 지금 실행 중인 onmaru의 버전과 커밋이다(Revision은 모르면 없다).
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	// Installed는 설치기가 남긴 설치 기록이다(/var/lib/onmaru-upgrade/installed.json). 없거나 읽을 수 없으면 null이다.
	// 실행 파일에 커밋이 없을 때(go run) 화면이 이 기록의 커밋을 보인다.
	Installed *config.InstallRecord `json:"installed"`
	// Status는 업그레이드 서비스의 상태 파일이다. 없거나 읽을 수 없으면 null이다.
	Status *config.UpgradeStatus `json:"status"`
}

// StartUpgradeCheckRequest는 POST /api/upgrade/check의 본문이다.
type StartUpgradeCheckRequest struct {
	// Mode는 확인 방식이다. remote는 git fetch 뒤 추적 브랜치, local은 지금 체크아웃(HEAD)을 대상으로 삼는다.
	Mode string `json:"mode" tstype:"'remote' | 'local'"`
}

// StartUpgradeRequest는 POST /api/upgrade/apply의 본문이다. root 권한 상승이 필요하다.
type StartUpgradeRequest struct {
	// Target은 확인이 정한 대상 커밋(40자리 16진수)이다.
	Target string `json:"target"`
	// Settings는 업그레이드 계획이 물은 설정의 이름과 값이다(confirm은 yes/no).
	Settings map[string]string `json:"settings"`
}

// 파일 탐색기. 파일은 권한 도우미가 로그인한 사용자의 권한으로 다룬다(fs.names, fs.statmany, fs.info, fs.read, fs.write).
// 경로는 모두 절대 경로이고, 대시보드가 보이는 폴더(홈, 공유 폴더, 디스크) 안이어야 한다.

// SiteFiles는 파일 탐색기의 전역 설정이다(site.json).
type SiteFiles struct {
	// Shares는 관리자 세션이 OMV에서 받은 공유 폴더 목록이다(이름표). 실제로 닿을 수 있는 범위는 도우미가 정한다.
	Shares []SharedFolder `json:"shares,omitempty"`
	// SharesUpdatedAt은 Shares를 마지막으로 받은 시각(RFC 3339)이다. 비어 있으면 받은 적이 없다.
	SharesUpdatedAt string `json:"sharesUpdatedAt,omitempty"`
	// ShowDisks는 관리자에게 /srv 아래 디스크의 맨 위까지 보이는지다(디스크 최상위 보기).
	ShowDisks bool `json:"showDisks,omitempty"`
}

// SharedFolder는 OMV 공유 폴더 하나다(ShareMgmt.enumerateSharedFolders).
type SharedFolder struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	// Path는 공유 폴더의 절대 경로다(OMV absdirpath).
	Path string `json:"path"`
}

// FilesRoots는 GET /api/files/roots의 응답이다. 파일 탐색기 사이드바에 처음 보이는 폴더다.
type FilesRoots struct {
	// Home은 로그인한 사용자의 홈 폴더다. 권한 도우미가 쓸 수 있는 홈이 없으면 없다.
	Home *FilesRoot `json:"home,omitempty"`
	// Shares는 공유 폴더 스냅숏이다(이름순).
	Shares []FilesShare `json:"shares"`
	// Disks는 /srv 아래 마운트다. 스냅숏이 없거나(Fallback) 관리자가 디스크 최상위 보기를 켰을 때 관리자에게만 있다.
	Disks []FilesRoot `json:"disks"`
	// Fallback은 공유 폴더 스냅숏을 아직 받지 못해 디스크를 대신 보인다는 뜻이다.
	Fallback bool `json:"fallback"`
	// SharesUpdatedAt은 스냅숏을 받은 시각(RFC 3339)이다.
	SharesUpdatedAt string `json:"sharesUpdatedAt,omitempty"`
}

// FilesRoot는 사이드바 항목 하나다.
type FilesRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// FilesShare는 사이드바의 공유 폴더 하나다.
type FilesShare struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Path string `json:"path"`
	// Reachable은 권한 도우미가 닿을 수 있는 곳(/srv 아래 마운트)인지다. 루트 파일시스템의 공유 폴더는 아직 열 수 없다.
	Reachable bool `json:"reachable"`
}

// FilesListing은 GET /api/files/list의 응답이다. 폴더의 이름 목록을 서버가 세션별로 잠시 두고 ID로 가리킨다.
type FilesListing struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// Total은 폴더의 항목 수다(찾기를 적용하지 않은 수).
	Total int `json:"total"`
	// Truncated는 항목이 상한(30만 개)을 넘어 일부만 읽었다는 뜻이다.
	Truncated bool `json:"truncated"`
	// Writable은 이 사용자가 이 폴더에 파일을 만들 수 있는지다(올리기).
	Writable bool `json:"writable"`
}

// FilesPage는 GET /api/files/list/{id}의 응답이다. 정렬·찾기를 적용한 보기에서 offset부터의 항목이다.
type FilesPage struct {
	// Total은 보기의 항목 수다(찾기를 적용한 수).
	Total  int        `json:"total"`
	Offset int        `json:"offset"`
	Items  []FileItem `json:"items"`
}

// FileItem은 목록의 항목 하나다. names=1로 이름만 받으면 State가 pending이고 정보가 없다.
type FileItem struct {
	Name string `json:"name"`
	// Type은 항목 종류다. unknown은 파일시스템이 종류를 주지 않았고 아직 정보를 읽지 않았다는 뜻이다.
	Type string `json:"type" tstype:"'dir' | 'file' | 'symlink' | 'other' | 'unknown'"`
	// State는 정보를 읽었는지다. gone은 읽는 사이에 지워졌다는 뜻, error는 읽지 못했다는 뜻이다.
	State string `json:"state" tstype:"'ok' | 'pending' | 'gone' | 'error'"`
	// Size는 일반 파일의 크기(바이트)다.
	Size int64 `json:"size,omitempty"`
	// ModTime은 수정 시각(Unix 밀리초)이다.
	ModTime int64 `json:"modTime,omitempty"`
	// Locked는 폴더인데 이 사용자가 들어갈 수 없다는 뜻이다.
	Locked bool `json:"locked,omitempty"`
	// LinkType은 심볼릭 링크를 따라간 대상의 종류다(깨졌거나 범위 밖이면 없다).
	LinkType string `json:"linkType,omitempty" tstype:"'dir' | 'file' | 'symlink' | 'other'"`
	// BadName은 이름이 UTF-8이 아니라 열거나 내려받을 수 없다는 뜻이다(Name은 바꿔 그린 이름이다).
	BadName bool `json:"badName,omitempty"`
}

// FilesFindResponse는 GET /api/files/list/{id}/find의 응답이다. 만들거나 이름을 바꾼 항목을 목록에서 고를 때 쓴다.
type FilesFindResponse struct {
	// Index는 보기(정렬·찾기)에서 그 이름의 자리다. 보기에 없으면 -1이다.
	Index int `json:"index"`
}

// FilesMkdirRequest는 POST /api/files/mkdir의 본문이다. 응답은 만든 폴더의 FileItem이다.
type FilesMkdirRequest struct {
	// Path는 만들 폴더의 절대 경로다.
	Path string `json:"path"`
}

// FilesRenameRequest는 POST /api/files/rename의 본문이다. 이름을 바꾸거나 다른 폴더로 옮긴다(같은 디스크 안).
// 응답은 옮긴 뒤의 FileItem이다. To가 이미 있으면 file_exists이고 덮어쓰지 않는다.
type FilesRenameRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// FilesDeleteRequest는 POST /api/files/delete의 본문이다. 되돌릴 수 없는 삭제이고 응답 본문은 없다(204).
type FilesDeleteRequest struct {
	Path string `json:"path"`
	// Recursive면 폴더 안의 항목까지 모두 지운다. 아니면 파일과 빈 폴더만 지운다(not_empty).
	Recursive bool `json:"recursive"`
}

// FilesTrashRequest는 POST /api/files/trash의 본문이다. 항목을 휴지통으로 옮긴다.
type FilesTrashRequest struct {
	Path string `json:"path"`
}

// FilesTrashResult는 POST /api/files/trash의 응답이다. 넣은 휴지통과 그 안의 이름이다(되돌릴 때 쓴다).
type FilesTrashResult struct {
	Trash string `json:"trash"`
	Name  string `json:"name"`
}

// TrashVolume은 홈 폴더나 디스크 하나의 휴지통이다.
type TrashVolume struct {
	// Top은 홈 폴더나 디스크(/srv 아래 마운트 지점)의 경로다. 그 안의 항목은 이 휴지통에 들어간다.
	Top string `json:"top"`
	// Kind는 휴지통 종류다: home(홈 휴지통), shared(디스크의 .Trash/<uid>), own(디스크의 .Trash-<uid>),
	// none(휴지통이 없어 영구 삭제만 할 수 있다).
	Kind string `json:"kind" tstype:"'home' | 'shared' | 'own' | 'none'"`
	// Shared는 디스크 맨 위에 모든 사용자가 쓰는 .Trash가 있는지다.
	Shared bool `json:"shared"`
}

// FilesTrashVolumes는 GET /api/files/trash/volumes의 응답이다.
type FilesTrashVolumes struct {
	Volumes []TrashVolume `json:"volumes"`
}

// TrashItem은 휴지통 항목 하나다.
type TrashItem struct {
	// Trash는 휴지통 디렉터리, Name은 그 안의 이름이다(되돌리기·영구 삭제에 그대로 보낸다).
	Trash string `json:"trash"`
	Name  string `json:"name"`
	// Path는 원래 경로다.
	Path string `json:"path"`
	// DeletedAt은 지운 시각(Unix 밀리초)이다. 모르면 0이다.
	DeletedAt int64  `json:"deletedAt"`
	Type      string `json:"type" tstype:"'dir' | 'file' | 'symlink' | 'other'"`
	Size      int64  `json:"size"`
	// ModTime은 수정 시각(Unix 밀리초)이다.
	ModTime int64 `json:"modTime"`
	// BadName은 이름이나 원래 경로가 UTF-8이 아니라 되돌리거나 지울 수 없다는 뜻이다.
	BadName bool `json:"badName,omitempty"`
}

// FilesTrashList는 GET /api/files/trash의 응답이다(지운 시각이 늦은 것부터).
type FilesTrashList struct {
	Items []TrashItem `json:"items"`
	// Truncated는 항목이 많아 일부만 담았다는 뜻이다.
	Truncated bool `json:"truncated"`
}

// FilesTrashRestoreRequest는 POST /api/files/trash/restore의 본문이다. 항목을 To(원래 경로나 같은 폴더의 새 이름)로
// 되돌린다. 응답은 되돌린 FileItem이다.
type FilesTrashRestoreRequest struct {
	Trash string `json:"trash"`
	Name  string `json:"name"`
	To    string `json:"to"`
}

// FilesTrashItemRequest는 POST /api/files/trash/purge의 본문이다. 휴지통 항목 하나를 영구 삭제한다(204).
type FilesTrashItemRequest struct {
	Trash string `json:"trash"`
	Name  string `json:"name"`
}

// SiteTrashDisk는 디스크 하나의 공유 휴지통(.Trash) 상태다(관리자).
type SiteTrashDisk struct {
	// Name은 디스크 이름표(라벨, 없으면 마운트 지점 이름), Path는 마운트 지점이다.
	Name string `json:"name"`
	Path string `json:"path"`
	// Shared는 디스크 맨 위에 모든 사용자가 쓰는 .Trash(1777)가 있는지다.
	Shared bool `json:"shared"`
}

// SiteTrashDisks는 GET·POST /api/site/files/trash의 응답이다(관리자).
type SiteTrashDisks struct {
	Disks []SiteTrashDisk `json:"disks"`
}

// SiteTrashPrepareRequest는 POST /api/site/files/trash의 본문이다. 그 디스크 맨 위에 .Trash(1777)를 만든다
// (root 권한 상승 필요).
type SiteTrashPrepareRequest struct {
	Disk string `json:"disk"`
}

// FilesPrepareResponse는 POST /api/files/list/{id}/prepare의 응답이다. Filled가 Total이면 날짜·크기순으로 볼 수 있다.
type FilesPrepareResponse struct {
	Filled int `json:"filled"`
	Total  int `json:"total"`
}

// FileInfo는 GET /api/files/info의 응답이다(정보 패널).
type FileInfo struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Type     string `json:"type" tstype:"'dir' | 'file' | 'symlink' | 'other'"`
	Size     int64  `json:"size"`
	ModTime  int64  `json:"modTime"`
	Mode     uint32 `json:"mode"`
	Target   string `json:"target,omitempty"`
	LinkType string `json:"linkType,omitempty" tstype:"'dir' | 'file' | 'symlink' | 'other'"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	// Owner, Group은 이름이다. 찾지 못했으면 없다(화면은 UID, GID를 보인다).
	Owner string `json:"owner,omitempty"`
	Group string `json:"group,omitempty"`
	// ACL은 POSIX ACL이 있는지다. 확인하지 못했으면 없다(심볼릭 링크 포함).
	ACL *bool `json:"acl,omitempty"`
	// Access는 이 사용자가 읽고 쓸 수 있는지다. 확인하지 못했으면 없다(심볼릭 링크 포함).
	Access *FileAccess `json:"access,omitempty"`
	// ShareUUID는 이 경로가 속한 공유 폴더다(관리자에게 OMV ACL 화면 링크를 줄 때 쓴다). 관리자에게만 준다.
	ShareUUID string `json:"shareUuid,omitempty"`
}

// FileAccess는 이 사용자의 권한이다. 폴더는 들어가 목록을 보는 것이 읽기, 안에 만드는 것이 쓰기다.
type FileAccess struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
}

// FilesSettings는 GET /api/site/files의 응답이자 POST /api/site/files/refresh의 응답이다(관리자).
type FilesSettings struct {
	Shares          []SharedFolder `json:"shares"`
	SharesUpdatedAt string         `json:"sharesUpdatedAt,omitempty"`
	ShowDisks       bool           `json:"showDisks"`
}

// FilesSettingsRequest는 PUT /api/site/files의 본문이다.
type FilesSettingsRequest struct {
	ShowDisks bool `json:"showDisks"`
}
