package containers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"omv-desktop-dashboard/backend/internal/addons"
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

// fakeRPC는 컨테이너 조작이 부르는 RPC를 흉내 내고 호출을 기록한다.
type fakeRPC struct {
	calls []string
	cmd   map[string]string
}

func (f *fakeRPC) Call(_ context.Context, service, method string, params any) (json.RawMessage, error) {
	name := service + "." + method
	f.calls = append(f.calls, name)
	switch name {
	case "Compose.getContainers":
		return json.RawMessage(`[
			{"image": "", "name": "Docker", "status": "running"},
			{"image": "example/web:1.0", "name": "web", "status": "Up 3 days"},
			{"image": "example/db:16", "name": "my_db-1.x", "status": "Exited (0) 1 hour ago"}
		]`), nil
	case "Compose.doContainerCommand":
		f.cmd = params.(map[string]string)
		return json.RawMessage(`"bgstatus1"`), nil
	case "Exec.isRunning":
		return json.RawMessage(`{"filename": "bgstatus1", "running": false}`), nil
	}
	return nil, fmt.Errorf("unexpected rpc %s", name)
}

func TestActions(t *testing.T) {
	a := New()
	if _, err := addons.NewRegistry(a); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"start", "stop", "restart"} {
		for _, target := range []string{"web", "my_db-1.x"} {
			f := &fakeRPC{}
			err := a.Actions[action].Run(context.Background(), addons.ActionRequest{Admin: true, Target: target, RPC: f})
			if err != nil {
				t.Fatalf("%s %s: %v", action, target, err)
			}
			want := map[string]string{"command": action, "command2": "", "id": target}
			if fmt.Sprint(f.cmd) != fmt.Sprint(want) {
				t.Errorf("%s %s: params %v", action, target, f.cmd)
			}
			if fmt.Sprint(f.calls) != "[Compose.getContainers Compose.doContainerCommand Exec.isRunning]" {
				t.Errorf("%s %s: calls %v", action, target, f.calls)
			}
		}
	}
}

func TestActionRejectsBadTargets(t *testing.T) {
	stop := New().Actions["stop"].Run
	for _, target := range []string{"", "-rf", " web", "web ", "web;reboot", "web$(id)", "a/b", "Docker", "missing",
		"x" + string(make([]byte, 128))} {
		f := &fakeRPC{}
		err := stop(context.Background(), addons.ActionRequest{Admin: true, Target: target, RPC: f})
		if !errors.Is(err, addons.ErrInvalidTarget) {
			t.Errorf("%q: err %v", target, err)
		}
		if f.cmd != nil {
			t.Errorf("%q: doContainerCommand called", target)
		}
	}
}
