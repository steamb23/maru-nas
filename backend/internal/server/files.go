package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/addons/storage"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/files"
	"onmaru/backend/internal/helper/client"
	"onmaru/backend/internal/helper/proto"
)

// 파일 탐색기 API. 파일은 권한 도우미가 로그인한 사용자의 권한으로 다루고(fs.names, fs.statmany, fs.info, fs.read,
// fs.write, fs.mkdir, fs.rename, fs.remove), 이 백엔드는 보이는 폴더(홈, 공유 폴더, 디스크) 밖의 경로를 도우미에 보내기 전에 거른다. 실제로 닿을
// 수 있는 범위는 도우미가 자기 기준 디렉터리로 다시 정한다.

// MountSource는 /srv 아래 마운트(디스크)다. *storage.Reader가 구현한다.
type MountSource interface {
	MountsUnder(dir string) ([]storage.MountLabel, error)
}

const (
	// filesPageLimit은 정보를 함께 주는 범위 요청의 최대 항목 수, filesNamesLimit은 이름만 주는 범위의 최대 수다.
	filesPageLimit  = 500
	filesNamesLimit = 2000
	// filesPrepareBatch는 prepare 한 번에 정보를 읽는 항목 수다(도우미 호출 4번 안팎, 연산마다 30초 제한).
	filesPrepareBatch = 2000
	// statManyBytes는 fs.statmany 한 번에 보내는 이름 JSON의 크기 상한이다(도우미 요청 머리 64KiB 안).
	statManyBytes = 48 << 10
	// sharesTimeout은 공유 폴더 스냅숏을 받는 OMV 호출의 시간 제한이다.
	sharesTimeout = 10 * time.Second
	// maxQuery는 "이 폴더에서 찾기" 검색어의 최대 바이트 수다.
	maxQuery = 255
	// uploadTempPrefix는 권한 도우미가 올리는 중인 임시 파일에 쓰는 이름 접두사다. 이 이름으로는 만들거나 바꾸지 않는다.
	uploadTempPrefix = ".onmaru-upload-"
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
		if next.SharesUpdatedAt == "" && !next.ShowDisks {
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

// filesSettings는 파일 탐색기 전역 설정(스냅숏, 디스크 보기)이다(관리자).
func (h *handler) filesSettings(w http.ResponseWriter, _ *http.Request, _ auth.Session) {
	writeJSON(w, http.StatusOK, filesSettingsOf(h.siteFiles()))
}

func filesSettingsOf(f api.SiteFiles) api.FilesSettings {
	out := api.FilesSettings{Shares: f.Shares, SharesUpdatedAt: f.SharesUpdatedAt, ShowDisks: f.ShowDisks}
	if out.Shares == nil {
		out.Shares = []api.SharedFolder{}
	}
	return out
}

// setFilesSettings는 디스크 최상위 보기를 바꾼다(관리자).
func (h *handler) setFilesSettings(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.FilesSettingsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.updateFilesSite(func(f *api.SiteFiles) { f.ShowDisks = req.ShowDisks }); err != nil {
		log.Printf("site settings: save file explorer settings: %v", err)
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to save settings"})
		return
	}
	log.Printf("files: show disks=%t by=%q", req.ShowDisks, s.Username)
	writeJSON(w, http.StatusOK, filesSettingsOf(h.siteFiles()))
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

// filesSrvDir는 데이터 디스크가 마운트되는 디렉터리다(권한 도우미와 같은 /srv).
func (h *handler) filesSrvDir() string {
	if h.FilesSrvDir != "" {
		return h.FilesSrvDir
	}
	return "/srv"
}

// requireFiles는 파일 탐색기 API의 공통 조건이다: 보안 강화 정책(files), 도우미 티켓.
func (h *handler) requireFiles(next func(http.ResponseWriter, *http.Request, auth.Session, string)) http.HandlerFunc {
	return h.requireSession(func(w http.ResponseWriter, r *http.Request, s auth.Session) {
		if lvl := h.securityPolicy().Files; !satisfies(s, lvl) {
			writeSecurityRequired(w, lvl)
			return
		}
		ticket, ok := h.helperTicket(w, s)
		if !ok {
			return
		}
		next(w, r, s, ticket)
	})
}

// home은 s 사용자의 홈 폴더다(도우미 id의 home, 세션별로 기억). 없으면 빈 값이다.
func (h *handler) home(ctx context.Context, s auth.Session, ticket string) (string, error) {
	if home, ok := h.files.Home(s.ID); ok {
		return home, nil
	}
	id, err := h.Helper.Identity(ctx, ticket, proto.ScopeUser)
	if err != nil {
		return "", err
	}
	home := ""
	if files.ValidPath(id.Home) {
		home = id.Home
	}
	h.files.SetHome(s.ID, home)
	return home, nil
}

// roots는 s에게 보이는 폴더다. 디스크는 스냅숏이 없을 때(대체) 모두에게, 관리자가 디스크 최상위 보기를 켰을 때
// 관리자에게 보인다.
func (h *handler) roots(ctx context.Context, s auth.Session, ticket string) (api.FilesRoots, error) {
	home, err := h.home(ctx, s, ticket)
	if err != nil {
		return api.FilesRoots{}, err
	}
	site := h.siteFiles()
	out := api.FilesRoots{Shares: []api.FilesShare{}, Disks: []api.FilesRoot{}, SharesUpdatedAt: site.SharesUpdatedAt}
	if home != "" {
		out.Home = &api.FilesRoot{Name: path.Base(home), Path: home}
	}
	for _, sh := range site.Shares {
		out.Shares = append(out.Shares, api.FilesShare{UUID: sh.UUID, Name: sh.Name, Path: sh.Path,
			Reachable: files.Reachable(sh.Path, h.filesSrvDir())})
	}
	out.Fallback = site.SharesUpdatedAt == ""
	if (out.Fallback || (s.Admin && site.ShowDisks)) && h.Mounts != nil {
		mounts, err := h.Mounts.MountsUnder(h.filesSrvDir())
		if err != nil {
			log.Printf("files: read mounts: %v", err)
		}
		for _, m := range mounts {
			name := m.Label
			if name == "" {
				name = path.Base(m.MountPoint)
			}
			out.Disks = append(out.Disks, api.FilesRoot{Name: name, Path: m.MountPoint})
		}
	}
	return out, nil
}

// allowedPath는 p가 s에게 보이는 폴더 안인지 확인한다. 아니면 오류 응답을 쓰고 false다.
func (h *handler) allowedPath(w http.ResponseWriter, r *http.Request, s auth.Session, ticket, p string) bool {
	_, ok := h.allowedPaths(w, r, s, ticket, p)
	return ok
}

// allowedPaths는 ps가 모두 s에게 보이는 폴더 안인지 확인하고 보이는 폴더를 돌려준다. 아니면 오류 응답을 쓰고 false다.
func (h *handler) allowedPaths(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string, ps ...string) (api.FilesRoots, bool) {
	for _, p := range ps {
		if !files.ValidPath(p) {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "path must be an absolute, clean path")
			return api.FilesRoots{}, false
		}
	}
	roots, err := h.roots(r.Context(), s, ticket)
	if err != nil {
		h.writeFilesError(w, s, err)
		return api.FilesRoots{}, false
	}
	for _, p := range ps {
		if !withinRoots(roots, p) {
			writeError(w, http.StatusForbidden, api.ErrOutsideRoots, "path is outside the folders the file explorer shows")
			return api.FilesRoots{}, false
		}
	}
	return roots, true
}

// withinRoots는 p가 보이는 폴더(홈, 닿을 수 있는 공유 폴더, 디스크) 가운데 하나의 안인지다.
func withinRoots(roots api.FilesRoots, p string) bool {
	ok := roots.Home != nil && files.Within(p, roots.Home.Path)
	for _, sh := range roots.Shares {
		ok = ok || (sh.Reachable && files.Within(p, sh.Path))
	}
	for _, d := range roots.Disks {
		ok = ok || files.Within(p, d.Path)
	}
	return ok
}

// rootFolder는 p가 보이는 폴더(홈, 공유 폴더, 디스크) 자신이거나 그것을 품은 폴더인지다. 이런 폴더는 이름을 바꾸거나
// 옮기거나 지우지 않는다. 권한 도우미는 자기 기준 디렉터리(홈, 마운트 지점)만 지키므로, 디스크 안의 공유 폴더는
// 여기서 지킨다(디스크 최상위 보기에서는 공유 폴더가 디스크의 한 폴더로 보인다).
func rootFolder(roots api.FilesRoots, p string) bool {
	var paths []string
	if roots.Home != nil {
		paths = append(paths, roots.Home.Path)
	}
	for _, sh := range roots.Shares {
		paths = append(paths, sh.Path)
	}
	for _, d := range roots.Disks {
		paths = append(paths, d.Path)
	}
	for _, rp := range paths {
		if files.ValidPath(rp) && files.Within(rp, p) {
			return true
		}
	}
	return false
}

// newNameOK는 만들거나 바꿀 이름으로 쓸 수 있는지다: 도우미의 이름 검사, UTF-8(브라우저가 다시 가리킬 수 있게),
// 올리는 중인 임시 파일의 접두사가 아님.
func newNameOK(name string) bool {
	return proto.ValidName(name) && utf8.ValidString(name) && !strings.HasPrefix(name, uploadTempPrefix)
}

// splitNew는 새로 만들 경로를 폴더와 이름으로 나눈다. 경로가 올바르지 않거나 이름을 쓸 수 없으면 false다.
func splitNew(p string) (dir, name string, ok bool) {
	if !files.ValidPath(p) {
		return "", "", false
	}
	dir, name = path.Split(p)
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" || !newNameOK(name) {
		return "", "", false
	}
	return dir, name, true
}

// filesRoots는 사이드바에 처음 보이는 폴더다.
func (h *handler) filesRoots(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	roots, err := h.roots(r.Context(), s, ticket)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, roots)
}

