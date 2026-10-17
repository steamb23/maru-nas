// Package security는 보안 강화 모드의 인증 수단(패스키, TOTP, 복구 코드)과 사용자별 수단 기록을 다룬다.
//
// 보안에 민감한 기능(웹 터미널, root 권한 상승)을 2차 인증을 거친 세션에서만 쓰게 하는 데 쓴다. 확인은
// onmaru가 하며 권한 도우미에는 넣지 않는다. 표준 라이브러리만 쓴다(WebAuthn도 CBOR 없이 처리한다).
//
// 기록(Record)에는 TOTP 비밀과 복구 코드 해시가 들어 있으므로 internal/api(프론트 타입 생성의 원본)에 두지
// 않고, 응답·로그에 그대로 넣지 않는다.
package security

import (
	"bytes"
	"errors"
	"slices"
	"time"
)

// Record는 사용자 한 명의 인증 수단 기록이다. 상태 디렉터리의 security/<사용자 이름>.json(0600)에 둔다.
type Record struct {
	Passkeys []Passkey `json:"passkeys,omitempty"`
	TOTP     *TOTP     `json:"totp,omitempty"`
	// Recovery는 남은 복구 코드의 해시(RecoveryHash)다. 쓴 코드는 지운다.
	Recovery []string `json:"recovery,omitempty"`
	// Handle은 패스키의 사용자 핸들(WebAuthn user.id)이다. 처음 패스키를 등록할 때 무작위로 만든다
	// (사용자 이름을 인증기에 넣지 않게). 패스키가 모두 없어지면 지운다.
	Handle []byte `json:"handle,omitempty"`
}

// Passkey는 등록한 패스키(WebAuthn 자격 증명) 하나다.
type Passkey struct {
	// ID는 자격 증명 ID(rawId)의 base64url(패딩 없음)이다.
	ID   string `json:"id"`
	Name string `json:"name"`
	// PublicKey는 공개 키의 SubjectPublicKeyInfo(DER)다. JSON에서는 base64다.
	PublicKey []byte `json:"publicKey"`
	// Alg는 COSE 알고리즘 번호다(AlgES256, AlgEdDSA, AlgRS256).
	Alg       int       `json:"alg"`
	SignCount uint32    `json:"signCount"`
	Created   time.Time `json:"created"`
	LastUsed  time.Time `json:"lastUsed,omitzero"`
}

// TOTP는 등록한 인증 앱(RFC 6238) 하나다.
type TOTP struct {
	// Secret은 base32(패딩 없음) 비밀 키다.
	Secret  string    `json:"secret"`
	Created time.Time `json:"created"`
	// LastStep은 마지막으로 받은 코드의 시간 단계다. 같은 코드를 다시 쓰지 못하게 한다.
	LastStep int64 `json:"lastStep,omitempty"`
}

// HasMethods는 2차 인증 수단(패스키, TOTP)이 하나라도 있는지다. 복구 코드는 수단으로 치지 않는다.
func (r Record) HasMethods() bool { return len(r.Passkeys) > 0 || r.TOTP != nil }

// Clone은 고쳐도 원래 값에 영향이 없는 복사본이다.
func (r Record) Clone() Record {
	if r.Passkeys != nil {
		ps := make([]Passkey, len(r.Passkeys))
		for i, p := range r.Passkeys {
			p.PublicKey = bytes.Clone(p.PublicKey)
			ps[i] = p
		}
		r.Passkeys = ps
	}
	if r.TOTP != nil {
		t := *r.TOTP
		r.TOTP = &t
	}
	r.Recovery = slices.Clone(r.Recovery)
	r.Handle = bytes.Clone(r.Handle)
	return r
}

// Passkey는 id의 패스키다. 없으면 nil이다.
func (r *Record) Passkey(id string) *Passkey {
	for i := range r.Passkeys {
		if r.Passkeys[i].ID == id {
			return &r.Passkeys[i]
		}
	}
	return nil
}

// Store는 사용자별 수단 기록 저장소다. prefs.FileStore·MemoryStore가 구현한다. 여러 고루틴에서 함께 써도 된다.
//
// 읽을 수 없는 기록(형식이 틀린 파일 등)은 빈 기록이 아니라 오류로 돌려준다. 빈 기록으로 보면 그 사용자의
// 2차 인증이 조용히 사라지기 때문이다.
type Store interface {
	// GetSecurity는 username의 기록이다. 없으면 빈 기록이다.
	GetSecurity(username string) (Record, error)
	// UpdateSecurity는 기록을 읽어 apply로 고친 뒤 저장한다. 읽기부터 쓰기까지 잠근다. apply가 오류를 돌려주면
	// 저장하지 않고 그 오류를 돌려준다. 고친 기록에 수단이 없으면 복구 코드까지 지우고 기록 자체를 없앤다.
	UpdateSecurity(username string, apply func(*Record) error) error
	// DeleteSecurity는 기록을 지운다. 있었는지를 돌려준다.
	DeleteSecurity(username string) (bool, error)
	// ListSecurity는 기록이 있는 사용자 이름이다.
	ListSecurity() ([]string, error)
}

// Normalize는 저장하기 전의 기록을 정리한다. 수단이 없으면 복구 코드도 지운다(2차 인증을 되살리는 데만 쓴다).
// 저장소 구현이 UpdateSecurity에서 부른다. 비었으면 true다.
func Normalize(r *Record) bool {
	if !r.HasMethods() {
		r.Recovery = nil
	}
	if len(r.Passkeys) == 0 {
		r.Passkeys, r.Handle = nil, nil
	}
	return !r.HasMethods() && len(r.Recovery) == 0
}

// ErrVerify는 인증 수단 확인이 실패했다는 뜻이다. 자세한 이유는 감싼 오류의 문구에 있다(로그용, 비밀 값 없음).
var ErrVerify = errors.New("security: verification failed")
