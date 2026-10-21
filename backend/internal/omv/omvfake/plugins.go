package omvfake

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// 코어·플러그인 RPC의 합성 응답. 형식은 OMV 8 소스를 따른다.
//
//   - Smart.getList (admin): engined/rpc/smart.inc. rpc.common.getlist 파라미터, {total, data}
//   - Compose.getContainers (모두): engined/rpc/compose.inc(openmediavault-compose).
//     첫 행은 런타임 행(name Docker/Podman, status는 번역된 문구), 나머지는 docker container ls
//   - CpuTemp.get (모두): engined/rpc/cputemp.inc(openmediavault-cputemp). {cputemp: 숫자}
//   - ShareMgmt.enumerateSharedFolders (admin): engined/rpc/sharemgmt.inc. 배열(rpc.common.getlist 없음), 항목마다
//     설정 필드와 mntent·absdirpath·status(8.1.0부터)
//   - Services.getStatus (admin): engined/rpc/services.inc. IServiceStatus 모듈마다 {name, title, enabled, running},
//     {total, data}. title은 OMV가 번역한 문구다
//   - Compose.doContainerCommand (admin): docker <command> <command2> <id>를 백그라운드 작업으로 돌리고
//     상태 파일 이름을 돌려준다. 실제 OMV처럼 파라미터를 검사하지 않는다. 작업이 끝나면(Exec.isRunning)
//     start·restart는 실행 중, stop은 정지로 바꾼다. 없는 컨테이너나 FailContainers는 실패한다
//
// 역할이 모자라면 OMV의 validateMethodContext처럼 403 "Invalid context role."을 돌려준다.
// 값은 모두 가짜다(모델명, 장치, 시리얼, 이미지 이름).

// SmartDevice는 Smart.getList의 data 한 행이다.
type SmartDevice struct {
	DeviceName          string   `json:"devicename"`
	CanonicalDeviceFile string   `json:"canonicaldevicefile"`
	DeviceFile          string   `json:"devicefile"`
	DeviceLinks         []string `json:"devicelinks"`
	Model               string   `json:"model"`
	// Size는 바이트 수 문자열이다(OMV는 큰 정수를 문자열로 준다).
	Size string `json:"size"`
	// Temperature는 섭씨 정수, 모르면 빈 문자열이다.
	Temperature   any    `json:"temperature"`
	Description   string `json:"description"`
	Vendor        string `json:"vendor"`
	SerialNumber  string `json:"serialnumber"`
	WWN           string `json:"wwn"`
	OverallStatus string `json:"overallstatus"`
	UUID          string `json:"uuid"`
	Monitor       bool   `json:"monitor"`
	Used          bool   `json:"_used"`
}

