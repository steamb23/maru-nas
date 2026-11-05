// SPDX-License-Identifier: MIT

package maruaddon

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
)

// Request is one data request or action from maru.
type Request struct {
	// User is the dashboard user's OMV user name.
	User string
	// Admin reports whether the user is an OMV administrator.
	Admin bool
	// Lang is the dashboard display language, such as "en" or "ko". maru only sends languages the dashboard
	// supports and falls back to "en".
	Lang string

	// Query is the query string the frontend sent with a data request. It is nil for actions.
	Query url.Values

	// Action is the action name for an action, and empty for a data request.
	Action string
	// Target is the action's target string. It may be empty.
	Target string
	// Input is the action's input object as raw JSON. It is nil when the action has no input (the frontend sent
	// none, or null) and for data requests. Use [Request.DecodeInput] to decode it.
	Input json.RawMessage

	// Host calls OMV through maru with this request's call token. It is valid only while the handler runs: maru
	// rejects the token once the request has ended. Never nil for requests made by this package.
	Host *Host
}

// DecodeInput decodes the action's input object into v, which must be a pointer.
//
// Decoding is strict: fields of the input that v does not have are an error, so a typo in the frontend does not
// pass silently. Decoding errors are returned as an [*Error] with status 400 and code "bad_input", so a handler can
// return them as they are. If the action has no input, DecodeInput leaves v unchanged and returns nil; check
// Input == nil when the input is required.
func (r *Request) DecodeInput(v any) error {
	if r == nil || r.Input == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(r.Input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		// v가 포인터가 아닌 것은 처리기의 잘못이라 400으로 바꾸지 않는다(500으로 기록된다)
		var ie *json.InvalidUnmarshalError
		if errors.As(err, &ie) {
			return err
		}
		return &Error{Status: http.StatusBadRequest, Code: "bad_input", Message: "invalid input: " + err.Error()}
	}
	return nil
}

// 요청 헤더(.claude/docs/addon-packages.md의 "maru ↔ 백엔드").
const (
	headerUser  = "X-Maru-User"
	headerAdmin = "X-Maru-Admin"
	headerLang  = "X-Maru-Lang"
	headerCall  = "X-Maru-Call"
)

// maxRequestBytes는 조작 본문의 상한이다. maru는 input 8KiB를 넣은 봉투만 보내므로 넉넉하다.
const maxRequestBytes = 64 << 10

func badRequest(message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: message}
}

// newRequest는 공통 헤더를 읽는다. 호출 토큰은 Host에만 둔다.
func newRequest(r *http.Request, host *hostClient) (*Request, *Error) {
	user := r.Header.Get(headerUser)
	if user == "" {
		return nil, badRequest("missing " + headerUser + " header")
	}
	lang := r.Header.Get(headerLang)
	if lang == "" {
		lang = "en"
	}
	return &Request{
		User:  user,
		Admin: r.Header.Get(headerAdmin) == "1",
		Lang:  lang,
		Host:  &Host{c: host, token: r.Header.Get(headerCall)},
	}, nil
}

// readActionBody는 조작 본문 {"target": 문자열, "input": 객체 또는 null}을 읽는다. maru가 필드를 더해도 옛 백엔드가
// 깨지지 않게 봉투의 모르는 필드는 무시한다.
func readActionBody(w http.ResponseWriter, r *http.Request) (target string, input json.RawMessage, e *Error) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return "", nil, badRequest("Content-Type must be application/json")
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return "", nil, badRequest("request body is larger than 64 KiB")
		}
		return "", nil, badRequest("cannot read request body")
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return "", nil, badRequest("request body must be a JSON object")
	}
	var body struct {
		Target string          `json:"target"`
		Input  json.RawMessage `json:"input"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&body); err != nil {
		return "", nil, badRequest("invalid request body: " + err.Error())
	}
	if _, err := dec.Token(); err != io.EOF {
		return "", nil, badRequest("invalid request body: trailing data")
	}
	// RawMessage는 JSON null도 "null" 바이트로 받으므로 직접 nil로 맞춘다
	in := bytes.TrimSpace(body.Input)
	switch {
	case len(in) == 0 || string(in) == "null":
		in = nil
	case in[0] != '{':
		return "", nil, badRequest("input must be a JSON object or null")
	}
	return body.Target, in, nil
}
