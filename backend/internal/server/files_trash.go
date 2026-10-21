package server

import (
	"context"
	"log"
	"net/http"
	"path"

	"onmaru/backend/internal/addons/storage"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/files"
	"onmaru/backend/internal/helper/proto"
)

// 파일 탐색기의 휴지통 API. 휴지통은 권한 도우미가 FreeDesktop 휴지통 규격(trash-put과 같은 곳·형식)으로 다룬다
// (fs.trash, fs.trashdirs, fs.trashlist, fs.trashrestore, fs.trashpurge, fs.trashempty). 디스크 맨 위의 공유
// 휴지통(.Trash)은 관리자가 설정 앱에서 root 권한 상승 뒤 만든다(trash.prepare).

// filesTrash는 항목 하나를 휴지통으로 옮긴다. 보이는 폴더 자신과 그것을 품은 폴더는 넣지 않는다.
func (h *handler) filesTrash(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	var req api.FilesTrashRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	roots, ok := h.allowedPaths(w, r, s, ticket, req.Path)
	if !ok {
		return
	}
	if rootFolder(roots, req.Path) {
		writeError(w, http.StatusConflict, api.ErrRootFolder, "folders shown in the sidebar cannot be deleted")
		return
	}
	res, err := h.Helper.Trash(r.Context(), ticket, req.Path)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, api.FilesTrashResult{Trash: res.Trash, Name: res.Name})
}

// filesTrashVolumes는 홈 폴더와 디스크마다 휴지통 종류다(삭제 대화상자가 휴지통으로 옮길 수 있는지 고를 때).
func (h *handler) filesTrashVolumes(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	res, err := h.Helper.TrashDirs(r.Context(), ticket)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	out := api.FilesTrashVolumes{Volumes: []api.TrashVolume{}}
	for _, v := range res.Volumes {
		out.Volumes = append(out.Volumes, api.TrashVolume{Top: v.Top, Kind: v.Kind, Shared: v.Shared})
	}
	writeJSON(w, http.StatusOK, out)
}

// filesTrashList는 사용자의 모든 휴지통 항목이다.
func (h *handler) filesTrashList(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	res, err := h.Helper.TrashList(r.Context(), ticket)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	out := api.FilesTrashList{Items: make([]api.TrashItem, 0, len(res.Items)), Truncated: res.Truncated}
	for _, it := range res.Items {
		out.Items = append(out.Items, api.TrashItem{Trash: it.Trash, Name: it.Name, Path: it.Path, DeletedAt: it.DeletedAt * 1000,
			Type: it.Type, Size: it.Size, ModTime: it.ModTime * 1000, BadName: it.BadName})
	}
	writeJSON(w, http.StatusOK, out)
}

// filesTrashRestore는 휴지통 항목 하나를 To로 되돌린다. To는 보이는 폴더 안의 새 경로여야 한다(원래 경로든 새 이름이든
// 이 백엔드가 먼저 거른다).
func (h *handler) filesTrashRestore(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	var req api.FilesTrashRestoreRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	dir, _, ok := splitNew(req.To)
	if !ok || !files.ValidPath(req.Trash) || !proto.ValidName(req.Name) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid trash item or name")
		return
	}
	if !h.allowedPath(w, r, s, ticket, dir) {
		return
	}
	e, err := h.Helper.TrashRestore(r.Context(), ticket, req.Trash, req.Name, req.To)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, fileItemOf(e))
}

// filesTrashPurge는 휴지통 항목 하나를 영구 삭제한다.
func (h *handler) filesTrashPurge(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	var req api.FilesTrashItemRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !files.ValidPath(req.Trash) || !proto.ValidName(req.Name) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid trash item")
		return
	}
	if err := h.Helper.TrashPurge(r.Context(), ticket, req.Trash, req.Name); err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// filesTrashEmpty는 사용자의 모든 휴지통을 비운다.
func (h *handler) filesTrashEmpty(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	if !requireJSON(w, r) {
		return
	}
	if err := h.Helper.TrashEmpty(r.Context(), ticket); err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// siteTrashDisks는 디스크마다 공유 휴지통(.Trash)이 있는지다(관리자, 설정 앱). 관리자의 사용자 권한으로 본다.
func (h *handler) siteTrashDisks(ctx context.Context, ticket string) (api.SiteTrashDisks, error) {
	out := api.SiteTrashDisks{Disks: []api.SiteTrashDisk{}}
	res, err := h.Helper.TrashDirs(ctx, ticket)
	if err != nil {
		return out, err
	}
	shared := map[string]bool{}
	for _, v := range res.Volumes {
		shared[v.Top] = v.Shared
	}
	for _, m := range h.trashDisks() {
		name := m.Label
		if name == "" {
			name = path.Base(m.MountPoint)
		}
		out.Disks = append(out.Disks, api.SiteTrashDisk{Name: name, Path: m.MountPoint, Shared: shared[m.MountPoint]})
	}
	return out, nil
}

// trashDisks는 휴지통을 만들 수 있는 디스크(/srv 아래 마운트)다.
func (h *handler) trashDisks() []storage.MountLabel {
	if h.Mounts == nil {
		return nil
	}
	mounts, err := h.Mounts.MountsUnder(h.filesSrvDir())
	if err != nil {
		log.Printf("files: read mounts: %v", err)
	}
	return mounts
}

// getSiteTrash는 디스크마다 공유 휴지통이 있는지다(관리자).
func (h *handler) getSiteTrash(w http.ResponseWriter, r *http.Request, s auth.Session) {
	ticket, ok := h.helperTicket(w, s)
	if !ok {
		return
	}
	out, err := h.siteTrashDisks(r.Context(), ticket)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// prepareSiteTrash는 디스크 맨 위에 모든 사용자가 쓰는 휴지통 .Trash(1777)를 만든다(관리자, root 권한 상승 필요).
// 상승하지 않았으면 도우미의 elevation_required를 그대로 돌려주고 설정 앱이 상승한 뒤 다시 부른다.
func (h *handler) prepareSiteTrash(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.SiteTrashPrepareRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	known := false
	for _, m := range h.trashDisks() {
		known = known || m.MountPoint == req.Disk
	}
	if !known {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "unknown disk")
		return
	}
	ticket, ok := h.helperTicket(w, s)
	if !ok {
		return
	}
	if err := h.Helper.TrashPrepare(r.Context(), ticket, req.Disk); err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	log.Printf("files: shared trash prepared disk=%q by=%q", req.Disk, s.Username)
	out, err := h.siteTrashDisks(r.Context(), ticket)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
