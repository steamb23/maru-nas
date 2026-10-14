package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"testing"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
)

func TestParseStatus(t *testing.T) {
	raw := json.RawMessage(`{"total": 4, "data": [
		{"name": "ssh", "title": "SSH", "enabled": true, "running": true},
		{"name": "nfs", "title": "NFS", "enabled": false, "running": false},
		{"name": "fake-plugin", "title": "", "enabled": 1, "running": "0"},
		{"name": "", "title": "broken", "enabled": true, "running": true}
	]}`)
	res, err := ParseStatus(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sources) != 1 || res.Sources[0] != Source {
		t.Errorf("sources %v", res.Sources)
	}
	got := res.Services
	if len(got) != 3 || got[0].Name != "fake-plugin" || got[1].Name != "nfs" || got[2].Name != "ssh" {
		t.Fatalf("services %+v", got)
	}
	// 제목이 없으면 이름을 쓰고, 숫자·문자열 참거짓도 읽는다.
	if got[0].Title != "fake-plugin" || !got[0].Enabled || got[0].Running {
		t.Errorf("fake-plugin %+v", got[0])
	}
	if !got[2].Enabled || !got[2].Running || got[1].Enabled {
		t.Errorf("ssh/nfs %+v %+v", got[2], got[1])
	}

	res, err = ParseStatus(json.RawMessage(`{"total": 0, "data": []}`))
	if err != nil || res.Services == nil || len(res.Services) != 0 {
		t.Errorf("empty %+v %v", res, err)
	}
	if _, err := ParseStatus(json.RawMessage(`[1]`)); err == nil {
		t.Error("want error")
	}
}

// manifest의 RPC 목록이 어댑터 표와 맞는지 확인한다.
func TestManifestCoversAdapters(t *testing.T) {
	if _, err := addons.NewRegistry(New()); err != nil {
		t.Fatal(err)
	}
	for name, a := range adapters {
		if !slices.Contains(Manifest.RPC.Read, a.service+"."+a.get) || !slices.Contains(Manifest.RPC.Write, a.service+"."+a.set) {
			t.Errorf("%s: %s.%s/%s not declared", name, a.service, a.get, a.set)
		}
		if !slices.Contains(a.fields, "enable") {
			t.Errorf("%s: fields without enable", name)
		}
	}
}

// fakeRPC는 켜기·끄기가 부르는 RPC를 흉내 내고 호출을 기록한다.
type fakeRPC struct {
	dirty    bool
	settings map[string]any
	setErr   error
	calls    []string
	set      map[string]json.RawMessage
}

func (f *fakeRPC) Call(_ context.Context, service, method string, params any) (json.RawMessage, error) {
	name := service + "." + method
	f.calls = append(f.calls, name)
	switch {
	case name == "Config.isDirty":
		return json.Marshal(f.dirty)
	case name == "Config.applyChangesBg":
		if fmt.Sprint(params) != "map[force:false modules:[]]" {
			return nil, fmt.Errorf("apply params %v", params)
		}
		return json.RawMessage(`"bgstatus1"`), nil
	case name == "Exec.isRunning":
		return json.RawMessage(`{"filename": "bgstatus1", "running": false}`), nil
	case method == "get" || method == "getSettings":
		return json.Marshal(f.settings)
	case method == "set" || method == "setSettings":
		f.set = params.(map[string]json.RawMessage)
		return json.RawMessage(`{}`), f.setErr
	}
	return nil, fmt.Errorf("unexpected rpc %s", name)
}

func run(t *testing.T, action, target string, f *fakeRPC) error {
	t.Helper()
	return New().Actions[action].Run(context.Background(), addons.ActionRequest{Admin: true, Target: target, RPC: f})
}

func TestToggle(t *testing.T) {
	// NFS.getSettings는 set이 받지 않는 shares도 준다.
	f := &fakeRPC{settings: map[string]any{"enable": false, "versions": []string{"3", "4"}, "shares": map[string]any{}}}
	if err := run(t, "enable", "nfs", f); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(f.calls) != "[Config.isDirty NFS.getSettings NFS.setSettings Config.applyChangesBg Exec.isRunning]" {
		t.Errorf("calls %v", f.calls)
	}
	keys := make([]string, 0, len(f.set))
	for k := range f.set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if fmt.Sprint(keys) != "[enable versions]" || string(f.set["enable"]) != "true" || string(f.set["versions"]) != `["3","4"]` {
		t.Errorf("set params %v", f.set)
	}

	f = &fakeRPC{settings: map[string]any{"enable": true, "port": 873, "extraoptions": ""}}
	if err := run(t, "disable", "rsyncd", f); err != nil || string(f.set["enable"]) != "false" || string(f.set["port"]) != "873" {
		t.Errorf("disable rsyncd: %v %v", err, f.set)
	}
}

func TestToggleSkipsAndRejects(t *testing.T) {
	// 이미 꺼져 있으면 바꾸지 않는다.
	f := &fakeRPC{settings: map[string]any{"enable": false, "port": 873, "extraoptions": ""}}
	if err := run(t, "disable", "rsyncd", f); err != nil || f.set != nil {
		t.Errorf("already disabled: %v %v", err, f.calls)
	}

	// 적용하지 않은 변경이 있으면 아무것도 바꾸지 않는다.
	f = &fakeRPC{dirty: true, settings: map[string]any{"enable": true}}
	var ae *addons.ActionError
	if err := run(t, "disable", "ssh", f); !errors.As(err, &ae) || ae.Code != api.ErrConfigDirty || len(f.calls) != 1 {
		t.Errorf("dirty: %v %v", err, f.calls)
	}

	// 표에 없는 서비스는 OMV에 묻지 않는다.
	for _, target := range []string{"", "fail2ban", "Docker", "SSH"} {
		f = &fakeRPC{}
		if err := run(t, "enable", target, f); !errors.Is(err, addons.ErrInvalidTarget) || len(f.calls) != 0 {
			t.Errorf("%q: %v %v", target, err, f.calls)
		}
	}

	// get 결과에 set 속성이 빠져 있으면 보내지 않는다.
	f = &fakeRPC{settings: map[string]any{"enable": true}}
	if err := run(t, "disable", "ssh", f); err == nil || f.set != nil {
		t.Errorf("missing field: %v", err)
	}

	// OMV가 설정을 거절하면 실패다.
	f = &fakeRPC{setErr: errors.New("hostname too long"), settings: map[string]any{"enable": false, "versions": []string{}}}
	if err := run(t, "enable", "nfs", f); !errors.Is(err, addons.ErrActionFailed) {
		t.Errorf("set error: %v", err)
	}
}
