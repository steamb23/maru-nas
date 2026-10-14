package network

import (
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"onmaru/backend/internal/api"
)

// Reader는 네트워크 인터페이스 상태와 누적 카운터를 읽는 provider다(이름 proc).
type Reader struct {
	proc  fs.FS
	net   fs.FS
	start time.Time
	now   func() time.Time
}

// NewReader는 호스트의 /proc, /sys/class/net을 읽는 Reader를 반환한다.
func NewReader() *Reader {
	return NewReaderFS(os.DirFS("/proc"), os.DirFS("/sys/class/net"), time.Now)
}

// NewReaderFS는 proc의 "net/dev"와 net(/sys/class/net 형태)의 "<이름>/operstate" 등을 읽는다.
// now는 단조 시각을 주는 시계다(sampledAtMs 계산).
func NewReaderFS(proc, net fs.FS, now func() time.Time) *Reader {
	return &Reader{proc: proc, net: net, start: now(), now: now}
}

// hidden은 보여 주지 않는 인터페이스다. 루프백과 컨테이너의 veth 짝은 사용자에게 의미가 없다.
func hidden(name string) bool {
	return name == "lo" || strings.HasPrefix(name, "veth")
}

// Network는 인터페이스 목록과 누적 카운터를 반환한다. 속도는 프론트가 두 응답의 차이로 계산한다.
// 물리 인터페이스(device 링크가 있음)를 먼저, 같은 종류 안에서는 /proc/net/dev 순서로 둔다.
func (r *Reader) Network() (api.NetworkResponse, error) {
	f, err := r.proc.Open("net/dev")
	if err != nil {
		return api.NetworkResponse{}, err
	}
	names, counters, err := ParseNetDev(f)
	f.Close()
	if err != nil {
		return api.NetworkResponse{}, err
	}
	res := api.NetworkResponse{
		AddonSources: api.AddonSources{Sources: []string{r.Name()}},
		SampledAtMs:  r.now().Sub(r.start).Milliseconds(),
		Interfaces:   []api.NetInterface{},
	}
	var virtual []api.NetInterface
	for _, name := range names {
		if hidden(name) {
			continue
		}
		c := counters[name]
		iface := api.NetInterface{
			Name:      name,
			State:     r.readString(name + "/operstate"),
			RxBytes:   c.RxBytes,
			TxBytes:   c.TxBytes,
			RxPackets: c.RxPackets,
			TxPackets: c.TxPackets,
			RxErrors:  c.RxErrors,
			TxErrors:  c.TxErrors,
			RxDropped: c.RxDropped,
			TxDropped: c.TxDropped,
		}
		if iface.State == "" {
			iface.State = "unknown"
		}
		// 링크가 내려가 있으면 speed 읽기가 EINVAL로 실패하고, 가상 인터페이스는 -1을 준다.
		if n, err := strconv.Atoi(r.readString(name + "/speed")); err == nil && n > 0 {
			iface.SpeedMbps = n
		}
		// 물리 인터페이스는 device가 장치 디렉터리로의 심볼릭 링크다. 링크만 확인한다(대상은 /sys 다른 곳).
		if _, err := fs.ReadLink(r.net, name+"/device"); err == nil {
			res.Interfaces = append(res.Interfaces, iface)
		} else {
			iface.Virtual = true
			virtual = append(virtual, iface)
		}
	}
	res.Interfaces = append(res.Interfaces, virtual...)
	return res, nil
}

func (r *Reader) readString(name string) string {
	b, err := fs.ReadFile(r.net, name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
