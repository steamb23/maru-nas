package temperature

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"testing/fstest"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
)

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s + "\n")} }
func link(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s), Mode: os.ModeSymlink} }

// sampleHwmon은 CPU(coretemp), NVMe, SATA(drivetemp), ACPI 센서가 있는 합성 hwmon이다.
func sampleHwmon() fstest.MapFS {
	return fstest.MapFS{
		"hwmon0/name":        file("acpitz"),
		"hwmon0/temp1_input": file("27800"),
		"hwmon0/temp1_crit":  file("119000"),

		"hwmon1/name": file("acpi_fan"),

		"hwmon2/name":         file("nvme"),
		"hwmon2/device":       link("../../nvme0"),
		"hwmon2/temp1_input":  file("38850"),
		"hwmon2/temp1_label":  file("Composite"),
		"hwmon2/temp1_max":    file("81850"),
		"hwmon2/temp1_crit":   file("84850"),
		"hwmon2/temp2_input":  file("41850"),
		"hwmon2/temp2_label":  file("Sensor 1"),
		"hwmon2/temp10_input": file("50000"),

		"hwmon3/name":               file("coretemp"),
		"hwmon3/temp1_input":        file("45000"),
		"hwmon3/temp1_label":        file("Package id 0"),
		"hwmon3/temp1_max":          file("100000"),
		"hwmon3/temp1_crit":         file("100000"),
		"hwmon3/temp2_input":        file("43000"),
		"hwmon3/temp2_label":        file("Core 0"),
		"hwmon3/temp3_input":        file("garbage"),
		"hwmon4/name":               file("drivetemp"),
		"hwmon4/device/block/sdx/x": file(""),
		"hwmon4/temp1_input":        file("33000"),
	}
}

func TestReadHwmon(t *testing.T) {
	groups, err := ReadHwmon(sampleHwmon())
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ kind, name, device string }
	var got []row
	for _, g := range groups {
		got = append(got, row{g.Kind, g.Name, g.Device})
	}
	want := []row{{"cpu", "coretemp", ""}, {"disk", "nvme", "nvme0"}, {"disk", "drivetemp", "sdx"}, {"other", "acpitz", ""}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("group %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	cpu := groups[0]
	if len(cpu.Sensors) != 2 || cpu.Sensors[0].Label != "Package id 0" || cpu.Sensors[0].Celsius != 45 ||
		*cpu.Sensors[0].CritCelsius != 100 || cpu.Sensors[1].HighCelsius != nil {
		t.Errorf("cpu %+v", cpu.Sensors)
	}
	nvme := groups[1]
	// temp10이 temp2 뒤에 온다(숫자 순서).
	if len(nvme.Sensors) != 3 || nvme.Sensors[0].Celsius != 38.8 || nvme.Sensors[2].Celsius != 50 {
		t.Errorf("nvme %+v", nvme.Sensors)
	}
}

type fakeRPC struct {
	calls  int
	result string
	err    error
}

func (f *fakeRPC) Call(context.Context, string, string, any) (json.RawMessage, error) {
	f.calls++
	return json.RawMessage(f.result), f.err
}

func fetchWith(t *testing.T, hw *Hwmon, rpc *fakeRPC, sources ...string) (api.TemperatureResponse, error) {
	t.Helper()
	a := New(hw)
	res, err := a.Fetch(context.Background(), addons.Request{Sources: sources, RPC: rpc})
	return res.(api.TemperatureResponse), err
}

func TestFetchSkipsCPUTempWhenHwmonHasCPU(t *testing.T) {
	rpc := &fakeRPC{result: `{"cputemp": 47}`}
	res, err := fetchWith(t, NewHwmonFS(sampleHwmon()), rpc, SourceHwmon, SourceCPUTemp)
	if err != nil {
		t.Fatal(err)
	}
	if rpc.calls != 0 || len(res.Sources) != 1 || res.Groups[0].Source != SourceHwmon {
		t.Errorf("calls %d sources %v", rpc.calls, res.Sources)
	}
}

func TestFetchUsesCPUTempWithoutHwmonCPU(t *testing.T) {
	hw := sampleHwmon()
	for k := range hw {
		if len(k) > 6 && k[:6] == "hwmon3" {
			delete(hw, k)
		}
	}
	rpc := &fakeRPC{result: `{"cputemp": 47.25}`}
	res, err := fetchWith(t, NewHwmonFS(hw), rpc, SourceHwmon, SourceCPUTemp)
	if err != nil {
		t.Fatal(err)
	}
	if rpc.calls != 1 || res.Groups[0].Source != SourceCPUTemp || res.Groups[0].Sensors[0].Celsius != 47.3 {
		t.Errorf("groups %+v", res.Groups)
	}
	// 스크립트가 없으면 0을 준다 → 묶음을 만들지 않는다.
	rpc = &fakeRPC{result: `{"cputemp": 0}`}
	res, _ = fetchWith(t, NewHwmonFS(hw), rpc, SourceHwmon, SourceCPUTemp)
	if HasCPU(res.Groups) {
		t.Error("zero cputemp used")
	}
}

func TestFetchPartialAndTotalFailure(t *testing.T) {
	broken := NewHwmonFS(fstest.MapFS{})
	rpc := &fakeRPC{err: errors.New("rpc failed")}
	// hwmon 디렉터리가 비어 있으면 실패가 아니라 센서 없음이다.
	res, err := fetchWith(t, broken, rpc, SourceHwmon, SourceCPUTemp)
	if err != nil || len(res.FailedSources) != 1 || res.FailedSources[0] != SourceCPUTemp {
		t.Errorf("res %+v err %v", res, err)
	}
	// 쓸 수 있는 출처가 모두 실패하면 오류다.
	if _, err := fetchWith(t, broken, rpc, SourceCPUTemp); err == nil {
		t.Error("want error")
	}
}
