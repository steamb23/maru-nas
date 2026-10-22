package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/files"
	"maru/backend/internal/helper/proto"
	"maru/backend/internal/omv/omvfake"
)

func TestFilesFollowPrivileges(t *testing.T) {
	e := newFilesEnv(t, envOptions{})
	e.fake.SetVersion(omvfake.Version85)
	d := omvfake.DefaultPluginData()
	d.SharedFolders = []omvfake.SharedFolder{
		// user는 읽기만, admin은 읽기·쓰기다.
		{UUID: "u-media", Name: "media", MountDir: e.disk, RelDirPath: "media/", Privileges: []omvfake.Privilege{
			{Type: "user", Name: "user", Perms: 5}, {Type: "user", Name: "admin", Perms: 7}}},
		// users 그룹은 거부다(admin도 users 그룹이다).
		{UUID: "u-docs", Name: "docs", MountDir: e.disk, RelDirPath: "docs/", Privileges: []omvfake.Privilege{
			{Type: "group", Name: "users", Perms: 0}}},
		// 정한 권한이 없으면 모두에게 열린다.
		{UUID: "u-open", Name: "open", MountDir: e.disk, RelDirPath: "open/"},
	}
	e.fake.SetPluginData(d)
	e.file("media/a.txt", "a")
	e.file("docs/secret.txt", "s")
	e.file("open/c.txt", "c")
	admin := e.browser()
	admin.login("admin", "admin")
	decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	user := e.browser()
	user.login("user", "user")
	media, docs := e.disk+"/media", e.disk+"/docs"

	// 끄면 권한을 보지 않는다.
	if r := decode[api.FilesRoots](t, user.do(http.MethodGet, "/api/files/roots", nil)); len(r.Shares) != 3 {
		t.Fatalf("roots without privileges = %+v", r)
	}

	wantStatus(t, user.do(http.MethodPut, "/api/site/files", api.FilesSettingsRequest{FollowPrivileges: new(true)}), http.StatusForbidden)
	settings := decode[api.FilesSettings](t, admin.do(http.MethodPut, "/api/site/files", api.FilesSettingsRequest{FollowPrivileges: new(true)}))
	if !settings.FollowPrivileges || settings.ShowDisks {
		t.Fatalf("settings = %+v", settings)
	}

	roots := decode[api.FilesRoots](t, user.do(http.MethodGet, "/api/files/roots", nil))
	if len(roots.Shares) != 2 || roots.Shares[0].Name != "media" || !roots.Shares[0].ReadOnly || roots.Shares[1].ReadOnly {
		t.Fatalf("user roots = %+v", roots)
	}
	// 숨긴 공유 폴더는 열 수 없다.
	wantCode(t, user.do(http.MethodGet, "/api/files/list?path="+q(docs), nil), http.StatusForbidden, api.ErrOutsideRoots)
	wantCode(t, user.do(http.MethodGet, "/api/files/download?path="+q(docs+"/secret.txt"), nil), http.StatusForbidden, api.ErrOutsideRoots)

	// 읽기 전용 공유 폴더는 읽을 수 있지만 쓸 수 없다.
	l := decode[api.FilesListing](t, user.do(http.MethodGet, "/api/files/list?path="+q(media), nil))
	if l.Writable || !l.ShareReadOnly {
		t.Fatalf("listing = %+v", l)
	}
	if info := decode[api.FileInfo](t, user.do(http.MethodGet, "/api/files/info?path="+q(media+"/a.txt"), nil)); !info.ShareReadOnly ||
		info.Access == nil || info.Access.Write {
		t.Fatalf("info = %+v", info)
	}
	wantStatus(t, user.do(http.MethodGet, "/api/files/download?path="+q(media+"/a.txt"), nil), http.StatusOK)
	for name, rec := range map[string]func() int{
		"mkdir": func() int {
			return user.do(http.MethodPost, "/api/files/mkdir", api.FilesMkdirRequest{Path: media + "/new"}).Code
		},
		"upload check": func() int {
			return user.do(http.MethodPost, "/api/files/upload/check", api.FilesUploadCheckRequest{Path: media + "/b.txt"}).Code
		},
		"rename": func() int {
			return user.do(http.MethodPost, "/api/files/rename", api.FilesRenameRequest{From: media + "/a.txt", To: media + "/z.txt"}).Code
		},
		"move out": func() int {
			return user.do(http.MethodPost, "/api/files/rename", api.FilesRenameRequest{From: media + "/a.txt", To: e.disk + "/open/a.txt"}).Code
		},
		"delete": func() int {
			return user.do(http.MethodPost, "/api/files/delete", api.FilesDeleteRequest{Path: media + "/a.txt"}).Code
		},
		"trash": func() int {
			return user.do(http.MethodPost, "/api/files/trash", api.FilesTrashRequest{Path: media + "/a.txt"}).Code
		},
		"copy in": func() int {
			return user.do(http.MethodPost, "/api/files/copy", api.FilesCopyRequest{Dir: e.disk + "/open", Names: []string{"c.txt"}, To: media}).Code
		},
	} {
		if code := rec(); code != http.StatusForbidden {
			t.Errorf("%s: status %d", name, code)
		}
	}
	rec := user.raw(http.MethodPut, "/api/files/upload?path="+q(media+"/b.txt"), "application/octet-stream", []byte("b"))
	wantCode(t, rec, http.StatusForbidden, api.ErrShareReadOnly)

	// 관리자도 자기 계정의 권한을 따른다. 디스크 최상위 보기에서도 숨긴 공유 폴더 안은 열 수 없고 찾기에도 나오지 않는다.
	wantStatus(t, admin.do(http.MethodPut, "/api/site/files", api.FilesSettingsRequest{ShowDisks: new(true)}), http.StatusOK)
	if r := decode[api.FilesRoots](t, admin.do(http.MethodGet, "/api/files/roots", nil)); len(r.Shares) != 2 || r.Shares[0].ReadOnly || len(r.Disks) != 1 {
		t.Fatalf("admin roots = %+v", r)
	}
	wantStatus(t, admin.do(http.MethodGet, "/api/files/list?path="+q(e.disk), nil), http.StatusOK)
	wantCode(t, admin.do(http.MethodGet, "/api/files/list?path="+q(docs), nil), http.StatusForbidden, api.ErrOutsideRoots)
	wantStatus(t, admin.do(http.MethodPost, "/api/files/mkdir", api.FilesMkdirRequest{Path: media + "/admin"}), http.StatusOK)
	evs := searchEvents(t, admin.do(http.MethodGet, "/api/files/search?path="+q(e.disk)+"&q=t", nil))
	for _, ev := range evs {
		for _, h := range ev.Hits {
			if strings.HasPrefix(h.Path, docs) {
				t.Errorf("hidden share in search: %s", h.Path)
			}
		}
	}

	// 끄면 다시 보인다.
	wantStatus(t, admin.do(http.MethodPut, "/api/site/files", api.FilesSettingsRequest{FollowPrivileges: new(false)}), http.StatusOK)
	if r := decode[api.FilesRoots](t, user.do(http.MethodGet, "/api/files/roots", nil)); len(r.Shares) != 3 {
		t.Fatalf("roots after turning off = %+v", r)
	}
	if s := decode[api.FilesSettings](t, admin.do(http.MethodGet, "/api/site/files", nil)); s.FollowPrivileges || !s.ShowDisks {
		t.Fatalf("settings after turning off = %+v", s)
	}
}

