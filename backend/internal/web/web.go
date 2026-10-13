// Package web은 실행 파일에 포함된 프론트엔드 빌드 결과를 서빙한다.
//
// 프론트엔드는 `npm run build`로 static/dist/에 빌드된다(frontend/vite.config.ts 참고).
// Vite가 빌드 전에 static/dist/를 비우므로 .gitkeep은 한 단계 위 static/에 둔다.
// 저장소에는 static/.gitkeep만 커밋하므로, 프론트엔드 없이 빌드해도
// 컴파일은 되고 대신 짧은 안내 문구를 반환한다.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:static
var staticFS embed.FS

// Handler는 포함된 프론트엔드를 서빙한다. 경로는 대시보드 루트 기준이며,
// /desk 접두사는 리버스 프록시가 제거한다.
func Handler() http.Handler {
	dist, err := fs.Sub(staticFS, "static/dist")
	if err != nil {
		panic(err)
	}
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend not built: run `make build`", http.StatusNotFound)
		})
	}
	return http.FileServerFS(dist)
}
