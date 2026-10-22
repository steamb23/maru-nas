package server

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"maru/backend/internal/api"
	"maru/backend/internal/prefs"
)

// testPNG, testJPEG는 테스트용으로 합성한 작은 이미지다.
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 0x3c, G: 0x84, B: 0xf7, A: 0xff})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// testWebP는 브라우저 형식 판별(RIFF....WEBPVP)을 통과하는 최소 머리다. 내용은 그리지 않는다.
var testWebP = []byte("RIFF\x24\x00\x00\x00WEBPVP8 \x18\x00\x00\x00fake webp payload......")

func TestAppearanceRoundTrip(t *testing.T) {
	store := prefs.NewMemoryStore()
	e := prefsEnv(t, store)
	b := e.browser()
	off := false
	want := api.AppearancePrefs{
		Theme:         api.ThemeDark,
		Wallpaper:     &api.Wallpaper{Kind: api.WallpaperPreset, Preset: "dusk"},
		Animations:    &off,
		TaskbarAlign:  api.TaskbarAlignLeft,
		WidgetsLocked: true,
	}
	wantCode(t, b.do(http.MethodPut, "/api/preferences/appearance", api.AppearanceRequest{Appearance: &want}),
		http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/drawer", api.DrawerRequest{Drawer: &api.DrawerPrefs{Sort: api.DrawerSortName}}),
		http.StatusNoContent)
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/appearance", api.AppearanceRequest{Appearance: &want}), http.StatusNoContent)
	got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.Appearance == nil || got.Appearance.Theme != api.ThemeDark || got.Appearance.Wallpaper == nil ||
		got.Appearance.Wallpaper.Preset != "dusk" || got.Appearance.Animations == nil || *got.Appearance.Animations ||
		got.Appearance.TaskbarAlign != api.TaskbarAlignLeft || !got.Appearance.WidgetsLocked {
		t.Fatalf("appearance = %+v", got.Appearance)
	}
	if got.Drawer == nil || got.Drawer.Sort != api.DrawerSortName {
		t.Errorf("appearance save changed drawer: %+v", got.Drawer)
	}
	// 빈 값은 기본값을 따른다는 뜻이다.
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/appearance", api.AppearanceRequest{Appearance: &api.AppearancePrefs{}}),
		http.StatusNoContent)
	got = decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.Appearance == nil || got.Appearance.Theme != "" || got.Appearance.Wallpaper != nil || got.Appearance.Animations != nil ||
		got.Appearance.TaskbarAlign != "" || got.Appearance.WidgetsLocked {
		t.Errorf("cleared appearance = %+v", got.Appearance)
	}
}

func TestSetAppearanceRejectsInvalid(t *testing.T) {
	store := prefs.NewMemoryStore()
	e := prefsEnv(t, store)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	cases := map[string]any{
		"missing":          map[string]any{},
		"unknown theme":    map[string]any{"appearance": map[string]any{"theme": "sepia"}},
		"unknown kind":     map[string]any{"appearance": map[string]any{"wallpaper": map[string]any{"kind": "url"}}},
		"bad preset":       map[string]any{"appearance": map[string]any{"wallpaper": map[string]any{"kind": "preset", "preset": "../x"}}},
		"empty preset":     map[string]any{"appearance": map[string]any{"wallpaper": map[string]any{"kind": "preset"}}},
		"image with id":    map[string]any{"appearance": map[string]any{"wallpaper": map[string]any{"kind": "image", "preset": "dusk"}}},
		"animations type":  map[string]any{"appearance": map[string]any{"animations": "no"}},
		"unknown align":    map[string]any{"appearance": map[string]any{"taskbarAlign": "right"}},
		"lock type":        map[string]any{"appearance": map[string]any{"widgetsLocked": "yes"}},
		"wallpaper string": map[string]any{"appearance": map[string]any{"wallpaper": "dusk"}},
	}
	for name, body := range cases {
		if rec := b.do(http.MethodPut, "/api/preferences/appearance", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d (body %s)", name, rec.Code, rec.Body.String())
		}
	}
	if p, _ := store.Get("admin"); p.Appearance != nil {
		t.Errorf("invalid appearance saved: %+v", *p.Appearance)
	}
}

