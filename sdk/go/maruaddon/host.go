// SPDX-License-Identifier: MIT

package maruaddon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// MaxApplyCalls is the largest number of calls [Host.Apply] takes at once.
const MaxApplyCalls = 16

// 호스트 소켓의 상한(.claude/docs/addon-packages.md의 "호스트 소켓").
const (
	// maxHostRequestBytes는 maru가 받는 요청 본문의 상한이다. 넘으면 보내기 전에 오류로 돌려준다.
	maxHostRequestBytes = 64 << 10
	// maxHostResponseBytes는 SDK가 읽는 성공 응답의 상한이다. OMV 결과가 커도 메모리를 끝없이 쓰지 않게 한다.
	maxHostResponseBytes = 32 << 20
	// maxHostErrorBytes는 오류 응답 본문을 읽는 상한이다.
	maxHostErrorBytes = 64 << 10
	// hostConns는 maru가 애드온마다 받는 동시 요청 수다. 연결을 그 수로 묶어 넘는 호출은 429 대신 차례를 기다린다.
	hostConns = 8
	// hostIdleTimeout은 maru의 유휴 시간 제한(30초)보다 짧게 두어, maru가 막 닫은 연결을 다시 쓰지 않게 한다.
	hostIdleTimeout = 20 * time.Second
)

// Call is one OMV RPC call for [Host.Apply].
type Call struct {
	// Service is the OMV RPC service, such as "Smb".
	Service string `json:"service"`
	// Method is the OMV RPC method, such as "setSettings".
	Method string `json:"method"`
	// Params is encoded as JSON. nil is JSON null.
	Params any `json:"params"`
}

// Host calls OMV through maru's host socket on behalf of the request it belongs to ([Request.Host]).
//
// maru only runs methods the addon's manifest declares in "rpc.read" and "rpc.write". During a data request only
// read methods are allowed; during an action, declared write methods are allowed too. Errors from maru are returned
// as a [*HostError]; other errors (for example when the socket cannot be reached) are plain errors.
//
// A Host is valid only while its request's handler runs. Its methods are safe to call from several goroutines.
type Host struct {
	c     *hostClient
	token string
}

// redactedHost는 Host를 출력할 때 쓰는 문구다. 호출 토큰은 넣지 않는다.
//
// String과 Format은 값 수신자로 둔다. *Host와 Host 모두, 다른 구조체(Request)의 필드로 출력될 때도 쓰이고, nil
// *Host는 fmt가 "<nil>"로 출력한다. fmt는 내보내지 않은 필드의 메서드를 부르지 않으므로 토큰 필드에 형을 붙이는
// 것으로는 막을 수 없고, Stringer만으로는 %d 같은 동사에서 필드를 그대로 출력하므로 Format도 둔다.
const redactedHost = "maruaddon.Host{token: <redacted>}"

// String returns a fixed description without the call token, so printing a Host does not log the token.
func (Host) String() string { return redactedHost }

// Format prints the same as [Host.String] for every fmt verb (including %#v and %d), so the call token never
// appears in formatted output.
func (Host) Format(f fmt.State, _ rune) { io.WriteString(f, redactedHost) }

// RPC calls an OMV RPC method and returns its result as raw JSON. params is encoded as JSON (nil is JSON null).
func (h *Host) RPC(ctx context.Context, service, method string, params any) (json.RawMessage, error) {
	var resp struct {
		Result json.RawMessage `json:"result"`
	}
	if err := h.post(ctx, "/v1/rpc", Call{Service: service, Method: method, Params: params}, &resp); err != nil {
		return nil, err
	}
	// RawMessage는 JSON null을 "null"로 받으므로 nil은 result가 없었다는 뜻이다
	if resp.Result == nil {
		return nil, errors.New("maruaddon: host: /v1/rpc: response has no result")
	}
	return resp.Result, nil
}

// RPCInto calls an OMV RPC method like [Host.RPC] and decodes its result into out, which must be a pointer (or
// nil to discard the result). Decoding is lenient: fields of the result that out does not have are ignored.
func (h *Host) RPCInto(ctx context.Context, service, method string, params, out any) error {
	raw, err := h.RPC(ctx, service, method, params)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("maruaddon: decode %s.%s result: %w", service, method, err)
	}
	return nil
}

