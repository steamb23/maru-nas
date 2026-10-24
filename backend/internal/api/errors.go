package api

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
	// ErrShareReadOnly는 OMV 공유 폴더 권한이 읽기만 허락해 그 공유 폴더 안에 쓸 수 없다는 뜻이다(공유 폴더 권한 따르기).
	ErrShareReadOnly = "share_read_only"
	// ErrJobBusy는 서버 백그라운드 작업(시스템 업데이트의 확인·설치)이 이미 진행 중이라 시작하지 않았다는 뜻이다.
	ErrJobBusy = "job_busy"
	// ErrAptLocked는 다른 프로세스(자동 업데이트 등)가 패키지 데이터베이스를 쓰고 있어 시작하지 않았다는 뜻이다.
	ErrAptLocked = "apt_locked"
)

// ErrorResponse는 2xx가 아닌 상태 코드와 함께 반환된다.
type ErrorResponse struct {
	Error string `json:"error"`
	// Code는 화면에서 분기할 때 쓰는 오류 종류다. 없을 수 있다.
	Code string `json:"code,omitempty" tstype:"'invalid_credentials' | 'rate_limited' | 'challenge_failed' | 'challenge_expired' | 'omv_unavailable' | 'unauthorized' | 'bad_request' | 'rpc_forbidden' | 'admin_required' | 'plugin_missing' | 'omv_error' | 'action_busy' | 'action_failed' | 'action_timeout' | 'config_dirty' | 'image_too_large' | 'unsupported_image' | 'profile_readonly' | 'helper_unavailable' | 'helper_reauth' | 'elevation_failed' | 'elevation_not_allowed' | 'elevation_required' | 'terminal_disabled' | 'security_required' | 'factor_failed' | 'manage_required' | 'passkey_unavailable' | 'passkey_uv_unsupported' | 'security_unavailable' | 'passkey_login_unknown' | 'passkey_login_expired' | 'passkey_login_stale' | 'upgrade_disabled' | 'upgrade_busy' | 'upgrade_outdated' | 'not_found' | 'permission_denied' | 'outside_roots' | 'file_exists' | 'not_dir' | 'is_dir' | 'not_regular' | 'no_space' | 'read_only' | 'helper_busy' | 'transfer_timeout' | 'listing_expired' | 'sort_pending' | 'not_empty' | 'cross_device' | 'root_folder' | 'no_trash' | 'share_read_only' | 'job_busy' | 'apt_locked'"`
	// RetryAfterSeconds는 rate_limited일 때 다시 시도할 수 있기까지 남은 초다.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`
	// Required는 security_required일 때 거쳐야 하는 단계다.
	Required string `json:"required,omitempty" tstype:"'password' | 'twoFactor'"`
}
