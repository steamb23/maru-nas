package services

import (
	"encoding/json"
	"testing"
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
