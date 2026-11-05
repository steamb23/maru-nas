package server

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/omv"
	"maru/backend/internal/omv/omvfake"
)

// 패키지 백엔드 프록시(addon_backend.go)와 호스트 소켓(addon_host.go) 시험. 가짜 백엔드는 짧은 임시 디렉터리의 유닉스
// 소켓에서 받는다(t.TempDir는 테스트 이름이 들어가 소켓 경로 한도 108바이트를 넘을 수 있다).

// shortSocketDir는 소켓을 둘 짧은 임시 디렉터리다.
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mab")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// probeManifest는 시험용 백엔드 패키지 manifest다. Fake.startJob은 시험 OMV 감싸개(bgOMV)가 백그라운드 작업을 만든다.
func probeManifest(id string) map[string]any {
	return map[string]any{
		"format": 1, "id": id, "version": "1.0.0", "sdk": 1,
		"name": map[string]any{"en": "Probe"}, "author": map[string]any{"name": "Example"}, "icon": "icon.svg",
		"views": map[string]any{"window": map[string]any{"entry": "window.html", "defaultSize": map[string]any{"width": 400, "height": 300}}},
		"rpc": map[string]any{
			"read":  []string{"System.getInformation", "Rsyncd.getSettings", "Fake.startJob"},
			"write": []string{"Rsyncd.setSettings"},
		},
		"actions": []any{
			map[string]any{"name": "run"},
			map[string]any{"name": "admin-only", "admin": true},
			map[string]any{"name": "slow", "timeoutSeconds": 1},
		},
		"backend": map[string]any{"network": []string{}},
	}
}

// devPackage는 manifest m으로 풀린 개발 패키지 디렉터리를 만든다.
func devPackage(t *testing.T, m map[string]any) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), m["id"].(string))
	manifest, _ := json.Marshal(m)
	for name, body := range map[string]string{
		"manifest.json":  string(manifest),
		"icon.svg":       `<svg xmlns="http://www.w3.org/2000/svg"/>`,
		"i18n/en.json":   `{}`,
		"ui/window.html": `<!doctype html>`,
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// hostReply는 가짜 백엔드가 호스트 API를 부른 결과다. 백엔드는 이것을 200 JSON으로 그대로 돌려준다(relay).
type hostReply struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

func (r hostReply) code() string {
	var e struct{ Code string }
	json.Unmarshal(r.Body, &e)
	return e.Code
}

// hostClient는 가짜 백엔드가 호스트 소켓을 부르는 클라이언트다.
type hostClient struct{ path, token string }

func (c hostClient) post(route string, body any) hostReply {
	return c.request(http.MethodPost, route, body)
}

func (c hostClient) request(method, route string, body any) hostReply {
	hc := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", c.path)
	}}}
	defer hc.CloseIdleConnections()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, "http://maru"+route, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set(hostCallHeader, c.token)
	}
	res, err := hc.Do(req)
	if err != nil {
		return hostReply{Status: -1, Body: json.RawMessage(strconv.Quote(err.Error()))}
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if !json.Valid(raw) {
		raw = json.RawMessage(strconv.Quote(string(raw)))
	}
	return hostReply{Status: res.StatusCode, Body: raw}
}

// backendFunc는 가짜 백엔드의 처리기다. host는 그 요청의 토큰으로 호스트 소켓을 부른다.
type backendFunc func(w http.ResponseWriter, r *http.Request, host hostClient)

// fakeAddonBackend는 유닉스 소켓에서 받는 가짜 패키지 백엔드다. 받은 요청(헤더, 본문)과 토큰을 남긴다.
type fakeAddonBackend struct {
	host string

	mu     sync.Mutex
	fn     backendFunc
	tokens []string
	last   *http.Request
	body   string
}

func (f *fakeAddonBackend) set(fn backendFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fn = fn
}

func (f *fakeAddonBackend) lastRequest() (*http.Request, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last, f.body
}

func (f *fakeAddonBackend) lastToken() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokens[len(f.tokens)-1]
}

func (f *fakeAddonBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	tok := r.Header.Get(hostCallHeader)
	f.mu.Lock()
	f.tokens = append(f.tokens, tok)
	f.last, f.body = r, string(body)
	fn := f.fn
	f.mu.Unlock()
	if fn == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	fn(w, r, hostClient{path: f.host, token: tok})
}

func writeBackendJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// relay는 호스트 API 하나를 부르고 그 결과를 200으로 돌려주는 처리기다.
func relay(route string, body any) backendFunc {
	return func(w http.ResponseWriter, r *http.Request, host hostClient) {
		writeBackendJSON(w, http.StatusOK, host.post(route, body))
	}
}

// bgOMV는 Fake.startJob(가짜 백그라운드 메서드)에 가짜 OMV의 상태 파일 이름을 돌려준다. 나머지는 그대로 넘긴다.
type bgOMV struct {
	OMV
	fake func() *omvfake.Server
}

func (b bgOMV) Call(ctx context.Context, r omv.Request) (omv.Response, error) {
	if r.Service == "Fake" && r.Method == "startJob" {
		raw, _ := json.Marshal(b.fake().StartBg(""))
		return omv.Response{Result: raw}, nil
	}
	return b.OMV.Call(ctx, r)
}

// backendEnv는 개발 백엔드 패키지 probe(가짜 백엔드가 받음)와 other(백엔드 없이 호스트 소켓만)를 둔 환경이다.
type backendEnv struct {
	*env
	paths   addonpkg.BackendPaths
	backend *fakeAddonBackend
}

func backendPaths(dir string) addonpkg.BackendPaths {
	return addonpkg.BackendPaths{
		Backend:  func(id string) string { return filepath.Join(dir, id+".b") },
		Host:     func(id string) string { return filepath.Join(dir, id+".h") },
		HostMode: 0o600,
	}
}

