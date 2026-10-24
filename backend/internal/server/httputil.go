package server

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"maru/backend/internal/api"
)

// 핸들러가 함께 쓰는 HTTP 도우미: JSON 응답·오류 응답, JSON 본문 읽기, 브라우저 IP, 실패 지연.

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

// clientIP는 브라우저 IP다. 앞단 프록시가 붙인 X-Forwarded-For의 마지막 항목을 쓴다.
// 앞쪽 항목은 브라우저가 임의로 보낼 수 있으므로 믿지 않는다.
// 알 수 없으면 빈 문자열이다(Unix 소켓으로 들어온 요청은 RemoteAddr가 IP가 아니다).
// 빈 값끼리는 로그인 시도 제한에서 같은 IP로 취급된다.
func clientIP(r *http.Request) string {
	if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
		parts := strings.Split(xff[len(xff)-1], ",")
		if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
			return ip.String()
		}
	}
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}

// requireJSON은 본문이 있는 요청(POST, PUT)의 Content-Type이 application/json인지 확인한다.
// SameSite=Strict 쿠키와 함께 쓰는 CSRF 보조 장치다(다른 출처의 폼은 이 형식으로 보낼 수 없다).
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, api.ErrBadRequest, "content type must be application/json")
		return false
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeJSONLimit(w, r, v, maxBodyBytes)
}

// decodeJSONLimit은 limit 바이트까지의 JSON 본문을 v로 읽는다. 실패하면 오류 응답을 쓰고 false를 반환한다.
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if !requireJSON(w, r) {
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, api.ErrorResponse{Error: msg, Code: code})
}

func writeRateLimited(w http.ResponseWriter, wait time.Duration) {
	secs := int(math.Ceil(wait.Seconds()))
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeJSON(w, http.StatusTooManyRequests, api.ErrorResponse{
		Error:             "too many failed attempts, try again later",
		Code:              api.ErrRateLimited,
		RetryAfterSeconds: secs,
	})
}

// sleep은 d만큼 기다린다. 요청이 취소되면 바로 돌아온다.
func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
