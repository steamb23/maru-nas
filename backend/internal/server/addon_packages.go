package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"maps"
	"net/http"
	"os"
	"slices"
	"sync"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/addonpkg/format"
	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
)

// addons는 지금 레지스트리다. 애드온 패키지가 있으면 내장과 받은 패키지를 함께 담는다. /api/rpc 중계만은
// 내장 레지스트리(h.Addons)를 본다(.claude/docs/addon-packages.md의 "레지스트리").
func (h *handler) addons() *addons.Registry {
	if h.Packages != nil {
		return h.Packages.Registry()
	}
	return h.Addons
}

// visiblePackage는 이 세션이 쓸 수 있는(켜져 있고, 받았고, capability를 만족하는) 패키지다.
func (h *handler) visiblePackage(id string, s auth.Session) (*addonpkg.Installed, bool) {
	if h.Packages == nil {
		return nil, false
	}
	p, ok := h.Packages.Get(id)
	if !ok {
		return nil, false
	}
	a, ok := h.addons().Get(id)
	if !ok || !addons.Capability(a, h.Plugins.Plugins(), s.Admin).Available {
		return nil, false
	}
	return p, true
}

// packageViews는 capabilities에 담을 이 사용자의 패키지 목록이다. 관리자 전용 패키지는 관리자에게만 준다.
// 플러그인이 없는 패키지는 넣는다(창 안에 안내를 보인다, 내장 애드온과 같다).
func (h *handler) packageViews(s auth.Session) []api.AddonPackageView {
	out := []api.AddonPackageView{}
	if h.Packages == nil {
		return out
	}
	for _, p := range h.Packages.Active() {
		m := p.Manifest
		if m.Requires.Admin && !s.Admin {
			continue
		}
		v := api.AddonPackageView{
			ID:          p.ID,
			Rev:         p.Rev,
			Dev:         p.Dev,
			Version:     m.Version,
			Name:        maps.Clone(m.Name),
			Description: maps.Clone(m.Description),
			Author:      m.Author.Name,
			Icon:        "api/addons/" + p.ID + "/icon?rev=" + p.Rev,
			UIBase:      "api/addon-ui/" + h.uiTokens.token(s.ID, p.ID) + "/" + p.Rev + "/",
			Requires:    api.Requirements{Admin: m.Requires.Admin, Plugins: []string{}},
			Actions:     []api.AddonPackageAction{},
			Languages:   slices.Clone(p.Languages),
			SDK:         m.SDK,
			Backend:     m.HasBackend(),
			Elevated:    len(m.RPC.Write) > 0,
		}
		for _, pl := range m.Requires.Plugins {
			v.Requires.Plugins = append(v.Requires.Plugins, pl.Package)
		}
		if w := m.Views.Window; w != nil {
			v.Views.Window = &api.AddonPackageWindow{Entry: w.Entry, DefaultSize: w.DefaultSize, MinSize: w.MinSize}
		}
		if w := m.Views.Widget; w != nil {
			v.Views.Widget = &api.AddonPackageWidget{Entry: w.Entry, DefaultSize: w.DefaultSize, MinSize: w.MinSize, MaxSize: w.MaxSize}
		}
		for _, a := range m.Actions {
			v.Actions = append(v.Actions, api.AddonPackageAction{
				Name: a.Name, Admin: a.Admin, Confirm: a.Confirm,
				Label: maps.Clone(a.Label), ConfirmText: maps.Clone(a.ConfirmText),
			})
		}
		out = append(out, v)
	}
	return out
}

// 패키지 아이콘의 헤더. <img>로만 그리므로 스크립트는 돌지 않지만, 주소를 바로 열어도 같은 출처 권한이 없게 한다.
const addonIconCSP = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

// addonIcon은 패키지 아이콘(icon.svg)이다(GET /api/addons/{id}/icon). 이 세션이 쓸 수 있는 패키지만 주고,
// 관리자에게는 꺼지거나 받지 않은 설치본의 아이콘도 준다(관리자 페이지).
func (h *handler) addonIcon(w http.ResponseWriter, r *http.Request, s auth.Session) {
	id := r.PathValue("id")
	dir := ""
	if p, ok := h.visiblePackage(id, s); ok {
		dir = p.Dir
	} else if s.Admin && h.Packages != nil {
		if st, ok := h.Packages.Status(id); ok && st.Manifest != nil {
			dir = st.Dir
		}
	}
	if dir == "" {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	defer root.Close()
	f, err := root.Open(format.IconFile)
	if err != nil {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, addonpkg.DefaultLimits.Icon+1))
	if err != nil || int64(len(b)) > addonpkg.DefaultLimits.Icon {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "image/svg+xml")
	hd.Set("Content-Security-Policy", addonIconCSP)
	hd.Set("X-Content-Type-Options", "nosniff")
	// 주소의 rev가 바뀌면 다른 주소이므로 오래 둔다.
	hd.Set("Cache-Control", "private, max-age=86400")
	w.Write(b)
}

// addonMessages는 패키지 번역(GET /api/addons/{id}/i18n/{lang})이다. en 위에 그 언어를 덮어 준다.
func (h *handler) addonMessages(w http.ResponseWriter, r *http.Request, s auth.Session) {
	p, ok := h.visiblePackage(r.PathValue("id"), s)
	lang := r.PathValue("lang")
	if !ok || !slices.Contains(format.Languages, lang) {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	defer root.Close()
	out := map[string]string{}
	for _, l := range []string{format.FallbackLanguage, lang} {
		if !slices.Contains(p.Languages, l) {
			continue
		}
		table, err := readMessages(root, "i18n/"+l+".json")
		if err != nil {
			log.Printf("addon %s: read i18n %s: %v", p.ID, l, err)
			continue
		}
		maps.Copy(out, table)
	}
	writeJSON(w, http.StatusOK, out)
}

func readMessages(root *os.Root, name string) (map[string]string, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var table map[string]string
	if err := json.NewDecoder(io.LimitReader(f, addonpkg.DefaultLimits.I18n)).Decode(&table); err != nil {
		return nil, err
	}
	return table, nil
}

// uiTokens는 애드온 프레임 파일 경로의 토큰이다. 불투명 출처의 프레임은 하위 자원을 받을 때 쿠키를 보내지
// 않으므로, 경로의 토큰이 그 세션과 패키지를 대신 밝힌다. 세션·패키지마다 하나이고 세션이 끝나면 지운다
// (고정 만료가 없어 capabilities를 다시 받아도 프레임 주소가 바뀌지 않는다).
type uiTokens struct {
	mu        sync.Mutex
	byToken   map[string]uiGrant
	bySession map[string]map[string]string
}

type uiGrant struct {
	session string
	addon   string
}

func newUITokens() *uiTokens {
	return &uiTokens{byToken: map[string]uiGrant{}, bySession: map[string]map[string]string{}}
}

// token은 세션·패키지의 토큰이다. 없으면 만든다.
func (t *uiTokens) token(session, addon string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tok, ok := t.bySession[session][addon]; ok {
		return tok
	}
	b := make([]byte, 32)
	rand.Read(b)
	tok := hex.EncodeToString(b)
	t.byToken[tok] = uiGrant{session: session, addon: addon}
	if t.bySession[session] == nil {
		t.bySession[session] = map[string]string{}
	}
	t.bySession[session][addon] = tok
	return tok
}

func (t *uiTokens) lookup(tok string) (uiGrant, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	g, ok := t.byToken[tok]
	return g, ok
}

func (t *uiTokens) removeSession(session string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tok := range t.bySession[session] {
		delete(t.byToken, tok)
	}
	delete(t.bySession, session)
}
