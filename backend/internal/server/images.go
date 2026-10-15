package server

import (
	"errors"
	"io"
	"log"
	"mime"
	"net/http"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/prefs"
)

// maxImageBytes는 올리는 배경 이미지 한 장의 상한이다. 4K 사진도 넉넉히 들어간다.
const maxImageBytes = 20 << 20

// maxAvatarBytes는 프로필 사진의 상한이다. 브라우저가 512px로 잘라 줄여 보내므로 PNG여도 넉넉하다.
const maxAvatarBytes = 2 << 20

// readImageBody는 PUT으로 올린 배경 이미지를 읽는다. 실패하면 오류 응답을 쓰고 ok가 false다.
//
// 본문은 multipart가 아니라 이미지 바이트 그대로이고 Content-Type이 image/png·jpeg·webp여야 한다.
// 다른 출처의 HTML 폼은 이런 본문을 보낼 수 없으므로 JSON만 받는 규칙(requireJSON)과 같은 CSRF 보조 장치가 된다.
// 형식은 Content-Type을 믿지 않고 내용을 보고 다시 판별해 그 값으로 저장한다(SVG 등 스크립트를 품을 수 있는 형식 차단).
func readImageBody(w http.ResponseWriter, r *http.Request, limit int64) (data []byte, contentType string, ok bool) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !prefs.IsImageType(mt) {
		writeError(w, http.StatusUnsupportedMediaType, api.ErrUnsupportedImage, "image must be png, jpeg or webp")
		return nil, "", false
	}
	data, err = io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, api.ErrImageTooLarge, "image is too large")
		} else {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "failed to read image")
		}
		return nil, "", false
	}
	sniffed := http.DetectContentType(data)
	if len(data) == 0 || !prefs.IsImageType(sniffed) {
		writeError(w, http.StatusUnsupportedMediaType, api.ErrUnsupportedImage, "image must be png, jpeg or webp")
		return nil, "", false
	}
	return data, sniffed, true
}

// serveImage는 저장된 배경 이미지를 보낸다. 없으면 404다.
// 형식은 저장할 때 판별한 값으로 정하고 브라우저가 다시 추측하지 않게 하며(nosniff), 문서로 열려도
// 스크립트가 돌지 않게 한다(sandbox). ETag(리비전)로 바뀌지 않았으면 304를 준다.
func (h *handler) serveImage(w http.ResponseWriter, r *http.Request, key prefs.ImageKey) {
	img, err := h.Images.OpenImage(key)
	if errors.Is(err, prefs.ErrNoImage) {
		writeError(w, http.StatusNotFound, "", "no image")
		return
	}
	if err != nil {
		log.Printf("images: open: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to read image")
		return
	}
	defer img.Close()
	hdr := w.Header()
	hdr.Set("Content-Type", img.ContentType)
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	hdr.Set("Cache-Control", "private, no-cache")
	hdr.Set("ETag", `"`+img.Info.Rev+`"`)
	http.ServeContent(w, r, "", img.ModTime, img.Content)
}

// putImage는 올린 이미지(limit 바이트 이하)를 key 자리에 저장하고 정보를 돌려준다.
// 실패하면 오류 응답을 쓰고 ok가 false다.
func (h *handler) putImage(w http.ResponseWriter, r *http.Request, key prefs.ImageKey, limit int64) (api.ImageInfo, bool) {
	data, contentType, ok := readImageBody(w, r, limit)
	if !ok {
		return api.ImageInfo{}, false
	}
	info, err := h.Images.PutImage(key, contentType, data)
	if err != nil {
		log.Printf("images: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save image")
		return api.ImageInfo{}, false
	}
	return info, true
}

func (h *handler) deleteImage(w http.ResponseWriter, key prefs.ImageKey) bool {
	if err := h.Images.DeleteImage(key); err != nil {
		log.Printf("images: delete: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to delete image")
		return false
	}
	return true
}
