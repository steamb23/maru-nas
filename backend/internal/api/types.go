// Package api는 HTTP API가 주고받는 JSON 형태를 정의한다.
// 이 패키지에서 tygo로 TypeScript 타입을 생성한다(tygo.yaml 참고).
package api

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
)

// ErrorResponse는 2xx가 아닌 상태 코드와 함께 반환된다.
type ErrorResponse struct {
	Error string `json:"error"`
	// Code는 화면에서 분기할 때 쓰는 오류 종류다. 없을 수 있다.
	Code string `json:"code,omitempty" tstype:"'invalid_credentials' | 'rate_limited' | 'challenge_failed' | 'challenge_expired' | 'omv_unavailable' | 'unauthorized' | 'bad_request'"`
	// RetryAfterSeconds는 rate_limited일 때 다시 시도할 수 있기까지 남은 초다.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`
}
