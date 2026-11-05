package addonpkg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"maru/backend/config"
	"maru/backend/internal/api"
)

// 패키지 백엔드 연결(.claude/docs/addon-packages.md의 "maru ↔ 백엔드"). maru는 백엔드 프로세스를 실행하지 않고
// 소켓에 붙기만 한다. 운영은 systemd 소켓 유닛이 소켓을 만들고, 개발은 개발자가 띄운 백엔드가 만든다.

// 백엔드 요청의 상한.
const (
	// BackendDialTimeout은 백엔드 소켓에 연결하는 시간 제한이다.
	BackendDialTimeout = 2 * time.Second
	// BackendDataTimeout은 데이터 요청(GET /v1/data) 전체의 시간 제한이다.
	BackendDataTimeout = 15 * time.Second
	// MaxBackendResponse는 백엔드 응답 본문의 상한이다.
	MaxBackendResponse = 4 << 20
	// MaxBackendQuery는 데이터 요청에 넘기는 쿼리 문자열의 상한이다.
	MaxBackendQuery = 2 << 10
	// MaxActionInput은 조작 입력(input)의 상한이다.
	MaxActionInput = 8 << 10
	// maxBackendMessage는 백엔드 오류 문구를 브라우저에 넘길 때의 글자 수 상한이다.
	maxBackendMessage = 300
	// maxBackendHeader는 백엔드 응답 헤더의 상한이다(헤더는 넘기지 않는다).
	maxBackendHeader = 16 << 10
)

// defaultBackendMessage는 백엔드가 문구를 주지 않은 오류의 문구다.
const defaultBackendMessage = "addon backend error"

// backendCodePattern은 백엔드가 준 오류 코드 중 브라우저에 넘기는 형식이다(api.ErrorResponse.AddonCode).
var backendCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// BackendPaths는 백엔드 소켓과 호스트 소켓의 위치다. 서버가 Config.AddonHost로 받는다(nil이면 백엔드가 꺼져 있다).
type BackendPaths struct {
	// Backend는 id 백엔드의 소켓 경로다. maru가 붙는다.
	Backend func(id string) string
	// Host는 id 백엔드가 OMV를 부를 호스트 소켓 경로다. maru가 연다.
	Host func(id string) string
	// HostDirFromUnit이면 호스트 소켓의 디렉터리(운영 <AddonHostDir>/<id>/)를 백엔드의 소켓 유닛이 만든다. 상위
	// 디렉터리는 root 소유라 maru는 <id>/를 만들거나 바꾸지 못하고(유닛이 바인드하는 원본을 maru가 다른 곳으로 돌리지
	// 못하게), 그 안에 소켓만 연다. 디렉터리가 아직 없으면 소켓을 열지 않고 나중에 다시 본다.
	HostDirFromUnit bool
	// HostMode는 호스트 소켓 파일의 권한이다(운영 0666, 개발 0600).
	HostMode fs.FileMode
	// HostPeer가 있으면 호스트 소켓에 붙은 연결의 uid(SO_PEERCRED)가 이 함수를 통과해야 받는다. 받지 않은 연결은
	// 동시 연결 칸을 쓰기 전에 닫는다. nil이면 보지 않는다(개발 경로는 0600이라 같은 uid만 붙는다).
	HostPeer func(uid int) bool
}

// 개발 경로(MARU_DEV_ADDON_BACKENDS): 상태 디렉터리 안. 백엔드는 개발자가 MARU_ADDON_SOCKET·MARU_ADDON_HOST로 띄운다.
const (
	// DevBackendDir는 개발 백엔드 소켓 디렉터리(<state>/addon-dev)다.
	DevBackendDir = "addon-dev"
	// DevHostDir는 개발 호스트 소켓 디렉터리(<state>/addon-host)다.
	DevHostDir = "addon-host"
)

// DevBackendPaths는 개발 서버의 경로다. 백엔드 소켓은 <state>/addon-dev/<id>.sock, 호스트 소켓은
// <state>/addon-host/<id>.sock(0600, maru와 개발 백엔드가 같은 uid)이다. 두 디렉터리는 부르는 쪽이 0700으로 만든다.
func DevBackendPaths(stateDir string) BackendPaths {
	return BackendPaths{
		Backend:  func(id string) string { return filepath.Join(stateDir, DevBackendDir, id+".sock") },
		Host:     func(id string) string { return filepath.Join(stateDir, DevHostDir, id+".sock") },
		HostMode: 0o600,
	}
}

