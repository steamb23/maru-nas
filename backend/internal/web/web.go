// Package web은 실행 파일에 포함된 프론트엔드 빌드 결과를 서빙한다.
//
// 프론트엔드는 `npm run build`로 static/dist/에 빌드된다(frontend/vite.config.ts 참고).
// Vite가 빌드 전에 static/dist/를 비우므로 .gitkeep은 한 단계 위 static/에 둔다.
// 저장소에는 static/.gitkeep만 커밋하므로, 프론트엔드 없이 빌드해도
// 컴파일은 되고 대신 짧은 안내 문구를 반환한다.
//
// 캐시: Vite는 assets/ 아래 파일 이름에 내용 해시를 넣으므로 내용이 바뀌면 이름도 바뀐다. 이 파일들은
// 1년 동안 다시 묻지 않게 하고(immutable), 이름이 고정된 index.html은 매번 다시 확인하게 한다(no-cache).
// 그래야 업그레이드 뒤 새로 고치면 새 해시의 청크를 받는다. embed 파일은 수정 시각이 0이라
// Last-Modified가 없으므로, 내용 해시로 ETag를 붙여 다시 확인할 때 304로 답한다.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:static
var staticFS embed.FS

// assetsDir는 Vite가 해시를 붙인 이름으로 내보내는 디렉터리다(build.assetsDir 기본값).
const assetsDir = "assets/"

const (
	immutableCache  = "public, max-age=31536000, immutable"
	revalidateCache = "no-cache"
)

// Handler는 포함된 프론트엔드를 서빙한다. 경로는 대시보드 루트 기준이며,
// 경로 접두사가 있으면 리버스 프록시가 제거한다.
func Handler() http.Handler {
	dist, err := fs.Sub(staticFS, "static/dist")
	if err != nil {
		panic(err)
	}
	return newHandler(dist)
}

// newHandler는 빌드 결과 루트(dist)를 서빙한다. 테스트는 fstest.MapFS를 넣는다.
func newHandler(dist fs.FS) http.Handler {
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend not built: run `make build`", http.StatusNotFound)
		})
	}
	etags, err := fileETags(dist)
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		// 있는 파일에만 붙인다. 없는 해시 이름(업그레이드 전 탭이 요청한 옛 청크)의 404가 캐시되면
		// 그 버전으로 되돌렸을 때 브라우저가 계속 404를 쓴다.
		if etag, ok := etags[name]; ok {
			h := w.Header()
			h.Set("ETag", etag)
			if strings.HasPrefix(name, assetsDir) {
				h.Set("Cache-Control", immutableCache)
			} else {
				h.Set("Cache-Control", revalidateCache)
			}
		}
		files.ServeHTTP(w, r)
	})
}

// fileETags는 dist의 파일마다 내용 해시로 만든 ETag다. 포함된 파일은 실행 중에 바뀌지 않으므로 시작할 때
// 한 번 계산한다.
func fileETags(dist fs.FS) (map[string]string, error) {
	etags := make(map[string]string)
	err := fs.WalkDir(dist, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(dist, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		etags[name] = `"` + hex.EncodeToString(sum[:16]) + `"`
		return nil
	})
	return etags, err
}
