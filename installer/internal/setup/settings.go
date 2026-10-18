package setup

// settings는 설치 설정 목록이다. 질문, 요약, 환경 파일의 줄, 사용법이 이 순서를 따른다.
// 새 설정은 setting_<이름>.go에 정의하고 여기에 한 줄 더한다(.claude/rules/installer.md).
var settings = Registry{
	settingUser,
	settingListen,
	settingAllowNonLoopback,
	settingOMVURL,
	settingSmartRefreshAt,
	settingWorkbenchURL,
	settingPublicOrigin,
	settingHelper,
	settingElevateGroup,
	settingTerminal,
}

// 설정 이름(플래그 이름, Values 키). 설정 정의 안의 함수가 다른 설정 값을 읽을 때 쓴다
// (설정 변수를 직접 가리키면 패키지 초기화 순환이 생긴다).
const (
	nameUser             = "user"
	nameListen           = "listen"
	nameAllowNonLoopback = "allow-non-loopback"
	nameOMVURL           = "omv-url"
	nameSmartRefreshAt   = "smart-refresh-at"
	nameWorkbenchURL     = "workbench-url"
	namePublicOrigin     = "public-origin"
	nameHelper           = "helper"
	nameElevateGroup     = "elevate-group"
	nameTerminal         = "terminal"
)
