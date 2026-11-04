package server

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	"maru/backend/internal/addons"
)

// addonFrameCSP는 애드온 프레임 파일의 CSP다. sandbox allow-scripts로 불투명 출처가 되어 대시보드·OMV의 쿠키와
// 저장소에 닿지 못하고(주소를 바로 열어도 같다), 네트워크 연결·폼·다른 출처 자원을 막는다. 데이터는 호스트와의
// postMessage 브리지로만 받는다(.claude/docs/addon-packages.md의 "프레임").
const addonFrameCSP = "sandbox allow-scripts; default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'none'; form-action 'none'; base-uri 'none'; " +
	"frame-ancestors 'self'; webrtc 'block'"

// addonFrameTypes는 프레임 경로로 내주는 파일 형식이다. /etc/mime.types에 기대지 않는다.
var addonFrameTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".json":  "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".webp":  "image/webp",
	".woff2": "font/woff2",
	".txt":   "text/plain; charset=utf-8",
}

// addonSDKFiles는 호스트가 프레임 경로의 _maru/ 아래에 주는 SDK 런타임 파일이다.
var addonSDKFiles = map[string]bool{"sdk.js": true, "maru.css": true}

// framePathSegment는 프레임 경로 조각이다(패키지 검사와 같은 규칙).
var framePathSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

const (
	// frameImmutableCache는 설치본 파일의 캐시다. 경로에 rev가 있어 내용이 바뀌면 주소도 바뀐다.
	frameImmutableCache = "private, max-age=31536000, immutable"
	// frameDevCache는 개발본과 SDK 런타임의 캐시다(매번 다시 확인).
	frameDevCache = "no-cache"
)

// addonUI는 애드온 프레임 파일이다(GET /api/addon-ui/{token}/{rev}/{path...}). 세션 쿠키가 아니라 경로의 토큰으로
// 세션과 패키지를 정한다(불투명 출처의 하위 자원 요청에는 SameSite=Strict 쿠키가 붙지 않는다). 토큰의 세션이
// 살아 있고, 패키지가 켜져 있고, 그 세션이 capability를 만족하고, rev가 같아야 한다. 아니면 모두 404다.
func (h *handler) addonUI(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("Content-Security-Policy", addonFrameCSP)
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Referrer-Policy", "no-referrer")
	hd.Set("X-DNS-Prefetch-Control", "off")
	// 불투명 출처의 프레임이 모듈 스크립트·글꼴을 받으려면 CORS 응답이어야 한다. 쿠키가 아닌 토큰이 권한이다.
	hd.Set("Access-Control-Allow-Origin", "*")
	notFound := func() { http.Error(w, "not found", http.StatusNotFound) }

	if h.Packages == nil {
		notFound()
		return
	}
	g, ok := h.uiTokens.lookup(r.PathValue("token"))
	if !ok {
		notFound()
		return
	}
	s, ok := h.Sessions.Get(g.session)
	if !ok {
		notFound()
		return
	}
	p, ok := h.Packages.Get(g.addon)
	if !ok || p.Rev != r.PathValue("rev") {
		notFound()
		return
	}
	a, ok := h.addons().Get(g.addon)
	if !ok || !addons.Capability(a, h.Plugins.Plugins(), s.Admin).Available {
		notFound()
		return
	}
	name := r.PathValue("path")
	if !validFramePath(name) {
		notFound()
		return
	}
	ctype, ok := addonFrameTypes[strings.ToLower(path.Ext(name))]
	if !ok {
		notFound()
		return
	}

	var (
		f     fs.File
		err   error
		cache = frameImmutableCache
	)
	if sdk, ok := strings.CutPrefix(name, "_maru/"); ok {
		if h.AddonSDK == nil || !addonSDKFiles[sdk] {
			notFound()
			return
		}
		f, err = h.AddonSDK.Open(sdk)
		cache = frameDevCache
	} else {
		var root *os.Root
		root, err = os.OpenRoot(p.Dir)
		if err == nil {
			defer root.Close()
			f, err = root.Open("ui/" + name)
		}
		if p.Dev {
			cache = frameDevCache
		}
	}
	if err != nil {
		notFound()
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		notFound()
		return
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		notFound()
		return
	}
	hd.Set("Content-Type", ctype)
	hd.Set("Cache-Control", cache)
	http.ServeContent(w, r, "", info.ModTime(), rs)
}

// validFramePath는 프레임 경로가 패키지 파일 이름 규칙(숨김 이름·..·빈 조각 없음, 깊이 16 이하)을 따르는지다.
// _maru/는 SDK 런타임이다.
func validFramePath(name string) bool {
	if name == "" || len(name) > 1024 {
		return false
	}
	parts := strings.Split(name, "/")
	if len(parts) > 16 {
		return false
	}
	for _, part := range parts {
		if part != "_maru" && !framePathSegment.MatchString(part) {
			return false
		}
	}
	if parts[0] == "_maru" {
		return len(parts) == 2
	}
	return !slices.Contains(parts, "_maru")
}
