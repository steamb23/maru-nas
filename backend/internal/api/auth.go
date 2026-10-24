package api

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
	// StepUp이 있으면 로그인을 마친 뒤 2차 인증을 권한다(보안 강화 모드). 세션은 이미 만들어졌으므로
	// 건너뛰면 일반 세션으로 쓴다. 로그인 응답에만 넣는다(세션 복원 때는 묻지 않는다).
	StepUp *StepUpInfo `json:"stepUp,omitempty"`
	// PasskeyLoginOffer가 있으면 이 사용자가 패스키로 로그인을 켤 수 있다는 뜻이다(공개 주소가 있고 패스키를 등록함).
	// 이 브라우저에 등록이 없으면 로그인 화면이 켜기를 권한다. 로그인 응답에만 넣는다.
	PasskeyLoginOffer *PasskeyLoginOffer `json:"passkeyLoginOffer,omitempty"`
}

// PasskeyLoginOffer는 로그인 직후 권하는 패스키로 로그인이다.
type PasskeyLoginOffer struct {
	// Days는 지금 켜면 쓸 수 있는 기간(일)이다(정책의 PasskeyLoginDays). 권유 문구에 보인다.
	Days int `json:"days"`
}

// StepUpInfo는 로그인 직후 권하는 2차 인증이다.
type StepUpInfo struct {
	// Methods는 이 사용자가 쓸 수 있는 수단이다(Factor*, password 제외).
	Methods []string `json:"methods" tstype:"Array<'passkey' | 'totp' | 'recovery'>"`
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
	// Assurance는 이 세션이 거친 확인의 수준이다(Assurance*, 보안 강화 모드).
	Assurance string `json:"assurance" tstype:"'login' | 'password' | 'twoFactor'"`
}

// ChallengeInfo는 2차 인증 요구 정보다. 종류는 OMV 플러그인이 정한다.
type ChallengeInfo struct {
	Kind string `json:"kind"`
	// RedirectURL이 있으면 대시보드에서 처리할 수 없는 인증이다(OMV 화면에서 진행).
	RedirectURL string `json:"redirectUrl,omitempty"`
}
