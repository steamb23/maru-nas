// SPDX-License-Identifier: MIT

package maruaddon

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
)

// Error is an error a handler returns to answer with a specific status and code instead of 500.
//
// maru passes the error to the addon's frontend only when Status is 400, 404, 409, 422 or 429 and Code matches
// ^[a-z][a-z0-9_]{0,31}$. maru removes control characters from Message and cuts it to 300 characters. Any other
// Status or Code is a programming error: the backend answers 500 and writes the error to [Handler.ErrorLog].
//
// Handlers may wrap an *Error (for example with fmt.Errorf and %w); the outermost *Error in the chain is used.
type Error struct {
	// Status is the HTTP status: 400, 404, 409, 422 or 429.
	Status int
	// Code is a short machine-readable code for the frontend, such as "not_found" or "quota_exceeded".
	Code string
	// Message is a human-readable description. The dashboard does not translate it.
	Message string
}

// NewError returns an *Error with the given status, code and message.
func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Error returns a description of e for logs.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message)
}

// codePattern은 maru가 백엔드 오류 코드를 넘기는 형식이다(.claude/docs/addon-packages.md의 "maru ↔ 백엔드").
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// valid는 maru가 그대로 넘기는 상태와 코드인지다. 그 밖의 상태는 maru가 502로 바꾸므로 SDK가 500으로 기록한다.
func (e *Error) valid() bool {
	if e == nil || !codePattern.MatchString(e.Code) {
		return false
	}
	switch e.Status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity,
		http.StatusTooManyRequests:
		return true
	}
	return false
}

// HostError is an error answer from the maru host socket (see [Host]).
//
// Codes maru uses include "bad_request" (400), "call_invalid" (401, the call token is missing, unknown or
// expired), "unauthorized" (401, the user's OMV session ended), "rpc_forbidden" (403, the method is not declared
// in the manifest, or a write from a data request), "admin_required" (403), "config_dirty" (409, OMV has pending
// configuration changes), "busy" (429), "omv_error" (502, Message is OMV's error message), "omv_unavailable"
// (503) and "timeout" (504).
//
// A handler that returns a *HostError answers 500, like any other error. To pass a host error to the frontend,
// check it with [IsCode] and return an [*Error].
type HostError struct {
	// Status is the HTTP status maru answered with.
	Status int
	// Code is maru's error code. It is empty if the answer had no JSON error body.
	Code string
	// Message is maru's error message, or the HTTP status text if there was none.
	Message string
}

// Error returns a description of e.
func (e *HostError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Code == "" {
		return fmt.Sprintf("maruaddon: host: %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("maruaddon: host: %d %s: %s", e.Status, e.Code, e.Message)
}

// IsCode reports whether err is, or wraps, a [*HostError] with the given code.
func IsCode(err error, code string) bool {
	var he *HostError
	return errors.As(err, &he) && he != nil && he.Code == code
}