// identityHelper는 신원만 답하는 도우미다(뿌리 계산을 도우미 없이 시험한다).
type identityHelper struct {
	Helper
	id proto.Identity
}

func (h identityHelper) Identity(context.Context, string, string) (proto.Identity, error) {
	return h.id, nil
}

func TestRootsFollowPrivileges(t *testing.T) {
	h := &handler{files: files.NewCache(), Config: Config{FilesSrvDir: "/srv",
		Helper: identityHelper{id: proto.Identity{User: "erin", Home: "/home/erin", Groups: []proto.Group{{Name: "users"}, {Name: "family"}}}}}}
	h.filesSite.v = api.SiteFiles{SharesUpdatedAt: "2026-10-21T00:00:00Z", FollowPrivileges: true, Shares: []api.SharedFolder{
		{UUID: "a", Name: "photos", Path: "/srv/d1/photos", Privileges: []api.SharePrivilege{{Type: "group", Name: "family", Perms: 5}}},
		{UUID: "b", Name: "private", Path: "/srv/d1/private", Privileges: []api.SharePrivilege{{Type: "user", Name: "frank", Perms: 7}}},
		{UUID: "c", Name: "open", Path: "/srv/d1/open"},
	}}
	s := auth.Session{ID: "s1", Username: "erin"}
	roots, err := h.roots(context.Background(), s, "ticket")
	if err != nil {
		t.Fatal(err)
	}
	if len(roots.Shares) != 2 || roots.Shares[0].Name != "photos" || !roots.Shares[0].ReadOnly || roots.Shares[1].ReadOnly || !roots.FollowPrivileges {
		t.Fatalf("shares = %+v", roots.Shares)
	}
	// 디스크 최상위 보기라도 숨긴 공유 폴더 안은 보이는 폴더가 아니고, 그것을 품은 폴더는 지킨다.
	roots.Disks = []api.FilesRoot{{Name: "d1", Path: "/srv/d1"}}
	for p, want := range map[string]bool{"/srv/d1/photos/a.jpg": true, "/srv/d1/private": false, "/srv/d1/private/x": false, "/srv/d1/other": true} {
		if withinRoots(roots, p) != want {
			t.Errorf("withinRoots(%s) = %t", p, !want)
		}
	}
	if !readOnlyAt(roots, "/srv/d1/photos/2024") || readOnlyAt(roots, "/srv/d1/open/x") || readOnlyAt(roots, "/srv/d1/photosx") {
		t.Fatal("readOnlyAt")
	}
	if !rootFolder(roots, "/srv/d1/private") || rootFolder(roots, "/srv/d1/other") {
		t.Fatal("rootFolder")
	}

	// 끄거나 스냅숏이 없으면 권한을 보지 않는다.
	h.filesSite.v.FollowPrivileges = false
	if r, _ := h.roots(context.Background(), s, "ticket"); len(r.Shares) != 3 || len(r.hidden) != 0 || len(r.readOnly) != 0 || r.FollowPrivileges {
		t.Fatalf("roots when off = %+v", r)
	}
	h.filesSite.v.FollowPrivileges, h.filesSite.v.SharesUpdatedAt = true, ""
	if r, _ := h.roots(context.Background(), s, "ticket"); len(r.hidden) != 0 || r.FollowPrivileges {
		t.Fatalf("roots without a snapshot = %+v", r)
	}
}
