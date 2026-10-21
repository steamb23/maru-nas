package files

import (
	"encoding/json"
	"path"
	"slices"

	"onmaru/backend/internal/api"
)

// omvShare는 ShareMgmt.enumerateSharedFolders 응답 항목에서 쓰는 필드다.
// absdirpath와 status는 OMV 8.1.0에서 생겼다. 8.0은 mntent.dir + reldirpath로 경로를 만든다.
type omvShare struct {
	UUID       string  `json:"uuid"`
	Name       string  `json:"name"`
	RelDirPath string  `json:"reldirpath"`
	AbsDirPath string  `json:"absdirpath"`
	Status     *string `json:"status"`
	MntEnt     struct {
		Dir string `json:"dir"`
	} `json:"mntent"`
	// Privileges는 OMV 설정 DB의 privileges다. 정한 권한이 없으면 privilege가 비었거나 없다.
	Privileges struct {
		Privilege []api.SharePrivilege `json:"privilege"`
	} `json:"privileges"`
}

// ParseShares는 ShareMgmt.enumerateSharedFolders 응답을 공유 폴더 스냅숏으로 바꾼다(이름순). 마운트 정보가 없거나
// 폴더가 없는 공유 폴더(status가 OK가 아님), 경로를 만들 수 없는 항목은 뺀다.
func ParseShares(raw json.RawMessage) ([]api.SharedFolder, error) {
	var in []omvShare
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	out := []api.SharedFolder{}
	for _, s := range in {
		if s.UUID == "" || s.Name == "" || (s.Status != nil && *s.Status != "OK") {
			continue
		}
		p := s.AbsDirPath
		if p == "" && s.MntEnt.Dir != "" {
			p = path.Join(s.MntEnt.Dir, s.RelDirPath)
		}
		if p == "" {
			continue
		}
		p = path.Clean(p)
		if !ValidPath(p) {
			continue
		}
		out = append(out, api.SharedFolder{UUID: s.UUID, Name: s.Name, Path: p, Privileges: privilegesOf(s.Privileges.Privilege)})
	}
	slices.SortFunc(out, func(a, b api.SharedFolder) int { return CompareNames(a.Name, b.Name) })
	return out, nil
}

// privilegesOf는 쓸 수 있는 권한 항목만 남긴다. 모르는 종류·값의 항목은 거부(0)로 둔다(모르는 권한으로 더 많이 열지 않게).
func privilegesOf(in []api.SharePrivilege) []api.SharePrivilege {
	var out []api.SharePrivilege
	for _, p := range in {
		if p.Name == "" || (p.Type != "user" && p.Type != "group") {
			continue
		}
		if p.Perms != api.SharePermsReadOnly && p.Perms != api.SharePermsReadWrite {
			p.Perms = api.SharePermsNone
		}
		out = append(out, p)
	}
	return out
}

// Access는 공유 폴더 권한이 한 사용자에게 허락하는 것이다.
type Access int

const (
	AccessNone Access = iota
	AccessRead
	AccessWrite
)

// ShareAccess는 OMV 공유 폴더 권한(privileges)을 사용자 user(속한 그룹 groups)에게 적용한 결과다. OMV가 Samba
// 설정을 만드는 규칙과 같다(srv/salt/omv/deploy/samba/files/shares.j2): 사용자 항목과 그룹 항목을 함께 보고,
// 거부(0)가 하나라도 맞으면 거부, 아니면 읽기·쓰기(7)가 있으면 쓰기, 읽기(5)만 있으면 읽기, 맞는 항목이 없으면
// 거부다. 정한 권한이 하나도 없는 공유 폴더는 Samba처럼 모두에게 열린다(파일 시스템 권한은 따로 적용된다).
func ShareAccess(privs []api.SharePrivilege, user string, groups []string) Access {
	if len(privs) == 0 {
		return AccessWrite
	}
	matched, read, write := false, false, false
	for _, p := range privs {
		if !(p.Type == "user" && p.Name == user) && !(p.Type == "group" && slices.Contains(groups, p.Name)) {
			continue
		}
		matched = true
		switch p.Perms {
		case api.SharePermsReadWrite:
			write = true
		case api.SharePermsReadOnly:
			read = true
		default:
			return AccessNone
		}
	}
	switch {
	case !matched:
		return AccessNone
	case write:
		return AccessWrite
	case read:
		return AccessRead
	}
	return AccessNone
}
