package addons

import (
	"strings"
)

// 패키지 금지 표. 애드온 패키지는 아래 메서드를 읽기로도 쓰기로도 선언할 수 없다.
//
// 기준(.claude/docs/addon-packages.md의 "금지 표"):
//   - 호출한 쪽이 명령·스크립트·패키지·파일 내용·호스트 경로·이스케이프하지 않는 CLI 인자를 정하는 메서드.
//     OMV는 명령 인자를 이스케이프 없이 잇는다(/usr/share/php/openmediavault/system/process.inc의
//     getCommandLine).
//   - 자격 증명을 돌려주거나 비밀번호를 확인하는 메서드
//   - maru가 혼자 맡은 흐름(세션, 백그라운드 상태 파일, 설정 적용, APT 작업)
//
// OMV는 서비스 이름을 대소문자 구분 없이 찾고(rpc/servicemanager.inc의 mb_strtolower) 메서드 이름은 등록한
// 이름 그대로 찾는다(rpc/serviceabstract.inc의 hasMethod). 그래서 서비스는 소문자로 맞춰 비교한다.
// 표를 바꿀 때는 OMV 소스(/usr/share/openmediavault/engined/rpc/*.inc)에서 근거를 확인해 주석과 설계
// 문서를 함께 고친다.

// deniedServices는 서비스 전체를 막는 표다(소문자 서비스 이름 → 이유).
var deniedServices = map[string]string{
	"session":    "login and logout are backend-only",
	"exec":       "background status files are handled by the host (/v1/rpc-bg)",
	"config":     "applying configuration is handled by the host (/v1/apply)",
	"cron":       "cron jobs take a username and a command (rpc.cron.json), and Cron.execute runs them",
	"scripts":    "openmediavault-scripts runs and stores arbitrary scripts (doRun, setScript, doJob, doGit)",
	"pluginmgmt": "installs, removes and uploads packages",
	"apttool":    "takes a command and a package name (apttool.inc)",
	"omvextras":  "adds package repositories",
	"envvars":    "sets environment variables used by OMV scripts",
}

// deniedMethods는 메서드 하나를 막는 표다("소문자 서비스.메서드" → 이유).
var deniedMethods = map[string]string{
	"apt.install":      "APT jobs run through maru's job slot",
	"apt.upgrade":      "APT jobs run through maru's job slot",
	"apt.update":       "APT jobs run through maru's job slot",
	"apt.upload":       "uploads a package",
	"apt.getChangeLog": "puts an unescaped file name into a shell command",

	// compose.inc: 파라미터로 docker·compose 명령줄을 만들거나 compose 파일·Dockerfile·환경 파일을 쓴다
	// (특권 컨테이너는 호스트의 root다).
	"compose.doCommand":          "builds a compose command line",
	"compose.doCommandList":      "builds compose command lines",
	"compose.doPrune":            "builds a docker command line",
	"compose.doServiceCommand":   "builds a compose command line",
	"compose.doContainerCommand": "builds a docker command line without escaping",
	"compose.getContainerLog":    "puts the container id into a shell command without escaping",
	"compose.doAutocompose":      "builds a command line",
	"compose.doDockerCmd":        "builds a docker command line",
	"compose.doDockerVolumeCmd":  "builds a docker command line",
	"compose.doDockerNetworkCmd": "builds a docker command line",
	"compose.doDockerImageCmd":   "builds a docker command line",
	"compose.doNetworkConnect":   "builds a docker command line",
	"compose.doGit":              "runs git with parameters",
	"compose.doBuild":            "builds an image from a Dockerfile",
	"compose.doJob":              "runs a scheduled compose job",
	"compose.setJob":             "stores a scheduled compose job",
	"compose.setFile":            "writes a compose file",
	"compose.setDockerfile":      "writes a Dockerfile",
	"compose.setConfig":          "writes compose configuration",
	"compose.setGlobalEnv":       "writes the global environment file",
	"compose.doRestore":          "restores compose files from a path",

	"usermgmt.authUser": "checks any account's password through an admin session",

	"emailnotification.get":    "returns the SMTP password (notification.inc)",
	"rsync.get":                "returns the job password (rsync.inc)",
	"network.getProxy":         "returns the proxy password",
	"network.getInterface":     "returns the wireless pre-shared key",
	"network.getInterfaceList": "returns the wireless pre-shared key",
	"network.getWirelessIface": "returns the wireless pre-shared key",
	"symlinks.setSymlink":      "creates a symlink to any host path",
	"kvm.setVmXml":             "writes libvirt domain XML (host devices, paths)",
	"kvm.addFsPass":            "passes a host path into a VM",
	"kvm.addHostDisk":          "passes a host disk into a VM",
}

// deniedMethodPrefixes는 메서드 이름 앞부분으로 막는 표다(소문자 서비스 → 막는 메서드 앞부분 → 이유).
var deniedMethodPrefixes = map[string]map[string]string{
	// 그룹을 정하면 sudo 그룹에 넣을 수 있고, 가져오기는 계정을 한꺼번에 만든다.
	"usermgmt": {
		"set":    "changes users and groups (adding a user to sudo)",
		"delete": "deletes users and groups",
		"import": "imports users and groups",
	},
}

// rpcKey는 금지 표와 종류 충돌을 비교하는 이름이다. 서비스만 소문자로 맞춘다.
func rpcKey(method string) string {
	service, name, ok := strings.Cut(method, ".")
	if !ok {
		return strings.ToLower(method)
	}
	return strings.ToLower(service) + "." + name
}

// DeniedForPackage는 애드온 패키지가 method("Service.method")를 선언할 수 없는지와 그 이유다.
func DeniedForPackage(method string) (reason string, denied bool) {
	key := rpcKey(method)
	service, name, _ := strings.Cut(key, ".")
	if r, ok := deniedServices[service]; ok {
		return r, true
	}
	if r, ok := deniedMethods[key]; ok {
		return r, true
	}
	for prefix, r := range deniedMethodPrefixes[service] {
		if strings.HasPrefix(name, prefix) {
			return r, true
		}
	}
	return "", false
}
