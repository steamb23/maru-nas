package omvfake

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
)

// 서비스 설정과 적용. 형식은 OMV 8의 engined/rpc/{ssh,smb,nfs,rsyncd,config}.inc를 따른다.
//
//   - SSH.get, SMB.getSettings, NFS.getSettings, Rsyncd.getSettings (admin): 설정 객체. NFS는 shares도 준다
//   - SSH.set 등 (admin): set 스키마의 속성이 모두 있어야 한다. 가짜 OMV는 모르는 속성도 거절한다(실제보다 엄격,
//     백엔드가 get 결과를 그대로 넘기지 않는지 확인하기 위해서다). 저장하면 그 모듈을 적용 대기(dirty)로 표시한다
//   - Config.isDirty (admin): 적용 대기 모듈이 있는지
//   - Config.applyChangesBg (admin): 백그라운드 작업. 끝나면 적용 대기를 비우고 Services.getStatus의 enabled·running을
//     설정의 enable로 바꾼다. FailApply면 실패한다

// serviceRPC는 RPC 서비스 이름 → 모듈 이름, get·set 메서드, set 스키마의 속성이다.
var serviceRPC = map[string]struct {
	module, get, set string
	fields           []string
}{
	"SSH": {"ssh", "get", "set", []string{"enable", "port", "passwordauthentication", "pubkeyauthentication",
		"permitrootlogin", "tcpforwarding", "compression", "extraoptions"}},
	"SMB": {"samba", "getSettings", "setSettings", []string{"enable", "workgroup", "serverstring", "loglevel",
		"usesendfile", "aio", "timeserver", "winssupport", "winsserver", "homesenable", "homesbrowseable", "extraoptions"}},
	"NFS":    {"nfs", "getSettings", "setSettings", []string{"enable", "versions"}},
	"Rsyncd": {"rsyncd", "getSettings", "setSettings", []string{"enable", "port", "extraoptions"}},
}

// defaultServiceSettings는 모듈 이름 → 설정 객체(get 응답)다. enable은 DefaultPluginData의 Services와 맞춘다.
func defaultServiceSettings() map[string]map[string]any {
	return map[string]map[string]any{
		"ssh": {"enable": true, "port": 22, "passwordauthentication": true, "pubkeyauthentication": true,
			"permitrootlogin": false, "tcpforwarding": false, "compression": false, "extraoptions": ""},
		"samba": {"enable": true, "workgroup": "WORKGROUP", "serverstring": "%h server", "loglevel": 0,
			"usesendfile": true, "aio": true, "timeserver": false, "winssupport": false, "winsserver": "",
			"homesenable": false, "homesbrowseable": true, "extraoptions": ""},
		"nfs":    {"enable": false, "versions": []string{"3", "4", "4.1", "4.2"}, "shares": map[string]any{"share": []any{}}},
		"rsyncd": {"enable": false, "port": 873, "extraoptions": ""},
	}
}

// handleService는 서비스 설정·적용 RPC를 처리한다. 모르는 메서드면 ok가 false다. s.mu를 쥔 채로 부른다.
func (s *Server) handleService(service, method string, params json.RawMessage) (any, *rpcError, bool) {
	switch service + "." + method {
	case "Config.isDirty":
		return len(s.plugin.Dirty) > 0, nil, true
	case "Config.applyChangesBg":
		var p struct {
			Modules []string `json:"modules"`
			Force   *bool    `json:"force"`
		}
		if json.Unmarshal(params, &p) != nil || p.Modules == nil || p.Force == nil {
			return nil, errStatus(http.StatusInternalServerError, "Invalid RPC parameters."), true
		}
		if s.plugin.FailApply {
			return s.startBg("Failed to execute command 'salt-call': fake failure", nil), nil, true
		}
		return s.startBg("", s.applyServices), nil, true
	}
	rpc, ok := serviceRPC[service]
	if !ok {
		return nil, nil, false
	}
	switch method {
	case rpc.get:
		return maps.Clone(s.plugin.ServiceSettings[rpc.module]), nil, true
	case rpc.set:
		var p map[string]json.RawMessage
		if json.Unmarshal(params, &p) != nil || len(p) != len(rpc.fields) {
			return nil, errStatus(http.StatusInternalServerError, "Invalid RPC parameters."), true
		}
		next := maps.Clone(s.plugin.ServiceSettings[rpc.module])
		for _, f := range rpc.fields {
			raw, ok := p[f]
			if !ok {
				return nil, errStatus(http.StatusInternalServerError, "The property '"+f+"' is required."), true
			}
			var v any
			json.Unmarshal(raw, &v)
			next[f] = v
		}
		if _, ok := next["enable"].(bool); !ok {
			return nil, errStatus(http.StatusInternalServerError, "The property 'enable' must be a boolean."), true
		}
		// SetPluginData가 넘긴 맵을 바꾸지 않게 복사한다.
		s.plugin.ServiceSettings = maps.Clone(s.plugin.ServiceSettings)
		s.plugin.ServiceSettings[rpc.module] = next
		if !slices.Contains(s.plugin.Dirty, rpc.module) {
			s.plugin.Dirty = append(slices.Clone(s.plugin.Dirty), rpc.module)
		}
		return next, nil, true
	}
	return nil, nil, false
}

// applyServices는 적용이 끝났을 때 서비스 상태를 설정에 맞춘다. s.mu를 쥔 채로 부른다.
func (s *Server) applyServices() {
	list := slices.Clone(s.plugin.Services)
	for i, st := range list {
		if cfg, ok := s.plugin.ServiceSettings[st.Name]; ok {
			on, _ := cfg["enable"].(bool)
			list[i].Enabled, list[i].Running = on, on
		}
	}
	s.plugin.Services = list
	s.plugin.Dirty = nil
}
