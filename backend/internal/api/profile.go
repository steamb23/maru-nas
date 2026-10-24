package api

// 프로필(전자 우편, 태그)을 바꿀 수 없는 이유. ProfileResponse.ReadOnlyReason에 들어간다.
const (
	// ProfileReadOnlySystem은 OMV 시스템 계정이다(OMV 기본 admin 계정 등). OMV 사용자 관리 목록에도 없다.
	ProfileReadOnlySystem = "system"
	// ProfileReadOnlyLocked는 OMV에서 이 사용자의 계정 수정을 막았다는 뜻이다(disallowusermod).
	ProfileReadOnlyLocked = "locked"
)

// ProfileResponse는 GET /api/profile의 응답이며 로그인한 사용자 자신의 OMV 계정 정보다.
// SSH 공개 키, uid, 홈 디렉터리 등은 넘기지 않는다.
type ProfileResponse struct {
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	// Tags는 계정 태그다. OMV 사용자 편집 화면의 "Tags"이며, 실제로는 Linux 계정의 설명 칸(GECOS)에
	// 쉼표로 이어 저장된다. 백엔드가 나누고 잇는다.
	Tags []string `json:"tags,omitempty"`
	// Groups는 속한 그룹이다. OMV가 관리자에게만 알려 주므로 일반 사용자는 없다.
	Groups []string `json:"groups,omitempty"`
	// SystemAccount는 OMV 사용자 관리 목록에 없는 시스템 계정인지다. 관리자만 알 수 있다.
	SystemAccount bool `json:"systemAccount,omitempty"`
	// ReadOnlyReason이 있으면 전자 우편·태그를 바꿀 수 없다.
	ReadOnlyReason string `json:"readOnlyReason,omitempty" tstype:"'system' | 'locked'"`
}

// ProfileRequest는 PUT /api/profile의 요청 본문이다. 사용자가 자기 계정에서 바꿀 수 있는 전자 우편과 태그를
// 바꾼다(OMV UserMgmt.setUserByContext와 같은 범위). 없는 값은 그대로 둔다. 비밀번호·그룹 등은 OMV에서 바꾼다.
type ProfileRequest struct {
	// Email이 빈 문자열이면 전자 우편을 지운다.
	Email *string   `json:"email,omitempty"`
	Tags  *[]string `json:"tags,omitempty"`
}

// ProfileSaveResponse는 PUT /api/profile의 응답이다.
type ProfileSaveResponse struct {
	// Applied는 OMV 적용까지 끝났는지다. 일반 사용자는 적용 권한이 없어 false이며, OMV에 적용 대기가 남는다
	// (관리자가 OMV 웹 UI에서 적용한다). 바꿀 것이 없었으면 true다.
	Applied bool `json:"applied"`
}