// Container는 Compose.getContainers의 한 행이다.
type Container struct {
	Image  string `json:"image"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ServiceStatus는 Services.getStatus의 data 한 행이다.
type ServiceStatus struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
}

// PluginData는 가짜 OMV가 돌려주는 코어·플러그인 RPC 값이다. SetPluginData로 바꾼다.
type PluginData struct {
	Smart []SmartDevice
	// Runtime은 Compose.getContainers 첫 행이다. RuntimeStatus는 OMV가 번역한 문구다.
	Runtime       string
	RuntimeStatus string
	Containers    []Container
	CPUTemp       float64
	Services      []ServiceStatus
	// ServiceSettings는 모듈 이름 → 서비스 설정 객체다(services.go).
	ServiceSettings map[string]map[string]any
	// Dirty는 적용 대기 중인 모듈 이름이다(Config.isDirty).
	Dirty []string
	// FailApply면 Config.applyChangesBg가 실패한다.
	FailApply bool
	// FailContainers는 doContainerCommand가 실패하는 컨테이너 이름이다.
	FailContainers []string
	// Missing은 설치되지 않은 것처럼 404("RPC service ... not found.")를 돌려줄 서비스다.
	Missing []string
	// SharedFolders는 ShareMgmt.enumerateSharedFolders의 공유 폴더다(관리자 전용).
	SharedFolders []SharedFolder
}

// SharedFolder는 공유 폴더 하나의 합성 값이다. 경로는 MountDir + RelDirPath다.
type SharedFolder struct {
	UUID       string
	Name       string
	MountDir   string
	RelDirPath string
	// NoMount면 마운트 지점 설정이 없는 것처럼(status ENOENT_DB_MNTENT, 경로 없음) 준다.
	NoMount bool
	// Missing이면 폴더가 없는 것처럼(status ENOENT) 준다.
	Missing bool
	// Privileges는 공유 폴더 권한이다(privileges.privilege). 비어 있으면 정한 권한이 없다.
	Privileges []Privilege
}

// Privilege는 공유 폴더 권한 하나다. Type은 user 또는 group, Perms는 0(없음), 5(읽기), 7(읽기·쓰기)이다.
type Privilege struct {
	Type  string
	Name  string
	Perms int
}

// DefaultPluginData는 개발 서버와 테스트가 쓰는 합성 값이다.
func DefaultPluginData() PluginData {
	return PluginData{
		Smart: []SmartDevice{
			fakeDisk("nvme0n1", "Fake NVMe SSD 1TB", "1000204886016", 38, "GOOD"),
			fakeDisk("sda", "Fake SATA SSD 2TB", "2000398934016", 31, "GOOD"),
			fakeDisk("sdb", "Fake HDD 8TB", "8001563222016", 36, "BAD_ATTRIBUTE_IN_THE_PAST"),
			fakeDisk("sdc", "Fake USB Disk", "500107862016", "", "GOOD"),
		},
		Runtime:       "Docker",
		RuntimeStatus: "running",
		Containers: []Container{
			{Image: "example/web:1.0", Name: "web", Status: "Up 3 days (healthy)"},
			{Image: "example/db:16", Name: "db", Status: "Up 3 days"},
			{Image: "example/cache:7", Name: "cache", Status: "Up 2 hours (Paused)"},
			{Image: "example/app:dev", Name: "app", Status: "Restarting (1) 10 seconds ago"},
			{Image: "example/worker:latest", Name: "worker", Status: "Up 5 minutes (unhealthy)"},
			{Image: "example/backup:latest", Name: "backup", Status: "Exited (0) 5 hours ago"},
			{Image: "example/init:latest", Name: "init", Status: "Created"},
			{Image: "example/broken:latest", Name: "broken", Status: "Exited (1) 2 hours ago"},
		},
		Services: []ServiceStatus{
			{Name: "ssh", Title: "SSH", Enabled: true, Running: true},
			{Name: "samba", Title: "SMB/CIFS", Enabled: true, Running: true},
			{Name: "nfs", Title: "NFS", Enabled: false, Running: false},
			{Name: "rsyncd", Title: "RSync server", Enabled: false, Running: false},
			{Name: "fail2ban", Title: "fail2ban", Enabled: true, Running: false},
			{Name: "Docker", Title: "Docker", Enabled: true, Running: true},
		},
		ServiceSettings: defaultServiceSettings(),
		CPUTemp:         47,
		FailContainers:  []string{"broken"},
	}
}

func fakeDisk(name, model, size string, temp any, status string) SmartDevice {
	return SmartDevice{
		DeviceName:          name,
		CanonicalDeviceFile: "/dev/" + name,
		DeviceFile:          "/dev/disk/by-id/fake-" + name,
		DeviceLinks:         []string{"/dev/disk/by-id/fake-" + name},
		Model:               model,
		Size:                size,
		Temperature:         temp,
		Description:         model,
		Vendor:              "Fake",
		SerialNumber:        "FAKE-SERIAL-" + name,
		WWN:                 "0x0000000000000000",
		OverallStatus:       status,
		UUID:                "fa4e0000-0000-4000-8000-000000000000",
		Monitor:             true,
	}
}

// SetPluginData는 코어·플러그인 RPC 응답을 바꾼다.
func (s *Server) SetPluginData(d PluginData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plugin = d
}

// handlePlugin은 코어·플러그인 RPC를 처리한다. 모르는 메서드면 ok가 false다.
func (s *Server) handlePlugin(sess *session, service, method string, params json.RawMessage) (any, *rpcError, bool) {
	d := s.plugin
	if slices.Contains(d.Missing, service) {
		return nil, errStatus(http.StatusNotFound, "RPC service '"+service+"' not found."), true
	}
	adminOnly := func() *rpcError {
		if !sess.admin {
			return errStatus(http.StatusForbidden, "Invalid context role.")
		}
		return nil
	}
	if _, ok := serviceRPC[service]; ok || service == "Config" {
		// 서비스 설정·적용 RPC는 모두 관리자 전용이다. 저장하기 전에 확인한다.
		if e := adminOnly(); e != nil {
			return nil, e, true
		}
		if res, rerr, ok := s.handleService(service, method, params); ok {
			return res, rerr, true
		}
	}
	switch service + "." + method {
	case "Smart.getList":
		if e := adminOnly(); e != nil {
			return nil, e, true
		}
		var p struct {
			Start *int `json:"start"`
		}
		if json.Unmarshal(params, &p) != nil || p.Start == nil {
			return nil, errStatus(http.StatusInternalServerError, "Invalid RPC parameters."), true
		}
		data := slices.Clone(d.Smart)
		if data == nil {
			data = []SmartDevice{}
		}
		return map[string]any{"total": len(data), "data": data}, nil, true
	case "Compose.getContainers":
		rows := []Container{{Image: "", Name: d.Runtime, Status: d.RuntimeStatus}}
		return append(rows, d.Containers...), nil, true
	case "CpuTemp.get":
		return map[string]any{"cputemp": d.CPUTemp}, nil, true
	case "Services.getStatus":
		if e := adminOnly(); e != nil {
			return nil, e, true
		}
		data := slices.Clone(d.Services)
		if data == nil {
			data = []ServiceStatus{}
		}
		return map[string]any{"total": len(data), "data": data}, nil, true
	case "ShareMgmt.enumerateSharedFolders":
		if e := adminOnly(); e != nil {
			return nil, e, true
		}
		return s.sharedFolders(d.SharedFolders), nil, true
	case "Compose.doContainerCommand":
		if e := adminOnly(); e != nil {
			return nil, e, true
		}
		return s.containerCommand(params), nil, true
	}
	return nil, nil, false
}

// containerCommand는 doContainerCommand의 백그라운드 작업을 등록한다. s.mu를 쥔 채로 부른다.
func (s *Server) containerCommand(params json.RawMessage) string {
	var p struct {
		Command string `json:"command"`
		ID      string `json:"id"`
	}
	json.Unmarshal(params, &p)
	i := slices.IndexFunc(s.plugin.Containers, func(c Container) bool { return c.Name == p.ID })
	status := map[string]string{
		"start":   "Up Less than a second",
		"restart": "Up Less than a second",
		"stop":    "Exited (0) Less than a second ago",
	}[p.Command]
	if i < 0 || status == "" || slices.Contains(s.plugin.FailContainers, p.ID) {
		return s.startBg("docker "+p.Command+" "+p.ID+": Error response from daemon: fake failure", nil)
	}
	return s.startBg("", func() {
		// SetPluginData가 넘긴 슬라이스를 바꾸지 않게 복사한다. 그사이 목록이 바뀌었을 수 있으므로 다시 찾는다.
		list := slices.Clone(s.plugin.Containers)
		if j := slices.IndexFunc(list, func(c Container) bool { return c.Name == p.ID }); j >= 0 {
			list[j].Status = status
		}
		s.plugin.Containers = list
	})
}

func privileges(list []Privilege) []any {
	out := []any{}
	for _, p := range list {
		out = append(out, map[string]any{"type": p.Type, "name": p.Name, "perms": p.Perms})
	}
	return out
}

// sharedFolders는 ShareMgmt.enumerateSharedFolders 응답이다(engined/rpc/sharemgmt.inc). absdirpath·status는
// OMV 8.1.0에서 생겼으므로 Version80에서는 주지 않는다.
func (s *Server) sharedFolders(list []SharedFolder) []map[string]any {
	out := []map[string]any{}
	for _, f := range list {
		item := map[string]any{
			"uuid":        f.UUID,
			"name":        f.Name,
			"comment":     "",
			"mntentref":   "fa4e0000-0000-4000-8000-00000000000" + strconv.Itoa(len(out)%10),
			"reldirpath":  f.RelDirPath,
			"privileges":  map[string]any{"privilege": privileges(f.Privileges)},
			"_used":       false,
			"device":      "",
			"description": f.Name,
			"snapshots":   false,
		}
		mntent := map[string]any{"devicefile": "", "fsname": "", "dir": "", "type": "", "posixacl": false}
		status := "OK"
		abs := ""
		if f.NoMount {
			status = "ENOENT_DB_MNTENT"
		} else {
			mntent["dir"], mntent["type"], mntent["fsname"] = f.MountDir, "ext4", "/dev/disk/by-uuid/fake"
			abs = strings.TrimSuffix(f.MountDir, "/") + "/" + f.RelDirPath
			if f.Missing {
				status = "ENOENT"
			}
		}
		item["mntent"] = mntent
		if s.version != Version80 {
			item["absdirpath"], item["status"] = abs, status
		}
		out = append(out, item)
	}
	return out
}
