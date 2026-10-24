package server

import (
	"context"
	"log"
	"net/http"
	"path"
	"slices"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/files"
	"maru/backend/internal/helper/proto"
)

// filesSrvDir는 데이터 디스크가 마운트되는 디렉터리다(권한 도우미와 같은 /srv).
func (h *handler) filesSrvDir() string {
	if h.FilesSrvDir != "" {
		return h.FilesSrvDir
	}
	return "/srv"
}

// requireFiles는 파일 탐색기 읽기 API의 공통 조건이다: 보안 강화 정책(files, 쓰기에만 적용하면 건너뜀), 도우미 티켓.
func (h *handler) requireFiles(next func(http.ResponseWriter, *http.Request, auth.Session, string)) http.HandlerFunc {
	return h.filesGuard(false, next)
}

// requireFilesWrite는 파일 탐색기 쓰기 API(올리기, 만들기, 이름 바꾸기·옮기기, 삭제, 휴지통 연산)의 공통 조건이다.
// 보안 강화 정책(files)을 언제나 따른다.
func (h *handler) requireFilesWrite(next func(http.ResponseWriter, *http.Request, auth.Session, string)) http.HandlerFunc {
	return h.filesGuard(true, next)
}

func (h *handler) filesGuard(write bool, next func(http.ResponseWriter, *http.Request, auth.Session, string)) http.HandlerFunc {
	return h.requireSession(func(w http.ResponseWriter, r *http.Request, s auth.Session) {
		p := h.securityPolicy()
		if (write || p.FilesScope != api.FilesScopeWrites) && !satisfies(s, p.Files) {
			writeSecurityRequired(w, p.Files)
			return
		}
		ticket, ok := h.helperTicket(w, s)
		if !ok {
			return
		}
		next(w, r, s, ticket)
	})
}

// identity는 s 사용자의 신원(도우미 id의 홈 폴더, 사용자·그룹 이름)이다. 세션별로 기억한다. 홈이 없으면 빈 값이다.
func (h *handler) identity(ctx context.Context, s auth.Session, ticket string) (files.Identity, error) {
	if id, ok := h.files.Identity(s.ID); ok {
		return id, nil
	}
	id, err := h.Helper.Identity(ctx, ticket, proto.ScopeUser)
	if err != nil {
		return files.Identity{}, err
	}
	out := files.Identity{User: id.User}
	if files.ValidPath(id.Home) {
		out.Home = id.Home
	}
	for _, g := range id.Groups {
		out.Groups = append(out.Groups, g.Name)
	}
	h.files.SetIdentity(s.ID, out)
	return out, nil
}

// fileRoots는 s에게 보이는 폴더(응답)와, 공유 폴더 권한 따르기로 숨기거나 읽기 전용으로 둔 공유 폴더의 경로다.
// hidden은 디스크 최상위 보기에서도 들어갈 수 없고, readOnly 안에는 쓸 수 없다. 둘은 브라우저에 보내지 않는다.
type fileRoots struct {
	api.FilesRoots
	hidden   []string
	readOnly []string
}