// filesList는 폴더의 목록을 열거나, 같은 세션이 연 목록이 그대로면(폴더 수정 시각이 같으면) 다시 쓴다.
// refresh=1이면 언제나 다시 읽는다(폴더 수정 시각은 안의 파일 내용이 바뀌어도 그대로다).
func (h *handler) filesList(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	p := r.URL.Query().Get("path")
	if !h.allowedPath(w, r, s, ticket, p) {
		return
	}
	if r.URL.Query().Get("refresh") != "1" {
		if l, ok := h.files.Find(s.ID, p); ok {
			st, err := h.Helper.Stat(r.Context(), ticket, p)
			if err == nil && st.Type == proto.TypeDir && st.ModTimeNs == l.DirModTimeNs {
				writeJSON(w, http.StatusOK, listingOf(l))
				return
			}
		}
	}
	res, names, err := h.Helper.Names(r.Context(), ticket, p)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	l := files.NewListing(files.NewID(), p, res, names)
	h.files.Add(s.ID, l)
	writeJSON(w, http.StatusOK, listingOf(l))
}

func listingOf(l *files.Listing) api.FilesListing {
	return api.FilesListing{ID: l.ID, Path: l.Path, Total: l.Len(), Truncated: l.Truncated, Writable: l.Writable}
}

