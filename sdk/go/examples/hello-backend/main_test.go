// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/steamb23/maru-nas/sdk/go/maruaddon"
)

// 예제 백엔드를 SDK의 ServeContext로 유닉스 소켓에 띄우고, maru의 호스트 소켓은 가짜(fakeHost)로 둔다. 네트워크와
// 실제 OMV에는 닿지 않는다.

const testToken = "fake-call-token"

// shortDir는 유닉스 소켓 경로 한도(107바이트) 안에 드는 임시 디렉터리다.
func shortDir(t *testing.T) string {
	t.Helper()
	base := os.TempDir()
	if len(base) > 40 {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "hb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// fakeHost는 maru의 호스트 소켓을 흉내 낸다: System.getInformation, PerfStats.get, /v1/apply의 PerfStats.set.
type fakeHost struct {
	mu     sync.Mutex
	enable bool
	dirty  bool
	calls  []string
}

func (h *fakeHost) enabled() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.enable
}

func (h *fakeHost) log() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.calls...)
}

func replyJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *fakeHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Maru-Call") != testToken {
		replyJSON(w, http.StatusUnauthorized, map[string]string{"code": "call_invalid", "error": "call token is not valid"})
		return
	}
	body, _ := io.ReadAll(r.Body)
	h.mu.Lock()
	defer h.mu.Unlock()
	switch r.URL.Path {
	case "/v1/rpc":
		var req struct{ Service, Method string }
		json.Unmarshal(body, &req)
		h.calls = append(h.calls, "rpc "+req.Service+"."+req.Method)
		switch req.Service + "." + req.Method {
		case "System.getInformation":
			replyJSON(w, http.StatusOK, map[string]any{"result": map[string]any{"hostname": "example-nas", "version": "8.0-1 (Synthetic)"}})
		case "PerfStats.get":
			replyJSON(w, http.StatusOK, map[string]any{"result": map[string]bool{"enable": h.enable}})
		default:
			replyJSON(w, http.StatusForbidden, map[string]string{"code": "rpc_forbidden", "error": "not declared"})
		}
	case "/v1/apply":
		var req struct {
			Calls []struct {
				Service, Method string
				Params          map[string]any
			}
		}
		json.Unmarshal(body, &req)
		if h.dirty {
			h.calls = append(h.calls, "apply refused")
			replyJSON(w, http.StatusConflict, map[string]string{"code": "config_dirty", "error": "omv has pending configuration changes"})
			return
		}
		if len(req.Calls) != 1 || req.Calls[0].Service != "PerfStats" || req.Calls[0].Method != "set" || len(req.Calls[0].Params) != 1 {
			replyJSON(w, http.StatusBadRequest, map[string]string{"code": "bad_request", "error": string(body)})
			return
		}
		on, _ := req.Calls[0].Params["enable"].(bool)
		h.enable = on
		h.calls = append(h.calls, "apply PerfStats.set "+map[bool]string{true: "on", false: "off"}[on])
		replyJSON(w, http.StatusOK, map[string]any{"results": []any{map[string]bool{"enable": on}}})
	default:
		replyJSON(w, http.StatusNotFound, map[string]string{"code": "not_found", "error": "not found"})
	}
}

type testBackend struct {
	host   *fakeHost
	client *http.Client
}