// InstalledBackendPaths는 운영 경로다(백엔드 소켓 config.AddonSocketDir/<id>.sock, 호스트 소켓
// config.AddonHostDir/<id>/host.sock). 유닛 안에는 config.AddonHostDir/<id>만 바인드되고 백엔드는 유닛의 동적 uid로
// 돌므로 호스트 소켓은 0666이고(토큰이 권한이다), 연결한 uid가 InstalledHostPeer를 통과해야 한다. <id>/는 백엔드의
// 소켓 유닛이 maru 사용자 소유로 만든다(HostDirFromUnit). 설치기의 --addon-backends가 켠 설치에서만 쓴다(cmd/maru의
// 운영 감지).
func InstalledBackendPaths() BackendPaths {
	return BackendPaths{
		Backend: func(id string) string { return filepath.Join(config.AddonSocketDir, id+".sock") },
		Host: func(id string) string {
			return filepath.Join(config.AddonHostDir, id, config.AddonHostSocketName)
		},
		HostDirFromUnit: true,
		HostMode:        0o666,
		HostPeer:        InstalledHostPeer,
	}
}

// InstalledHostPeer는 운영 호스트 소켓에 붙어도 되는 uid인지다: root와 systemd 동적 사용자 범위
// (config.AddonDynamicUIDMin~Max). 그 밖의 로컬 계정(로그인한 사용자, 동적 사용자가 아닌 서비스)은 소켓이 0666이어도
// 연결하자마자 닫혀 동시 연결 칸을 쥐지 못한다. 동적 사용자로 도는 다른 서비스(다른 패키지의 백엔드 포함)는 통과한다.
// 백엔드가 다른 패키지의 호스트 소켓에 닿지 못하는 것은 유닛의 /run에 자기 디렉터리만 있어서이고, 다른 애드온의 토큰은
// 그 소켓에서 거부된다.
//
// 가정: 백엔드 유닛은 PrivateUsers=yes라 자기 사용자 이름공간 안에서 돌지만, maru(호스트 이름공간)가 SO_PEERCRED로 보는
// uid는 호스트 쪽 동적 uid로 옮겨진 값일 것으로 본다. VM에서 확인한다(.claude/docs/addon-packages.md의 "호스트 소켓").
func InstalledHostPeer(uid int) bool {
	return uid == 0 || (uid >= config.AddonDynamicUIDMin && uid <= config.AddonDynamicUIDMax)
}

// ValidBackendQuery는 브라우저가 보낸 쿼리 문자열을 백엔드 요청 줄에 그대로 넣어도 되는지다. 상한 안이고 출력 가능한
// ASCII만 있어야 한다(공백·제어 문자가 요청 줄을 깨지 않게, 브라우저는 그 밖의 바이트를 퍼센트 인코딩한다).
func ValidBackendQuery(raw string) bool {
	if len(raw) > MaxBackendQuery {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if c := raw[i]; c <= ' ' || c >= 0x7f {
			return false
		}
	}
	return true
}

// ValidActionInput은 조작 입력이 JSON 객체 또는 null이고 상한 안인지다. 비어 있으면(보내지 않음) null로 본다.
func ValidActionInput(raw json.RawMessage) bool {
	b := bytes.TrimSpace(raw)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return true
	}
	return len(b) <= MaxActionInput && b[0] == '{' && json.Valid(b)
}

// BackendCall은 백엔드 요청 하나에 붙는 헤더 값이다.
type BackendCall struct {
	// User는 OMV 사용자 이름이다(X-Maru-User).
	User string
	// Admin은 OMV 관리자인지다(X-Maru-Admin: 1·0).
	Admin bool
	// Lang은 대시보드 화면 언어다(X-Maru-Lang). 부르는 쪽이 지원 언어인지 확인한다.
	Lang string
	// Token은 호스트 소켓의 호출 토큰이다(X-Maru-Call). 로그에 남기지 않는다.
	Token string
}

// BackendReply는 백엔드 응답을 브라우저에 줄 꼴로 바꾼 것이다. 백엔드의 상태·헤더를 그대로 넘기지 않는다.
type BackendReply struct {
	// Status는 브라우저에 줄 HTTP 상태다. 200이면 Body를, 204면 본문 없이 준다.
	Status int
	// Body는 Status가 200일 때 줄 JSON 값이다(json.Valid로 확인했다).
	Body json.RawMessage
	// Code는 오류일 때의 api 오류 코드다(addon_error, addon_backend_unavailable, action_timeout).
	Code string
	// AddonCode는 addon_error일 때 백엔드가 준 코드다(형식을 확인한 값만).
	AddonCode string
	// Message는 오류일 때 브라우저에 줄 문구다(백엔드 문구는 제어 문자를 지우고 줄였다).
	Message string
	// Err는 로그에 남길 원인이다. 브라우저에 주지 않는다.
	Err error
}

// OK는 성공한 응답(200·204)인지다.
func (r BackendReply) OK() bool { return r.Status == http.StatusOK || r.Status == http.StatusNoContent }