// parseView는 보기(정렬·방향·찾기어) 쿼리다. 잘못된 값이 있으면 false다.
func parseView(q url.Values) (files.View, bool) {
	sortKey, ok := files.ParseSortKey(q.Get("sort"))
	order := q.Get("order")
	query := q.Get("q")
	if !ok || (order != "" && order != "asc" && order != "desc") || len(query) > maxQuery || !utf8.ValidString(query) {
		return files.View{}, false
	}
	return files.View{Sort: sortKey, Desc: order == "desc", Query: query}, true
}

// filesPage는 목록의 보기(정렬·찾기)에서 범위 하나다. 정보가 없는 항목은 도우미에서 읽어 채운다.
func (h *handler) filesPage(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	l, ok := h.files.Get(s.ID, r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusGone, api.ErrListingExpired, "listing expired, open the folder again")
		return
	}
	q := r.URL.Query()
	view, okView := parseView(q)
	offset, errOff := strconv.Atoi(cmp.Or(q.Get("offset"), "0"))
	limit, errLim := strconv.Atoi(cmp.Or(q.Get("limit"), "100"))
	names := q.Get("names") == "1"
	maxLimit := filesPageLimit
	if names {
		maxLimit = filesNamesLimit
	}
	if !okView || errOff != nil || errLim != nil || offset < 0 || limit < 1 || limit > maxLimit {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid range")
		return
	}
	idx, total, err := l.Page(view, offset, limit)
	if errors.Is(err, files.ErrSortPending) {
		writeError(w, http.StatusConflict, api.ErrSortPending, "entry info is not loaded yet")
		return
	}
	if !names {
		if err := h.fillStats(r.Context(), ticket, l, l.Unfilled(idx)); err != nil {
			h.writeFilesError(w, s, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, api.FilesPage{Total: total, Offset: offset, Items: l.Items(idx, names)})
}

// filesFind는 보기에서 이름 하나의 자리다(만들거나 이름을 바꾼 항목을 목록에서 고를 때). 도우미를 부르지 않는다.
func (h *handler) filesFind(w http.ResponseWriter, r *http.Request, s auth.Session, _ string) {
	l, ok := h.files.Get(s.ID, r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusGone, api.ErrListingExpired, "listing expired, open the folder again")
		return
	}
	q := r.URL.Query()
	view, okView := parseView(q)
	name := q.Get("name")
	if !okView || name == "" {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid view or name")
		return
	}
	i, err := l.IndexOf(view, name)
	if errors.Is(err, files.ErrSortPending) {
		writeError(w, http.StatusConflict, api.ErrSortPending, "entry info is not loaded yet")
		return
	}
	writeJSON(w, http.StatusOK, api.FilesFindResponse{Index: i})
}

