// Package config는 백엔드(onmaru, onmaru-helper)와 설치기(installer)가 함께 쓰는 설정 규칙이다.
// 환경 변수 키, 기본값, 설정 값 검사 함수를 둔다.
//
// 설치기는 별도 Go 모듈이라 backend/internal을 import할 수 없으므로 이 공개 패키지만 쓴다.
// 설치기에 백엔드 의존성이 딸려 가지 않게 표준 라이브러리만 쓰고 backend/internal을 import하지 않는다
// (TestStandardLibraryOnly).
package config

// onmaru 실행 파일의 환경 변수 키. 같은 이름의 명령줄 플래그보다 우선순위가 낮다.
const (
	EnvListen           = "ONMARU_LISTEN"
	EnvAllowNonLoopback = "ONMARU_ALLOW_NON_LOOPBACK"
	EnvOMVURL           = "ONMARU_OMV_URL"
	EnvCookiePath       = "ONMARU_COOKIE_PATH"
	EnvStateDir         = "ONMARU_STATE_DIR"
	// EnvSystemdStateDir는 systemd의 StateDirectory=가 넘겨주는 상태 디렉터리다(EnvStateDir가 없을 때 쓴다).
	EnvSystemdStateDir = "STATE_DIRECTORY"
	EnvWorkbenchURL    = "ONMARU_WORKBENCH_URL"
	EnvSmartRefreshAt  = "ONMARU_SMART_REFRESH_AT"
	EnvHelperSocket    = "ONMARU_HELPER_SOCKET"
	EnvTerminal        = "ONMARU_TERMINAL"
	EnvSSHAddr         = "ONMARU_SSH_ADDR"
	EnvSSHHostKeys     = "ONMARU_SSH_HOST_KEYS"
	EnvPublicOrigin    = "ONMARU_PUBLIC_ORIGIN"
)

// onmaru-helper 실행 파일의 환경 변수 키.
const (
	EnvHelperClientUser   = "ONMARU_HELPER_CLIENT_USER"
	EnvHelperElevateGroup = "ONMARU_HELPER_ELEVATE_GROUP"
)

// 기본값. 빈 값이 기본인 설정(워크벤치 주소, 공개 주소, 도우미 소켓 등)은 따로 두지 않는다.
const (
	DefaultListen         = "127.0.0.1:8090"
	DefaultOMVURL         = "http://127.0.0.1"
	DefaultCookiePath     = "/"
	DefaultSmartRefreshAt = "00:00"
	// DefaultSSHAddr는 sshd_config의 Port로 루프백에 붙는다는 뜻이다.
	DefaultSSHAddr = "auto"
	// DefaultSSHHostKeys는 sshd 호스트 공개 키 파일의 glob 패턴이다(0644라 누구나 읽는다).
	DefaultSSHHostKeys = "/etc/ssh/ssh_host_*_key.pub"

	// DefaultHelperClientUser는 도우미가 연결을 받는 사용자(onmaru 서비스 사용자)의 기본값이다.
	DefaultHelperClientUser = "onmaru"
	// DefaultElevateGroup은 root 권한 상승에 openmediavault-admin과 함께 필요한 그룹의 기본값이다.
	DefaultElevateGroup = "sudo"
)
