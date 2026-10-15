package server

import (
	"bytes"
	"net/http"
	"testing"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/prefs"
)

func TestAvatarImage(t *testing.T) {
	store := prefs.NewMemoryStore()
	e := newEnv(t, envOptions{prefs: store, images: store})
	b := e.browser()
	wantCode(t, b.raw(http.MethodPut, "/api/preferences/avatar-image", "image/png", testPNG(t)),
		http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	wantStatus(t, b.do(http.MethodGet, "/api/preferences/avatar-image", nil), http.StatusNotFound)
	if got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil)); got.AvatarImage != nil {
		t.Fatalf("new user avatar %+v", got.AvatarImage)
	}

	img := testPNG(t)
	rec := b.raw(http.MethodPut, "/api/preferences/avatar-image", "image/png", img)
	wantStatus(t, rec, http.StatusOK)
	info := decode[api.ImageInfo](t, rec)
	got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.AvatarImage == nil || got.AvatarImage.Rev != info.Rev || got.WallpaperImage != nil {
		t.Fatalf("preferences avatar %+v wallpaper %+v", got.AvatarImage, got.WallpaperImage)
	}
	// 프로필 사진은 바탕화면 설정을 바꾸지 않는다.
	if got.Appearance != nil {
		t.Errorf("appearance changed: %+v", got.Appearance)
	}
	rec = b.do(http.MethodGet, "/api/preferences/avatar-image", nil)
	wantStatus(t, rec, http.StatusOK)
	if !bytes.Equal(rec.Body.Bytes(), img) || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("avatar body %d bytes, headers %v", rec.Body.Len(), rec.Header())
	}

	// 다른 사용자는 남의 사진을 받지 않는다.
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantStatus(t, admin.do(http.MethodGet, "/api/preferences/avatar-image", nil), http.StatusNotFound)

	// 사진은 바탕화면 이미지보다 작은 상한을 쓴다.
	big := append(testPNG(t), make([]byte, maxAvatarBytes)...)
	wantCode(t, b.raw(http.MethodPut, "/api/preferences/avatar-image", "image/png", big),
		http.StatusRequestEntityTooLarge, api.ErrImageTooLarge)
	wantCode(t, b.raw(http.MethodPut, "/api/preferences/avatar-image", "image/svg+xml", []byte("<svg/>")),
		http.StatusUnsupportedMediaType, api.ErrUnsupportedImage)

	wantStatus(t, b.do(http.MethodDelete, "/api/preferences/avatar-image", nil), http.StatusNoContent)
	if got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil)); got.AvatarImage != nil {
		t.Errorf("after delete %+v", got.AvatarImage)
	}
	wantStatus(t, b.do(http.MethodGet, "/api/preferences/avatar-image", nil), http.StatusNotFound)
}
