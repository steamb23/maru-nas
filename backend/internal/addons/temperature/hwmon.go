// Package temperature는 온도 애드온이다. hwmon(/sys/class/hwmon)을 1차로 읽고, CPU 센서가 없으면
// cputemp 플러그인 값을, 관리자에게는 SMART 캐시의 디스크 온도를 더한다.
package temperature

import (
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"onmaru/backend/internal/api"
)

// CPU 온도를 주는 hwmon 드라이버 이름.
var cpuChips = []string{"coretemp", "k10temp", "k8temp", "zenpower", "cpu_thermal", "soc_thermal"}

// 디스크 온도를 주는 hwmon 드라이버 이름. 장치 이름은 device 링크로 얻는다.
var diskChips = []string{"nvme", "drivetemp"}

var tempInput = regexp.MustCompile(`^temp(\d+)_input$`)

// ReadHwmon은 hwmon(/sys/class/hwmon 형태)의 온도 센서를 묶음별로 읽는다.
// 각 hwmonN 디렉터리의 name, tempN_input(밀리도), tempN_label, tempN_max, tempN_crit를 본다.
// 온도 센서가 없거나 값을 읽지 못한 칩은 뺀다.
func ReadHwmon(hw fs.FS) ([]api.SensorGroup, error) {
	entries, err := fs.ReadDir(hw, ".")
	if err != nil {
		return nil, err
	}
	var groups []api.SensorGroup
	for _, e := range entries {
		dir := e.Name()
		name := readTrim(hw, dir+"/name")
		if name == "" {
			continue
		}
		files, err := fs.ReadDir(hw, dir)
		if err != nil {
			continue
		}
		type indexed struct {
			n int
			s api.Sensor
		}
		var sensors []indexed
		for _, f := range files {
			m := tempInput.FindStringSubmatch(f.Name())
			if m == nil {
				continue
			}
			v, ok := milli(readTrim(hw, dir+"/"+f.Name()))
			if !ok {
				continue
			}
			n, _ := strconv.Atoi(m[1])
			prefix := dir + "/temp" + m[1] + "_"
			s := api.Sensor{Label: readTrim(hw, prefix+"label"), Celsius: v}
			if hi, ok := limit(readTrim(hw, prefix+"max")); ok {
				s.HighCelsius = &hi
			}
			if cr, ok := limit(readTrim(hw, prefix+"crit")); ok {
				s.CritCelsius = &cr
			}
			sensors = append(sensors, indexed{n, s})
		}
		if len(sensors) == 0 {
			continue
		}
		slices.SortFunc(sensors, func(a, b indexed) int { return a.n - b.n })
		g := api.SensorGroup{ID: "hwmon:" + name + ":" + dir, Kind: api.SensorOther, Name: name, Source: "hwmon"}
		switch {
		case slices.Contains(cpuChips, name):
			g.Kind = api.SensorCPU
		case slices.Contains(diskChips, name):
			g.Kind = api.SensorDisk
			g.Device = diskDevice(hw, dir, name)
		}
		for _, s := range sensors {
			g.Sensors = append(g.Sensors, s.s)
		}
		groups = append(groups, g)
	}
	SortGroups(groups)
	return groups, nil
}

// diskDevice는 디스크 hwmon의 장치 이름이다. nvme는 컨트롤러(nvme0), drivetemp는 블록 장치(sda)다.
func diskDevice(hw fs.FS, dir, chip string) string {
	if chip == "drivetemp" {
		if entries, err := fs.ReadDir(hw, dir+"/device/block"); err == nil && len(entries) > 0 {
			return entries[0].Name()
		}
		return ""
	}
	if target, err := fs.ReadLink(hw, dir+"/device"); err == nil {
		return path.Base(target)
	}
	return ""
}

// SortGroups는 CPU, 디스크, 그 밖의 순서로 정렬한다. 같은 종류 안에서는 장치·이름 순서다.
func SortGroups(groups []api.SensorGroup) {
	rank := map[string]int{api.SensorCPU: 0, api.SensorDisk: 1, api.SensorOther: 2}
	slices.SortStableFunc(groups, func(a, b api.SensorGroup) int {
		if d := rank[a.Kind] - rank[b.Kind]; d != 0 {
			return d
		}
		if c := strings.Compare(a.Device, b.Device); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// HasCPU는 CPU 센서 묶음이 있는지다.
func HasCPU(groups []api.SensorGroup) bool {
	return slices.ContainsFunc(groups, func(g api.SensorGroup) bool { return g.Kind == api.SensorCPU })
}

// milli는 밀리도 정수 문자열을 섭씨로 바꾼다. 소수 첫째 자리까지 쓴다.
func milli(s string) (float64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= -273000 {
		return 0, false
	}
	return float64(n/100) / 10, true
}

// limit은 경고·위험 온도 값이다. 칩에 따라 "없음"을 0이나 아주 큰 값(NVMe 센서의 65535-273.15 등)으로
// 주므로 0 초과 200 미만만 쓴다.
func limit(s string) (float64, bool) {
	v, ok := milli(s)
	return v, ok && v > 0 && v < 200
}

func readTrim(fsys fs.FS, name string) string {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
