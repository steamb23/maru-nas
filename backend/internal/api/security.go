package api

// 보안 강화 모드. 보안에 민감한 기능(웹 터미널, root 권한 상승)을 확인을 거친 세션에서만 쓰게 한다.
// 확인은 세션의 속성(Assurance)이라 따로 만료가 없다.

// 기능별 단계. SecurityPolicy의 값이다.
const (
	SecurityOff       = "off"
	SecurityPassword  = "password"
	SecurityTwoFactor = "twoFactor"
	// 파일 탐색기 정책의 적용 범위(SecurityPolicy.FilesScope)
	FilesScopeAll    = "all"
	FilesScopeWrites = "writes"
)

// 세션의 보증 수준. SessionInfo.Assurance, SecurityStatus.Assurance에 들어간다.
const (
	// AssuranceLogin은 로그인만 한 세션이다. 로그인할 때 넣은 비밀번호는 재확인으로 치지 않는다.
	AssuranceLogin     = "login"
	AssurancePassword  = "password"
	AssuranceTwoFactor = "twoFactor"
)

// 확인 수단. SecurityVerifyRequest.Method, StepUpInfo.Methods에 들어간다.
const (
	FactorPassword = "password"
	FactorTOTP     = "totp"
	FactorRecovery = "recovery"
	FactorPasskey  = "passkey"
)

// SecurityPolicy는 보안 강화 모드의 기능별 단계(Security*)다. 관리자 전역 설정(site.json)이며 빈 값은 off다.
type SecurityPolicy struct {
	Terminal  string `json:"terminal,omitempty" tstype:"'off' | 'password' | 'twoFactor'"`
	Elevation string `json:"elevation,omitempty" tstype:"'off' | 'password' | 'twoFactor'"`
	// Files는 파일 탐색기다. 기본으로 목록·내려받기 같은 읽기까지 모두 막는다(빼앗은 세션이 내려받기만 해도 정보가 샌다).
	Files string `json:"files,omitempty" tstype:"'off' | 'password' | 'twoFactor'"`
	// FilesScope는 Files를 어디에 적용하는지다. writes면 쓰기·삭제(올리기, 폴더 만들기, 이름 바꾸기·옮기기, 삭제,
	// 휴지통 연산)에만 적용하고 목록·내려받기는 연다. 없으면 all이다.
	FilesScope string `json:"filesScope,omitempty" tstype:"'all' | 'writes'"`
	// PasskeyLoginDays는 패스키로 로그인 등록의 만료 기간(일)이다. 없으면 PasskeyLoginDaysDefault다. 바꾸려면
	// 비밀번호를 다시 확인한 세션이어야 한다. 줄이면 기존 등록에도 바로 적용되고, 늘리면 새 등록부터 적용된다.
	PasskeyLoginDays int `json:"passkeyLoginDays,omitempty" tstype:"7 | 30 | 90 | 180 | 365"`
}

// PasskeyLoginDaysDefault는 패스키로 로그인 등록의 기본 만료 기간(일)이다.
const PasskeyLoginDaysDefault = 30

// SecurityPolicyRequest는 PUT /api/site/security의 본문이다. 정책 전체를 바꾼다. 바꾸는 기능마다 이전·새 단계 중
// 높은 쪽을 이 세션이 거쳤어야 한다(아니면 403 security_required).
type SecurityPolicyRequest struct {
	Security *SecurityPolicy `json:"security"`
}

// SecurityPolicyResponse는 GET /api/site/security의 응답이다(관리자).
type SecurityPolicyResponse struct {
	// Policy는 지금 정책이다. 모든 값이 채워져 있다.
	Policy SecurityPolicy `json:"policy"`
	// AutoLogoutMinutes는 OMV 자동 로그아웃 시간(분)이다. 0이면 꺼져 있어 강화 상태가 로그아웃할 때까지 남는다.
	// 모르면 없다.
	AutoLogoutMinutes *int `json:"autoLogoutMinutes,omitempty"`
	// PasskeyRPID는 패스키의 RP ID(공개 주소의 호스트 이름)다. 공개 주소를 설정하지 않아 패스키를 쓸 수 없으면 없다.
	PasskeyRPID string `json:"passkeyRpId,omitempty"`
}

