package addons

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"log"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"onmaru/backend/internal/api"
)

// knownPlugins는 manifest의 requires.plugins에 쓸 수 있는 OMV 플러그인 패키지와
// 그 패키지가 설치하는 RPC 파일(/usr/share/openmediavault/engined/rpc/<파일>)이다.
// 파일 이름은 dpkg -S로 확인한 값만 넣는다. 새 플러그인을 쓰는 애드온을 만들 때 추가한다.
var knownPlugins = map[string]string{
	"openmediavault-bcache":   "bcache.inc",
	"openmediavault-compose":  "compose.inc",
	"openmediavault-cputemp":  "cputemp.inc",
	"openmediavault-kvm":      "kvm.inc",
	"openmediavault-mergerfs": "mergerfs.inc",
}

// Detector는 OMV 플러그인 설치 여부를 감지한다. 플러그인은 RPC 파일이 있고 dpkg 상태가
// "install ok installed"일 때만 설치된 것으로 본다. 두 곳 모두 root 없이 읽힌다.
//
// 결과는 TTL 동안 보관하고, 지난 뒤 처음 묻는 요청에서 다시 확인한다(백그라운드 고루틴 없음).
// 실행 중에 플러그인을 설치하거나 지운 경우를 반영하기 위해서다.
type Detector struct {
	rpcDir  fs.FS
	dpkgDir fs.FS
	// TTL은 감지 결과를 다시 확인하기까지의 시간이다.
	TTL time.Duration
	Now func() time.Time

	mu      sync.Mutex
	plugins map[string]bool
	checked time.Time
	stale   bool
}

// NewDetector는 호스트의 OMV RPC 디렉터리와 dpkg 상태를 읽는 Detector를 반환한다.
func NewDetector() *Detector {
	return NewDetectorFS(os.DirFS("/usr/share/openmediavault/engined/rpc"), os.DirFS("/var/lib/dpkg"))
}

// NewDetectorFS는 rpcDir(OMV RPC 파일)과 dpkgDir(status 파일)을 읽는 Detector를 반환한다.
func NewDetectorFS(rpcDir, dpkgDir fs.FS) *Detector {
	return &Detector{rpcDir: rpcDir, dpkgDir: dpkgDir, TTL: 5 * time.Minute, Now: time.Now, stale: true}
}

// Plugins는 설치된 알려진 플러그인 집합을 반환한다. 결과가 오래됐으면 다시 확인한다.
func (d *Detector) Plugins() map[string]bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stale || d.Now().Sub(d.checked) >= d.TTL {
		d.refreshLocked()
	}
	return maps.Clone(d.plugins)
}

// Refresh는 바로 다시 확인한다. 백엔드 시작과 로그인 때 부른다.
func (d *Detector) Refresh() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refreshLocked()
}

// Invalidate는 다음 Plugins 호출 때 다시 확인하게 한다. OMV가 RPC 서비스를 찾지 못했다고
// 응답했을 때(플러그인이 지워졌을 수 있음) 부른다.
func (d *Detector) Invalidate() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stale = true
}

func (d *Detector) refreshLocked() {
	plugins, err := d.detect()
	if err != nil {
		// 읽지 못하면 직전 결과를 유지한다. 처음이면 아무것도 설치되지 않은 것으로 본다.
		log.Printf("plugin detection: %v", err)
		if d.plugins == nil {
			d.plugins = map[string]bool{}
		}
	} else {
		d.plugins = plugins
	}
	d.checked, d.stale = d.Now(), false
}

func (d *Detector) detect() (map[string]bool, error) {
	f, err := d.dpkgDir.Open("status")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	installed, err := ParseInstalledPackages(f)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for pkg, file := range knownPlugins {
		if !installed[pkg] {
			continue
		}
		if _, err := fs.Stat(d.rpcDir, file); err == nil {
			out[pkg] = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return out, nil
}

// ParseInstalledPackages는 dpkg status 파일에서 상태가 "install ok installed"인 패키지 이름을 읽는다.
// 문단은 빈 줄로 나뉘고, 공백으로 시작하는 줄은 앞 필드의 연속이다.
func ParseInstalledPackages(r io.Reader) (map[string]bool, error) {
	out := make(map[string]bool)
	var pkg, status string
	flush := func() {
		if pkg != "" && status == "install ok installed" {
			out[pkg] = true
		}
		pkg, status = "", ""
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.TrimSpace(line) == "":
			flush()
		case strings.HasPrefix(line, "Package:"):
			pkg = strings.TrimSpace(strings.TrimPrefix(line, "Package:"))
		case strings.HasPrefix(line, "Status:"):
			status = strings.Join(strings.Fields(strings.TrimPrefix(line, "Status:")), " ")
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	flush()
	return out, nil
}

// unmet는 조건을 만족하지 못한 이유를 반환한다. 만족하면 빈 문자열이다.
// 관리자 조건을 먼저 본다. 일반 사용자에게는 플러그인 설치 안내가 쓸모없기 때문이다.
func unmet(req api.Requirements, plugins map[string]bool, admin bool) string {
	if req.Admin && !admin {
		return api.ReasonAdminRequired
	}
	for _, p := range req.Plugins {
		if !plugins[p] {
			return api.ReasonPluginMissing
		}
	}
	return ""
}

// Capability는 플러그인 설치 상태와 세션 역할로 애드온 하나를 쓸 수 있는지 계산한다.
// manifest의 requires를 만족하고 쓸 수 있는 provider가 하나 이상 있어야 available이다.
func Capability(a Addon, plugins map[string]bool, admin bool) api.AddonCapability {
	c := api.AddonCapability{ID: a.Manifest.ID, Sources: []string{}}
	if c.Reason = unmet(a.Manifest.Requires, plugins, admin); c.Reason != "" {
		return c
	}
	first := ""
	for _, p := range a.Providers {
		reason := unmet(p.Requires(), plugins, admin)
		if reason == "" {
			c.Sources = append(c.Sources, p.Name())
		} else if first == "" {
			first = reason
		}
	}
	if len(c.Sources) == 0 {
		c.Reason = first
		return c
	}
	c.Available = true
	return c
}

// Capabilities는 등록된 모든 애드온의 capability와 설치된 알려진 플러그인 목록이다.
func (r *Registry) Capabilities(plugins map[string]bool, admin bool) api.CapabilitiesResponse {
	res := api.CapabilitiesResponse{Addons: []api.AddonCapability{}, Plugins: []string{}}
	for _, a := range r.addons {
		res.Addons = append(res.Addons, Capability(a, plugins, admin))
	}
	for p, ok := range plugins {
		if ok {
			res.Plugins = append(res.Plugins, p)
		}
	}
	slices.Sort(res.Plugins)
	return res
}
