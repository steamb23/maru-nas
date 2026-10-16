package omv

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 로그인 결과 상태. OMV Session.login·Session.verify 응답의 status 값이다.
const (
	StatusAuthenticated     = "authenticated"
	StatusChallengeRequired = "challengeRequired"
)

// Challenge는 2차 인증 요구 정보다. 종류와 형식은 OMV 플러그인이 정한다.
type Challenge struct {
	Kind        string `json:"kind"`
	RedirectURL string `json:"redirecturl,omitempty"`
}

// LoginResult는 Session.login·Session.verify 결과다.
type LoginResult struct {
	Status   string
	Username string
	Admin    bool
	// SessionID는 이후 호출에 쓸 OMV 세션 ID다. challengeRequired일 때는
	// Session.verify에만 쓸 수 있는 대기 중 세션이다.
	SessionID string
	Challenge *Challenge
}

// loginResponse는 Session.login·Session.verify 응답이다. OMV 버전에 따라 형식이 둘이다.
//   - 8.5.0-2 이상: status가 authenticated 또는 challengeRequired다.
//   - 8.4 이하: status 없이 authenticated(bool)만 준다. 2차 인증이 없고, 틀린 비밀번호는
//     false가 아니라 HTTP 400으로 온다.
//
// 8.5의 응답에도 authenticated가 들어 있을 수 있으므로 옛 형식은 status가 없는 것으로 판단한다.
type loginResponse struct {
	Status        string `json:"status"`
	Authenticated *bool  `json:"authenticated"`
	Username      string `json:"username"`
	SessionID     string `json:"sessionid"`
	Permissions   struct {
		Role string `json:"role"`
	} `json:"permissions"`
	Challenge *Challenge `json:"challenge"`
}

var errBadLogin = errors.New("omv login: unexpected response")

func (c *Client) loginResult(res Response) (LoginResult, error) {
	var lr loginResponse
	if err := json.Unmarshal(res.Result, &lr); err != nil {
		return LoginResult{}, errBadLogin
	}
	// 옛 형식(OMV 8.4 이하)은 새 형식의 authenticated 상태로 바꿔 같은 흐름을 탄다.
	if lr.Status == "" && lr.Authenticated != nil && *lr.Authenticated {
		lr.Status = StatusAuthenticated
	}
	out := LoginResult{
		Status:    lr.Status,
		Username:  lr.Username,
		Admin:     lr.Permissions.Role == "admin",
		SessionID: lr.SessionID,
		Challenge: lr.Challenge,
	}
	// 2차 인증이 필요한 경우 OMV는 세션 ID를 본문 없이 Set-Cookie로만 준다.
	if out.SessionID == "" {
		out.SessionID = res.SessionID
	}
	switch {
	case out.SessionID == "":
		return LoginResult{}, errBadLogin
	case out.Status == StatusAuthenticated:
	case out.Status == StatusChallengeRequired && out.Challenge != nil:
	default:
		return LoginResult{}, errBadLogin
	}
	return out, nil
}

// Login은 Session.login을 호출한다. 틀린 비밀번호는 HTTP 400 *Error로 돌아온다.
func (c *Client) Login(ctx context.Context, username, password, clientIP string) (LoginResult, error) {
	username = strings.TrimSpace(username)
	res, err := c.Call(ctx, Request{
		Service:  "Session",
		Method:   "login",
		Params:   map[string]string{"username": username, "password": password},
		ClientIP: clientIP,
		cookies:  c.login.get(username),
	})
	if err != nil {
		return LoginResult{}, err
	}
	c.login.save(username, res.cookies)
	return c.loginResult(res)
}

// Verify는 대기 중 세션으로 Session.verify를 호출해 2차 인증을 마친다.
// response는 플러그인이 정한 형식 그대로 challengeresponse로 넘긴다.
func (c *Client) Verify(ctx context.Context, sessionID, username string, response any, clientIP string) (LoginResult, error) {
	res, err := c.Call(ctx, Request{
		SessionID: sessionID,
		Service:   "Session",
		Method:    "verify",
		Params:    map[string]any{"challengeresponse": response},
		ClientIP:  clientIP,
		cookies:   c.login.get(username),
	})
	if err != nil {
		return LoginResult{}, err
	}
	c.login.save(username, res.cookies)
	return c.loginResult(res)
}

