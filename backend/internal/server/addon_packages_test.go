package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/api"
	"maru/backend/internal/prefs"
)

// addonArchive는 manifest 수정(edit)을 적용한 최소 패키지 압축 파일이다.
func addonArchive(t *testing.T, edit func(map[string]any)) []byte {
	t.Helper()
	m := map[string]any{
		"format": 1, "id": "hello", "version": "1.0.0", "sdk": 1,
		"name": map[string]any{"en": "Hello", "ko": "안녕"}, "author": map[string]any{"name": "Example"}, "icon": "icon.svg",
		"views": map[string]any{
			"window": map[string]any{"entry": "window.html", "defaultSize": map[string]any{"width": 480, "height": 320}},
			"widget": map[string]any{"entry": "widget.html", "defaultSize": map[string]any{"cols": 2, "rows": 2}},
		},
		"rpc": map[string]any{"read": []string{"System.getInformation"}},
	}
	if edit != nil {
		edit(m)
	}
	manifest, _ := json.Marshal(m)
	files := []struct{ name, body string }{
		{"manifest.json", string(manifest)},
		{"icon.svg", `<svg xmlns="http://www.w3.org/2000/svg"/>`},
		{"i18n/en.json", `{"title":"Hello","bye":"Bye"}`},
		{"i18n/ko.json", `{"title":"안녕"}`},
		{"ui/window.html", `<!doctype html><script src="./_maru/sdk.js"></script>`},
		{"ui/widget.html", `<!doctype html>`},
		{"ui/assets/app.js", `console.log(1)`},
		{"ui/assets/data.bin", `x`},
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(f.body))}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(f.body))
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func packagesEnv(t *testing.T) *env {
	t.Helper()
	return newEnv(t, envOptions{
		packages: true,
		addonSDK: fstest.MapFS{"sdk.js": {Data: []byte("globalThis.maru={}")}, "maru.css": {Data: []byte(":root{}")}},
	})
}

// adminWithManage는 관리 창을 연 관리자 브라우저다.
func (e *env) adminWithManage() *browser {
	e.t.Helper()
	b := e.browser()
	wantStatus(e.t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(e.t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "admin"}), http.StatusOK)
	return b
}

// installAddon은 b(관리 창을 연 관리자)로 패키지를 올리고 설치한다.
func (b *browser) installAddon(data []byte) api.AddonPackageStatus {
	b.e.t.Helper()
	rec := b.raw(http.MethodPost, "/api/site/addons/uploads", "application/octet-stream", data)
	wantStatus(b.e.t, rec, http.StatusOK)
	p := decode[api.AddonPendingInstall](b.e.t, rec)
	rec = b.do(http.MethodPost, "/api/site/addons/uploads/"+p.PendingID+"/install", struct{}{})
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.AddonPackageStatus](b.e.t, rec)
}

func (b *browser) capabilities() api.CapabilitiesResponse {
	b.e.t.Helper()
	rec := b.do(http.MethodGet, "/api/capabilities", nil)
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.CapabilitiesResponse](b.e.t, rec)
}

