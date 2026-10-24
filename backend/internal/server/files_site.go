package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/files"
)

// sharesMethods는 공유 폴더 스냅숏이 부르는 고정 메서드다(관리자 전용 RPC).
var sharesMethods = map[string]addons.RPCKind{"ShareMgmt.enumerateSharedFolders": addons.RPCRead}

// filesSiteState는 site.json의 파일 탐색기 설정이다. 이 프로세스만 쓰므로 메모리에 두고 저장할 때 함께 바꾼다.
type filesSiteState struct {
	mu sync.Mutex
	v  api.SiteFiles
	// refresh는 스냅숏 갱신을 하나씩 하게 한다.
	refresh sync.Mutex
}

func (h *handler) loadFilesSite() {
	site, err := h.Site.GetSite()
	if err != nil {
		log.Printf("site settings: read file explorer settings: %v", err)
		return
	}
	if site.Files != nil {
		h.filesSite.mu.Lock()
		h.filesSite.v = *site.Files
		h.filesSite.mu.Unlock()
	}
}

func (h *handler) siteFiles() api.SiteFiles {
	h.filesSite.mu.Lock()
	defer h.filesSite.mu.Unlock()
	v := h.filesSite.v
	v.Shares = append([]api.SharedFolder(nil), v.Shares...)
	return v
}

// updateFilesSite는 파일 탐색기 설정을 바꿔 저장한다.
func (h *handler) updateFilesSite(apply func(*api.SiteFiles)) error {
	h.filesSite.mu.Lock()
	defer h.filesSite.mu.Unlock()
	next := h.filesSite.v
	next.Shares = append([]api.SharedFolder(nil), next.Shares...)
	apply(&next)
	err := h.Site.UpdateSite(func(v *api.SiteSettings) {
		if next.SharesUpdatedAt == "" && !next.ShowDisks && !next.FollowPrivileges {
			v.Files = nil
			return
		}
		f := next
		v.Files = &f
	})
	if err != nil {
		return err
	}
	h.filesSite.v = next
	return nil
}

// refreshShares는 관리자 세션 s로 공유 폴더 목록을 받아 스냅숏을 바꾼다. 실패하면 이전 스냅숏을 둔다.
// OMV가 세션 만료로 거부하면 errSessionExpired다.
func (h *handler) refreshShares(ctx context.Context, s auth.Session, ip string) ([]api.SharedFolder, error) {
	h.filesSite.refresh.Lock()
	defer h.filesSite.refresh.Unlock()
	caller := &rpcCaller{h: h, s: s, addon: "files", clientIP: ip, fixed: sharesMethods}
	var raw json.RawMessage
	err := addons.CallInto(ctx, caller, "ShareMgmt", "enumerateSharedFolders", map[string]any{}, &raw)
	if caller.unauthorized.Load() {
		return nil, errSessionExpired
	}
	if err != nil {
		return nil, err
	}
	shares, err := files.ParseShares(raw)
	if err != nil {
		return nil, fmt.Errorf("shared folders: %w", err)
	}
	now := h.Now().UTC().Format(time.RFC3339)
	if err := h.updateFilesSite(func(f *api.SiteFiles) { f.Shares, f.SharesUpdatedAt = shares, now }); err != nil {
		return nil, err
	}
	log.Printf("files: shared folders snapshot updated by=%q count=%d", s.Username, len(shares))
	return shares, nil
}

var errSessionExpired = errors.New("omv session expired")

// refreshSharesOnLogin은 관리자가 로그인했을 때 스냅숏을 뒤에서 갱신한다. 로그인 응답을 기다리게 하지 않는다.
// 파일 탐색기는 권한 도우미가 있어야 쓸 수 있으므로 도우미가 없으면 하지 않는다(설정 앱의 새로 고침은 된다).
func (h *handler) refreshSharesOnLogin(s auth.Session, ip string) {
	if !s.Admin || h.Helper == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), sharesTimeout)
		defer cancel()
		if _, err := h.refreshShares(ctx, s, ip); err != nil {
			log.Printf("files: shared folders snapshot on login user=%q: %v", s.Username, err)
		}
	}()
}

// filesSettings는 파일 탐색기 전역 설정(스냅숏, 디스크 보기, 공유 폴더 권한 따르기)이다(관리자).
func (h *handler) filesSettings(w http.ResponseWriter, _ *http.Request, _ auth.Session) {
	writeJSON(w, http.StatusOK, filesSettingsOf(h.siteFiles()))
}

func filesSettingsOf(f api.SiteFiles) api.FilesSettings {
	out := api.FilesSettings{Shares: f.Shares, SharesUpdatedAt: f.SharesUpdatedAt, ShowDisks: f.ShowDisks, FollowPrivileges: f.FollowPrivileges}
	if out.Shares == nil {
		out.Shares = []api.SharedFolder{}
	}
	return out
}

// setFilesSettings는 디스크 최상위 보기와 공유 폴더 권한 따르기를 바꾼다(관리자). 보내지 않은 설정은 그대로 둔다.
// 권한 따르기를 켤 때는 권한이 담긴 스냅숏을 먼저 받는다(받지 못하면 켜지 않는다). 그 전의 스냅숏(옛 버전이 받았거나
// 오래된 권한)으로 열어 주지 않기 위해서다.
func (h *handler) setFilesSettings(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.FilesSettingsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.FollowPrivileges != nil && *req.FollowPrivileges && !h.siteFiles().FollowPrivileges {
		ctx, cancel := context.WithTimeout(r.Context(), sharesTimeout)
		defer cancel()
		_, err := h.refreshShares(ctx, s, clientIP(r))
		switch {
		case errors.Is(err, errSessionExpired):
			h.expire(w, r, s)
			return
		case err != nil:
			log.Printf("files: refresh shared folders user=%q: %v", s.Username, err)
			writeError(w, http.StatusBadGateway, api.ErrOMVError, "failed to read shared folders")
			return
		}
	}
	err := h.updateFilesSite(func(f *api.SiteFiles) {
		if req.ShowDisks != nil {
			f.ShowDisks = *req.ShowDisks
		}
		if req.FollowPrivileges != nil {
			f.FollowPrivileges = *req.FollowPrivileges
		}
	})
	if err != nil {
		log.Printf("site settings: save file explorer settings: %v", err)
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to save settings"})
		return
	}
	f := h.siteFiles()
	log.Printf("files: show disks=%t follow privileges=%t by=%q", f.ShowDisks, f.FollowPrivileges, s.Username)
	writeJSON(w, http.StatusOK, filesSettingsOf(f))
}

// refreshFilesShares는 공유 폴더 스냅숏을 지금 갱신한다(관리자, 설정 앱의 새로 고침 단추).
func (h *handler) refreshFilesShares(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !requireJSON(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sharesTimeout)
	defer cancel()
	_, err := h.refreshShares(ctx, s, clientIP(r))
	switch {
	case errors.Is(err, errSessionExpired):
		h.expire(w, r, s)
		return
	case err != nil:
		log.Printf("files: refresh shared folders user=%q: %v", s.Username, err)
		writeError(w, http.StatusBadGateway, api.ErrOMVError, "failed to read shared folders")
		return
	}
	writeJSON(w, http.StatusOK, filesSettingsOf(h.siteFiles()))
}