// filesPrepare는 정보가 없는 항목을 filesPrepareBatch개까지 더 읽는다. 브라우저가 진행 막대를 그리며 Filled가
// Total이 될 때까지 부른다(날짜·크기순). 취소는 부르지 않는 것이다.
func (h *handler) filesPrepare(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	if !requireJSON(w, r) {
		return
	}
	l, ok := h.files.Get(s.ID, r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusGone, api.ErrListingExpired, "listing expired, open the folder again")
		return
	}
	if err := h.fillStats(r.Context(), ticket, l, l.NextUnfilled(filesPrepareBatch)); err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	filled, total := l.Filled()
	writeJSON(w, http.StatusOK, api.FilesPrepareResponse{Filled: filled, Total: total})
}

// fillStats는 항목 idx의 정보를 fs.statmany로 읽어 목록에 둔다. 한 번에 보내는 이름은 개수와 JSON 크기로 나눈다.
// UTF-8이 아닌 이름은 JSON으로 옮길 수 없으므로 읽지 않는다.
func (h *handler) fillStats(ctx context.Context, ticket string, l *files.Listing, idx []int) error {
	var batch []int
	var names []string
	size := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		res, err := h.Helper.StatMany(ctx, ticket, l.Path, names)
		if err != nil {
			return err
		}
		if len(res.Items) != len(batch) {
			return fmt.Errorf("%w: statmany returned %d items for %d names", client.ErrUnavailable, len(res.Items), len(batch))
		}
		for k, it := range res.Items {
			l.SetStat(batch[k], files.Stat{Type: it.Type, Size: it.Size, ModTimeNs: it.ModTimeNs, Locked: it.Locked,
				LinkType: it.LinkType, Code: it.Code})
		}
		batch, names, size = batch[:0], names[:0], 0
		return nil
	}
	for _, i := range idx {
		name := l.Name(i)
		if !utf8.ValidString(name) {
			l.SetStat(i, files.Stat{Code: files.CodeBadName})
			continue
		}
		b, _ := json.Marshal(name)
		if len(batch) >= proto.MaxStatMany || size+len(b)+1 > statManyBytes {
			if err := flush(); err != nil {
				return err
			}
		}
		batch, names, size = append(batch, i), append(names, name), size+len(b)+1
	}
	return flush()
}