func packageView(t *testing.T, caps api.CapabilitiesResponse, id string) api.AddonPackageView {
	t.Helper()
	for _, v := range caps.AddonPackages {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("package %s not in capabilities: %+v", id, caps.AddonPackages)
	return api.AddonPackageView{}
}

func TestAddonPackageInstallFlow(t *testing.T) {
	e := packagesEnv(t)
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)

	// 관리 창 없이 설치하면 대기를 남긴 채 403이다.
	rec := admin.raw(http.MethodPost, "/api/site/addons/uploads", "application/octet-stream", addonArchive(t, nil))
	wantStatus(t, rec, http.StatusOK)
	pending := decode[api.AddonPendingInstall](t, rec)
	if pending.Package.ID != "hello" || pending.Package.Permissions == nil || len(pending.Package.Permissions.Read) != 1 || pending.Installed != nil {
		t.Fatalf("pending = %+v", pending)
	}
	wantCode(t, admin.do(http.MethodPost, "/api/site/addons/uploads/"+pending.PendingID+"/install", struct{}{}), http.StatusForbidden, api.ErrManageRequired)
	wantStatus(t, admin.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "admin"}), http.StatusOK)
	rec = admin.do(http.MethodPost, "/api/site/addons/uploads/"+pending.PendingID+"/install", struct{}{})
	wantStatus(t, rec, http.StatusOK)
	if st := decode[api.AddonPackageStatus](t, rec); st.State != "active" || st.InstalledBy != "admin" || st.SHA256 == "" {
		t.Fatalf("installed = %+v", st)
	}
	// 대기는 한 번만 쓴다.
	wantCode(t, admin.do(http.MethodPost, "/api/site/addons/uploads/"+pending.PendingID+"/install", struct{}{}), http.StatusNotFound, api.ErrUploadExpired)

	caps := admin.capabilities()
	v := packageView(t, caps, "hello")
	if !strings.HasPrefix(v.UIBase, "api/addon-ui/") || !strings.HasSuffix(v.UIBase, "/"+v.Rev+"/") || v.Views.Window == nil || v.Views.Widget == nil || v.Name["ko"] != "안녕" {
		t.Errorf("view = %+v", v)
	}
	found := false
	for _, c := range caps.Addons {
		if c.ID == "hello" && c.Available {
			found = true
		}
	}
	if !found {
		t.Errorf("capability missing: %+v", caps.Addons)
	}
	// 같은 세션에서 다시 받아도 프레임 주소는 같다.
	if again := packageView(t, admin.capabilities(), "hello"); again.UIBase != v.UIBase {
		t.Errorf("uiBase changed: %s -> %s", v.UIBase, again.UIBase)
	}

	list := decode[api.AddonPackagesResponse](t, admin.do(http.MethodGet, "/api/site/addons", nil))
	if !list.CanInstall || len(list.Packages) != 1 || list.Packages[0].State != "active" {
		t.Errorf("list = %+v", list)
	}

	// 일반 사용자에게는 관리 API가 없다.
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	wantStatus(t, user.raw(http.MethodPost, "/api/site/addons/uploads", "application/octet-stream", addonArchive(t, nil)), http.StatusForbidden)
	packageView(t, user.capabilities(), "hello")

	// 위젯 배치를 저장하고, 끄면 capabilities에서 빠지고 배치는 그대로 저장된다.
	wantStatus(t, admin.do(http.MethodPut, "/api/preferences/widgets", api.WidgetsRequest{Widgets: []api.WidgetPlacement{{Addon: "hello", Cols: 2, Rows: 2}}}), http.StatusNoContent)
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: false}), http.StatusOK)
	for _, p := range admin.capabilities().AddonPackages {
		if p.ID == "hello" {
			t.Error("disabled package still in capabilities")
		}
	}
	wantStatus(t, admin.do(http.MethodPut, "/api/preferences/widgets", api.WidgetsRequest{Widgets: []api.WidgetPlacement{{Addon: "hello", Cols: 2, Rows: 2}}}), http.StatusNoContent)

	// 켜기도 관리 창이 필요하다(열려 있으면 바로 된다). 끄기는 확인하지 않는다.
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: true}), http.StatusOK)
	other := e.browser()
	wantStatus(t, other.login("admin", "admin"), http.StatusOK)
	wantStatus(t, other.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: false}), http.StatusOK)
	wantCode(t, other.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: true}), http.StatusForbidden, api.ErrManageRequired)

	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello", nil), http.StatusNoContent)
	wantCode(t, admin.do(http.MethodDelete, "/api/site/addons/hello", nil), http.StatusNotFound, api.ErrNotFound)
	if !strings.Contains(e.logs.String(), `action=install id=hello version="1.0.0"`) {
		t.Errorf("install not logged: %s", e.logs.String())
	}
}

