package server

import (
	"log"
	"net/http"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
)

// 운영 백엔드의 root 동기화(.claude/docs/addon-packages.md의 "root 동기화"). maru는 원하는 상태와 요청을 쓰고 결과를
// 읽기만 한다(addonpkg.Sync). 동기화는 설치기의 --addon-backends가 켠 설치(Config.AddonSync)에서만 한다.

// packagesChanged는 레지스트리가 바뀔 때마다(Manager.OnChange, Manager의 잠금을 쥔 채로) 부른다. 호스트 소켓을 먼저
// 맞추고 원하는 상태를 쓴다. 둘 다 잠금 없이 읽는 Active만 쓴다.
func (h *handler) packagesChanged() {
	h.hosts.reconcile()
	if h.sync == nil {
		return
	}
	if err := h.sync.Update(); err != nil {
		log.Printf("addon backends: %v", err)
	}
}

// purgeBackend는 제거한 패키지의 백엔드 데이터를 root 동기화가 지우게 한다(DELETE /api/site/addons/{id}?purge=1).
// 지울 목록에 넣었으면 true다. 파일을 쓰지 못해도 목록은 메모리에 남아 다음 쓰기 때 들어가고, 관리자 페이지가 그 오류를
// 보인다. 동기화가 없는 설치(백엔드가 꺼짐, 개발 백엔드)에서는 지울 데이터가 없으므로 false다.
func (h *handler) purgeBackend(id string) bool {
	if h.sync == nil {
		log.Printf("addon %s: purge ignored: backends are not managed on this installation", id)
		return false
	}
	if err := h.sync.Purge(id); err != nil {
		log.Printf("addon %s: purge: %v", id, err)
	}
	return true
}

// backendState는 백엔드 설치본 p의 동기화 상태다(addonpkg.SyncState.Backend). 동기화가 없는 설치(개발 백엔드)와
// 개발본은 늘 running이다(개발자가 띄운 백엔드에 바로 붙는다).
func (h *handler) backendState(p *addonpkg.Installed) (state, reason string) {
	if h.sync == nil || p.Dev {
		return addonpkg.BackendRunning, ""
	}
	return h.sync.Backend(p.ID, p.Rev, p.InstallUID)
}

// backendFailed는 마지막 동기화 결과가 설치본 p의 지금 rev·설치를 켜지 못했는지다(capability backend_unavailable).
func (h *handler) backendFailed(p *addonpkg.Installed) bool {
	state, _ := h.backendState(p)
	return state == addonpkg.BackendFailed
}

// backendSync는 관리자 페이지의 백엔드 상태다. 동기화를 하는 설치면 그 상태(ss)도 돌려준다.
func (h *handler) backendSync() (api.AddonBackendSync, *addonpkg.SyncState) {
	out := api.AddonBackendSync{Enabled: h.backendsEnabled()}
	if h.sync == nil {
		return out, nil
	}
	ss := h.sync.State()
	out.Managed, out.Pending, out.Stalled, out.Applied = true, ss.Pending, ss.Stalled, ss.Applied()
	if ss.Status != nil {
		out.Error = ss.Status.Error
		// 설치기가 써 둔 빈 결과(digest·오류 없음)는 동기화한 것이 아니다. 전체가 실패한 결과는 그 시각을 보인다.
		if (ss.Synced() || ss.Status.Error != "") && !ss.Status.UpdatedAt.IsZero() {
			at := ss.Status.UpdatedAt
			out.SyncedAt = &at
		}
	}
	switch {
	case ss.WriteErr != nil:
		out.FileError = ss.WriteErr.Error()
	case ss.StatusErr != nil:
		out.FileError = "read sync status: " + ss.StatusErr.Error()
	}
	return out, &ss
}

// packageBackendState는 관리자 목록 항목의 백엔드 상태다. 동기화를 하는 설치에서 레지스트리에 들어간 백엔드 설치본만
// 있다.
func packageBackendState(st addonpkg.Status, ss *addonpkg.SyncState) *api.AddonPackageBackend {
	if ss == nil || st.Dev || st.State != addonpkg.StateActive || st.Manifest == nil || !st.Manifest.HasBackend() || st.Record == nil {
		return nil
	}
	state, reason := ss.Backend(st.ID, st.Rev, st.Record.InstallUID)
	return &api.AddonPackageBackend{State: state, Error: reason}
}

// syncAddonBackends는 root 동기화를 다시 요청한다(POST /api/site/addons/sync). 결과가 지금 원하는 상태를 맞췄어도
// 요청한다(실패한 백엔드를 다시 켜 본다). 아직 가져가지 않은 요청이 있으면 새로 쓰지 않고, 1분 넘게 남은 요청은
// 덮어쓴다. 응답은 GET /api/site/addons의 backends와 같다. 관리 창은 필요 없다(원하는 상태를 바꾸지 않는다).
func (h *handler) syncAddonBackends(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var body struct{}
	if !decodeJSON(w, r, &body) {
		return
	}
	if h.sync == nil {
		writeError(w, http.StatusConflict, api.ErrAddonBackendDisabled, "addon backends are not managed on this installation")
		return
	}
	wrote, err := h.sync.Request()
	if err != nil {
		log.Printf("addon: user=%q action=sync result=error: %v", s.Username, err)
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to request a backend sync"})
		return
	}
	log.Printf("addon: user=%q action=sync requested=%t result=ok", s.Username, wrote)
	out, _ := h.backendSync()
	writeJSON(w, http.StatusOK, out)
}
