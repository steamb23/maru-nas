package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"mime"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
)

const (
	// pendingUploadTTL은 올린 패키지가 검토를 기다리는 시간이다.
	pendingUploadTTL = 30 * time.Minute
	// maxPendingUploads는 검토 대기 전체 상한이다(스테이징 디렉터리가 디스크를 쓴다).
	maxPendingUploads = 4
	// maxPendingPerSession은 세션 하나의 검토 대기 상한이다.
	maxPendingPerSession = 2
)

// pendingUploads는 올렸지만 아직 설치하지 않은 패키지다. 올린 세션에 묶인다.
type pendingUploads struct {
	now  func() time.Time
	mu   sync.Mutex
	list map[string]*pendingUpload
}

type pendingUpload struct {
	session string
	pending *addonpkg.Pending
	expires time.Time
}

func newPendingUploads(now func() time.Time) *pendingUploads {
	return &pendingUploads{now: now, list: map[string]*pendingUpload{}}
}

// add는 대기를 더하고 id를 돌려준다. 상한을 넘으면 오래된 것부터 버린다.
func (u *pendingUploads) add(session string, p *addonpkg.Pending) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.pruneLocked()
	for u.countLocked(session) >= maxPendingPerSession || len(u.list) >= maxPendingUploads {
		u.dropOldestLocked(session)
	}
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)
	u.list[id] = &pendingUpload{session: session, pending: p, expires: u.now().Add(pendingUploadTTL)}
	return id
}

// take는 그 세션의 대기를 꺼낸다(한 번만 쓴다).
func (u *pendingUploads) take(session, id string) (*addonpkg.Pending, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.pruneLocked()
	e, ok := u.list[id]
	if !ok || e.session != session {
		return nil, false
	}
	delete(u.list, id)
	return e.pending, true
}

// removeSession은 세션이 끝날 때 그 세션의 대기를 버린다.
func (u *pendingUploads) removeSession(session string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for id, e := range u.list {
		if e.session == session {
			e.pending.Discard()
			delete(u.list, id)
		}
	}
}

func (u *pendingUploads) pruneLocked() {
	now := u.now()
	for id, e := range u.list {
		if now.After(e.expires) {
			e.pending.Discard()
			delete(u.list, id)
		}
	}
}

func (u *pendingUploads) countLocked(session string) int {
	n := 0
	for _, e := range u.list {
		if e.session == session {
			n++
		}
	}
	return n
}

// dropOldestLocked는 session의 가장 오래된 대기를, 그 세션에 없으면 전체에서 가장 오래된 대기를 버린다.
func (u *pendingUploads) dropOldestLocked(session string) {
	oldest := ""
	for _, mine := range []bool{true, false} {
		for id, e := range u.list {
			if mine && e.session != session {
				continue
			}
			if oldest == "" || e.expires.Before(u.list[oldest].expires) {
				oldest = id
			}
		}
		if oldest != "" {
			break
		}
	}
	if oldest != "" {
		u.list[oldest].pending.Discard()
		delete(u.list, oldest)
	}
}

// packageWriteLevel은 쓰기·백엔드가 있는 패키지의 설치·켜기와 쓰기 조작에 필요한 단계다. 정책이 꺼져 있어도
// 비밀번호 재확인이고, root 권한 상승 정책이 2차 인증이면 2차 인증이다(.claude/rules/security.md).
func (h *handler) packageWriteLevel() string {
	if h.securityPolicy().Elevation == api.SecurityTwoFactor {
		return api.SecurityTwoFactor
	}
	return api.SecurityPassword
}

// requireElevated는 summary가 단계 확인이 필요한 권한이면 그 단계를 거쳤는지 본다. 아니면 403 security_required를 쓴다.
func (h *handler) requireElevated(w http.ResponseWriter, s auth.Session, sum addonpkg.Summary) bool {
	if !sum.Elevated() {
		return true
	}
	if level := h.packageWriteLevel(); !satisfies(s, level) {
		writeSecurityRequired(w, level)
		return false
	}
	return true
}

