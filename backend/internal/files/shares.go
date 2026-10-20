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
		out = append(out, api.SharedFolder{UUID: s.UUID, Name: s.Name, Path: p})
	}
	slices.SortFunc(out, func(a, b api.SharedFolder) int { return CompareNames(a.Name, b.Name) })
	return out, nil
}