func newBackendEnv(t *testing.T, o envOptions) *backendEnv {
	t.Helper()
	paths := backendPaths(shortSocketDir(t))
	o.addonHost = &paths
	if len(o.devAddons) == 0 {
		o.devAddons = []string{devPackage(t, probeManifest("probe")), devPackage(t, probeManifest("other"))}
	}
	var fake *omvfake.Server
	o.wrapOMV = func(c OMV) OMV { return bgOMV{OMV: c, fake: func() *omvfake.Server { return fake }} }
	e := newEnv(t, o)
	fake = e.fake
	e.srv.h.hosts.bgInterval.Store(int64(time.Millisecond))

	fb := &fakeAddonBackend{host: paths.Host("probe")}
	ln, err := net.Listen("unix", paths.Backend("probe"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(fb)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return &backendEnv{env: e, paths: paths, backend: fb}
}

// stepped는 로그인하고 비밀번호를 다시 확인한 브라우저다(쓰기를 선언한 패키지의 조작에 필요하다).
func (e *backendEnv) stepped(user string) *browser {
	e.t.Helper()
	b := e.browser()
	wantStatus(e.t, b.login(user, user), http.StatusOK)
	wantStatus(e.t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: user}), http.StatusOK)
	return b
}

// relayed는 백엔드가 돌려준 호스트 결과다(브라우저 응답 200의 본문).
func relayed(t *testing.T, rec *httptest.ResponseRecorder) hostReply {
	t.Helper()
	wantStatus(t, rec, http.StatusOK)
	return decode[hostReply](t, rec)
}

func TestPackageBackendData(t *testing.T) {
	e := newBackendEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	e.backend.set(func(w http.ResponseWriter, r *http.Request, _ hostClient) {
		w.Header().Set("Set-Cookie", "maru_session=evil")
		w.Write([]byte(` {"big": 12345678901234567890} `))
	})
	b.header.Set("X-Maru-Lang", "ko")
	rec := b.do(http.MethodGet, "/api/addons/probe?a=1&b=%20x", nil)
	wantStatus(t, rec, http.StatusOK)
	// 응답은 다시 직렬화하지 않고(큰 정수), 백엔드의 헤더를 넘기지 않는다.
	if got := rec.Body.String(); got != `{"big": 12345678901234567890}` || rec.Header().Get("Set-Cookie") != "" ||
		rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("body %q headers %v", got, rec.Header())
	}
	req, _ := e.backend.lastRequest()
	if req.Method != http.MethodGet || req.URL.Path != "/v1/data" || req.URL.RawQuery != "a=1&b=%20x" || req.Host != "addon" ||
		req.Header.Get("X-Maru-User") != "admin" || req.Header.Get("X-Maru-Admin") != "1" || req.Header.Get("X-Maru-Lang") != "ko" ||
		len(req.Header.Get(hostCallHeader)) != 43 {
		t.Errorf("backend request %s %s?%s host=%s headers=%v", req.Method, req.URL.Path, req.URL.RawQuery, req.Host, req.Header)
	}
	if strings.Contains(e.logs.String(), req.Header.Get(hostCallHeader)) {
		t.Error("call token in the log")
	}
	// 지원하지 않는 언어는 en이다.
	b.header.Set("X-Maru-Lang", "xx")
	wantStatus(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusOK)
	if req, _ := e.backend.lastRequest(); req.Header.Get("X-Maru-Lang") != "en" {
		t.Errorf("lang = %q", req.Header.Get("X-Maru-Lang"))
	}
	b.header.Del("X-Maru-Lang")

	// 데이터의 204는 200 null이다.
	e.backend.set(func(w http.ResponseWriter, _ *http.Request, _ hostClient) { w.WriteHeader(http.StatusNoContent) })
	rec = b.do(http.MethodGet, "/api/addons/probe", nil)
	wantStatus(t, rec, http.StatusOK)
	if rec.Body.String() != "null" {
		t.Errorf("204 data = %q", rec.Body.String())
	}

	// 일반 사용자는 X-Maru-Admin: 0이다.
	u := e.browser()
	wantStatus(t, u.login("user", "user"), http.StatusOK)
	wantStatus(t, u.do(http.MethodGet, "/api/addons/probe", nil), http.StatusOK)
	if req, _ := e.backend.lastRequest(); req.Header.Get("X-Maru-User") != "user" || req.Header.Get("X-Maru-Admin") != "0" {
		t.Errorf("user headers %v", req.Header)
	}

	// 쿼리는 2KiB까지, 출력 가능한 ASCII만 넘긴다.
	wantCode(t, b.do(http.MethodGet, "/api/addons/probe?q="+strings.Repeat("a", 2048), nil), http.StatusBadRequest, api.ErrBadRequest)
}