// Logout은 Session.logout으로 OMV 세션을 파기한다.
func (c *Client) Logout(ctx context.Context, sessionID string) error {
	_, err := c.Call(ctx, Request{SessionID: sessionID, Service: "Session", Method: "logout"})
	return err
}

// SessionState는 Probe가 알아낸 OMV 세션 정보다.
type SessionState struct {
	// Timeout은 OMV 자동 로그아웃 시간(웹 관리 설정의 timeout)이다. 0이면 자동 로그아웃이 꺼져 있다.
	Timeout time.Duration
	// TimeoutKnown은 Timeout을 읽었는지다. RPC 오류로 세션 생존만 확인한 경우 false다.
	TimeoutKnown bool
}

// Probe는 OMV 세션이 살아 있는지 확인하고 자동 로그아웃 시간을 읽는다.
// WebGui.getAutoLogoutSettings를 쓴다. 엔진 RPC라 인증된 세션이 필요하고, 역할 제한이
// 없으며 읽기만 한다(OMV workbench도 이 값으로 자동 로그아웃 타이머를 돈다).
// touch가 false면 OMV 세션의 마지막 접근 시각을 갱신하지 않는다.
//
// OMV가 401을 반환할 때만 만료로 본다(IsUnauthorized). rpc.php가 돌려준 그 밖의
// RPC 오류는 인증 검사를 통과한 뒤에 난 것이므로 세션은 살아 있다고 보고, 시간을 모르는
// 채로 nil 오류를 반환한다. 네트워크 오류나 nginx 오류 페이지는 그대로 반환한다(일시적 장애).
//
// 2차 인증 대기 중인 세션에는 쓰지 않는다. OMV는 인증되지 않은 세션으로 엔진 RPC를
// 부르면 세션을 파기하므로 대기 중 로그인이 사라진다.
func (c *Client) Probe(ctx context.Context, sessionID string, touch bool) (SessionState, error) {
	res, err := c.Call(ctx, Request{
		SessionID: sessionID,
		Service:   "WebGui",
		Method:    "getAutoLogoutSettings",
		NoTouch:   !touch,
	})
	if e, ok := errors.AsType[*Error](err); ok && e.rpc && e.HTTPStatus != http.StatusUnauthorized {
		return SessionState{}, nil
	}
	if err != nil {
		return SessionState{}, err
	}
	var body struct {
		Timeout *int `json:"timeout"`
	}
	if json.Unmarshal(res.Result, &body) != nil || body.Timeout == nil || *body.Timeout < 0 {
		return SessionState{}, nil
	}
	return SessionState{Timeout: time.Duration(*body.Timeout) * time.Minute, TimeoutKnown: true}, nil
}

// loginCookies는 사용자별 OPENMEDIAVAULT-LOGIN-* 쿠키를 메모리에 보관한다.
// OMV는 이 쿠키가 없는 로그인을 "처음 보는 브라우저"로 보고 알림 메일을 보내므로,
// 다음 로그인에 실어 보내 백엔드 재시작 후 첫 로그인에만 메일이 가게 한다.
// 값은 비밀이 아니다(사용자 이름 해시와 임의 문구).
type loginCookies struct {
	mu sync.Mutex
	m  map[string][]*http.Cookie
}

// maxLoginCookieUsers는 보관할 사용자 수 상한이다. 넘치면 새로 저장하지 않는다.
const maxLoginCookieUsers = 256

func (l *loginCookies) get(username string) []*http.Cookie {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m[username]
}

func (l *loginCookies) save(username string, cookies []*http.Cookie) {
	var found []*http.Cookie
	for _, ck := range cookies {
		if strings.HasPrefix(ck.Name, loginCookiePrefix) && ck.Value != "" && ck.MaxAge >= 0 {
			found = append(found, &http.Cookie{Name: ck.Name, Value: ck.Value})
		}
	}
	if len(found) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = make(map[string][]*http.Cookie)
	}
	if _, ok := l.m[username]; !ok && len(l.m) >= maxLoginCookieUsers {
		return
	}
	l.m[username] = found
}