// start는 가짜 호스트와 예제 백엔드를 띄운다. stateDir이 비면 메모리에만 둔다.
func start(t *testing.T, stateDir string) *testBackend {
	t.Helper()
	dir := shortDir(t)
	fh := &fakeHost{enable: true}
	hl, err := net.Listen("unix", filepath.Join(dir, "host.sock"))
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: fh}
	go hs.Serve(hl)
	t.Cleanup(func() { hs.Close() })
	// ServeContext는 호스트 소켓 경로를 MARU_ADDON_HOST에서 읽는다.
	t.Setenv("MARU_ADDON_HOST", filepath.Join(dir, "host.sock"))

	st, err := openStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }
	bl, err := net.Listen("unix", filepath.Join(dir, "backend.sock"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- maruaddon.ServeContext(ctx, bl, newHandler(&backend{state: st, now: now})) }()
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "backend.sock"))
	}}, Timeout: 10 * time.Second}
	t.Cleanup(func() {
		client.CloseIdleConnections()
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	return &testBackend{host: fh, client: client}
}

// do는 maru처럼 요청 하나를 보낸다.
func (b *testBackend) do(t *testing.T, method, path, user string, admin bool, body string) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, "http://addon"+path, rd)
	req.Header.Set("X-Maru-User", user)
	req.Header.Set("X-Maru-Admin", map[bool]string{true: "1", false: "0"}[admin])
	req.Header.Set("X-Maru-Lang", "en")
	req.Header.Set("X-Maru-Call", testToken)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func (b *testBackend) data(t *testing.T, user string, admin bool) dataReply {
	t.Helper()
	status, body := b.do(t, http.MethodGet, "/v1/data", user, admin, "")
	if status != http.StatusOK {
		t.Fatalf("data as %s: %d %s", user, status, body)
	}
	var d dataReply
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestData(t *testing.T) {
	b := start(t, "")
	d := b.data(t, "admin", true)
	if d.Hostname != "example-nas" || d.Version == "" || d.PerfStats == nil || !d.PerfStats.Enable || d.DataRequests != 1 ||
		d.Persistent || d.LastChange != nil {
		t.Errorf("admin data = %+v", d)
	}
	// 일반 사용자 요청에서는 관리자 전용 메서드(PerfStats.get)를 부르지 않고, 관리자 기록도 주지 않는다.
	before := len(b.host.log())
	d = b.data(t, "alice", false)
	if d.PerfStats != nil || d.LastChange != nil || d.DataRequests != 2 {
		t.Errorf("user data = %+v", d)
	}
	if got := b.host.log()[before:]; len(got) != 1 || got[0] != "rpc System.getInformation" {
		t.Errorf("host calls for a user = %v", got)
	}
}

func TestPerfStatsAction(t *testing.T) {
	b := start(t, "")
	act := func(body string) (int, string) {
		return b.do(t, http.MethodPost, "/v1/actions/"+actionPerfStats, "admin", true, body)
	}

	// 끄고 다시 켠다. 바꿀 때마다 /v1/apply로 적용까지 한다.
	if status, body := act(`{"target":"","input":{"enable":false}}`); status != http.StatusNoContent {
		t.Fatalf("turn off: %d %s", status, body)
	}
	d := b.data(t, "admin", true)
	if d.PerfStats == nil || d.PerfStats.Enable || d.LastChange == nil || d.LastChange.User != "admin" || d.LastChange.Enable {
		t.Errorf("after turning off: %+v", d)
	}
	if status, body := act(`{"target":"","input":{"enable":true}}`); status != http.StatusNoContent {
		t.Fatalf("turn on: %d %s", status, body)
	}
	// 이미 켜져 있으면 바꾸지도 적용하지도 않는다.
	if status, body := act(`{"target":"","input":{"enable":true}}`); status != http.StatusNoContent {
		t.Fatalf("again: %d %s", status, body)
	}
	applies := 0
	for _, c := range b.host.log() {
		if strings.HasPrefix(c, "apply") {
			applies++
		}
	}
	if applies != 2 || !b.host.enabled() {
		t.Errorf("host calls = %v", b.host.log())
	}

	// 입력이 없거나 틀리면 400 bad_input이다.
	for _, in := range []string{`{"target":"","input":null}`, `{"target":"","input":{"enable":"no"}}`, `{"target":"","input":{"on":true}}`} {
		if status, body := act(in); status != http.StatusBadRequest || !strings.Contains(body, `"bad_input"`) {
			t.Errorf("%s: %d %s", in, status, body)
		}
	}

	// 적용 대기가 있으면 409 config_dirty(maru가 addonCode로 넘긴다)이고 OMV는 그대로다.
	b.host.mu.Lock()
	b.host.dirty = true
	b.host.mu.Unlock()
	if status, body := act(`{"target":"","input":{"enable":false}}`); status != http.StatusConflict || !strings.Contains(body, `"config_dirty"`) {
		t.Errorf("dirty: %d %s", status, body)
	}
	if !b.host.enabled() {
		t.Error("perfstats changed while config was dirty")
	}
}

func TestStatePersists(t *testing.T) {
	dir := t.TempDir()
	b := start(t, dir)
	if status, body := b.do(t, http.MethodPost, "/v1/actions/"+actionPerfStats, "admin", true, `{"input":{"enable":false}}`); status != http.StatusNoContent {
		t.Fatalf("action: %d %s", status, body)
	}
	b.data(t, "alice", false)

	st, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.persistent() || st.st.DataRequests != 1 || st.st.LastChange == nil || st.st.LastChange.User != "admin" ||
		!st.st.LastChange.At.Equal(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("saved = %+v", st.st)
	}
	fi, err := os.Stat(filepath.Join(dir, stateFile))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("state file: %v %v", fi, err)
	}

	// 손상된 파일은 처음부터 다시 센다.
	os.WriteFile(filepath.Join(dir, stateFile), []byte("{broken"), 0o600)
	if st, err := openStore(dir); err != nil || st.st.DataRequests != 0 {
		t.Errorf("broken file: %+v %v", st, err)
	}
}
