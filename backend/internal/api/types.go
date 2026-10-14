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
	// Write는 상태를 바꾸는 메서드다. 쓰기 허용 여부는 addons.WritesEnabled가 정한다.
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

// Preferences는 GET /api/preferences의 응답이며 로그인한 사용자의 대시보드 설정이다.
// 백엔드 상태 디렉터리에 사용자별로 저장되어 로그아웃해도 남는다.
type Preferences struct {
	// Widgets는 바탕화면 위젯 배치다. 한 번도 저장하지 않았으면 없다(프론트가 기본 배치를 쓴다).
	Widgets *[]WidgetPlacement `json:"widgets,omitempty"`
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
	// ErrRPCForbidden는 허용 목록에 없거나 현재 단계가 허용하지 않는 OMV RPC다.
	ErrRPCForbidden = "rpc_forbidden"
	// ErrAdminRequired, ErrPluginMissing은 RPC를 선언한 애드온을 이 사용자가 쓸 수 없는 이유다.
	ErrAdminRequired = ReasonAdminRequired
	ErrPluginMissing = ReasonPluginMissing
	// ErrOMVError는 중계한 OMV RPC가 실패했다는 뜻이다(OMV의 오류 메시지는 넘기지 않는다).
	ErrOMVError = "omv_error"
)

// ErrorResponse는 2xx가 아닌 상태 코드와 함께 반환된다.
type ErrorResponse struct {
	Error string `json:"error"`
	// Code는 화면에서 분기할 때 쓰는 오류 종류다. 없을 수 있다.
	Code string `json:"code,omitempty" tstype:"'invalid_credentials' | 'rate_limited' | 'challenge_failed' | 'challenge_expired' | 'omv_unavailable' | 'unauthorized' | 'bad_request' | 'rpc_forbidden' | 'admin_required' | 'plugin_missing' | 'omv_error'"`
	// RetryAfterSeconds는 rate_limited일 때 다시 시도할 수 있기까지 남은 초다.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`
}