// filesInfo는 파일 하나의 정보 패널 내용이다. 관리자에게는 그 경로가 속한 공유 폴더(OMV ACL 화면 링크)를 함께 준다.
func (h *handler) filesInfo(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	p := r.URL.Query().Get("path")
	if !h.allowedPath(w, r, s, ticket, p) {
		return
	}
	in, err := h.Helper.Info(r.Context(), ticket, p)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	out := api.FileInfo{Path: p, Name: in.Name, Type: in.Type, Size: in.Size, ModTime: in.ModTimeNs / int64(time.Millisecond),
		Mode: in.Mode, Target: in.Target, LinkType: in.LinkType, UID: in.UID, GID: in.GID, Owner: in.Owner, Group: in.Group, ACL: in.ACL}
	if in.Access != nil {
		out.Access = &api.FileAccess{Read: in.Access.Read, Write: in.Access.Write}
	}
	if s.Admin {
		best := ""
		for _, sh := range h.siteFiles().Shares {
			if files.Within(p, sh.Path) && len(sh.Path) > len(best) {
				best, out.ShareUUID = sh.Path, sh.UUID
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// filesDownload는 파일 내용을 내려받기로 보낸다. 범위 하나(재개용)와 If-Range만 지원한다. 브라우저가 내용을
// 그리지 않게 언제나 application/octet-stream과 attachment, sandbox CSP로 보낸다.
func (h *handler) filesDownload(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	p := r.URL.Query().Get("path")
	if !h.allowedPath(w, r, s, ticket, p) {
		return
	}
	var offset, length int64
	partial := false
	if rh := r.Header.Get("Range"); rh != "" {
		st, err := h.Helper.Stat(r.Context(), ticket, p)
		if err != nil {
			h.writeFilesError(w, s, err)
			return
		}
		if st.Type == proto.TypeFile && ifRangeMatches(r.Header.Get("If-Range"), etagOf(st.Size, st.ModTimeNs)) {
			start, n, ok, satisfiable := parseRange(rh, st.Size)
			if ok && !satisfiable {
				w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(st.Size, 10))
				writeError(w, http.StatusRequestedRangeNotSatisfiable, api.ErrBadRequest, "range not satisfiable")
				return
			}
			if ok {
				offset, length, partial = start, n, true
			}
		}
	}
	res, body, err := h.Helper.Read(r.Context(), ticket, p, offset, length)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	defer body.Close()
	hd := w.Header()
	hd.Set("Content-Type", "application/octet-stream")
	hd.Set("Content-Disposition", contentDisposition(path.Base(p)))
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	hd.Set("Cache-Control", "private, no-store")
	hd.Set("Accept-Ranges", "bytes")
	hd.Set("ETag", etagOf(res.FileSize, res.ModTimeNs))
	hd.Set("Last-Modified", time.Unix(0, res.ModTimeNs).UTC().Format(http.TimeFormat))
	hd.Set("Content-Length", strconv.FormatInt(body.Size, 10))
	status := http.StatusOK
	if partial {
		status = http.StatusPartialContent
		hd.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+body.Size-1, res.FileSize))
	}
	w.WriteHeader(status)
	if _, err := io.Copy(w, body); err != nil && r.Context().Err() == nil {
		log.Printf("files: download user=%q: %v", s.Username, err)
	}
}

// etagOf는 파일 크기와 수정 시각으로 만든 ETag다(재개한 내려받기가 같은 파일인지 If-Range로 확인한다).
func etagOf(size, modTimeNs int64) string {
	return `"` + strconv.FormatInt(size, 36) + "-" + strconv.FormatInt(modTimeNs, 36) + `"`
}

// ifRangeMatches는 If-Range가 없거나 ETag가 같은지다. 날짜 형식의 If-Range는 받지 않는다(전체를 보낸다).
func ifRangeMatches(ifRange, etag string) bool {
	return ifRange == "" || ifRange == etag
}

// parseRange는 Range 머리의 범위 하나("bytes=a-b", "bytes=a-", "bytes=-n")다. ok가 false면 Range를 무시하고
// 전체를 보낸다(여러 범위, 모르는 단위, 잘못된 형식). satisfiable이 false면 416이다.
func parseRange(h string, size int64) (offset, length int64, ok, satisfiable bool) {
	spec, found := strings.CutPrefix(h, "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, 0, false, false
	}
	a, b, found := strings.Cut(strings.TrimSpace(spec), "-")
	if !found {
		return 0, 0, false, false
	}
	if a == "" {
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil || n < 0 {
			return 0, 0, false, false
		}
		if n == 0 || size == 0 {
			return 0, 0, true, false
		}
		n = min(n, size)
		return size - n, n, true, true
	}
	start, err := strconv.ParseInt(a, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false, false
	}
	end := size - 1
	if b != "" {
		e, err := strconv.ParseInt(b, 10, 64)
		if err != nil || e < start {
			return 0, 0, false, false
		}
		end = min(e, size-1)
	}
	if start >= size {
		return 0, 0, true, false
	}
	return start, end - start + 1, true, true
}

// contentDisposition은 내려받을 파일 이름이다. ASCII가 아닌 글자는 filename*(RFC 5987)에 담고, filename에는
// 바꿔 쓴 ASCII 이름을 둔다.
func contentDisposition(name string) string {
	var ascii strings.Builder
	for _, r := range name {
		if r < 0x20 || r >= 0x7f || r == '"' || r == '\\' {
			ascii.WriteByte('_')
		} else {
			ascii.WriteRune(r)
		}
	}
	return `attachment; filename="` + ascii.String() + `"; filename*=UTF-8''` + rfc5987(name)
}

func rfc5987(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// filesUpload는 요청 본문을 파일 하나로 올린다. 본문은 메모리에 모으지 않고 도우미로 바로 흘려 보내며, 도우미가
// 같은 폴더 임시 파일에 다 받은 뒤 이름을 바꾼다. 원시 본문(application/octet-stream)만 받는다(CSRF 보조 장치,
// HTML 폼은 이 형식을 보낼 수 없고 다른 출처의 fetch는 preflight에서 막힌다).
func (h *handler) filesUpload(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/octet-stream" {
		writeError(w, http.StatusUnsupportedMediaType, api.ErrBadRequest, "content type must be application/octet-stream")
		return
	}
	p := r.URL.Query().Get("path")
	dir, _, ok := splitNew(p)
	if !ok {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid file name")
		return
	}
	if !h.allowedPath(w, r, s, ticket, dir) {
		return
	}
	if r.ContentLength < 0 {
		writeError(w, http.StatusLengthRequired, api.ErrBadRequest, "content length required")
		return
	}
	overwrite := r.URL.Query().Get("overwrite") == "1"
	e, err := h.Helper.Write(r.Context(), ticket, p, overwrite, r.Body, r.ContentLength)
	if err != nil {
		if r.Context().Err() != nil {
			// 브라우저가 올리기를 취소했다. 도우미가 임시 파일을 지운다.
			return
		}
		h.writeFilesError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, fileItemOf(e))
}

// fileItemOf는 도우미가 돌려준 항목을 목록 항목 형태로 바꾼다(만들기·올리기·이름 바꾸기의 응답).
func fileItemOf(e proto.Entry) api.FileItem {
	return api.FileItem{Name: e.Name, Type: e.Type, State: "ok", Size: e.Size, ModTime: e.ModTimeNs / int64(time.Millisecond),
		LinkType: e.LinkType}
}

// filesMkdir는 폴더를 만든다. 만든 폴더의 항목을 돌려준다.
func (h *handler) filesMkdir(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	var req api.FilesMkdirRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	dir, _, ok := splitNew(req.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid folder name")
		return
	}
	if !h.allowedPath(w, r, s, ticket, dir) {
		return
	}
	e, err := h.Helper.Mkdir(r.Context(), ticket, req.Path)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, fileItemOf(e))
}

