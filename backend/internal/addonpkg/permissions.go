package addonpkg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"maru/backend/internal/addonpkg/format"
)

// 검토 화면이 보이는 위험 코드(Summary.Risks).
const (
	// RiskOMVWrite는 OMV 설정을 바꾸는 메서드를 선언했다는 뜻이다. OMV 관리자 쓰기는 root에 준한다.
	RiskOMVWrite = "omv_write"
	// RiskBackend는 서버에서 도는 코드(패키지 백엔드)가 있다는 뜻이다.
	RiskBackend = "backend"
	// RiskBackendSharedData는 백엔드가 OMV를 부르고 모든 사용자가 같은 백엔드를 쓴다는 뜻이다. 관리자 세션으로
	// 읽은 값을 일반 사용자에게 보여 줄 수 있다.
	RiskBackendSharedData = "backend_shared_data"
	// RiskNetLAN은 백엔드가 LAN(사설 주소)에 닿는다는 뜻이다.
	RiskNetLAN = "net_lan"
	// RiskNetLoopback은 백엔드가 NAS 자신의 루프백 서비스에 닿는다는 뜻이다.
	RiskNetLoopback = "net_loopback"
)

// Summary는 패키지가 선언한 권한 요약이다. 검토 화면과 설치 기록의 grant에 쓴다.
type Summary struct {
	Admin   bool     `json:"admin"`
	Plugins []string `json:"plugins"`
	Read    []string `json:"read"`
	Write   []string `json:"write"`
	Actions []string `json:"actions"`
	Backend bool     `json:"backend"`
	Network []string `json:"network"`
	Risks   []string `json:"risks"`
}

// Summarize는 manifest의 권한 요약이다. 목록은 정렬해 둔다(같은 권한이면 같은 digest).
func Summarize(m *format.Manifest) Summary {
	s := Summary{
		Admin:   m.Requires.Admin,
		Plugins: []string{},
		Read:    sortedCopy(m.RPC.Read),
		Write:   sortedCopy(m.RPC.Write),
		Actions: []string{},
		Backend: m.HasBackend(),
		Network: []string{},
		Risks:   []string{},
	}
	for _, p := range m.Requires.Plugins {
		s.Plugins = append(s.Plugins, p.Package)
	}
	slices.Sort(s.Plugins)
	for _, a := range m.Actions {
		s.Actions = append(s.Actions, a.Name)
	}
	slices.Sort(s.Actions)
	if m.Backend != nil {
		s.Network = sortedCopy(m.Backend.Network)
	}
	if len(s.Write) > 0 {
		s.Risks = append(s.Risks, RiskOMVWrite)
	}
	if s.Backend {
		s.Risks = append(s.Risks, RiskBackend)
		if len(s.Read)+len(s.Write) > 0 {
			s.Risks = append(s.Risks, RiskBackendSharedData)
		}
	}
	if slices.Contains(s.Network, format.NetworkLAN) {
		s.Risks = append(s.Risks, RiskNetLAN)
	}
	if slices.Contains(s.Network, format.NetworkLoopback) {
		s.Risks = append(s.Risks, RiskNetLoopback)
	}
	return s
}

// Elevated는 설치·켜기·조작에 단계 확인(비밀번호 재확인 이상)이 필요한 권한인지다.
func (s Summary) Elevated() bool { return len(s.Write) > 0 || s.Backend }

// Digest는 요약의 sha256이다. 설치 기록의 grant와 업데이트 때의 비교에 쓴다.
func (s Summary) Digest() string {
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// SummaryDiff는 업데이트할 때 더해지거나 빠지는 권한이다.
type SummaryDiff struct {
	AdminAdded     bool     `json:"adminAdded"`
	AdminRemoved   bool     `json:"adminRemoved"`
	AddedPlugins   []string `json:"addedPlugins"`
	AddedRead      []string `json:"addedRead"`
	RemovedRead    []string `json:"removedRead"`
	AddedWrite     []string `json:"addedWrite"`
	RemovedWrite   []string `json:"removedWrite"`
	AddedActions   []string `json:"addedActions"`
	BackendAdded   bool     `json:"backendAdded"`
	BackendRemoved bool     `json:"backendRemoved"`
	AddedNetwork   []string `json:"addedNetwork"`
	RemovedNetwork []string `json:"removedNetwork"`
}

// Diff는 old에서 new로 바뀌는 권한이다.
func Diff(old, new Summary) SummaryDiff {
	return SummaryDiff{
		AdminAdded:     new.Admin && !old.Admin,
		AdminRemoved:   old.Admin && !new.Admin,
		AddedPlugins:   minus(new.Plugins, old.Plugins),
		AddedRead:      minus(new.Read, old.Read),
		RemovedRead:    minus(old.Read, new.Read),
		AddedWrite:     minus(new.Write, old.Write),
		RemovedWrite:   minus(old.Write, new.Write),
		AddedActions:   minus(new.Actions, old.Actions),
		BackendAdded:   new.Backend && !old.Backend,
		BackendRemoved: old.Backend && !new.Backend,
		AddedNetwork:   minus(new.Network, old.Network),
		RemovedNetwork: minus(old.Network, new.Network),
	}
}

// Widens는 업데이트가 권한을 넓히는지다(검토 화면이 새 권한을 강조한다).
func (d SummaryDiff) Widens() bool {
	return d.AdminRemoved || len(d.AddedPlugins)+len(d.AddedRead)+len(d.AddedWrite)+len(d.AddedActions)+len(d.AddedNetwork) > 0 ||
		d.BackendAdded
}

func sortedCopy(in []string) []string {
	out := slices.Clone(in)
	if out == nil {
		out = []string{}
	}
	slices.Sort(out)
	return out
}

// minus는 a에만 있는 값이다(a, b는 정렬됨).
func minus(a, b []string) []string {
	out := []string{}
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}
