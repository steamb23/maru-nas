package api

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