// filesRename은 이름을 바꾸거나 다른 폴더로 옮긴다(끌어 놓기). 옮기기는 이름 바꾸기 하나라 같은 디스크 안에서만
// 되고(cross_device), 대상이 있으면 덮어쓰지 않는다(file_exists). 보이는 폴더 자신과 그것을 품은 폴더는 다루지 않는다.
func (h *handler) filesRename(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	var req api.FilesRenameRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	toDir, _, ok := splitNew(req.To)
	if !ok || !files.ValidPath(req.From) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid name")
		return
	}
	if files.Within(req.To, req.From) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "cannot move a folder into itself")
		return
	}
	roots, ok := h.allowedPaths(w, r, s, ticket, req.From, toDir)
	if !ok {
		return
	}
	if rootFolder(roots, req.From) {
		writeError(w, http.StatusConflict, api.ErrRootFolder, "folders shown in the sidebar cannot be renamed or moved")
		return
	}
	e, err := h.Helper.Rename(r.Context(), ticket, req.From, req.To)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, fileItemOf(e))
}

// filesDelete는 파일이나 폴더를 지운다. SMB 휴지통(.recycle)을 거치지 않아 되돌릴 수 없으므로 브라우저가 확인
// 대화상자를 먼저 띄운다. 보이는 폴더 자신과 그것을 품은 폴더는 지우지 않는다.
func (h *handler) filesDelete(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	var req api.FilesDeleteRequest
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
	if err := h.Helper.Remove(r.Context(), ticket, req.Path, req.Recursive); err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeFilesError는 파일 연산의 도우미 오류를 HTTP 응답으로 바꾼다. 파일 오류가 아니면 writeHelperError로 넘긴다.
func (h *handler) writeFilesError(w http.ResponseWriter, s auth.Session, err error) {
	var pe *proto.Error
	if !errors.As(err, &pe) {
		h.writeHelperError(w, s, err)
		return
	}
	status, code := 0, ""
	switch pe.Code {
	case proto.CodeNotFound:
		status, code = http.StatusNotFound, api.ErrNotFound
	case proto.CodePermissionDenied:
		status, code = http.StatusForbidden, api.ErrPermissionDenied
	case proto.CodeOutsideRoots:
		status, code = http.StatusForbidden, api.ErrOutsideRoots
	case proto.CodeExists:
		status, code = http.StatusConflict, api.ErrFileExists
	case proto.CodeNotDir:
		status, code = http.StatusBadRequest, api.ErrNotDir
	case proto.CodeIsDir:
		status, code = http.StatusBadRequest, api.ErrIsDir
	case proto.CodeNotRegular:
		status, code = http.StatusBadRequest, api.ErrNotRegular
	case proto.CodeNoSpace:
		status, code = http.StatusInsufficientStorage, api.ErrNoSpace
	case proto.CodeReadOnly:
		status, code = http.StatusConflict, api.ErrReadOnly
	case proto.CodeBusy:
		status, code = http.StatusTooManyRequests, api.ErrHelperBusy
	case proto.CodeTimeout:
		status, code = http.StatusGatewayTimeout, api.ErrTransferTimeout
	case proto.CodeNotEmpty:
		status, code = http.StatusConflict, api.ErrNotEmpty
	case proto.CodeCrossDevice:
		status, code = http.StatusConflict, api.ErrCrossDevice
	default:
		h.writeHelperError(w, s, err)
		return
	}
	writeError(w, status, code, strings.ReplaceAll(code, "_", " "))
}