func TestPackageBackendErrors(t *testing.T) {
	e := newBackendEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	reply := func(status int, body string) {
		e.backend.set(func(w http.ResponseWriter, _ *http.Request, _ hostClient) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			io.WriteString(w, body)
		})
	}

	// 백엔드의 401은 넘기지 않는다(가짜 401로 로그아웃시키지 못하게). 세션은 그대로다.
	reply(http.StatusUnauthorized, `{"code":"unauthorized","message":"log in again"}`)
	if got := wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusBadGateway, api.ErrAddonError); got.AddonCode != "" {
		t.Errorf("401 addonCode = %q", got.AddonCode)
	}
	wantStatus(t, b.do(http.MethodGet, "/api/session", nil), http.StatusOK)

	reply(http.StatusConflict, `{"code":"already_running","message":"it is\u0000 already\nrunning"}`)
	got := wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusConflict, api.ErrAddonError)
	if got.AddonCode != "already_running" || got.Error != "it is alreadyrunning" {
		t.Errorf("409 = %+v", got)
	}
	reply(http.StatusUnprocessableEntity, `{"code":"Not-Valid","message":"`+strings.Repeat("가", 400)+`"}`)
	got = wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusUnprocessableEntity, api.ErrAddonError)
	if got.AddonCode != "" || len([]rune(got.Error)) != 300 {
		t.Errorf("422 = code %q, %d chars", got.AddonCode, len([]rune(got.Error)))
	}
	// 넘기는 4xx라도 본문이 JSON 객체가 아니면 502다.
	reply(http.StatusNotFound, "404 page not found\n")
	if got := wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusBadGateway, api.ErrAddonError); got.AddonCode != "" {
		t.Errorf("text 404 addonCode = %q", got.AddonCode)
	}
	reply(http.StatusOK, `{"broken":`)
	wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusBadGateway, api.ErrAddonError)
	reply(http.StatusOK, `"`+strings.Repeat("x", addonpkg.MaxBackendResponse)+`"`)
	wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusBadGateway, api.ErrAddonError)
	reply(http.StatusInternalServerError, `{"code":"boom"}`)
	if got := wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusBadGateway, api.ErrAddonError); got.AddonCode != "" {
		t.Errorf("500 addonCode = %q", got.AddonCode)
	}

	// 백엔드 소켓이 없으면 503이다(other는 백엔드를 띄우지 않았다).
	wantCode(t, b.do(http.MethodGet, "/api/addons/other", nil), http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable)

	// 데이터 시간 제한을 넘으면 504 addon_backend_unavailable이다.
	e.srv.h.backendDataTimeout = 50 * time.Millisecond
	e.backend.set(func(w http.ResponseWriter, r *http.Request, _ hostClient) { <-r.Context().Done() })
	wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusGatewayTimeout, api.ErrAddonBackendUnavailable)
	e.srv.h.backendDataTimeout = addonpkg.BackendDataTimeout

	// 토큰 표가 가득 차면 503이다.
	e.srv.h.hostCalls.max = 0
	wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable)
	e.srv.h.hostCalls.max = maxHostCalls
}