// PasskeyInfo는 등록한 패스키 하나다.
type PasskeyInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Created, LastUsed는 RFC 3339 시각이다. 쓴 적이 없으면 LastUsed가 없다.
	Created  string `json:"created"`
	LastUsed string `json:"lastUsed,omitempty"`
	// BackupEligible은 동기화 저장소(iCloud 키체인, 비밀번호 관리자)의 패스키라는 뜻이다(등록할 때의 BE 플래그).
	BackupEligible bool `json:"backupEligible,omitempty"`
}

// PasskeyLoginInfo는 패스키로 로그인 등록 하나(브라우저 하나와 패스키 하나의 짝)다. 키 조각은 넣지 않는다.
type PasskeyLoginInfo struct {
	ID string `json:"id"`
	// Passkey는 이 등록에 쓰는 패스키의 ID(PasskeyInfo.ID)다.
	Passkey string `json:"passkey"`
	// Created, ExpiresAt, LastUsed는 RFC 3339 시각이다. ExpiresAt은 지금 만료 기간을 적용한 값이다. 쓴 적이 없으면
	// LastUsed가 없다.
	Created   string `json:"created"`
	ExpiresAt string `json:"expiresAt"`
	LastUsed  string `json:"lastUsed,omitempty"`
}

// TOTPInfo는 등록한 인증 앱(TOTP)이다.
type TOTPInfo struct {
	Created string `json:"created"`
}

// SecurityStatus는 GET /api/security와 확인·관리 요청의 응답이며, 로그인한 사용자 자신의 보안 상태다.
type SecurityStatus struct {
	Assurance string `json:"assurance" tstype:"'login' | 'password' | 'twoFactor'"`
	// ManageSeconds는 인증 수단을 바꿀 수 있는 관리 창이 닫히기까지 남은 초다. 닫혀 있으면 0이다.
	ManageSeconds int `json:"manageSeconds"`
	// PasskeysAvailable은 이 서버에서 패스키를 쓸 수 있는지다(공개 주소 설정). 브라우저 지원은 프론트가 따로 본다.
	PasskeysAvailable bool          `json:"passkeysAvailable"`
	Passkeys          []PasskeyInfo `json:"passkeys"`
	TOTP              *TOTPInfo     `json:"totp,omitempty"`
	// RecoveryCodes는 남은 복구 코드 수다.
	RecoveryCodes int `json:"recoveryCodes"`
	// Methods는 지금 확인에 쓸 수 있는 2차 수단이다(Factor*, password 제외).
	Methods []string       `json:"methods" tstype:"Array<'passkey' | 'totp' | 'recovery'>"`
	Policy  SecurityPolicy `json:"policy"`
	// PasskeyLogins는 만료되지 않은 패스키로 로그인 등록이다.
	PasskeyLogins []PasskeyLoginInfo `json:"passkeyLogins"`
}

// SecurityVerifyRequest는 POST /api/security/verify의 본문이다. 맞으면 이 세션의 보증 수준을 올린다
// (password는 password, 나머지는 twoFactor).
type SecurityVerifyRequest struct {
	Method   string `json:"method" tstype:"'password' | 'totp' | 'recovery' | 'passkey'"`
	Password string `json:"password,omitempty"`
	// Code는 TOTP 코드 또는 복구 코드다.
	Code    string            `json:"code,omitempty"`
	Passkey *PasskeyAssertion `json:"passkey,omitempty"`
}

// SecurityManageRequest는 POST /api/security/manage의 본문이다. 비밀번호와(등록한 수단이 있으면) 그 수단을 다시
// 확인하면 이 세션에 관리 창이 잠깐 열린다. 세션을 빼앗은 쪽이 자기 수단을 등록하지 못하게 한다.
type SecurityManageRequest struct {
	Password string `json:"password"`
	// Factor는 등록한 2차 수단의 확인이다. 수단이 없으면 비운다.
	Factor *SecurityVerifyRequest `json:"factor,omitempty"`
	// DeviceCode는 등록한 수단 대신 받는 기기 추가 코드다(POST /api/security/device-code). Factor와 함께 보내지
	// 않는다. 맞으면 관리 창을 열되 보증 수준은 password까지만 올린다.
	DeviceCode string `json:"deviceCode,omitempty"`
}

