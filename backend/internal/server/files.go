package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"maru/backend/internal/addons/storage"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/files"
	"maru/backend/internal/helper/client"
	"maru/backend/internal/helper/proto"
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
	uploadTempPrefix = ".maru-upload-"
)

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
	writeJSON(w, http.StatusOK, roots.FilesRoots)
}

// filesList는 폴더의 목록을 열거나, 같은 세션이 연 목록이 그대로면(폴더 수정 시각이 같으면) 다시 쓴다.
// refresh=1이면 언제나 다시 읽는다(폴더 수정 시각은 안의 파일 내용이 바뀌어도 그대로다).
func (h *handler) filesList(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	p := r.URL.Query().Get("path")
	roots, ok := h.allowedPaths(w, r, s, ticket, p)
	if !ok {
		return
	}
	shareRO := readOnlyAt(roots, p)
	if r.URL.Query().Get("refresh") != "1" {
		if l, ok := h.files.Find(s.ID, p); ok {
			st, err := h.Helper.Stat(r.Context(), ticket, p)
			if err == nil && st.Type == proto.TypeDir && st.ModTimeNs == l.DirModTimeNs {
				writeJSON(w, http.StatusOK, listingOf(l, shareRO))
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
	writeJSON(w, http.StatusOK, listingOf(l, shareRO))
}

// listingOf는 목록 응답이다. shareRO면 공유 폴더 권한이 읽기만 허락하므로 파일 시스템이 허락해도 쓸 수 없다고 알린다.
func listingOf(l *files.Listing, shareRO bool) api.FilesListing {
	return api.FilesListing{ID: l.ID, Path: l.Path, Total: l.Len(), Truncated: l.Truncated, Writable: l.Writable && !shareRO,
		ShareReadOnly: shareRO}
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
	roots, ok := h.allowedPaths(w, r, s, ticket, p)
	if !ok {
		return
	}
	in, err := h.Helper.Info(r.Context(), ticket, p)
	if err != nil {
		h.writeFilesError(w, s, err)
		return
	}
	out := api.FileInfo{Path: p, Name: in.Name, Type: in.Type, Size: in.Size, ModTime: in.ModTimeNs / int64(time.Millisecond),
		Mode: in.Mode, Target: in.Target, LinkType: in.LinkType, UID: in.UID, GID: in.GID, Owner: in.Owner, Group: in.Group, ACL: in.ACL}
	out.ShareReadOnly = readOnlyAt(roots, p)
	if in.Access != nil {
		out.Access = &api.FileAccess{Read: in.Access.Read, Write: in.Access.Write && !out.ShareReadOnly}
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
	if _, ok := h.allowedWritePaths(w, r, s, ticket, dir); !ok {
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
	roots, ok := h.allowedWritePaths(w, r, s, ticket, req.From, toDir)
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
	roots, ok := h.allowedWritePaths(w, r, s, ticket, req.Path)
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
	status, code, ok := filesErrorCode(pe.Code)
	if !ok {
		h.writeHelperError(w, s, err)
		return
	}
	writeError(w, status, code, strings.ReplaceAll(code, "_", " "))
}

// filesErrorCode는 도우미 파일 오류 코드의 HTTP 상태와 API 오류 코드다. 파일 오류가 아니면 ok가 false다.
func filesErrorCode(c string) (status int, code string, ok bool) {
	switch c {
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
	case proto.CodeNoTrash:
		status, code = http.StatusConflict, api.ErrNoTrash
	default:
		return 0, "", false
	}
	return status, code, true
}
