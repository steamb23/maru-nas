package api

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
	// Files는 파일 탐색기의 보기 설정이다. 한 번도 저장하지 않았으면 없다(프론트가 기본값을 쓴다).
	Files *FilesPrefs `json:"files,omitempty"`
	// Appearance는 화면 설정(테마, 바탕화면 등)이다. 없거나 비어 있는 값은 관리자가 정한 기본 화면을 따른다.
	Appearance *AppearancePrefs `json:"appearance,omitempty"`
	// Notifications는 알림 읽음 상태다. 한 번도 저장하지 않았으면 없다.
	Notifications *NotificationPrefs `json:"notifications,omitempty"`
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

// 파일 탐색기의 보기 모양. FilesPrefs.View에 들어간다.
const (
	// FilesViewList는 자세히 목록(이름·수정한 날짜·크기·종류 열)이다.
	FilesViewList = "list"
	// FilesViewGrid는 아이콘 보기(종류 아이콘과 이름의 칸)다.
	FilesViewGrid = "grid"
)

// FilesPrefs는 파일 탐색기의 보기 설정이다. 새로 여는 탐색기 창이 이 보기로 시작한다.
type FilesPrefs struct {
	// View는 FilesView* 중 하나다. 프론트는 모르는 값이면 list를 쓴다.
	View string `json:"view"`
	// HiddenShares는 사이드바에서 숨긴 공유 폴더의 OMV uuid다(FilesShare.UUID). 사이드바 표시만 바꾸며 접근과는
	// 관계없다(숨긴 공유 폴더도 보이는 폴더에 남는다). 스냅숏에 없는 uuid는 무시한다.
	HiddenShares []string `json:"hiddenShares,omitempty"`
}

// FilesPrefs.HiddenShares의 상한.
const (
	// MaxHiddenShares는 숨긴 공유 폴더 수의 상한이다.
	MaxHiddenShares = 512
	// MaxHiddenShareBytes는 항목 하나(uuid)의 바이트 상한이다.
	MaxHiddenShareBytes = 64
)

// FilesPrefsRequest는 PUT /api/preferences/files의 요청 본문이다.
type FilesPrefsRequest struct {
	Files *FilesPrefs `json:"files"`
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