// DeviceCodeResponse는 POST /api/security/device-code의 응답이다. 수단이 있는 기기가 만들어, 그 패스키가 없는
// 새 기기의 관리 확인(SecurityManageRequest.DeviceCode)에 입력하게 한다. 사용자마다 하나이고 다시 만들면 이전
// 코드는 쓸 수 없다.
type DeviceCodeResponse struct {
	// Code는 한 번만 쓸 수 있는 코드(XXXX-XXXX)다. 이 응답에만 담긴다.
	Code string `json:"code"`
	// ExpiresInSeconds는 코드를 쓸 수 있는 남은 초다.
	ExpiresInSeconds int `json:"expiresInSeconds"`
}

// 패스키 challenge의 목적. PasskeyChallengeRequest.Purpose에 들어간다.
const (
	// PasskeyAssert는 확인(로그인 단계, 세션 중 확인, 관리 확인)이다.
	PasskeyAssert = "assert"
	// PasskeyRegister는 새 패스키 등록이다(관리 창 필요).
	PasskeyRegister = "register"
)

// PasskeyChallengeRequest는 POST /api/security/passkeys/challenge의 본문이다.
type PasskeyChallengeRequest struct {
	Purpose string `json:"purpose" tstype:"'assert' | 'register'"`
}

// PasskeyOptions는 navigator.credentials.get()·create()에 넘길 값이다. 이진 값은 base64url(패딩 없음)이다.
type PasskeyOptions struct {
	Challenge string `json:"challenge"`
	RPID      string `json:"rpId"`
	// TimeoutMs는 브라우저에 주는 제한 시간이다.
	TimeoutMs int `json:"timeoutMs"`
	// Credentials는 확인에서는 쓸 수 있는 자격 증명(allowCredentials), 등록에서는 이미 등록한 자격 증명
	// (excludeCredentials)이다.
	Credentials []PasskeyDescriptor `json:"credentials"`
	// 아래는 등록에만 있다.
	RPName     string       `json:"rpName,omitempty"`
	User       *PasskeyUser `json:"user,omitempty"`
	Algorithms []int        `json:"algorithms,omitempty"`
}

