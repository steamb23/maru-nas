// Package api는 HTTP API가 주고받는 JSON 형태를 정의한다.
// 이 패키지에서 tygo로 TypeScript 타입을 생성한다(tygo.yaml 참고).
package api

import "encoding/json"

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
	SampledAtMs   int64       `json:"sampledAtMs"`
	CPU           CPUCounters `json:"cpu"`
	Memory        MemoryInfo  `json:"memory"`
	UptimeSeconds float64     `json:"uptimeSeconds"`
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
	// Cores는 켜져 있는 코어별 값이다(cpuN 순서). 꺼진 코어는 빠지므로 ID로 맞춘다.
	Cores []CoreTicks `json:"cores"`
}

// CoreTicks는 코어 하나의 누적 CPU 시간이다.
type CoreTicks struct {
	// ID는 cpuN의 N이다.
	ID         int    `json:"id"`
	TotalTicks uint64 `json:"totalTicks"`
	IdleTicks  uint64 `json:"idleTicks"`
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
)

// ErrorResponse는 2xx가 아닌 상태 코드와 함께 반환된다.
type ErrorResponse struct {
	Error string `json:"error"`
	// Code는 화면에서 분기할 때 쓰는 오류 종류다. 없을 수 있다.
	Code string `json:"code,omitempty" tstype:"'invalid_credentials' | 'rate_limited' | 'challenge_failed' | 'challenge_expired' | 'omv_unavailable' | 'unauthorized' | 'bad_request' | 'rpc_forbidden' | 'admin_required' | 'plugin_missing' | 'omv_error' | 'action_busy' | 'action_failed' | 'action_timeout' | 'config_dirty' | 'image_too_large' | 'unsupported_image' | 'profile_readonly' | 'helper_unavailable' | 'helper_reauth' | 'elevation_failed' | 'elevation_not_allowed' | 'elevation_required'"`
	// RetryAfterSeconds는 rate_limited일 때 다시 시도할 수 있기까지 남은 초다.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`
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