func TestAddonPackageUploadRejects(t *testing.T) {
	e := packagesEnv(t)
	admin := e.adminWithManage()
	wantStatus(t, admin.raw(http.MethodPost, "/api/site/addons/uploads", "application/json", []byte("{}")), http.StatusUnsupportedMediaType)
	got := wantCode(t, admin.raw(http.MethodPost, "/api/site/addons/uploads", "application/octet-stream", []byte("not gzip")), http.StatusBadRequest, api.ErrAddonInvalid)
	if got.Reason != addonpkg.ReasonArchive {
		t.Errorf("reason = %q", got.Reason)
	}
	got = wantCode(t, admin.raw(http.MethodPost, "/api/site/addons/uploads", "application/octet-stream",
		addonArchive(t, func(m map[string]any) { m["rpc"] = map[string]any{"read": []string{"Exec.getOutput"}} })), http.StatusConflict, api.ErrAddonInvalid)
	if got.Reason != "rpc_denied" || got.Detail == "" {
		t.Errorf("rejection = %+v", got)
	}
	got = wantCode(t, admin.raw(http.MethodPost, "/api/site/addons/uploads", "application/octet-stream",
		addonArchive(t, func(m map[string]any) { m["id"] = "settings" })), http.StatusConflict, api.ErrAddonInvalid)
	if got.Reason != "reserved_id" {
		t.Errorf("reserved: %+v", got)
	}

	// 상태 디렉터리가 없으면 설치할 수 없다.
	mem := newEnv(t, envOptions{devAddons: []string{t.TempDir()}})
	b := mem.adminWithManage()
	wantCode(t, b.raw(http.MethodPost, "/api/site/addons/uploads", "application/octet-stream", addonArchive(t, nil)), http.StatusConflict, api.ErrAddonsUnavailable)
}

func TestAddonFrameFiles(t *testing.T) {
	e := packagesEnv(t)
	admin := e.adminWithManage()
	admin.installAddon(addonArchive(t, nil))
	v := packageView(t, admin.capabilities(), "hello")
	base := "/" + v.UIBase

	// 프레임 파일은 쿠키 없이 토큰 경로로 받는다.
	anon := e.browser()
	rec := anon.do(http.MethodGet, base+"window.html", nil)
	wantStatus(t, rec, http.StatusOK)
	hd := rec.Header()
	if hd.Get("Content-Security-Policy") != addonFrameCSP || hd.Get("X-Content-Type-Options") != "nosniff" ||
		hd.Get("Access-Control-Allow-Origin") != "*" || hd.Get("Referrer-Policy") != "no-referrer" ||
		hd.Get("Content-Type") != "text/html; charset=utf-8" || hd.Get("Cache-Control") != frameImmutableCache {
		t.Errorf("headers = %v", hd)
	}
	if !strings.HasPrefix(addonFrameCSP, "sandbox allow-scripts;") || !strings.Contains(addonFrameCSP, "connect-src 'none'") {
		t.Errorf("CSP = %q", addonFrameCSP)
	}
	wantStatus(t, anon.do(http.MethodGet, base+"assets/app.js", nil), http.StatusOK)
	rec = anon.do(http.MethodGet, base+"_maru/sdk.js", nil)
	wantStatus(t, rec, http.StatusOK)
	if rec.Body.String() != "globalThis.maru={}" || rec.Header().Get("Cache-Control") != frameDevCache {
		t.Errorf("sdk = %q %v", rec.Body.String(), rec.Header())
	}

	bad := map[string]string{
		"unknown ext":   base + "assets/data.bin",
		"missing":       base + "nope.html",
		"hidden":        base + ".env",
		"dot dot":       base + "assets/../window.html",
		"reserved file": base + "_maru/other.js",
		"nested maru":   base + "assets/_maru/sdk.js",
		"rev":           strings.Replace(base, v.Rev, "9.9.9-00000000", 1) + "window.html",
		"token":         "/api/addon-ui/" + strings.Repeat("0", 64) + "/" + v.Rev + "/window.html",
		"manifest":      base + "../manifest.json",
	}
	for name, path := range bad {
		rec := anon.do(http.MethodGet, path, nil)
		// ServeMux는 ..이 든 경로를 정리한 주소로 돌려보낸다(그 주소도 404다).
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusTemporaryRedirect {
			t.Errorf("%s: status %d", name, rec.Code)
		}
		if rec.Code == http.StatusNotFound && rec.Header().Get("Content-Security-Policy") == "" && strings.HasPrefix(path, "/api/addon-ui/") {
			t.Errorf("%s: no CSP on 404", name)
		}
	}

	// 다른 사용자의 토큰은 다르고, 로그아웃하면 그 토큰은 쓸 수 없다.
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	uv := packageView(t, user.capabilities(), "hello")
	if uv.UIBase == v.UIBase {
		t.Error("two sessions share a frame token")
	}
	wantStatus(t, anon.do(http.MethodGet, "/"+uv.UIBase+"window.html", nil), http.StatusOK)
	wantStatus(t, user.do(http.MethodPost, "/api/logout", struct{}{}), http.StatusNoContent)
	wantStatus(t, anon.do(http.MethodGet, "/"+uv.UIBase+"window.html", nil), http.StatusNotFound)

	// 끄면 토큰이 있어도 404다.
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: false}), http.StatusOK)
	wantStatus(t, anon.do(http.MethodGet, base+"window.html", nil), http.StatusNotFound)
}