// BackendClient는 패키지 백엔드에 HTTP/1.1로 붙는 클라이언트다. 소켓마다 Transport 하나를 둔다. 연결은 요청마다 새로
// 맺는다(keep-alive를 끈다): 백엔드가 닫은 유휴 연결을 다시 쓰다 조작(POST)이 실패하지 않게 하고, 패키지를 지운 뒤
// 남는 연결이 없게 하기 위해서다. 유닉스 소켓 연결은 싸다.
type BackendClient struct {
	socket      func(id string) string
	dialTimeout time.Duration

	mu         sync.Mutex
	transports map[string]*http.Transport
}

// NewBackendClient는 socket(id → 백엔드 소켓 경로)에 붙는 클라이언트다.
func NewBackendClient(socket func(id string) string) *BackendClient {
	return &BackendClient{socket: socket, dialTimeout: BackendDialTimeout, transports: map[string]*http.Transport{}}
}

// dialError는 백엔드 소켓에 연결하지 못했다는 뜻이다(503으로 바꾼다).
type dialError struct{ err error }

func (e *dialError) Error() string { return "dial addon backend: " + e.err.Error() }
func (e *dialError) Unwrap() error { return e.err }

// transport는 id 백엔드 소켓의 Transport다. 경로가 바뀌면(설정이 바뀐 테스트 등) 새로 만든다.
func (c *BackendClient) transport(id string) *http.Transport {
	path := c.socket(id)
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.transports[path]; ok {
		return t
	}
	timeout := c.dialTimeout
	t := &http.Transport{
		// 환경 변수의 프록시를 쓰지 않는다(Proxy가 nil).
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: timeout}
			conn, err := d.DialContext(ctx, "unix", path)
			if err != nil {
				return nil, &dialError{err}
			}
			return conn, nil
		},
		DisableKeepAlives:      true,
		DisableCompression:     true,
		MaxResponseHeaderBytes: maxBackendHeader,
	}
	c.transports[path] = t
	return t
}

// Retain은 keep에 있는 id의 Transport만 남긴다. 레지스트리가 바뀔 때 부른다(지운 패키지의 Transport를 버린다).
func (c *BackendClient) Retain(keep map[string]bool) {
	paths := map[string]bool{}
	for id := range keep {
		paths[c.socket(id)] = true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for path, t := range c.transports {
		if !paths[path] {
			t.CloseIdleConnections()
			delete(c.transports, path)
		}
	}
}

// Data는 데이터 요청이다(GET /v1/data?<rawQuery>). rawQuery는 ValidBackendQuery를 거친 값이어야 한다. 시간 제한은
// ctx의 마감 시각이고(부르는 쪽이 BackendDataTimeout으로 정한다), 넘으면 504 addon_backend_unavailable이다.
// 백엔드의 204는 200 null로 준다.
func (c *BackendClient) Data(ctx context.Context, id, rawQuery string, call BackendCall) BackendReply {
	u := &url.URL{Scheme: "http", Host: "addon", Path: "/v1/data", RawQuery: rawQuery}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return invalidReply(err)
	}
	return c.do(ctx, id, req, call, true)
}

// actionBody는 POST /v1/actions/{name}의 본문이다.
type actionBody struct {
	Target string          `json:"target"`
	Input  json.RawMessage `json:"input"`
}

// Action은 조작 요청이다(POST /v1/actions/{name}). input은 ValidActionInput을 거친 값이고 비면 null로 보낸다. 시간
// 제한은 ctx의 마감 시각(manifest의 timeoutSeconds)이고, 넘으면 504 action_timeout이다.
func (c *BackendClient) Action(ctx context.Context, id, name, target string, input json.RawMessage, call BackendCall) BackendReply {
	if len(bytes.TrimSpace(input)) == 0 {
		input = json.RawMessage("null")
	}
	body, err := json.Marshal(actionBody{Target: target, Input: input})
	if err != nil {
		return invalidReply(err)
	}
	u := &url.URL{Scheme: "http", Host: "addon", Path: "/v1/actions/" + name}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return invalidReply(err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(ctx, id, req, call, false)
}

func (c *BackendClient) do(ctx context.Context, id string, req *http.Request, call BackendCall, data bool) BackendReply {
	req.Host = "addon"
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Maru-User", call.User)
	admin := "0"
	if call.Admin {
		admin = "1"
	}
	req.Header.Set("X-Maru-Admin", admin)
	req.Header.Set("X-Maru-Lang", call.Lang)
	req.Header.Set("X-Maru-Call", call.Token)
	// RoundTrip은 리디렉션을 따라가지 않는다(http.Client를 쓰지 않는다).
	res, err := c.transport(id).RoundTrip(req)
	if err != nil {
		return failedReply(ctx, err, data)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxBackendResponse+1))
	if err != nil {
		return failedReply(ctx, err, data)
	}
	return Translate(res.StatusCode, body, data)
}