func TestPackageBackendActions(t *testing.T) {
	e := newBackendEnv(t, envOptions{})
	admin := e.stepped("admin")

	e.backend.set(func(w http.ResponseWriter, r *http.Request, _ hostClient) {
		if r.URL.Path == "/v1/actions/admin-only" {
			writeBackendJSON(w, http.StatusOK, map[string]any{"done": true})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	rec := admin.do(http.MethodPost, "/api/addons/probe/actions/run", map[string]any{"target": "web", "input": map[string]any{"n": 1}})
	wantStatus(t, rec, http.StatusNoContent)
	req, body := e.backend.lastRequest()
	if req.Method != http.MethodPost || req.URL.Path != "/v1/actions/run" || req.Header.Get("Content-Type") != "application/json" ||
		body != `{"target":"web","input":{"n":1}}` {
		t.Errorf("backend action %s %s %q", req.Method, req.URL.Path, body)
	}
	if !strings.Contains(e.logs.String(), `action: user="admin" addon=probe action=run target="web" result=ok`) {
		t.Errorf("audit log missing: %s", e.logs.String())
	}
	rec = admin.do(http.MethodPost, "/api/addons/probe/actions/admin-only", map[string]any{"target": ""})
	wantStatus(t, rec, http.StatusOK)
	if rec.Body.String() != `{"done":true}` {
		t.Errorf("action result = %q", rec.Body.String())
	}
	if _, body := e.backend.lastRequest(); body != `{"target":"","input":null}` {
		t.Errorf("action without input = %q", body)
	}

	// 입력은 JSON 객체 또는 null이고 8KiB까지, 본문은 16KiB까지다.
	wantCode(t, admin.do(http.MethodPost, "/api/addons/probe/actions/run", map[string]any{"input": []int{1}}), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(t, admin.do(http.MethodPost, "/api/addons/probe/actions/run",
		map[string]any{"input": map[string]any{"x": strings.Repeat("a", addonpkg.MaxActionInput)}}), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(t, admin.do(http.MethodPost, "/api/addons/probe/actions/run",
		map[string]any{"target": strings.Repeat("a", maxPackageActionBody)}), http.StatusBadRequest, api.ErrBadRequest)
	wantStatus(t, admin.do(http.MethodPost, "/api/addons/probe/actions/nope", map[string]any{}), http.StatusNotFound)

	// 관리자 조작은 일반 사용자가 실행할 수 없다(단계 확인보다 먼저 본다).
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	wantCode(t, user.do(http.MethodPost, "/api/addons/probe/actions/admin-only", map[string]any{}), http.StatusForbidden, api.ErrAdminRequired)

	// 백엔드의 오류 코드는 addonCode로 준다.
	e.backend.set(func(w http.ResponseWriter, _ *http.Request, _ hostClient) {
		writeBackendJSON(w, http.StatusConflict, map[string]any{"code": "already_on", "message": "already on"})
	})
	if got := wantCode(t, admin.do(http.MethodPost, "/api/addons/probe/actions/run", map[string]any{"target": "web"}),
		http.StatusConflict, api.ErrAddonError); got.AddonCode != "already_on" {
		t.Errorf("addonCode = %q", got.AddonCode)
	}
	if !strings.Contains(e.logs.String(), `target="web" result=addon_error`) {
		t.Errorf("audit log for failure missing: %s", e.logs.String())
	}

	// manifest의 시간 제한(1초)을 넘으면 504 action_timeout이다.
	e.backend.set(func(w http.ResponseWriter, r *http.Request, _ hostClient) { <-r.Context().Done() })
	wantCode(t, admin.do(http.MethodPost, "/api/addons/probe/actions/slow", map[string]any{}), http.StatusGatewayTimeout, api.ErrActionTimeout)
}

func TestPackageBackendActionGuards(t *testing.T) {
	e := newBackendEnv(t, envOptions{})

	// 쓰기를 선언한 패키지의 조작은 정책이 꺼져 있어도 비밀번호 재확인이 필요하다.
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	got := wantCode(t, admin.do(http.MethodPost, "/api/addons/probe/actions/run", map[string]any{}), http.StatusForbidden, api.ErrSecurityRequired)
	if got.Required != api.SecurityPassword {
		t.Errorf("required = %q", got.Required)
	}
	wantStatus(t, admin.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "admin"}), http.StatusOK)
	wantStatus(t, admin.do(http.MethodPost, "/api/addons/probe/actions/run", map[string]any{}), http.StatusNoContent)

	// 같은 (패키지, 대상)의 조작이 진행 중이면 409다. 다른 대상은 된다.
	entered, release := make(chan struct{}), make(chan struct{})
	e.backend.set(func(w http.ResponseWriter, r *http.Request, _ hostClient) {
		var body struct{ Target string }
		_, raw := e.backend.lastRequest()
		json.Unmarshal([]byte(raw), &body)
		if body.Target == "hold" {
			entered <- struct{}{}
			<-release
		}
		w.WriteHeader(http.StatusNoContent)
	})
	other := e.browser()
	for k, v := range admin.cookies {
		other.cookies[k] = v
	}
	done := make(chan int)
	go func() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/addons/probe/actions/run", strings.NewReader(`{"target":"hold"}`))
		req.Header.Set("Content-Type", "application/json")
		for _, ck := range other.cookies {
			req.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
		}
		e.h.ServeHTTP(rec, req)
		done <- rec.Code
	}()
	<-entered
	wantCode(t, admin.do(http.MethodPost, "/api/addons/probe/actions/run", map[string]any{"target": "hold"}), http.StatusConflict, api.ErrActionBusy)
	// 진행 중인 조작이 있으면 끄거나 지울 수 없다(개발 패키지는 원래 바꿀 수 없으므로 addonBusy만 본다).
	if !e.srv.h.addonBusy("probe") {
		t.Error("addonBusy is false during a package action")
	}
	close(release)
	if code := <-done; code != http.StatusNoContent {
		t.Errorf("held action = %d", code)
	}
}

func TestPackageBackendsDisabled(t *testing.T) {
	e := newEnv(t, envOptions{devAddons: []string{devPackage(t, probeManifest("probe"))}})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	caps := b.capabilities()
	found := false
	for _, c := range caps.Addons {
		if c.ID == "probe" {
			found = true
			if c.Available || c.Reason != api.ReasonBackendUnavailable {
				t.Errorf("capability = %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("probe not in capabilities")
	}
	// 화면이 안내를 보일 수 있게 목록에는 남는다.
	v := packageView(t, caps, "probe")
	wantCode(t, b.do(http.MethodGet, "/api/addons/probe", nil), http.StatusServiceUnavailable, api.ErrAddonBackendDisabled)
	wantCode(t, b.do(http.MethodPost, "/api/addons/probe/actions/run", map[string]any{}), http.StatusServiceUnavailable, api.ErrAddonBackendDisabled)
	wantCode(t, b.do(http.MethodPost, "/api/addons/probe/rpc", api.RPCRequest{Service: "System", Method: "getInformation"}),
		http.StatusForbidden, api.ErrBackendUnavailable)
	wantStatus(t, b.do(http.MethodGet, "/"+v.UIBase+"window.html", nil), http.StatusNotFound)
	// 아이콘·번역은 그대로 준다(바로가기 아이콘이 깨지지 않게).
	wantStatus(t, b.do(http.MethodGet, "/api/addons/probe/icon", nil), http.StatusOK)
}

func TestHostSocketScopes(t *testing.T) {
	e := newBackendEnv(t, envOptions{})
	admin := e.stepped("admin")
	data := func(fn backendFunc) hostReply {
		t.Helper()
		e.backend.set(fn)
		return relayed(t, admin.do(http.MethodGet, "/api/addons/probe", nil))
	}
	action := func(fn backendFunc) hostReply {
		t.Helper()
		e.backend.set(fn)
		return relayed(t, admin.do(http.MethodPost, "/api/addons/probe/actions/run", map[string]any{}))
	}
	rsync := map[string]any{"enable": true, "port": 873, "extraoptions": ""}

	// 읽기 토큰은 선언한 읽기만 부른다.
	r := data(relay("/v1/rpc", map[string]any{"service": "System", "method": "getInformation"}))
	if r.Status != http.StatusOK || !strings.Contains(string(r.Body), `"hostname":"fake-nas.local"`) {
		t.Fatalf("read = %+v", r)
	}
	if r = data(relay("/v1/rpc", map[string]any{"service": "Rsyncd", "method": "setSettings", "params": rsync})); r.Status != 403 || r.code() != hostErrForbidden {
		t.Errorf("write with a read token = %+v", r)
	}
	if r = data(relay("/v1/apply", map[string]any{"calls": []any{map[string]any{"service": "Rsyncd", "method": "setSettings", "params": rsync}}})); r.Status != 403 || r.code() != hostErrForbidden {
		t.Errorf("apply with a read token = %+v", r)
	}
	// 선언하지 않은 메서드, Config·Exec는 /v1/rpc로 부를 수 없다.
	for _, m := range [][2]string{{"Config", "applyChangesBg"}, {"Config", "isDirty"}, {"Exec", "isRunning"}, {"CpuTemp", "get"}, {"system", "getInformation"}} {
		if r = action(relay("/v1/rpc", map[string]any{"service": m[0], "method": m[1]})); r.Status != 403 || r.code() != hostErrForbidden {
			t.Errorf("%s.%s = %+v", m[0], m[1], r)
		}
	}

	// 쓰기 토큰은 선언한 쓰기를 부른다. 적용하지 않은 변경이 남는다.
	if r = action(relay("/v1/rpc", map[string]any{"service": "Rsyncd", "method": "setSettings", "params": rsync})); r.Status != 200 {
		t.Fatalf("write = %+v", r)
	}
	// 적용 대기가 있으면 apply는 409 config_dirty다(남이 만든 변경을 대신 배포하지 않는다).
	apply := map[string]any{"calls": []any{map[string]any{"service": "Rsyncd", "method": "setSettings", "params": rsync}}}
	if r = action(relay("/v1/apply", apply)); r.Status != 409 || r.code() != "config_dirty" {
		t.Errorf("apply while dirty = %+v", r)
	}
	// 적용 대기를 비우면 apply가 바꾸고 적용까지 한다.
	d := omvfake.DefaultPluginData()
	d.Dirty = nil
	e.fake.SetPluginData(d)
	r = action(relay("/v1/apply", apply))
	var res struct{ Results []map[string]any }
	if json.Unmarshal(r.Body, &res); r.Status != 200 || len(res.Results) != 1 || res.Results[0]["enable"] != true {
		t.Fatalf("apply = %+v", r)
	}
	var dirty bool
	callJSON(t, e, "admin", "Config", "isDirty", &dirty)
	if dirty {
		t.Error("apply left pending changes")
	}
	// apply의 호출은 모두 선언한 쓰기여야 한다.
	bad := map[string]any{"calls": []any{map[string]any{"service": "Rsyncd", "method": "getSettings"}}}
	if r = action(relay("/v1/apply", bad)); r.Status != 403 {
		t.Errorf("apply with a read method = %+v", r)
	}
	if r = action(relay("/v1/apply", map[string]any{"calls": []any{}})); r.Status != 400 {
		t.Errorf("empty apply = %+v", r)
	}
	if r = action(relay("/v1/rpc", map[string]any{"service": "System", "method": "getInformation", "extra": 1})); r.Status != 400 || r.code() != hostErrBadRequest {
		t.Errorf("unknown field = %+v", r)
	}
}

// callJSON은 user 세션으로 OMV를 바로 불러 결과를 out에 읽는다(가짜 OMV의 상태 확인).
func callJSON(t *testing.T, e *backendEnv, user, service, method string, out any) {
	t.Helper()
	client := omv.NewClient(e.omvTS.URL, "maru-test/1")
	res, err := client.Login(context.Background(), user, user, "")
	if err != nil {
		t.Fatal(err)
	}
	r, err := client.Call(context.Background(), omv.Request{SessionID: res.SessionID, Service: service, Method: method})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(r.Result, out); err != nil {
		t.Fatal(err)
	}
}

func TestHostSocketBackground(t *testing.T) {
	e := newBackendEnv(t, envOptions{})
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	data := func(b *browser, fn backendFunc) hostReply {
		t.Helper()
		e.backend.set(fn)
		return relayed(t, b.do(http.MethodGet, "/api/addons/probe", nil))
	}

	r := data(admin, relay("/v1/rpc-bg", map[string]any{"service": "Fake", "method": "startJob"}))
	if r.Status != 200 || string(r.Body) != `{"output":"","truncated":false}` {
		t.Fatalf("rpc-bg = %+v %s", r, r.Body)
	}
	if r = data(admin, relay("/v1/rpc-bg", map[string]any{"service": "Fake", "method": "startJob", "output": true})); r.Status != 200 {
		t.Fatalf("rpc-bg output = %+v", r)
	}
	if n := e.fake.BgJobs(); n != 0 {
		t.Errorf("%d background jobs left", n)
	}
	// 결과가 상태 파일 이름이 아니면 502 omv_error다(그 이름에만 Exec를 쓴다).
	if r = data(admin, relay("/v1/rpc-bg", map[string]any{"service": "System", "method": "getInformation"})); r.Status != 502 || r.code() != hostErrOMV {
		t.Errorf("rpc-bg without a status file = %+v", r)
	}
	// 관리자 세션만 쓴다.
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	if r = data(user, relay("/v1/rpc-bg", map[string]any{"service": "Fake", "method": "startJob"})); r.Status != 403 || r.code() != hostErrAdminRequired {
		t.Errorf("rpc-bg as a user = %+v", r)
	}

	// 보조 호출기는 받은 상태 파일 이름에만 Exec를 부른다.
	bg := &bgCaller{outer: &rpcCaller{h: e.srv.h}, filename: "/tmp/bgstatusabc"}
	for _, c := range []struct {
		method string
		params any
	}{
		{"isRunning", map[string]string{"filename": "/tmp/bgstatusother"}},
		{"getOutput", map[string]any{"filename": "/etc/passwd", "pos": 0}},
		{"stop", map[string]string{"filename": "/tmp/bgstatusabc"}},
	} {
		if _, err := bg.Call(context.Background(), "Exec", c.method, c.params); err == nil {
			t.Errorf("bgCaller allowed Exec.%s %v", c.method, c.params)
		}
	}
	ap := &applyCaller{outer: &rpcCaller{h: e.srv.h}}
	if _, err := ap.Call(context.Background(), "Exec", "isRunning", map[string]string{"filename": "/tmp/bgstatusabc"}); err == nil {
		t.Error("applyCaller allowed Exec.isRunning before applyChangesBg")
	}
	if _, err := ap.Call(context.Background(), "Compose", "doContainerCommand", nil); err == nil {
		t.Error("applyCaller allowed another method")
	}
}

func TestHostSocketTokens(t *testing.T) {
	e := newBackendEnv(t, envOptions{})
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	info := map[string]any{"service": "System", "method": "getInformation"}

	// 바깥 요청이 끝난 뒤에는 그 토큰을 쓸 수 없다.
	e.backend.set(relay("/v1/rpc", info))
	if r := relayed(t, admin.do(http.MethodGet, "/api/addons/probe", nil)); r.Status != 200 {
		t.Fatalf("rpc = %+v", r)
	}
	tok := e.backend.lastToken()
	host := hostClient{path: e.paths.Host("probe"), token: tok}
	if r := host.post("/v1/rpc", info); r.Status != 401 || r.code() != hostErrCallInvalid {
		t.Errorf("token after the request = %+v", r)
	}
	// 토큰이 없거나 모르는 값이면 401, 경로·메서드가 다르면 404·405다.
	if r := (hostClient{path: e.paths.Host("probe")}).post("/v1/rpc", info); r.Status != 401 || r.code() != hostErrCallInvalid {
		t.Errorf("no token = %+v", r)
	}
	if r := (hostClient{path: e.paths.Host("probe"), token: "forged"}).post("/v1/rpc", info); r.Status != 401 {
		t.Errorf("forged token = %+v", r)
	}
	if r := host.post("/v1/nope", info); r.Status != 404 {
		t.Errorf("unknown path = %+v", r)
	}
	if r := host.request(http.MethodGet, "/v1/rpc", nil); r.Status != 405 {
		t.Errorf("GET = %+v", r)
	}

	// 다른 애드온의 소켓에서는 이 애드온의 토큰을 쓸 수 없다.
	e.backend.set(func(w http.ResponseWriter, r *http.Request, h hostClient) {
		writeBackendJSON(w, http.StatusOK, hostClient{path: e.paths.Host("other"), token: h.token}.post("/v1/rpc", info))
	})
	if r := relayed(t, admin.do(http.MethodGet, "/api/addons/probe", nil)); r.Status != 401 || r.code() != hostErrCallInvalid {
		t.Errorf("token on another addon's socket = %+v", r)
	}

	// 동시 요청이 넘치면 429 busy다.
	call := &hostCall{addon: "probe", caller: &rpcCaller{h: e.srv.h}, ctx: context.Background()}
	tok, ok := e.srv.h.hostCalls.issue(call)
	if !ok {
		t.Fatal("issue failed")
	}
	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{}`))
	req.Header.Set(hostCallHeader, tok)
	req.Header.Set("Content-Type", "application/json")
	e.srv.h.hosts.handler("probe", sem).ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), `"busy"`) {
		t.Errorf("busy = %d %s", rec.Code, rec.Body.String())
	}
}

// rawHostRequest는 호스트 소켓에 원시 요청 req를 보내고 wait 안에 받은 응답과 연결을 돌려준다.
func rawHostRequest(t *testing.T, path, req string, wait time.Duration) (*http.Response, net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(wait))
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("no response within %v: %v", wait, err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res, conn, br
}

func TestHostSocketConnections(t *testing.T) {
	e := newBackendEnv(t, envOptions{})
	path := e.paths.Host("probe")

	// 본문을 보내지 않은 채 거절할 요청을 보내도 바로(본문 읽기 제한보다 짧게) 응답한다. 서버는 남은 본문을 본문 읽기
	// 제한까지만 기다리고 연결을 닫으므로 동시 연결 칸을 끝없이 쥐지 않는다.
	var wg sync.WaitGroup
	for name, req := range map[string]string{
		"no token":     "POST /v1/rpc HTTP/1.1\r\nHost: maru\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n",
		"unknown path": "POST /v1/nope HTTP/1.1\r\nHost: maru\r\nContent-Length: 100\r\n\r\n",
		"method":       "PUT /v1/rpc HTTP/1.1\r\nHost: maru\r\nContent-Length: 100\r\n\r\n",
	} {
		res, conn, br := rawHostRequest(t, path, req, 2*time.Second)
		if res.StatusCode < 400 || !res.Close {
			t.Errorf("%s: status %d, close %v", name, res.StatusCode, res.Close)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn.SetReadDeadline(time.Now().Add(hostBodyTimeout + 2*time.Second))
			if _, err := br.ReadByte(); !errors.Is(err, io.EOF) {
				t.Errorf("%s: connection not closed: %v", name, err)
			}
		}()
	}

	// 요청 줄과 헤더를 합쳐 8KiB까지 받고, 넘으면 431이다.
	sized := func(n int) string {
		head := "POST /v1/rpc HTTP/1.1\r\nHost: maru\r\nContent-Type: application/json\r\nX-Pad: "
		return head + strings.Repeat("a", n-len(head)-len("\r\n\r\n")) + "\r\n\r\n"
	}
	if res, _, _ := rawHostRequest(t, path, sized(8<<10-64), 2*time.Second); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("header under 8KiB: status %d", res.StatusCode)
	}
	if res, _, _ := rawHostRequest(t, path, sized(8<<10+64), 2*time.Second); res.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
		t.Errorf("header over 8KiB: status %d", res.StatusCode)
	}
	wg.Wait()
}

func TestHostSocketOMVExpiry(t *testing.T) {
	e := newBackendEnv(t, envOptions{})
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	// 백엔드가 부른 OMV가 401을 주면 백엔드가 무엇을 답하든 바깥 요청이 대시보드 세션을 지우고 401로 끝난다.
	replies := make(chan hostReply, 2)
	e.backend.set(func(w http.ResponseWriter, r *http.Request, h hostClient) {
		replies <- h.post("/v1/rpc", map[string]any{"service": "System", "method": "getInformation"})
		replies <- h.post("/v1/rpc", map[string]any{"service": "System", "method": "getInformation"})
		writeBackendJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	e.fake.Expire("admin")
	wantCode(t, admin.do(http.MethodGet, "/api/addons/probe", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	if first := <-replies; first.Status != 401 || first.code() != hostErrUnauthorized {
		t.Errorf("call when expired = %+v", first)
	}
	// 그 뒤의 호출은 OMV에 보내지 않고 바로 401이다.
	if second := <-replies; second.Status != 401 || second.code() != hostErrUnauthorized {
		t.Errorf("call after expiry = %+v", second)
	}
	wantCode(t, admin.do(http.MethodGet, "/api/session", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	if n := e.store.Len(); n != 0 {
		t.Errorf("%d sessions left", n)
	}
}

func TestHostCallsTable(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tc := newHostCalls()
	tc.now = func() time.Time { return now }
	tc.max = 2
	ctx, cancel := context.WithDeadline(context.Background(), now.Add(time.Second))
	defer cancel()
	a, ok1 := tc.issue(&hostCall{addon: "a", session: auth.Session{ID: "s1"}, ctx: ctx})
	b, ok2 := tc.issue(&hostCall{addon: "a", session: auth.Session{ID: "s2"}, ctx: ctx})
	if _, ok3 := tc.issue(&hostCall{addon: "a", ctx: ctx}); !ok1 || !ok2 || ok3 || a == b || len(a) != 43 {
		t.Fatalf("issue = %q %v %q %v %v", a, ok1, b, ok2, ok3)
	}
	if _, ok := tc.lookup(a); !ok {
		t.Error("lookup failed")
	}
	// 만료는 제한 시각 + 5초다.
	now = now.Add(5 * time.Second)
	if _, ok := tc.lookup(a); !ok {
		t.Error("expired too early")
	}
	now = now.Add(2 * time.Second)
	if _, ok := tc.lookup(a); ok {
		t.Error("expired token accepted")
	}
	// 만료된 항목은 다음 발급 때 정리된다.
	if _, ok := tc.issue(&hostCall{addon: "a", ctx: context.Background()}); !ok {
		t.Error("issue after expiry failed")
	}
	tc.removeSession("s2")
	tc.revoke(a)
	if _, ok := tc.lookup(b); ok {
		t.Error("token of a removed session accepted")
	}
	if _, ok := tc.lookup(""); ok {
		t.Error("empty token accepted")
	}
}

// fakeBackendELF는 압축 파일 검사를 통과하는 최소 amd64 ELF 머리다.
func fakeBackendELF() []byte {
	b := make([]byte, 64)
	copy(b, elf.ELFMAG)
	b[elf.EI_CLASS] = byte(elf.ELFCLASS64)
	b[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	b[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	le := binary.LittleEndian
	le.PutUint16(b[16:], uint16(elf.ET_EXEC))
	le.PutUint16(b[18:], uint16(elf.EM_X86_64))
	le.PutUint32(b[20:], uint32(elf.EV_CURRENT))
	le.PutUint16(b[52:], 64)
	le.PutUint16(b[54:], 56)
	le.PutUint16(b[58:], 64)
	return b
}

// backendArchive는 백엔드가 있는 hello 패키지 압축 파일이다.
func backendArchive(t *testing.T) []byte {
	t.Helper()
	plain := addonArchive(t, func(m map[string]any) {
		m["backend"] = map[string]any{"network": []string{}}
		m["actions"] = []any{map[string]any{"name": "run"}}
	})
	// addonArchive의 tar에 백엔드 실행 파일을 더해 다시 묶는다.
	zr, err := gzip.NewReader(bytes.NewReader(plain))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for {
		hd, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(tr)
		tw.WriteHeader(hd)
		tw.Write(body)
	}
	elfBody := fakeBackendELF()
	tw.WriteHeader(&tar.Header{Name: "backend/linux-amd64", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(elfBody))})
	tw.Write(elfBody)
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func TestHostSocketLifecycle(t *testing.T) {
	paths := backendPaths(shortSocketDir(t))
	e := newEnv(t, envOptions{packages: true, addonHost: &paths})
	admin := e.adminWithManage()
	exists := func() bool {
		_, err := os.Stat(paths.Host("hello"))
		_, open := e.srv.h.hosts.socketPath("hello")
		if (err == nil) != open {
			t.Errorf("socket file %v, open %v", err, open)
		}
		return err == nil
	}
	// 백엔드가 켜진 설치의 검토에는 backends_disabled 경고가 없다.
	rec := admin.raw(http.MethodPost, "/api/site/addons/uploads", "application/octet-stream", backendArchive(t))
	wantStatus(t, rec, http.StatusOK)
	pend := decode[api.AddonPendingInstall](t, rec)
	for _, w := range pend.Warnings {
		if w == addonpkg.WarnBackendsDisabled {
			t.Error("backends_disabled warning with backends enabled")
		}
	}
	wantStatus(t, admin.do(http.MethodPost, "/api/site/addons/uploads/"+pend.PendingID+"/install", struct{}{}), http.StatusOK)
	if !exists() {
		t.Fatal("host socket not opened after install")
	}
	if fi, err := os.Stat(paths.Host("hello")); err != nil || fi.Mode().Type() != fs.ModeSocket || fi.Mode().Perm() != 0o600 {
		t.Errorf("socket mode = %v %v", fi.Mode(), err)
	}
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: false}), http.StatusOK)
	if exists() {
		t.Error("host socket left open after disabling")
	}
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: true}), http.StatusOK)
	if !exists() {
		t.Error("host socket not reopened after enabling")
	}
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello", nil), http.StatusNoContent)
	if exists() {
		t.Error("host socket left open after uninstalling")
	}
	// 백엔드가 없는 패키지에는 호스트 소켓을 열지 않는다.
	admin.installAddon(addonArchive(t, nil))
	if exists() {
		t.Error("host socket for a package without a backend")
	}
	// 서버를 닫으면 소켓을 모두 닫는다.
	admin.installAddon(backendArchive(t))
	if !exists() {
		t.Fatal("host socket not opened after reinstall")
	}
	e.srv.Close()
	if _, err := os.Stat(paths.Host("hello")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("socket after Close: %v", err)
	}
}

func TestPackageBackendsDisabledWarning(t *testing.T) {
	e := packagesEnv(t)
	admin := e.adminWithManage()
	rec := admin.raw(http.MethodPost, "/api/site/addons/uploads", "application/octet-stream", backendArchive(t))
	wantStatus(t, rec, http.StatusOK)
	pend := decode[api.AddonPendingInstall](t, rec)
	found := false
	for _, w := range pend.Warnings {
		found = found || w == addonpkg.WarnBackendsDisabled
	}
	if !found {
		t.Errorf("warnings = %v", pend.Warnings)
	}
	// 꺼진 설치에도 설치는 된다.
	wantStatus(t, admin.do(http.MethodPost, "/api/site/addons/uploads/"+pend.PendingID+"/install", struct{}{}), http.StatusOK)
	wantCode(t, admin.do(http.MethodGet, "/api/addons/hello", nil), http.StatusServiceUnavailable, api.ErrAddonBackendDisabled)
}

func TestBuiltinActionRejectsInput(t *testing.T) {
	e := newActionEnv(t)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	for _, body := range []string{`{"target":"ok","input":null}`, `{"target":"ok","input":{}}`} {
		rec := b.raw(http.MethodPost, "/api/addons/jobs/actions/run", "application/json", []byte(body))
		wantCode(t, rec, http.StatusBadRequest, api.ErrBadRequest)
	}
}

// TestAddonSDKBackendExample은 SDK 예제 hello-backend(sdk/examples/hello-backend)를 개발 디렉터리로 불러, 예제
// 백엔드(sdk/go/examples/hello-backend)가 하는 호출이 manifest의 선언과 가짜 OMV로 끝까지 되는지 본다. backend는
// sdk/go를 import하지 않으므로 예제 백엔드 대신 같은 호스트 호출을 하는 작은 가짜 백엔드를 쓴다(실제 예제 백엔드는
// sdk/go의 시험이 본다).
func TestAddonSDKBackendExample(t *testing.T) {
	dir, err := filepath.Abs("../../../sdk/examples/hello-backend")
	if err != nil {
		t.Fatal(err)
	}
	// 가짜 백엔드가 부르는 메서드가 manifest의 선언과 어긋나지 않게 manifest를 직접 본다.
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mf struct {
		ID       string
		Requires struct{ Admin bool }
		RPC      struct{ Read, Write []string }
		Actions  []struct {
			Name           string
			Admin, Confirm bool
		}
		Backend *struct{ Network []string }
	}
	if err := json.Unmarshal(raw, &mf); err != nil {
		t.Fatal(err)
	}
	if mf.ID != "hello-backend" || mf.Requires.Admin || mf.Backend == nil || len(mf.Backend.Network) != 0 ||
		!slices.Equal(mf.RPC.Read, []string{"System.getInformation", "PerfStats.get"}) || !slices.Equal(mf.RPC.Write, []string{"PerfStats.set"}) ||
		len(mf.Actions) != 1 || mf.Actions[0].Name != "perfstats" || !mf.Actions[0].Admin || !mf.Actions[0].Confirm {
		t.Fatalf("manifest = %+v", mf)
	}

	e := newBackendEnv(t, envOptions{devAddons: []string{dir}})
	fb := &fakeAddonBackend{host: e.paths.Host(mf.ID)}
	ln, err := net.Listen("unix", e.paths.Backend(mf.ID))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(fb)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	// 예제 백엔드처럼: 데이터는 System.getInformation과 관리자에게만 PerfStats.get, 조작은 /v1/apply의 PerfStats.set.
	fb.set(func(w http.ResponseWriter, r *http.Request, host hostClient) {
		if r.URL.Path == "/v1/data" {
			reply := map[string]hostReply{"info": host.post("/v1/rpc", map[string]any{"service": "System", "method": "getInformation"})}
			if r.Header.Get("X-Maru-Admin") == "1" {
				reply["perf"] = host.post("/v1/rpc", map[string]any{"service": "PerfStats", "method": "get"})
			} else {
				// 일반 사용자 요청에서 부르면 OMV가 관리자 전용이라 거절한다(예제 백엔드가 r.Admin으로 가르는 이유).
				reply["perfAsUser"] = host.post("/v1/rpc", map[string]any{"service": "PerfStats", "method": "get"})
			}
			writeBackendJSON(w, http.StatusOK, reply)
			return
		}
		// fakeAddonBackend가 본문을 이미 읽어 두었다.
		_, raw := fb.lastRequest()
		var body struct{ Input struct{ Enable bool } }
		json.Unmarshal([]byte(raw), &body)
		writeBackendJSON(w, http.StatusOK, host.post("/v1/apply", map[string]any{"calls": []any{
			map[string]any{"service": "PerfStats", "method": "set", "params": map[string]any{"enable": body.Input.Enable}},
		}}))
	})

	admin := e.stepped("admin")
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	for _, b := range []*browser{admin, user} {
		if v := packageView(t, b.capabilities(), mf.ID); !v.Dev {
			t.Errorf("hello-backend is not a development package: %+v", v)
		}
	}
	data := func(b *browser) map[string]hostReply {
		t.Helper()
		return decode[map[string]hostReply](t, b.do(http.MethodGet, "/api/addons/hello-backend", nil))
	}
	d := data(admin)
	if d["info"].Status != http.StatusOK || !strings.Contains(string(d["info"].Body), `"hostname":"fake-nas.local"`) ||
		d["perf"].Status != http.StatusOK || !strings.Contains(string(d["perf"].Body), `"enable":true`) {
		t.Fatalf("admin data = %+v", d)
	}
	d = data(user)
	if d["info"].Status != http.StatusOK || d["perfAsUser"].Status == http.StatusOK {
		t.Errorf("user data = %+v", d)
	}

	act := func(b *browser, enable bool) *httptest.ResponseRecorder {
		return b.do(http.MethodPost, "/api/addons/hello-backend/actions/perfstats", map[string]any{"input": map[string]any{"enable": enable}})
	}
	// 관리자 조작이고, 쓰기를 선언한 패키지라 비밀번호 재확인이 필요하다.
	wantCode(t, act(user, false), http.StatusForbidden, api.ErrAdminRequired)
	plain := e.browser()
	wantStatus(t, plain.login("admin", "admin"), http.StatusOK)
	wantCode(t, act(plain, false), http.StatusForbidden, api.ErrSecurityRequired)

	// 끄고 다시 켠다. 두 번째 적용이 되므로 첫 적용이 적용 대기를 남기지 않았다(남았으면 409 config_dirty).
	for _, enable := range []bool{false, true} {
		r := relayed(t, act(admin, enable))
		if r.Status != http.StatusOK {
			t.Fatalf("apply enable=%v = %+v", enable, r)
		}
		var ps struct{ Enable bool }
		callJSON(t, e, "admin", "PerfStats", "get", &ps)
		var dirty bool
		callJSON(t, e, "admin", "Config", "isDirty", &dirty)
		if ps.Enable != enable || dirty {
			t.Errorf("after enable=%v: perfstats %v, dirty %v", enable, ps.Enable, dirty)
		}
	}
}
