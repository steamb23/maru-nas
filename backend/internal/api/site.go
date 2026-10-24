package api

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
	// Upgrade는 대시보드에서 업그레이드의 설정이다. 없으면 업데이트 채널을 고르지 않은 것이다(지금 브랜치를 따른다).
	Upgrade *SiteUpgrade `json:"upgrade,omitempty"`
}

// SiteUpgrade는 대시보드에서 업그레이드의 전역 설정이다(.claude/docs/upgrade.md의 "업데이트 채널").
type SiteUpgrade struct {
	// Channel은 원격 확인이 따라갈 업데이트 채널이다(config.UpgradeChannel*). 빈 값이면 지금 브랜치의 추적 브랜치다.
	Channel string `json:"channel,omitempty" tstype:"'' | 'stable' | 'main'"`
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
// (/etc/default/maru) 값을 따른다.
type ServerOverrides struct {
	// WorkbenchURL은 -workbench-url을 덮어쓴다. 빈 문자열이면 OMV 워크벤치 앱을 끈다.
	WorkbenchURL *string `json:"workbenchUrl,omitempty"`
	// SmartRefreshAt은 -smart-refresh-at(HH:MM)을 덮어쓴다.
	SmartRefreshAt *string `json:"smartRefreshAt,omitempty"`
	// Terminal은 -terminal을 덮어쓴다. "true" 또는 "false"다.
	Terminal *string `json:"terminal,omitempty"`
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
	// Runtime은 시작할 때 정해진 가동 설정 전체다(플래그 순서). 바꾸려면 /etc/default/maru를 고치고 다시 시작한다.
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