func TestAddonFrameAdminOnly(t *testing.T) {
	e := packagesEnv(t)
	admin := e.adminWithManage()
	admin.installAddon(addonArchive(t, func(m map[string]any) { m["requires"] = map[string]any{"admin": true} }))
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	for _, p := range user.capabilities().AddonPackages {
		if p.ID == "hello" {
			t.Error("admin-only package shown to a user")
		}
	}
	wantStatus(t, user.do(http.MethodGet, "/api/addons/hello/icon", nil), http.StatusNotFound)
	wantStatus(t, user.do(http.MethodGet, "/api/addons/hello/i18n/en", nil), http.StatusNotFound)
}

func TestAddonIconAndMessages(t *testing.T) {
	e := packagesEnv(t)
	admin := e.adminWithManage()
	admin.installAddon(addonArchive(t, nil))
	rec := admin.do(http.MethodGet, "/api/addons/hello/icon", nil)
	wantStatus(t, rec, http.StatusOK)
	if rec.Header().Get("Content-Type") != "image/svg+xml" || rec.Header().Get("Content-Security-Policy") != addonIconCSP {
		t.Errorf("icon headers = %v", rec.Header())
	}
	msgs := decode[map[string]string](t, admin.do(http.MethodGet, "/api/addons/hello/i18n/ko", nil))
	if msgs["title"] != "안녕" || msgs["bye"] != "Bye" {
		t.Errorf("ko messages = %v", msgs)
	}
	wantStatus(t, admin.do(http.MethodGet, "/api/addons/hello/i18n/xx", nil), http.StatusNotFound)
	// 끈 패키지의 아이콘도 관리자는 받는다(관리자 페이지).
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: false}), http.StatusOK)
	wantStatus(t, admin.do(http.MethodGet, "/api/addons/hello/icon", nil), http.StatusOK)
	wantStatus(t, admin.do(http.MethodGet, "/api/addons/hello/i18n/en", nil), http.StatusNotFound)
}

func TestAddonPackageRPC(t *testing.T) {
	e := packagesEnv(t)
	admin := e.adminWithManage()
	admin.installAddon(addonArchive(t, nil))

	// 범용 중계는 패키지 메서드를 통과시키지 않는다.
	wantCode(t, admin.do(http.MethodPost, "/api/rpc", api.RPCRequest{Service: "System", Method: "getInformation"}), http.StatusForbidden, api.ErrRPCForbidden)

	rec := admin.do(http.MethodPost, "/api/addons/hello/rpc", api.RPCRequest{Service: "System", Method: "getInformation"})
	wantStatus(t, rec, http.StatusOK)
	if info := decode[map[string]any](t, rec); info["hostname"] != "fake-nas.local" || info["version"] == nil {
		t.Errorf("admin info = %v", info)
	}
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	if info := decode[map[string]any](t, user.do(http.MethodPost, "/api/addons/hello/rpc", api.RPCRequest{Service: "System", Method: "getInformation"})); info["version"] != nil {
		t.Errorf("user info = %v", info)
	}
	// 선언하지 않은 메서드, 금지 표, 내장 애드온 id는 거부한다.
	wantCode(t, admin.do(http.MethodPost, "/api/addons/hello/rpc", api.RPCRequest{Service: "CpuTemp", Method: "get"}), http.StatusForbidden, api.ErrRPCForbidden)
	wantCode(t, admin.do(http.MethodPost, "/api/addons/hello/rpc", api.RPCRequest{Service: "Exec", Method: "getOutput"}), http.StatusForbidden, api.ErrRPCForbidden)
	wantStatus(t, admin.do(http.MethodPost, "/api/addons/nope/rpc", api.RPCRequest{Service: "System", Method: "getInformation"}), http.StatusNotFound)
	// 패키지 데이터·조작은 백엔드가 없으면 404다.
	wantCode(t, admin.do(http.MethodGet, "/api/addons/hello", nil), http.StatusNotFound, api.ErrAddonNoBackend)
}