func TestWallpaperImage(t *testing.T) {
	store := prefs.NewMemoryStore()
	e := newEnv(t, envOptions{prefs: store})
	b := e.browser()
	wantCode(t, b.raw(http.MethodPut, "/api/preferences/wallpaper-image", "image/png", testPNG(t)),
		http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	wantStatus(t, b.do(http.MethodGet, "/api/preferences/wallpaper-image", nil), http.StatusNotFound)

	// 올리면 바탕화면이 그 이미지로 바뀐다.
	img := testPNG(t)
	rec := b.raw(http.MethodPut, "/api/preferences/wallpaper-image", "image/png", img)
	wantStatus(t, rec, http.StatusOK)
	info := decode[api.ImageInfo](t, rec)
	got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.WallpaperImage == nil || got.WallpaperImage.Rev != info.Rev || info.Rev == "" {
		t.Fatalf("wallpaperImage = %+v, put %+v", got.WallpaperImage, info)
	}
	if got.Appearance == nil || got.Appearance.Wallpaper == nil || got.Appearance.Wallpaper.Kind != api.WallpaperImage {
		t.Fatalf("appearance = %+v", got.Appearance)
	}
	// 응답 꼬리표(wallpaperImage)는 설정 파일에 저장하지 않는다.
	if p, _ := store.Get("user"); p.WallpaperImage != nil {
		t.Errorf("stored wallpaperImage: %+v", p.WallpaperImage)
	}

	rec = b.do(http.MethodGet, "/api/preferences/wallpaper-image", nil)
	wantStatus(t, rec, http.StatusOK)
	if !bytes.Equal(rec.Body.Bytes(), img) {
		t.Error("image body differs")
	}
	h := rec.Header()
	if h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(h.Get("Content-Security-Policy"), "sandbox") || h.Get("ETag") != `"`+info.Rev+`"` {
		t.Errorf("headers %v", h)
	}
	// 바뀌지 않았으면 304다.
	b.header.Set("If-None-Match", `"`+info.Rev+`"`)
	wantStatus(t, b.do(http.MethodGet, "/api/preferences/wallpaper-image", nil), http.StatusNotModified)
	b.header.Del("If-None-Match")

	// 다른 사용자는 남의 이미지를 받지 않는다.
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantStatus(t, admin.do(http.MethodGet, "/api/preferences/wallpaper-image", nil), http.StatusNotFound)

	// 프리셋으로 바꿔도 이미지는 남고, 지우면 이미지를 가리키던 바탕화면만 지운다.
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/appearance", api.AppearanceRequest{
		Appearance: &api.AppearancePrefs{Theme: api.ThemeLight, Wallpaper: &api.Wallpaper{Kind: api.WallpaperImage}},
	}), http.StatusNoContent)
	wantStatus(t, b.do(http.MethodDelete, "/api/preferences/wallpaper-image", nil), http.StatusNoContent)
	got = decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.WallpaperImage != nil || got.Appearance == nil || got.Appearance.Wallpaper != nil || got.Appearance.Theme != api.ThemeLight {
		t.Errorf("after delete: image %+v appearance %+v", got.WallpaperImage, got.Appearance)
	}
	wantStatus(t, b.do(http.MethodGet, "/api/preferences/wallpaper-image", nil), http.StatusNotFound)
}

func TestWallpaperImageFormats(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	put := func(contentType string, body []byte) *http.Response {
		return b.raw(http.MethodPut, "/api/preferences/wallpaper-image", contentType, body).Result()
	}
	// 받는 형식. 저장 형식은 Content-Type이 아니라 내용으로 정한다.
	for name, c := range map[string]struct {
		header string
		body   []byte
		want   string
	}{
		"png":            {"image/png", testPNG(t), "image/png"},
		"jpeg":           {"image/jpeg", testJPEG(t), "image/jpeg"},
		"webp":           {"image/webp", testWebP, "image/webp"},
		"header differs": {"image/webp", testPNG(t), "image/png"},
	} {
		if res := put(c.header, c.body); res.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d", name, res.StatusCode)
			continue
		}
		if ct := b.do(http.MethodGet, "/api/preferences/wallpaper-image", nil).Header().Get("Content-Type"); ct != c.want {
			t.Errorf("%s: served as %q", name, ct)
		}
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	for name, c := range map[string]struct {
		header string
		body   []byte
	}{
		"svg header":         {"image/svg+xml", svg},
		"svg as png":         {"image/png", svg},
		"html as png":        {"image/png", []byte("<!doctype html><script>alert(1)</script>")},
		"json":               {"application/json", testPNG(t)},
		"multipart":          {"multipart/form-data; boundary=x", testPNG(t)},
		"form":               {"application/x-www-form-urlencoded", testPNG(t)},
		"no content type":    {"", testPNG(t)},
		"empty":              {"image/png", nil},
		"gif (not accepted)": {"image/gif", []byte("GIF89a fake")},
	} {
		t.Run(name, func(t *testing.T) {
			rec := b.raw(http.MethodPut, "/api/preferences/wallpaper-image", c.header, c.body)
			wantCode(t, rec, http.StatusUnsupportedMediaType, api.ErrUnsupportedImage)
		})
	}
	big := append(testPNG(t), make([]byte, maxImageBytes)...)
	wantCode(t, b.raw(http.MethodPut, "/api/preferences/wallpaper-image", "image/png", big),
		http.StatusRequestEntityTooLarge, api.ErrImageTooLarge)
}
