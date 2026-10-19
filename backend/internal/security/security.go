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
	"crypto/rand"
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
	// PasskeyLogins는 패스키로 로그인(분할 키)의 등록이다. 등록 하나가 브라우저 하나와 패스키 하나의 짝이다.
	// 패스키를 지우면 그 패스키의 등록도 지운다(Normalize).
	PasskeyLogins []PasskeyLogin `json:"passkeyLogins,omitempty"`
}

// Passkey는 등록한 패스키(WebAuthn 자격 증명) 하나다.
type Passkey struct {
	// ID는 자격 증명 ID(rawId)의 base64url(패딩 없음)이다.
	ID   string `json:"id"`
	Name string `json:"name"`
	// PublicKey는 공개 키의 SubjectPublicKeyInfo(DER)다. JSON에서는 base64다.
	PublicKey []byte `json:"publicKey"`
	// Alg는 COSE 알고리즘 번호다(AlgES256, AlgEdDSA, AlgRS256).
	Alg       int    `json:"alg"`
	SignCount uint32 `json:"signCount"`
	// Transports는 브라우저가 알려 준 연결 방식(usb, internal 등)이다. 확인할 때 브라우저에 힌트로 준다.
	Transports []string `json:"transports,omitempty"`
	// BackupEligible은 등록할 때 인증기가 알린 백업 가능(BE) 플래그다. 동기화 저장소(iCloud 키체인, 비밀번호
	// 관리자)의 패스키라는 뜻이며 막지 않고 화면에 표시만 한다.
	BackupEligible bool      `json:"backupEligible,omitempty"`
	Created        time.Time `json:"created"`
	LastUsed       time.Time `json:"lastUsed,omitzero"`
}

// PasskeyLogin은 패스키로 로그인의 등록 하나다. 비밀번호 암호문은 브라우저에만 있고, 서버는 키의 나머지 조각
// (Share)만 둔다. 키는 패스키 PRF 결과와 Share를 함께 써야 만들어지므로 Share만으로는 아무것도 풀 수 없다.
type PasskeyLogin struct {
	// ID는 등록 ID(LoginIDBytes 바이트 무작위)의 base64url(패딩 없음)이다. 브라우저 항목이 가지고 있다.
	ID string `json:"id"`
	// Credential은 이 등록에 쓰는 패스키의 ID(Passkey.ID)다.
	Credential string `json:"credential"`
	// Share는 서버 쪽 키 조각(ShareBytes 바이트)이다. 패스키 서명을 확인한 뒤에만 브라우저에 준다. JSON에서는 base64다.
	Share   []byte    `json:"share"`
	Created time.Time `json:"created"`
	// ExpiresAt은 등록할 때의 만료 기간으로 정한 만료 시각이다. 실제 만료는 Expiry가 지금 기간과 함께 정한다.
	ExpiresAt time.Time `json:"expiresAt"`
	LastUsed  time.Time `json:"lastUsed,omitzero"`
}

const (
	// LoginIDBytes는 패스키로 로그인 등록 ID의 길이다.
	LoginIDBytes = 16
	// ShareBytes는 서버 쪽 키 조각의 길이다.
	ShareBytes = 32
)

// NewPasskeyLogin은 credential 패스키의 새 등록을 만든다. 만료 시각은 now에서 days일 뒤다.
func NewPasskeyLogin(credential string, now time.Time, days int) PasskeyLogin {
	id := make([]byte, LoginIDBytes)
	rand.Read(id)
	share := make([]byte, ShareBytes)
	rand.Read(share)
	return PasskeyLogin{
		ID:         B64.EncodeToString(id),
		Credential: credential,
		Share:      share,
		Created:    now,
		ExpiresAt:  now.Add(time.Duration(days) * 24 * time.Hour),
	}
}

// Expiry는 지금 만료 기간(days일)을 적용한 만료 시각이다. 등록할 때 정한 시각과 Created + days일 중 이른 쪽이므로,
// 관리자가 기간을 줄이면 기존 등록에도 바로 적용되고 늘리면 새 등록부터 적용된다.
func (l PasskeyLogin) Expiry(days int) time.Time {
	if e := l.Created.Add(time.Duration(days) * 24 * time.Hour); e.Before(l.ExpiresAt) {
		return e
	}
	return l.ExpiresAt
}

// Expired는 now에 만료됐는지다.
func (l PasskeyLogin) Expired(now time.Time, days int) bool { return !now.Before(l.Expiry(days)) }

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
			p.Transports = slices.Clone(p.Transports)
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
	if r.PasskeyLogins != nil {
		ls := make([]PasskeyLogin, len(r.PasskeyLogins))
		for i, l := range r.PasskeyLogins {
			l.Share = bytes.Clone(l.Share)
			ls[i] = l
		}
		r.PasskeyLogins = ls
	}
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

// PasskeyLogin은 id의 패스키로 로그인 등록이다. 없으면 nil이다.
func (r *Record) PasskeyLogin(id string) *PasskeyLogin {
	for i := range r.PasskeyLogins {
		if r.PasskeyLogins[i].ID == id {
			return &r.PasskeyLogins[i]
		}
	}
	return nil
}

// DeletePasskeyLogin은 id의 등록을 지운다. 있었는지를 돌려준다.
func (r *Record) DeletePasskeyLogin(id string) bool {
	n := len(r.PasskeyLogins)
	r.PasskeyLogins = slices.DeleteFunc(r.PasskeyLogins, func(l PasskeyLogin) bool { return l.ID == id })
	return len(r.PasskeyLogins) != n
}

// PruneExpiredLogins는 now에 만료된(지금 기간 days일) 등록을 지우고 지운 수를 돌려준다.
func (r *Record) PruneExpiredLogins(now time.Time, days int) int {
	n := len(r.PasskeyLogins)
	r.PasskeyLogins = slices.DeleteFunc(r.PasskeyLogins, func(l PasskeyLogin) bool { return l.Expired(now, days) })
	return n - len(r.PasskeyLogins)
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
// 없는 패스키의 패스키로 로그인 등록도 지운다. 저장소 구현이 UpdateSecurity에서 부른다. 비었으면 true다.
func Normalize(r *Record) bool {
	if !r.HasMethods() {
		r.Recovery = nil
	}
	r.PasskeyLogins = slices.DeleteFunc(r.PasskeyLogins, func(l PasskeyLogin) bool { return r.Passkey(l.Credential) == nil })
	if len(r.PasskeyLogins) == 0 {
		r.PasskeyLogins = nil
	}
	if len(r.Passkeys) == 0 {
		r.Passkeys, r.Handle = nil, nil
	}
	return !r.HasMethods() && len(r.Recovery) == 0
}

// ErrVerify는 인증 수단 확인이 실패했다는 뜻이다. 자세한 이유는 감싼 오류의 문구에 있다(로그용, 비밀 값 없음).
var ErrVerify = errors.New("security: verification failed")
