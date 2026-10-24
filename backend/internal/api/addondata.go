package api

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

// PackagesResponse는 GET /api/addons/packages의 응답이다(시스템 업데이트).
type PackagesResponse struct {
	AddonSources `tstype:",extends"`
	// Indexed가 false면 OMV가 아직 업데이트 목록을 만들지 않았다(확인한 적 없음). 그때 Packages는 비어 있다.
	Indexed bool `json:"indexed"`
	// IndexedAt은 OMV가 목록을 마지막으로 다시 만든 시각(RFC 3339)이다. apt-daily가 매일 다시 만든다.
	IndexedAt string `json:"indexedAt,omitempty"`
	// Packages는 설치할 수 있는 업데이트다(이름순).
	Packages []PackageUpdate `json:"packages"`
	Reboot   RebootInfo      `json:"reboot"`
}

// PackageUpdate는 업데이트할 패키지 하나다.
type PackageUpdate struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// OldVersion은 지금 설치된 버전이다. 새로 설치되는 패키지는 없다.
	OldVersion string `json:"oldVersion,omitempty"`
	Repository string `json:"repository,omitempty"`
	Section    string `json:"section,omitempty"`
	Summary    string `json:"summary,omitempty"`
	// Filename은 저장소 안 .deb 경로다. 워크벤치의 변경 기록 화면 주소에 쓴다.
	Filename string `json:"filename,omitempty"`
	// Size는 내려받을 크기(바이트)다.
	Size int64 `json:"size"`
}

// RebootInfo는 재부팅 권장 상태다.
type RebootInfo struct {
	Required bool `json:"required"`
	// Since는 권장이 생긴 시각(RFC 3339)이다.
	Since string `json:"since,omitempty"`
	// Packages는 재부팅을 요청한 패키지 이름이다.
	Packages []string `json:"packages,omitempty"`
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
