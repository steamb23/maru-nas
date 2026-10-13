// Package server는 HTTP 라우트와 핸들러를 연결한다.
//
// 라우트는 대시보드 루트(/api/...)에 둔다. 외부 접두사 /desk는
// 리버스 프록시(개발: Vite, 운영: Caddy)가 제거한다.
package server

import (
	"encoding/json"
	"log"
	"net/http"

	"omv-desktop-dashboard/backend/internal/api"
)

// SystemSource는 시스템 통계를 제공한다. *sysinfo.Reader가 구현한다.
type SystemSource interface {
	System() (api.SystemResponse, error)
}

// New는 루트 핸들러를 반환한다. /api/ 밖의 요청은 static이 처리한다.
func New(sys SystemSource, static http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, api.HealthResponse{Status: "ok"})
	})
	mux.HandleFunc("GET /api/system", func(w http.ResponseWriter, _ *http.Request) {
		info, err := sys.System()
		if err != nil {
			log.Printf("system info: %v", err)
			writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to read system info"})
			return
		}
		writeJSON(w, http.StatusOK, info)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
	})
	mux.Handle("/", static)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
