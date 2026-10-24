// Package omv는 OMV 웹 UI의 JSON RPC 엔드포인트(rpc.php)를 호출하는 클라이언트다.
//
// 백엔드는 대시보드 세션마다 OMV 세션 ID를 보관하고, 항상 같은 주소와 고정
// User-Agent로 OMV를 호출한다. OMV는 세션을 User-Agent에 묶어 두므로 브라우저의
// 쿠키나 User-Agent를 그대로 넘기지 않는다.
//
// 세션 ID는 X-OpenMediaVault-SessionID 헤더로 보내고, 응답의 Set-Cookie에 새
// ID가 오면 호출자에게 돌려준다. OMV는 2차 인증이 필요한 로그인에서 세션 ID를
// 응답 본문 없이 Set-Cookie로만 바꾸기 때문이다. net/http/cookiejar는 쓰지 않는다.
// HTTPS에서 OMV가 붙이는 Secure 쿠키를 평문 루프백 주소로는 돌려보내지 않고,
// 하나의 jar를 여러 세션이 공유하면 세션이 섞이기 때문이다.
package omv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SessionCookie는 OMV PHP 세션 쿠키 이름이다.
const SessionCookie = "OPENMEDIAVAULT-SESSIONID"

// loginCookiePrefix는 OMV가 "처음 보는 브라우저" 판별에 쓰는 쿠키 이름 접두사다.
const loginCookiePrefix = "OPENMEDIAVAULT-LOGIN-"

// maxResponseBytes는 rpc.php 응답 본문의 최대 크기다.
const maxResponseBytes = 8 << 20

// Client는 rpc.php를 호출한다. 여러 고루틴에서 함께 써도 된다.
type Client struct {
	// BaseURL은 OMV 웹 UI 주소다(예: http://127.0.0.1). 끝에 /rpc.php를 붙여 호출한다.
	BaseURL string
	// UserAgent는 모든 호출에 쓰는 고정 값이다. 세션 중에 바뀌면 OMV가 세션을 파기한다.
	UserAgent string
	HTTP      *http.Client

	login loginCookies
}

// NewClient는 기본 타임아웃을 둔 Client를 만든다.
func NewClient(baseURL, userAgent string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		UserAgent: userAgent,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
	}
}

// Request는 rpc.php 호출 하나다.
type Request struct {
	// SessionID가 비어 있으면 세션 없이 호출한다(로그인).
	SessionID string
	Service   string
	Method    string
	Params    any
	// NoTouch가 true면 OMV 세션의 마지막 접근 시각을 갱신하지 않는다
	// (options.updatelastaccess=false). 사용자 조작이 아닌 확인용 호출에 쓴다.
	NoTouch bool
	// ClientIP는 브라우저 IP다. X-Forwarded-For로 넘겨 OMV 인증 로그에 남게 한다.
	// OMV는 이 값을 세션 검증에 쓰지 않는다.
	ClientIP string
	cookies  []*http.Cookie
}

// Response는 성공한 호출의 결과다.
type Response struct {
	Result json.RawMessage
	// SessionID는 응답의 Set-Cookie로 받은 새 OMV 세션 ID다. 없으면 빈 문자열이다.
	SessionID string
	cookies   []*http.Cookie
}

// Error는 OMV가 반환한 RPC 오류다. 응답의 trace(서버 경로가 들어 있음)는 버린다.
type Error struct {
	HTTPStatus int
	Code       int
	Message    string
	// rpc는 rpc.php가 만든 JSON 오류 응답인지다. false면 nginx 오류 페이지 등이다.
	rpc bool
}

func (e *Error) Error() string {
	return fmt.Sprintf("omv rpc error (HTTP %d, code %d): %s", e.HTTPStatus, e.Code, e.Message)
}

// FromRPC는 rpc.php가 만든 JSON 오류 응답인지다. false면 nginx 오류 페이지처럼 OMV 엔진까지 가지 못한 응답이다.
func (e *Error) FromRPC() bool { return e.rpc }

// HTTPStatus는 err가 *Error면 그 HTTP 상태 코드를, 아니면 0을 반환한다.
func HTTPStatus(err error) int {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.HTTPStatus
	}
	return 0
}

// IsUnauthorized는 OMV가 세션을 인증되지 않음·만료로 거부했는지 확인한다.
func IsUnauthorized(err error) bool { return HTTPStatus(err) == http.StatusUnauthorized }

type rpcRequest struct {
	Service string         `json:"service"`
	Method  string         `json:"method"`
	Params  any            `json:"params"`
	Options map[string]any `json:"options,omitempty"`
}

type rpcResponse struct {
	Response json.RawMessage `json:"response"`
	Error    *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Call은 rpc.php를 한 번 호출한다. 네트워크 오류와 형식이 다른 응답은 *Error가 아닌
// 오류로, OMV가 반환한 RPC 오류는 *Error로 돌려준다.
//
// ctx에 마감 시각이 있으면 HTTP 클라이언트의 기본 제한 시간(HTTP.Timeout) 대신 그것을 쓴다.
// Smart.getList처럼 대기 중인 디스크를 깨워야 해서 오래 걸리는 호출에 쓴다.
func (c *Client) Call(ctx context.Context, r Request) (Response, error) {
	params := r.Params
	if params == nil {
		params = map[string]any{}
	}
	body := rpcRequest{Service: r.Service, Method: r.Method, Params: params}
	if r.NoTouch {
		body.Options = map[string]any{"updatelastaccess": false}
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/rpc.php", bytes.NewReader(buf))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if r.SessionID != "" {
		req.Header.Set("X-OpenMediaVault-SessionID", r.SessionID)
	}
	if r.ClientIP != "" {
		req.Header.Set("X-Forwarded-For", r.ClientIP)
	}
	for _, ck := range r.cookies {
		req.AddCookie(ck)
	}

	hc := c.HTTP
	if _, ok := ctx.Deadline(); ok && hc.Timeout > 0 {
		cp := *hc
		cp.Timeout = 0
		hc = &cp
	}
	res, err := hc.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("omv rpc %s.%s: %w", r.Service, r.Method, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return Response{}, fmt.Errorf("omv rpc %s.%s: read response: %w", r.Service, r.Method, err)
	}

	var parsed rpcResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		// nginx 오류 페이지 등. 상태 코드만 남긴다.
		if res.StatusCode != http.StatusOK {
			return Response{}, &Error{HTTPStatus: res.StatusCode, Message: http.StatusText(res.StatusCode)}
		}
		return Response{}, fmt.Errorf("omv rpc %s.%s: unexpected response", r.Service, r.Method)
	}
	if parsed.Error != nil || res.StatusCode != http.StatusOK {
		e := &Error{HTTPStatus: res.StatusCode, rpc: true}
		if parsed.Error != nil {
			e.Code, e.Message = parsed.Error.Code, parsed.Error.Message
		}
		if e.HTTPStatus == http.StatusOK {
			e.HTTPStatus = http.StatusInternalServerError
		}
		return Response{}, e
	}

	out := Response{Result: parsed.Response, cookies: res.Cookies()}
	for _, ck := range out.cookies {
		// 로그아웃 때 OMV가 보내는 삭제용 쿠키(Max-Age=0)는 새 ID가 아니다.
		if ck.Name == SessionCookie && ck.Value != "" && ck.MaxAge >= 0 {
			out.SessionID = ck.Value
		}
	}
	return out, nil
}
