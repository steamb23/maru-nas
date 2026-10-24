package api

import "encoding/json"

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