// PasskeyDescriptor는 자격 증명 하나를 가리킨다.
type PasskeyDescriptor struct {
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

// PasskeyUser는 등록할 때 인증기에 넣는 사용자 정보다. ID는 무작위 핸들이다(사용자 이름을 넣지 않는다).
type PasskeyUser struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// PasskeyAssertion은 navigator.credentials.get()의 결과다. 이진 값은 base64url이다.
type PasskeyAssertion struct {
	ID                string `json:"id"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle,omitempty"`
}

// PasskeyRegistration은 navigator.credentials.create()의 결과다. PublicKey는 getPublicKey()의 SPKI DER이며,
// 브라우저가 알고리즘을 모르면 비어 있다(등록하지 않는다).
type PasskeyRegistration struct {
	ID                 string   `json:"id"`
	ClientDataJSON     string   `json:"clientDataJSON"`
	AuthenticatorData  string   `json:"authenticatorData"`
	PublicKey          string   `json:"publicKey"`
	PublicKeyAlgorithm int      `json:"publicKeyAlgorithm"`
	Transports         []string `json:"transports,omitempty"`
}

// PasskeyRegisterRequest는 POST /api/security/passkeys의 본문이다(관리 창 필요). 등록 값이 맞으면 바로 저장하지
// 않고, 응답(PasskeyOptions)으로 그 패스키 하나만 허용하는 확인 challenge를 준다. 그 확인을
// POST /api/security/passkeys/confirm으로 보내 통과해야 저장한다.
type PasskeyRegisterRequest struct {
	Name       string               `json:"name"`
	Credential *PasskeyRegistration `json:"credential"`
}

// PasskeyConfirmRequest는 POST /api/security/passkeys/confirm의 본문이다(관리 창 필요). 등록 직후 새 패스키로 한
// 확인이며, 통과하면 패스키를 저장하고 SecurityChangeResponse를 준다.
type PasskeyConfirmRequest struct {
	Passkey *PasskeyAssertion `json:"passkey"`
}

// PasskeyLoginEnrollRequest는 POST /api/security/passkey-logins의 본문이다. 비밀번호와 그 패스키(PRF를 함께 요청한
// 확인)를 확인하면 이 브라우저의 패스키로 로그인을 켠다. 암호문은 브라우저에만 두므로 보내지 않는다.
type PasskeyLoginEnrollRequest struct {
	Password string            `json:"password"`
	Passkey  *PasskeyAssertion `json:"passkey"`
	// Replace는 이 브라우저가 전에 쓰던 등록 ID다. 있으면 함께 지운다(다시 켤 때).
	Replace string `json:"replace,omitempty"`
}

// PasskeyLoginEnrollResponse는 POST /api/security/passkey-logins의 응답이다. Share는 서버 쪽 키 조각(base64url)이며
// 브라우저가 PRF 결과와 함께 키를 만드는 데만 쓴다(저장하지 않는다).
type PasskeyLoginEnrollResponse struct {
	ID    string `json:"id"`
	Share string `json:"share"`
	// ExpiresAt은 RFC 3339 만료 시각이다.
	ExpiresAt string         `json:"expiresAt"`
	Status    SecurityStatus `json:"status"`
}

// PasskeyLoginOptionsRequest는 POST /api/login/passkey/options의 본문이다(인증 없음). 브라우저 항목의 사용자 이름과
// 등록 ID를 보내면 그 패스키 하나만 허용하는 확인 옵션(PasskeyOptions)을 준다.
type PasskeyLoginOptionsRequest struct {
	Username   string `json:"username"`
	Enrollment string `json:"enrollment"`
}

// PasskeyLoginRequest는 POST /api/login/passkey의 본문이다(인증 없음). 확인이 맞으면 PasskeyLoginUnlockResponse를 준다.
type PasskeyLoginRequest struct {
	Passkey *PasskeyAssertion `json:"passkey"`
}

// PasskeyLoginUnlockResponse는 POST /api/login/passkey의 응답이다. Share로 키를 만들어 비밀번호를 풀고(안 되면
// NextShare로 푼다) POST /api/login/passkey/finish로 보낸다. 둘 다 base64url 키 조각이다.
type PasskeyLoginUnlockResponse struct {
	Share string `json:"share"`
	// NextShare는 회전을 기다리는 다음 키 조각이다. 브라우저는 풀어 낸 비밀번호를 이것으로 다시 암호화해 항목을 바꾼다.
	// 확정(finish의 Rotated)할 때까지 같은 값을 준다.
	NextShare string `json:"nextShare"`
}

// PasskeyLoginFinishRequest는 POST /api/login/passkey/finish의 본문이다. 풀어 낸 비밀번호로 로그인하며 응답은
// LoginResponse다. 풀지 못했으면 Discard로 그 등록을 지운다(204).
type PasskeyLoginFinishRequest struct {
	Password string `json:"password,omitempty"`
	// Rotated는 브라우저가 항목을 NextShare용으로 바꿔 저장했다는 뜻이다. OMV가 비밀번호를 받으면 NextShare를 Share로
	// 확정한다.
	Rotated bool `json:"rotated,omitempty"`
	Discard bool `json:"discard,omitempty"`
}

// TOTPSetupResponse는 POST /api/security/totp/setup의 응답이다. 비밀 키는 확인(POST /api/security/totp)할 때까지
// 이 세션의 메모리에만 있다.
type TOTPSetupResponse struct {
	Secret string `json:"secret"`
	// URI는 인증 앱에 넣는 otpauth:// 주소다(QR 코드의 내용).
	URI string `json:"uri"`
}

// TOTPConfirmRequest는 POST /api/security/totp의 본문이다. 인증 앱이 보여 주는 코드로 설정을 마친다.
type TOTPConfirmRequest struct {
	Code string `json:"code"`
}

// SecurityChangeResponse는 수단을 등록·삭제하거나 복구 코드를 다시 만든 뒤의 응답이다.
type SecurityChangeResponse struct {
	Status SecurityStatus `json:"status"`
	// RecoveryCodes는 새로 만든 복구 코드다. 첫 수단을 등록했거나 다시 만들었을 때만 있고, 다시 볼 수 없다.
	RecoveryCodes []string `json:"recoveryCodes,omitempty"`
}

// SecurityUserInfo는 GET /api/security/users(관리자)의 한 줄이며, 인증 수단 기록이 있는 사용자다.
type SecurityUserInfo struct {
	Username      string `json:"username"`
	Passkeys      int    `json:"passkeys"`
	TOTP          bool   `json:"totp"`
	RecoveryCodes int    `json:"recoveryCodes"`
	// Unreadable은 기록을 읽을 수 없다는 뜻이다(손상된 파일). 초기화하면 지운다.
	Unreadable bool `json:"unreadable,omitempty"`
}