// failedReply는 응답을 받지 못한 요청의 결과다. 연결 실패는 503, 시간 초과는 504, 연결한 뒤 끊긴 것은 502다.
func failedReply(ctx context.Context, err error, data bool) BackendReply {
	var de *dialError
	switch {
	case errors.As(err, &de):
		return BackendReply{Status: http.StatusServiceUnavailable, Code: api.ErrAddonBackendUnavailable,
			Message: "addon backend is unavailable", Err: err}
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		if data {
			return BackendReply{Status: http.StatusGatewayTimeout, Code: api.ErrAddonBackendUnavailable,
				Message: "addon backend timed out", Err: err}
		}
		return BackendReply{Status: http.StatusGatewayTimeout, Code: api.ErrActionTimeout, Message: "action timed out", Err: err}
	}
	return invalidReply(err)
}

// invalidReply는 백엔드가 계약에 맞지 않게 답했다는 502 addon_error다(addonCode 없음).
func invalidReply(err error) BackendReply {
	return BackendReply{Status: http.StatusBadGateway, Code: api.ErrAddonError,
		Message: "addon backend returned an invalid response", Err: err}
}

// passStatus는 백엔드 오류 상태 중 같은 상태로 넘기는 것이다. 401·403·5xx는 넘기지 않는다(가짜 401로 로그아웃시키거나
// 호스트 오류를 사칭하지 못하게).
var passStatus = map[int]bool{
	http.StatusBadRequest:          true,
	http.StatusNotFound:            true,
	http.StatusConflict:            true,
	http.StatusUnprocessableEntity: true,
	http.StatusTooManyRequests:     true,
}

// Translate는 백엔드 응답(상태, 본문)을 브라우저에 줄 꼴로 바꾼다. data는 데이터 요청인지다(204를 200 null로).
//
//   - 200 + JSON 값(MaxBackendResponse까지, json.Valid): 그대로
//   - 204: 조작은 그대로, 데이터는 200 null
//   - 400·404·409·422·429 + JSON 객체: 같은 상태로 addon_error. 코드는 backendCodePattern일 때만, 문구는 제어
//     문자·양방향 제어 문자를 지우고 300자까지 넘긴다. 둘 다 없거나 형식이 틀리면 코드 없이 기본 문구다
//   - 그 밖의 상태, 상한 초과, 잘못된 JSON(4xx 본문이 JSON 객체가 아닌 것도): 502 addon_error(addonCode 없음)
func Translate(status int, body []byte, data bool) BackendReply {
	if len(body) > MaxBackendResponse {
		return invalidReply(fmt.Errorf("response is larger than %d bytes", MaxBackendResponse))
	}
	switch {
	case status == http.StatusOK:
		b := bytes.TrimSpace(body)
		if !json.Valid(b) {
			return invalidReply(errors.New("response body is not valid JSON"))
		}
		return BackendReply{Status: http.StatusOK, Body: b}
	case status == http.StatusNoContent:
		if data {
			return BackendReply{Status: http.StatusOK, Body: json.RawMessage("null")}
		}
		return BackendReply{Status: http.StatusNoContent}
	case passStatus[status]:
		var e map[string]json.RawMessage
		if json.Unmarshal(body, &e) != nil || e == nil {
			// null도 map으로 풀리므로 e == nil로 거른다.
			return invalidReply(fmt.Errorf("backend answered %d without a JSON object", status))
		}
		rep := BackendReply{Status: status, Code: api.ErrAddonError, Message: defaultBackendMessage,
			Err: fmt.Errorf("backend answered %d", status)}
		var code, msg string
		if json.Unmarshal(e["code"], &code) == nil && backendCodePattern.MatchString(code) {
			rep.AddonCode = code
		}
		if json.Unmarshal(e["message"], &msg) == nil {
			rep.Message = sanitizeMessage(msg)
		}
		return rep
	}
	return invalidReply(fmt.Errorf("backend answered %d", status))
}

// sanitizeMessage는 백엔드 오류 문구에서 제어 문자(Cc)와 양방향 제어 문자(U+202A~U+202E, U+2066~U+2069 등)를 지우고
// maxBackendMessage 글자로 자른다. 비면 기본 문구다.
func sanitizeMessage(s string) string {
	s = strings.ToValidUTF8(s, "")
	var b strings.Builder
	n := 0
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			continue
		}
		if n == maxBackendMessage {
			break
		}
		b.WriteRune(r)
		n++
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return defaultBackendMessage
	}
	return out
}
