package containers

import (
	"encoding/json"
	"testing"
)

func TestParseStatus(t *testing.T) {
	cases := []struct{ status, state, health string }{
		{"Up 3 days (healthy)", "running", "healthy"},
		{"Up 5 minutes (unhealthy)", "running", "unhealthy"},
		{"Up 5 seconds (health: starting)", "running", "starting"},
		{"Up 2 hours (Paused)", "paused", ""},
		{"Up About a minute", "running", ""},
		{"Exited (0) 5 hours ago", "exited", ""},
		{"Restarting (1) 10 seconds ago", "restarting", ""},
		{"Created", "created", ""},
		{"Dead", "dead", ""},
		{"Removal In Progress", "removing", ""},
		{"something new", "unknown", ""},
	}
	for _, c := range cases {
		state, health := ParseStatus(c.status)
		if state != c.state || health != c.health {
			t.Errorf("%q → %s/%s, want %s/%s", c.status, state, health, c.state, c.health)
		}
	}
}

func TestParseContainers(t *testing.T) {
	raw := json.RawMessage(`[
		{"image": "", "name": "Docker", "status": "실행 중"},
		{"image": "example/web:1.0", "name": "web", "status": "Up 3 days (healthy)"},
		{"image": "example/backup:latest", "name": "backup", "status": "Exited (0) 5 hours ago"}
	]`)
	res, err := ParseContainers(raw)
	if err != nil {
		t.Fatal(err)
	}
	// 번역된 런타임 상태 문구는 그대로 둔다.
	if res.Runtime != "docker" || res.RuntimeStatus != "실행 중" || len(res.Sources) != 1 {
		t.Errorf("runtime %q %q sources %v", res.Runtime, res.RuntimeStatus, res.Sources)
	}
	if len(res.Containers) != 2 || res.Containers[0].Name != "backup" || res.Containers[1].State != "running" ||
		res.Containers[1].Image != "example/web:1.0" {
		t.Errorf("containers %+v", res.Containers)
	}

	res, _ = ParseContainers(json.RawMessage(`[{"image": "", "name": "Podman", "status": "stopped"}]`))
	if res.Runtime != "podman" || len(res.Containers) != 0 {
		t.Errorf("podman %+v", res)
	}
	res, _ = ParseContainers(json.RawMessage(`[]`))
	if res.Runtime != "" || res.Containers == nil {
		t.Errorf("empty %+v", res)
	}
	if _, err := ParseContainers(json.RawMessage(`{"x":1}`)); err == nil {
		t.Error("want error")
	}
}