// addonPackages는 관리자의 패키지 목록이다(GET /api/site/addons).
func (h *handler) addonPackages(w http.ResponseWriter, _ *http.Request, _ auth.Session) {
	resp := api.AddonPackagesResponse{Packages: []api.AddonPackageStatus{}, MaxUploadBytes: addonpkg.DefaultLimits.Compressed, SDK: api.AddonSDKLevel}
	var ss *addonpkg.SyncState
	resp.Backends, ss = h.backendSync()
	if h.Packages != nil {
		list, err := h.Packages.List()
		resp.IndexError = err != nil
		resp.CanInstall = h.Packages.CanInstall()
		for _, st := range list {
			p := packageStatus(st)
			p.Backend = packageBackendState(st, ss)
			resp.Packages = append(resp.Packages, p)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// packageStatus는 관리자 목록의 항목이다.
func packageStatus(st addonpkg.Status) api.AddonPackageStatus {
	out := api.AddonPackageStatus{
		ID: st.ID, Version: st.Version, Rev: st.Rev, Dev: st.Dev, Enabled: st.Enabled,
		State: st.State, Reason: st.Reason, Detail: st.Detail,
	}
	if m := st.Manifest; m != nil {
		out.Name, out.Description, out.Author, out.AuthorURL = m.Name, m.Description, m.Author.Name, m.Author.URL
		out.Icon = "api/addons/" + st.ID + "/icon?rev=" + st.Rev
	}
	if st.Summary != nil {
		p := permissions(*st.Summary)
		out.Permissions = &p
	}
	if r := st.Record; r != nil {
		out.SHA256, out.InstalledBy = r.SHA256, r.InstalledBy
		installed, updated := r.InstalledAt, r.UpdatedAt
		out.InstalledAt, out.UpdatedAt = &installed, &updated
	}
	return out
}

func permissions(s addonpkg.Summary) api.AddonPermissions {
	return api.AddonPermissions{
		Admin: s.Admin, Plugins: s.Plugins, Read: s.Read, Write: s.Write, Actions: s.Actions,
		Backend: s.Backend, Network: s.Network, Risks: s.Risks, Elevated: s.Elevated(),
	}
}

// uploadAddon은 패키지 파일을 받아 검사하고 검토 대기를 만든다(POST /api/site/addons/uploads). 아직 설치하지 않는다.
// 본문은 원시 application/octet-stream이다(HTML 폼이 보낼 수 없는 형식이라 CSRF 보조 장치가 된다).
func (h *handler) uploadAddon(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/octet-stream" {
		writeError(w, http.StatusUnsupportedMediaType, api.ErrBadRequest, "content type must be application/octet-stream")
		return
	}
	if h.Packages == nil || !h.Packages.CanInstall() {
		writeError(w, http.StatusConflict, api.ErrAddonsUnavailable, "addon packages need a state directory")
		return
	}
	limit := addonpkg.DefaultLimits.Compressed
	if r.ContentLength > limit {
		writeError(w, http.StatusRequestEntityTooLarge, api.ErrAddonTooLarge, "package is too large")
		return
	}
	p, err := h.Packages.Prepare(http.MaxBytesReader(w, r.Body, limit+1))
	if err != nil {
		h.writeAddonRejected(w, s, "upload", "", err)
		return
	}
	id := h.uploads.add(s.ID, p)
	mf := p.Package.Manifest
	log.Printf("addon: user=%q action=upload id=%s version=%q sha256=%s result=ok", s.Username, mf.ID, mf.Version, p.Package.SHA256)
	resp := api.AddonPendingInstall{
		PendingID:        id,
		ExpiresInSeconds: int(pendingUploadTTL / time.Second),
		Package: packageStatus(addonpkg.Status{
			ID: mf.ID, Version: mf.Version, Rev: p.Package.Rev(), Enabled: true, State: addonpkg.StateDisabled,
			Manifest: mf, Summary: &p.Summary, Record: &addonpkg.Record{SHA256: p.Package.SHA256},
		}),
		Warnings: slices.Clone(p.Warnings),
	}
	resp.Package.InstalledAt, resp.Package.UpdatedAt = nil, nil
	resp.Package.Icon = ""
	if resp.Warnings == nil {
		resp.Warnings = []string{}
	}
	if prev := p.Previous; prev != nil {
		st := packageStatus(*prev)
		resp.Installed = &st
	}
	if d := p.Diff; d != nil {
		resp.Diff = &api.AddonPermissionsDiff{
			AdminAdded: d.AdminAdded, AdminRemoved: d.AdminRemoved, AddedPlugins: d.AddedPlugins,
			AddedRead: d.AddedRead, RemovedRead: d.RemovedRead, AddedWrite: d.AddedWrite, RemovedWrite: d.RemovedWrite,
			AddedActions: d.AddedActions, BackendAdded: d.BackendAdded, BackendRemoved: d.BackendRemoved,
			AddedNetwork: d.AddedNetwork, RemovedNetwork: d.RemovedNetwork, Widens: d.Widens(),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// writeAddonRejected는 패키지를 받지 않은 이유를 쓴다. 이유와 영어 설명은 관리자 화면에 보인다.
func (h *handler) writeAddonRejected(w http.ResponseWriter, s auth.Session, action, id string, err error) {
	var (
		pe *addonpkg.Error
		re *addonpkg.RejectError
		mb *http.MaxBytesError
	)
	resp := api.ErrorResponse{Error: "addon package rejected", Code: api.ErrAddonInvalid}
	status := http.StatusBadRequest
	switch {
	case errors.As(err, &mb), errors.As(err, &pe) && pe.Reason == addonpkg.ReasonTooLarge:
		status, resp.Code = http.StatusRequestEntityTooLarge, api.ErrAddonTooLarge
		if pe != nil {
			resp.Detail = pe.Detail
		}
	case errors.As(err, &pe):
		resp.Reason, resp.Detail = pe.Reason, pe.Detail
	case errors.As(err, &re):
		status = http.StatusConflict
		resp.Reason, resp.Detail = re.Code, re.Detail
	case errors.Is(err, addonpkg.ErrNoStore):
		status, resp.Code, resp.Error = http.StatusConflict, api.ErrAddonsUnavailable, "addon packages need a state directory"
	case errors.Is(err, addonpkg.ErrDev):
		status, resp.Code, resp.Error = http.StatusConflict, api.ErrAddonReadOnly, "development packages cannot be changed"
	case errors.Is(err, addonpkg.ErrNotFound):
		status, resp.Code, resp.Error = http.StatusNotFound, api.ErrNotFound, "not found"
	default:
		log.Printf("addon %s: %s: %v", id, action, err)
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to change addon packages"})
		return
	}
	log.Printf("addon: user=%q action=%s id=%q result=%s reason=%s", s.Username, action, id, resp.Code, resp.Reason)
	if resp.Detail != "" {
		log.Printf("addon %q: %s: %q", id, action, resp.Detail)
	}
	writeJSON(w, status, resp)
}

// installAddon은 검토한 대기를 설치한다(POST /api/site/addons/uploads/{pid}/install). 관리 창이 필요하고,
// 쓰기·백엔드가 있으면 그 단계 확인도 필요하다. 확인이 모자라면 대기를 남겨 두고 403을 준다(확인한 뒤 다시 부른다).
func (h *handler) installAddon(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var body struct{}
	if !decodeJSON(w, r, &body) {
		return
	}
	pid := r.PathValue("pid")
	p, ok := h.uploads.take(s.ID, pid)
	if !ok {
		writeError(w, http.StatusNotFound, api.ErrUploadExpired, "upload expired")
		return
	}
	if !h.requireManage(w, s) || !h.requireElevated(w, s, p.Summary) {
		h.uploads.restore(s.ID, pid, p)
		return
	}
	mf := p.Package.Manifest
	if h.addonBusy(mf.ID) {
		h.uploads.restore(s.ID, pid, p)
		writeError(w, http.StatusConflict, api.ErrActionBusy, "an action of this addon is in progress")
		return
	}
	st, err := h.Packages.Install(p, s.Username)
	if err != nil {
		h.writeAddonRejected(w, s, "install", mf.ID, err)
		return
	}
	log.Printf("addon: user=%q action=install id=%s version=%q sha256=%s result=ok", s.Username, mf.ID, mf.Version, p.Package.SHA256)
	writeJSON(w, http.StatusOK, packageStatus(*st))
}

// restore는 확인이 모자라 설치하지 못한 대기를 되돌려 둔다(같은 id, 같은 만료).
func (u *pendingUploads) restore(session, id string, p *addonpkg.Pending) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.list[id] = &pendingUpload{session: session, pending: p, expires: p.CreatedAt.Add(pendingUploadTTL)}
}

// discardUpload는 검토 대기를 버린다(DELETE /api/site/addons/uploads/{pid}).
func (h *handler) discardUpload(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if p, ok := h.uploads.take(s.ID, r.PathValue("pid")); ok {
		p.Discard()
	}
	w.WriteHeader(http.StatusNoContent)
}

// setAddonEnabled는 패키지를 켜거나 끈다(PUT /api/site/addons/{id}). 켜기는 설치와 같은 확인이 필요하다.
func (h *handler) setAddonEnabled(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.AddonEnabledRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	if h.Packages == nil {
		writeError(w, http.StatusNotFound, api.ErrNotFound, "not found")
		return
	}
	if req.Enabled {
		cur, ok := h.Packages.Status(id)
		if ok && !cur.Enabled {
			if !h.requireManage(w, s) {
				return
			}
			if cur.Summary != nil && !h.requireElevated(w, s, *cur.Summary) {
				return
			}
		}
	}
	if h.addonBusy(id) {
		writeError(w, http.StatusConflict, api.ErrActionBusy, "an action of this addon is in progress")
		return
	}
	st, err := h.Packages.SetEnabled(id, req.Enabled)
	if err != nil {
		h.writeAddonRejected(w, s, "enable", id, err)
		return
	}
	action := "disable"
	if req.Enabled {
		action = "enable"
	}
	log.Printf("addon: user=%q action=%s id=%s result=ok", s.Username, action, id)
	writeJSON(w, http.StatusOK, packageStatus(*st))
}

// uninstallAddon은 패키지를 지운다(DELETE /api/site/addons/{id}). 바로가기·위젯 배치는 남는다(다시 설치하면 돌아온다).
// ?purge=1이면 root 동기화가 패키지 백엔드의 데이터도 지우게 한다(관리 창 필요). 고르지 않으면 데이터는 다시 설치하거나
// 지우기를 고를 때까지 남고, 다시 설치하면 설치 인스턴스가 바뀌어 동기화가 켜기 전에 지운다. DELETE는 다른 출처에서
// preflight 없이 보낼 수 없다.
func (h *handler) uninstallAddon(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req struct{ Purge bool }
	req.Purge = r.URL.Query().Get("purge") == "1"
	id := r.PathValue("id")
	if h.Packages == nil {
		writeError(w, http.StatusNotFound, api.ErrNotFound, "not found")
		return
	}
	if req.Purge && !h.requireManage(w, s) {
		return
	}
	if h.addonBusy(id) {
		writeError(w, http.StatusConflict, api.ErrActionBusy, "an action of this addon is in progress")
		return
	}
	// 백엔드가 없는 패키지는 지울 백엔드 데이터가 없다(동기화가 없는 유닛을 정리하지 않게). manifest를 읽지 못한
	// 패키지도 백엔드가 있었는지 모르므로 넣지 않는다(다시 설치하면 설치 인스턴스가 바뀌어 동기화가 지운다).
	cur, _ := h.Packages.Status(id)
	hasBackend := cur != nil && !cur.Dev && cur.Manifest != nil && cur.Manifest.HasBackend()
	purge := req.Purge && hasBackend
	// 지울 목록이 원하는 상태 파일에 다 들어가지 않으면 제거하기 전에 거절한다(넣지 못한 purge를 잃지 않게).
	if purge && h.sync != nil && h.sync.PurgeFull(id) {
		log.Printf("addon: user=%q action=uninstall id=%s purge=true result=%s", s.Username, id, api.ErrAddonPurgeFull)
		writeError(w, http.StatusConflict, api.ErrAddonPurgeFull, "too many backend data removals are waiting for the backend sync")
		return
	}
	if err := h.Packages.Uninstall(id); err != nil {
		h.writeAddonRejected(w, s, "uninstall", id, err)
		return
	}
	// 제거가 레지스트리를 바꿀 때(OnChange) 원하는 상태에서 백엔드가 빠졌다. purge는 그 뒤에 더한다(제거가 실패하면
	// 켜져 있는 id에 purge가 남지 않게).
	if purge {
		purge = h.purgeBackend(id)
	}
	// purge는 실제로 지울 목록에 넣었는지다(고르지 않음, 백엔드 없음, 동기화 없는 설치는 false).
	log.Printf("addon: user=%q action=uninstall id=%s purge=%t result=ok", s.Username, id, purge)
	w.WriteHeader(http.StatusNoContent)
}

// addonBusy는 그 애드온의 조작이 진행 중인지다(조작 키는 id + "\x00" + 대상).
func (h *handler) addonBusy(id string) bool {
	h.actions.mu.Lock()
	defer h.actions.mu.Unlock()
	for key := range h.actions.keys {
		if strings.HasPrefix(key, id+"\x00") {
			return true
		}
	}
	return false
}
