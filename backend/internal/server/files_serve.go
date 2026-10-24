package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/helper/proto"
)

// filesDownload는 파일 내용을 내려받기로 보낸다. 범위 하나(재개용)와 If-Range만 지원한다. 브라우저가 내용을
// 그리지 않게 언제나 application/octet-stream과 attachment, sandbox CSP로 보낸다.
func (h *handler) filesDownload(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	p := r.URL.Query().Get("path")
	if !h.allowedPath(w, r, s, ticket, p) {
		return
	}
	h.serveFile(w, r, s, ticket, p, fileHeaders{
		contentType: "application/octet-stream",
		disposition: contentDisposition("attachment", path.Base(p)),
		csp:         sandboxCSP,
	})
}

// sandboxCSP는 파일 내용을 문서로 열어도 스크립트·하위 자원을 쓰지 못하게 하는 CSP다.
const sandboxCSP = "default-src 'none'; sandbox"

// fileHeaders는 serveFile이 내용에 붙이는 머리다(내려받기와 미리보기가 다르다).
type fileHeaders struct {
	contentType string
	disposition string
	csp         string
}

// serveFile은 파일 내용을 fs.read로 흘려 보낸다. 범위 하나(재개·동영상 탐색)와 ETag If-Range만 지원하고,
// 여러 범위는 200 전체다.
func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, s auth.Session, ticket, p string, fh fileHeaders) {
	var offset, length int64
	partial := false
	if rh := r.Header.Get("Range"); rh != "" {
		st, err := h.Helper.Stat(r.Context(), ticket, p)
		if err != nil {
			h.writeFilesError(w, s, err)
			return
		}
		if st.Type == proto.TypeFile && ifRangeMatches(r.Header.Get("If-Range"), etagOf(st.Size, st.ModTimeNs)) {
			start, n, ok, satisfiable := parseRange(rh, st.Size)
			if ok && !satisfiable {
				w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(st.Size, 10))
				writeError(w, http.StatusRequestedRangeNotSatisfiable, api.ErrBadRequest, "range not satisfiable")
				return
			}
			if ok {
				offset, length, partial = start, n, true
			}
		}
	}
	res, body, err := h.Helper.Read(r.Context(), ticket, p, offset, length)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	defer body.Close()
	hd := w.Header()
	hd.Set("Content-Type", fh.contentType)
	hd.Set("Content-Disposition", fh.disposition)
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", fh.csp)
	hd.Set("Cache-Control", "private, no-store")
	hd.Set("Accept-Ranges", "bytes")
	hd.Set("ETag", etagOf(res.FileSize, res.ModTimeNs))
	hd.Set("Last-Modified", time.Unix(0, res.ModTimeNs).UTC().Format(http.TimeFormat))
	hd.Set("Content-Length", strconv.FormatInt(body.Size, 10))
	status := http.StatusOK
	if partial {
		status = http.StatusPartialContent
		hd.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+body.Size-1, res.FileSize))
	}
	w.WriteHeader(status)
	if _, err := io.Copy(w, body); err != nil && r.Context().Err() == nil {
		log.Printf("files: read user=%q: %v", s.Username, err)
	}
}

// etagOf는 파일 크기와 수정 시각으로 만든 ETag다(재개한 내려받기가 같은 파일인지 If-Range로 확인한다).
func etagOf(size, modTimeNs int64) string {
	return `"` + strconv.FormatInt(size, 36) + "-" + strconv.FormatInt(modTimeNs, 36) + `"`
}

// ifRangeMatches는 If-Range가 없거나 ETag가 같은지다. 날짜 형식의 If-Range는 받지 않는다(전체를 보낸다).
func ifRangeMatches(ifRange, etag string) bool {
	return ifRange == "" || ifRange == etag
}

// parseRange는 Range 머리의 범위 하나("bytes=a-b", "bytes=a-", "bytes=-n")다. ok가 false면 Range를 무시하고
// 전체를 보낸다(여러 범위, 모르는 단위, 잘못된 형식). satisfiable이 false면 416이다.
func parseRange(h string, size int64) (offset, length int64, ok, satisfiable bool) {
	spec, found := strings.CutPrefix(h, "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, 0, false, false
	}
	a, b, found := strings.Cut(strings.TrimSpace(spec), "-")
	if !found {
		return 0, 0, false, false
	}
	if a == "" {
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil || n < 0 {
			return 0, 0, false, false
		}
		if n == 0 || size == 0 {
			return 0, 0, true, false
		}
		n = min(n, size)
		return size - n, n, true, true
	}
	start, err := strconv.ParseInt(a, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false, false
	}
	end := size - 1
	if b != "" {
		e, err := strconv.ParseInt(b, 10, 64)
		if err != nil || e < start {
			return 0, 0, false, false
		}
		end = min(e, size-1)
	}
	if start >= size {
		return 0, 0, true, false
	}
	return start, end - start + 1, true, true
}

// contentDisposition은 kind(attachment, inline)와 파일 이름이다. ASCII가 아닌 글자는 filename*(RFC 5987)에 담고,
// filename에는 바꿔 쓴 ASCII 이름을 둔다.
func contentDisposition(kind, name string) string {
	var ascii strings.Builder
	for _, r := range name {
		if r < 0x20 || r >= 0x7f || r == '"' || r == '\\' {
			ascii.WriteByte('_')
		} else {
			ascii.WriteRune(r)
		}
	}
	return kind + `; filename="` + ascii.String() + `"; filename*=UTF-8''` + rfc5987(name)
}

func rfc5987(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