// roots는 s에게 보이는 폴더다. 디스크는 스냅숏이 없을 때(대체) 모두에게, 관리자가 디스크 최상위 보기를 켰을 때
// 관리자에게 보인다. 공유 폴더 권한 따르기를 켰으면(스냅숏이 있을 때만) 권한이 없는 공유 폴더를 빼고, 읽기만
// 허락된 공유 폴더를 읽기 전용으로 둔다. 관리자도 자기 계정의 권한을 따른다.
func (h *handler) roots(ctx context.Context, s auth.Session, ticket string) (fileRoots, error) {
	id, err := h.identity(ctx, s, ticket)
	if err != nil {
		return fileRoots{}, err
	}
	site := h.siteFiles()
	out := fileRoots{FilesRoots: api.FilesRoots{Shares: []api.FilesShare{}, Disks: []api.FilesRoot{}, SharesUpdatedAt: site.SharesUpdatedAt}}
	if id.Home != "" {
		out.Home = &api.FilesRoot{Name: path.Base(id.Home), Path: id.Home}
	}
	follow := site.FollowPrivileges && site.SharesUpdatedAt != ""
	out.FollowPrivileges = follow
	for _, sh := range site.Shares {
		access := files.AccessWrite
		if follow {
			access = files.ShareAccess(sh.Privileges, id.User, id.Groups)
		}
		if access == files.AccessNone {
			out.hidden = append(out.hidden, sh.Path)
			continue
		}
		if access == files.AccessRead {
			out.readOnly = append(out.readOnly, sh.Path)
		}
		out.Shares = append(out.Shares, api.FilesShare{UUID: sh.UUID, Name: sh.Name, Path: sh.Path,
			Reachable: files.Reachable(sh.Path, h.filesSrvDir()), ReadOnly: access == files.AccessRead})
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
func (h *handler) allowedPaths(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string, ps ...string) (fileRoots, bool) {
	for _, p := range ps {
		if !files.ValidPath(p) {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "path must be an absolute, clean path")
			return fileRoots{}, false
		}
	}
	roots, err := h.roots(r.Context(), s, ticket)
	if err != nil {
		h.writeFilesError(w, s, err)
		return fileRoots{}, false
	}
	for _, p := range ps {
		if !withinRoots(roots, p) {
			writeError(w, http.StatusForbidden, api.ErrOutsideRoots, "path is outside the folders the file explorer shows")
			return fileRoots{}, false
		}
	}
	return roots, true
}

// allowedWritePaths는 allowedPaths에 더해 ps가 모두 쓸 수 있는 공유 폴더 안인지 확인한다(공유 폴더 권한 따르기의
// 읽기 전용). 쓰기 라우트가 바꿀 곳(만들 곳, 옮기거나 지울 항목, 복사·되돌리기 대상)을 넘긴다.
func (h *handler) allowedWritePaths(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string, ps ...string) (fileRoots, bool) {
	roots, ok := h.allowedPaths(w, r, s, ticket, ps...)
	if !ok {
		return roots, false
	}
	for _, p := range ps {
		if readOnlyAt(roots, p) {
			writeError(w, http.StatusForbidden, api.ErrShareReadOnly, "the shared folder privileges allow reading only")
			return fileRoots{}, false
		}
	}
	return roots, true
}

// withinRoots는 p가 보이는 폴더(홈, 닿을 수 있는 공유 폴더, 디스크) 가운데 하나의 안이고, 숨긴 공유 폴더(공유 폴더
// 권한 따르기) 안이 아닌지다. 디스크 최상위 보기에서는 숨긴 공유 폴더가 디스크 안에 있다.
func withinRoots(roots fileRoots, p string) bool {
	ok := roots.Home != nil && files.Within(p, roots.Home.Path)
	for _, sh := range roots.Shares {
		ok = ok || (sh.Reachable && files.Within(p, sh.Path))
	}
	for _, d := range roots.Disks {
		ok = ok || files.Within(p, d.Path)
	}
	for _, hp := range roots.hidden {
		ok = ok && !files.Within(p, hp)
	}
	return ok
}

// readOnlyAt은 p가 읽기만 허락된 공유 폴더(공유 폴더 권한 따르기) 안인지다.
func readOnlyAt(roots fileRoots, p string) bool {
	for _, rp := range roots.readOnly {
		if files.Within(p, rp) {
			return true
		}
	}
	return false
}

// rootFolder는 p가 보이는 폴더(홈, 공유 폴더, 디스크) 자신이거나 그것을 품은 폴더인지다. 이런 폴더는 이름을 바꾸거나
// 옮기거나 지우지 않는다. 권한 도우미는 자기 기준 디렉터리(홈, 마운트 지점)만 지키므로, 디스크 안의 공유 폴더는
// 여기서 지킨다(디스크 최상위 보기에서는 공유 폴더가 디스크의 한 폴더로 보인다).
func rootFolder(roots fileRoots, p string) bool {
	// 숨긴 공유 폴더도 디스크의 한 폴더라 지킨다(그것을 품은 디스크 폴더를 지우면 함께 지워진다).
	paths := slices.Clone(roots.hidden)
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
