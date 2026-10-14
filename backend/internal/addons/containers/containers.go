// Package containers는 컨테이너 애드온이다. openmediavault-compose 플러그인의 Compose.getContainers로
// 컨테이너 목록을 가져온다. Docker·Podman 소켓은 직접 읽지 않는다(docs/data-sources.md).
package containers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
)

// ID는 컨테이너 애드온의 id다.
const ID = "containers"

// Source는 provider 이름이다.
const Source = "omv-compose"

const composePlugin = "openmediavault-compose"

// Manifest는 컨테이너 애드온의 선언이다. Compose.getContainers는 모든 사용자가 부를 수 있고,
// 시작·정지·재시작(Compose.doContainerCommand, Exec.isRunning)은 관리자만 한다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.containers",
	Icon:  "preferences-virtualization-container",
	Views: api.AddonViews{
		// 크기는 브라우저 실측으로 정한다. 내용을 바꾸면 다시 잰다.
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 520, Height: 440},
			MinSize:     &api.Size{Width: 360, Height: 240},
		},
		Widget: &api.WidgetView{
			DefaultSize: api.GridSize{Cols: 3, Rows: 2},
			MinSize:     &api.GridSize{Cols: 2, Rows: 1},
			MaxSize:     &api.GridSize{Cols: 4, Rows: 5},
		},
	},
	Requires: api.Requirements{Plugins: []string{composePlugin}},
	RPC: api.RPCMethods{
		Read:  []string{"Compose.getContainers", "Exec.isRunning"},
		Write: []string{"Compose.doContainerCommand"},
	},
	Actions: []api.ActionDecl{
		{Name: "start", Admin: true},
		{Name: "stop", Admin: true, Confirm: true},
		{Name: "restart", Admin: true, Confirm: true},
	},
}

// actionTimeout은 조작 하나를 기다리는 시간이다. docker stop은 컨테이너의 종료 유예 시간(기본 10초,
// compose의 stop_grace_period로 늘릴 수 있음)만큼 걸린다.
const actionTimeout = 2 * time.Minute

type compose struct{}

func (compose) Name() string { return Source }
func (compose) Requires() api.Requirements {
	return api.Requirements{Plugins: []string{composePlugin}}
}

// New는 컨테이너 애드온을 반환한다.
func New() addons.Addon {
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{compose{}},
		Fetch: func(ctx context.Context, req addons.Request) (any, error) {
			raw, err := req.RPC.Call(ctx, "Compose", "getContainers", nil)
			if err != nil {
				return nil, err
			}
			return ParseContainers(raw)
		},
		Actions: map[string]addons.Action{
			"start":   {Timeout: actionTimeout, Run: command("start")},
			"stop":    {Timeout: actionTimeout, Run: command("stop")},
			"restart": {Timeout: actionTimeout, Run: command("restart")},
		},
	}
}

// namePattern은 docker 컨테이너 이름 형식이다(docker의 [a-zA-Z0-9][a-zA-Z0-9_.-]).
// '-'로 시작하는 값(옵션으로 읽힘), 공백·셸 특수문자를 막는다.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// command는 docker <cmd> <컨테이너 이름>을 실행하는 조작이다.
//
// Compose.doContainerCommand는 파라미터를 검사하지 않고 "docker command command2 id"를 이스케이프 없이
// 셸에서 실행한다(compose.inc, OMV\System\Process::getCommandLine). 그래서 command는 고정값만 쓰고,
// id는 형식을 검사한 뒤 지금 목록에 있는 이름인지까지 확인한다. command2는 workbench처럼 빈 문자열로
// 보낸다(빠지면 PHP 경고). 명령은 OMV 백그라운드 작업으로 돌므로 끝날 때까지 기다린다.
func command(cmd string) func(context.Context, addons.ActionRequest) error {
	return func(ctx context.Context, req addons.ActionRequest) error {
		name := req.Target
		if !namePattern.MatchString(name) {
			return addons.ErrInvalidTarget
		}
		raw, err := req.RPC.Call(ctx, "Compose", "getContainers", nil)
		if err != nil {
			return err
		}
		list, err := ParseContainers(raw)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(list.Containers, func(c api.Container) bool { return c.Name == name }) {
			return addons.ErrInvalidTarget
		}
		var filename string
		params := map[string]string{"command": cmd, "command2": "", "id": name}
		if err := addons.CallInto(ctx, req.RPC, "Compose", "doContainerCommand", params, &filename); err != nil {
			return err
		}
		return addons.WaitBg(ctx, req.RPC, filename, addons.BgPollInterval)
	}
}

type row struct {
	Image  string `json:"image"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ParseContainers는 Compose.getContainers 응답을 읽는다.
//
// 첫 행은 컨테이너가 아니라 런타임 행이다: name이 Docker 또는 Podman이고 status는 런타임 서비스 상태를
// gettext로 번역한 문구다. 런타임 종류는 번역되지 않는 name으로만 판단한다. 나머지 행은
// docker container ls --all의 이미지·이름·상태다.
func ParseContainers(raw json.RawMessage) (api.ContainersResponse, error) {
	var rows []row
	if err := json.Unmarshal(raw, &rows); err != nil {
		return api.ContainersResponse{}, fmt.Errorf("compose containers: %w", err)
	}
	res := api.ContainersResponse{
		AddonSources: api.AddonSources{Sources: []string{Source}},
		Containers:   []api.Container{},
	}
	if len(rows) == 0 {
		return res, nil
	}
	switch rows[0].Name {
	case "Docker":
		res.Runtime = "docker"
	case "Podman":
		res.Runtime = "podman"
	}
	res.RuntimeStatus = rows[0].Status
	for _, r := range rows[1:] {
		state, health := ParseStatus(r.Status)
		res.Containers = append(res.Containers, api.Container{
			Name: r.Name, Image: r.Image, State: state, Health: health, Status: r.Status,
		})
	}
	slices.SortFunc(res.Containers, func(a, b api.Container) int { return strings.Compare(a.Name, b.Name) })
	return res, nil
}

// ParseStatus는 docker container ls의 STATUS 문구에서 상태와 헬스체크 결과를 읽는다.
// 이 문구는 docker CLI가 만들며 OMV 언어 설정과 상관없이 영어다.
//
//	Up 3 days (healthy)            → running, healthy
//	Up 2 hours (Paused)            → paused
//	Up 5 seconds (health: starting) → running, starting
//	Exited (0) 2 hours ago         → exited
//	Restarting (1) 3 seconds ago   → restarting
//	Created, Dead, Removal In Progress
func ParseStatus(status string) (state, health string) {
	s := strings.TrimSpace(status)
	switch {
	case strings.HasPrefix(s, "Up"):
		state = "running"
		if strings.Contains(s, "(Paused)") {
			state = "paused"
		}
	case strings.HasPrefix(s, "Exited"):
		state = "exited"
	case strings.HasPrefix(s, "Restarting"):
		state = "restarting"
	case strings.HasPrefix(s, "Created"):
		state = "created"
	case strings.HasPrefix(s, "Dead"):
		state = "dead"
	case strings.HasPrefix(s, "Removal"):
		state = "removing"
	default:
		state = "unknown"
	}
	switch {
	case strings.Contains(s, "(unhealthy)"):
		health = "unhealthy"
	case strings.Contains(s, "(healthy)"):
		health = "healthy"
	case strings.Contains(s, "(health: starting)"):
		health = "starting"
	}
	return state, health
}