func TestAddonPackageStorage(t *testing.T) {
	e := packagesEnv(t)
	admin := e.adminWithManage()
	admin.installAddon(addonArchive(t, nil))
	get := func() json.RawMessage {
		rec := admin.do(http.MethodGet, "/api/preferences/addons/hello", nil)
		wantStatus(t, rec, http.StatusOK)
		return decode[struct {
			Data json.RawMessage `json:"data"`
		}](t, rec).Data
	}
	if d := get(); string(d) != "null" {
		t.Errorf("initial data = %s", d)
	}
	wantStatus(t, admin.do(http.MethodPut, "/api/preferences/addons/hello", map[string]any{"data": map[string]any{"count": 3}}), http.StatusNoContent)
	if d := get(); string(d) != `{"count":3}` {
		t.Errorf("data = %s", d)
	}
	wantStatus(t, admin.do(http.MethodPut, "/api/preferences/addons/hello", map[string]any{"data": strings.Repeat("x", prefs.MaxAddonData)}), http.StatusBadRequest)
	wantStatus(t, admin.do(http.MethodPut, "/api/preferences/addons/hello", map[string]any{}), http.StatusBadRequest)

	// 지웠다 다시 설치한 패키지는 이전 값을 읽지 못한다.
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello", nil), http.StatusNoContent)
	wantStatus(t, admin.do(http.MethodGet, "/api/preferences/addons/hello", nil), http.StatusNotFound)
	admin.installAddon(addonArchive(t, nil))
	if d := get(); string(d) != "null" {
		t.Errorf("reinstalled data = %s", d)
	}
}

// TestAddonSDKExamples는 SDK 예제(sdk/examples)를 개발 패키지로 불러, 보이는 사용자에게 capabilities에 들어가고
// 보기 진입 파일과 선언한 읽기 RPC가 동작하는지 본다(FAKEADDONS=examples와 같은 경로).
func TestAddonSDKExamples(t *testing.T) {
	manifests, err := filepath.Glob("../../../sdk/examples/*/manifest.json")
	if err != nil || len(manifests) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	var dirs []string
	for _, m := range manifests {
		abs, err := filepath.Abs(filepath.Dir(m))
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, abs)
	}
	// make dev-fake처럼 백엔드를 켠다(백엔드 예제는 백엔드가 꺼져 있으면 쓸 수 없다). 백엔드 프로세스는 띄우지 않는다.
	paths := backendPaths(shortSocketDir(t))
	e := newEnv(t, envOptions{
		devAddons: dirs,
		addonSDK:  fstest.MapFS{"sdk.js": {Data: []byte("globalThis.maru={}")}, "maru.css": {Data: []byte(":root{}")}},
		addonHost: &paths,
	})
	for _, who := range []string{"admin", "user"} {
		b := e.browser()
		wantStatus(t, b.login(who, who), http.StatusOK)
		caps := b.capabilities()
		for _, dir := range dirs {
			raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var mf struct {
				ID       string
				Requires struct{ Admin bool }
				RPC      struct{ Read []string }
				Views    map[string]struct{ Entry string }
			}
			if err := json.Unmarshal(raw, &mf); err != nil {
				t.Fatal(err)
			}
			if mf.Requires.Admin && who != "admin" {
				continue
			}
			v := packageView(t, caps, mf.ID)
			if !v.Dev {
				t.Errorf("%s: not a development package", mf.ID)
			}
			for name, view := range mf.Views {
				rec := b.do(http.MethodGet, "/"+v.UIBase+view.Entry, nil)
				if rec.Code != http.StatusOK {
					t.Errorf("%s %s view (%s): status %d", mf.ID, name, view.Entry, rec.Code)
				}
			}
			for _, m := range mf.RPC.Read {
				service, method, _ := strings.Cut(m, ".")
				rec := b.do(http.MethodPost, "/api/addons/"+mf.ID+"/rpc", api.RPCRequest{Service: service, Method: method})
				if rec.Code != http.StatusOK {
					t.Errorf("%s as %s: %s: status %d", mf.ID, who, m, rec.Code)
				}
			}
		}
	}
}