// RPCBg calls an OMV RPC method that runs in the background (one that returns a background status file name, such
// as many "...Bg" methods) and waits until it finishes. Only administrator sessions may call it.
//
// If output is true, it returns the background job's output (up to 1 MiB; truncated reports whether it was cut).
// If output is false, it only waits and returns an empty string.
func (h *Host) RPCBg(ctx context.Context, service, method string, params any, output bool) (out string, truncated bool, err error) {
	req := struct {
		Service string `json:"service"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
		Output  bool   `json:"output"`
	}{service, method, params, output}
	var resp struct {
		Output    *string `json:"output"`
		Truncated bool    `json:"truncated"`
	}
	if err := h.post(ctx, "/v1/rpc-bg", req, &resp); err != nil {
		return "", false, err
	}
	if resp.Output == nil {
		return "", false, errors.New("maruaddon: host: /v1/rpc-bg: response has no output")
	}
	return *resp.Output, resp.Truncated, nil
}

// Apply runs OMV write calls and then applies the configuration changes (OMV's "apply changes"), waiting until they
// are applied. It is allowed only during actions, every call must be a write method declared in the manifest, and it
// takes 1 to [MaxApplyCalls] calls. It returns one result per call.
//
// If OMV already has pending configuration changes, maru refuses with a [*HostError] with code "config_dirty"
// (see [IsCode]): the administrator should apply or revert them in the OMV workbench first.
func (h *Host) Apply(ctx context.Context, calls ...Call) ([]json.RawMessage, error) {
	if len(calls) == 0 {
		return nil, errors.New("maruaddon: Apply: no calls")
	}
	if len(calls) > MaxApplyCalls {
		return nil, fmt.Errorf("maruaddon: Apply: %d calls, at most %d are allowed", len(calls), MaxApplyCalls)
	}
	req := struct {
		Calls []Call `json:"calls"`
	}{calls}
	var resp struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := h.post(ctx, "/v1/apply", req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Results) != len(calls) {
		return nil, fmt.Errorf("maruaddon: host: /v1/apply: %d results for %d calls", len(resp.Results), len(calls))
	}
	return resp.Results, nil
}

// post는 호스트 소켓에 JSON 요청을 보내고 200 응답을 out에 읽는다. 토큰은 헤더에만 두고 오류 문구에 넣지 않는다.
func (h *Host) post(ctx context.Context, path string, body, out any) error {
	if h == nil || h.c == nil {
		return errors.New("maruaddon: host: not available outside a request")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("maruaddon: host: %s: encode request: %w", path, err)
	}
	if len(data) > maxHostRequestBytes {
		return fmt.Errorf("maruaddon: host: %s: request is %d bytes, larger than the 64 KiB maru accepts",
			path, len(data))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://maru"+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("maruaddon: host: %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h.token != "" {
		req.Header.Set(headerCall, h.token)
	}
	resp, err := h.c.http.Do(req)
	if err != nil {
		return fmt.Errorf("maruaddon: host: %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return readHostError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxHostResponseBytes+1))
	if err != nil {
		return fmt.Errorf("maruaddon: host: %s: read response: %w", path, err)
	}
	if len(raw) > maxHostResponseBytes {
		return fmt.Errorf("maruaddon: host: %s: response is larger than %d bytes", path, maxHostResponseBytes)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("maruaddon: host: %s: invalid response: %w", path, err)
	}
	return nil
}

// readHostError는 maru의 오류 본문 {"error": 문구, "code": 코드}를 HostError로 바꾼다.
func readHostError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxHostErrorBytes))
	he := &HostError{Status: resp.StatusCode}
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if json.Unmarshal(data, &body) == nil {
		he.Code, he.Message = body.Code, body.Error
	}
	if he.Message == "" {
		he.Message = http.StatusText(resp.StatusCode)
	}
	return he
}

// hostClient는 호스트 소켓 하나에 붙는 HTTP/1.1 클라이언트다. 서버의 모든 요청이 연결을 함께 쓴다.
type hostClient struct {
	path string
	http *http.Client
}

func newHostClient(path string) *hostClient {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	tr := &http.Transport{
		// 주소의 호스트(maru)는 Host 헤더로만 쓰고 늘 이 소켓에 붙는다. 프록시 환경 변수는 보지 않는다(Proxy가 nil)
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", path)
		},
		MaxConnsPerHost:     hostConns,
		MaxIdleConnsPerHost: hostConns,
		IdleConnTimeout:     hostIdleTimeout,
		DisableCompression:  true,
	}
	return &hostClient{path: path, http: &http.Client{
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (c *hostClient) close() {
	c.http.CloseIdleConnections()
}
