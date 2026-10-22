package server

import (
	"net/http"
	"path"
	"strings"

	"maru/backend/internal/auth"
)

// previewTypes는 미리보기가 그 형식으로 보내는 확장자(소문자, 점 없이)다. 브라우저가 그대로 그리거나 재생하는
// 형식만 둔다. 여기에 없는 파일은 text/plain으로 보낸다(뷰어가 텍스트로 보일 파일만 요청한다).
var previewTypes = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
	"webp": "image/webp",
	"avif": "image/avif",
	"bmp":  "image/bmp",
	"ico":  "image/x-icon",
	"svg":  "image/svg+xml",
	"mp4":  "video/mp4",
	"m4v":  "video/mp4",
	"webm": "video/webm",
	"ogv":  "video/ogg",
	"mov":  "video/quicktime",
	"mp3":  "audio/mpeg",
	"m4a":  "audio/mp4",
	"aac":  "audio/aac",
	"oga":  "audio/ogg",
	"ogg":  "audio/ogg",
	"opus": "audio/ogg",
	"wav":  "audio/wav",
	"flac": "audio/flac",
	"pdf":  "application/pdf",
}

// pdfCSP는 PDF 미리보기의 CSP다. 브라우저 내장 PDF 뷰어는 sandbox 문서에서 열리지 않으므로 sandbox를 빼고,
// 하위 자원을 막고 대시보드 안의 iframe에서만 열리게 한다.
const pdfCSP = "default-src 'none'; frame-ancestors 'self'"

// previewHeaders는 path의 확장자로 정한 미리보기 머리다. 내용을 보고 형식을 짐작하지 않는다(nosniff).
func previewHeaders(p string) fileHeaders {
	name := path.Base(p)
	ext := ""
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		ext = strings.ToLower(name[i+1:])
	}
	ct, ok := previewTypes[ext]
	if !ok {
		ct = "text/plain; charset=utf-8"
	}
	csp := sandboxCSP
	if ct == "application/pdf" {
		csp = pdfCSP
	}
	return fileHeaders{contentType: ct, disposition: contentDisposition("inline", name), csp: csp}
}

// filesPreview는 파일 내용을 브라우저가 그리는 형식으로 보낸다(뷰어 창의 이미지·동영상·오디오·PDF·텍스트).
// 내려받기와 같은 범위·If-Range를 받는다. SVG·HTML 같은 형식도 sandbox CSP라 스크립트가 돌지 않고, 허용 목록에
// 없는 파일은 text/plain이다.
func (h *handler) filesPreview(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	p := r.URL.Query().Get("path")
	if !h.allowedPath(w, r, s, ticket, p) {
		return
	}
	h.serveFile(w, r, s, ticket, p, previewHeaders(p))
}
