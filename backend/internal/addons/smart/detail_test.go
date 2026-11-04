package smart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/omv"
	"maru/backend/internal/omv/omvfake"
)

func TestLeadingNumber(t *testing.T) {
	for in, want := range map[string]float64{
		"8,214":                       8214,
		"48,213,904 [24.6 TB]":        48213904,
		"2,000,398,934,016 [2.00 TB]": 2000398934016,
		"31482":                       31482,
		"31482h+12m+34.567s":          31482,
		"34261 (123 45 0)":            34261,
		"34 (Min/Max 19/46)":          34,
		" 38":                         38,
		"100%":                        100,
		"1.5":                         1.5,
	} {
		if got, ok := leadingNumber(in); !ok || got != want {
			t.Errorf("%q → %v %v, want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "-", "Solid State Device", ",5"} {
		if v, ok := leadingNumber(in); ok {
			t.Errorf("%q → %v, want none", in, v)
		}
	}
	if v, ok := parseValue("0x04", unitHex); !ok || v != 4 {
		t.Errorf("hex = %v %v", v, ok)
	}
	if _, ok := parseValue("4", unitHex); ok {
		t.Error("hex without 0x")
	}
}

func TestFieldsNVMe(t *testing.T) {
	info := omvfake.DefaultSmartDetails()["nvme0n1"].Information
	got := fields(info, infoFields)
	byKey := map[string]api.SmartField{}
	for _, f := range got {
		byKey[f.Key] = f
	}
	// NVMe는 usercapacity가 비어 있어 totalnvmcapacity를 쓴다.
	if c := byKey["capacity"]; c.Value != "1,000,204,886,016 [1.00 TB]" || c.Num == nil || *c.Num != 1000204886016 || c.Unit != unitBytes {
		t.Errorf("capacity %+v", c)
	}
	if byKey["model"].Value != "Fake NVMe SSD 1TB" || byKey["serial"].Value != "FAKE-SERIAL-nvme0n1" || byKey["nvmeVersion"].Value != "1.4" {
		t.Errorf("info %+v", got)
	}
	if lba := byKey["lbaSize"]; lba.Num == nil || *lba.Num != 512 {
		t.Errorf("lba %+v", lba)
	}
	// WWN·EUI-64 같은 식별자와 표에 없는 키는 넣지 않는다.
	out, _ := json.Marshal(got)
	for _, leak := range []string{"eui.", "fa4e00 0000000001", "localtime", "0x000000"} {
		if strings.Contains(string(out), leak) {
			t.Errorf("info contains %q: %s", leak, out)
		}
	}

	health := fields(info, healthFields)
	h := map[string]api.SmartField{}
	for _, f := range health {
		h[fmt.Sprint(f.Key, f.N)] = f
	}
	check := func(key string, want float64, unit string) {
		t.Helper()
		f, ok := h[key]
		if !ok || f.Num == nil || *f.Num != want || f.Unit != unit {
			t.Errorf("%s = %+v, want %v %s", key, f, want, unit)
		}
	}
	check("criticalWarning0", 0, unitHex)
	check("percentageUsed0", 3, unitPercent)
	check("availableSpareThreshold0", 10, unitPercent)
	check("dataUnitsRead0", 48213904*dataUnitBytes, unitBytes)
	check("powerOnHours0", 8214, unitHours)
	check("warningTempTime0", 0, unitMinutes)
	check("temperatureSensor1", 38, unitCelsius)
	check("temperatureSensor2", 45, unitCelsius)
	if _, ok := h["temperatureSensor3"]; ok {
		t.Error("missing sensor reported")
	}
	// 정해진 순서를 지킨다.
	if health[0].Key != "criticalWarning" || health[len(health)-1].Key != "temperatureSensor" {
		t.Errorf("order %v", health)
	}
}

func TestAttributeToAPI(t *testing.T) {
	cases := []struct {
		in     omvAttribute
		status string
		raw    float64
	}{
		{omvAttribute{ID: 5, AttrName: "Reallocated_Sector_Ct", RawValue: "8", Prefailure: true, Assessment: "BAD_SECTOR"}, api.SmartWarning, 8},
		{omvAttribute{ID: 3, RawValue: "412 (Average 398)", Prefailure: true, Assessment: "BAD_ATTRIBUTE_IN_THE_PAST"}, api.SmartFailing, 412},
		{omvAttribute{ID: 1, RawValue: "0", Prefailure: true, Assessment: "GOOD"}, api.SmartGood, 0},
		// 평가 대상이 아닌 속성은 상태가 없다.
		{omvAttribute{ID: 194, RawValue: "34 (Min/Max 19/46)", Assessment: "BAD_STATUS"}, "", 34},
		{omvAttribute{ID: 9, RawValue: "21456h+05m+12.345s", Assessment: "BAD_STATUS"}, "", 21456},
	}
	for _, c := range cases {
		got := c.in.toAPI()
		if got.Status != c.status || got.RawNum == nil || *got.RawNum != c.raw || got.Raw != c.in.RawValue {
			t.Errorf("%d → %+v", c.in.ID, got)
		}
	}
	if got := (omvAttribute{RawValue: "-"}).toAPI(); got.RawNum != nil {
		t.Errorf("raw num %v", *got.RawNum)
	}
}

func TestSelfTestToAPI(t *testing.T) {
	got, ok := omvSelfTest{Num: "2", Description: "Short offline", Status: "Completed: read failure       ",
		Remaining: "90", Lifetime: "41650", LBAOfFirstError: "123456789"}.toAPI()
	if !ok || got.Num != 2 || got.Type != "short" || got.Result != "failed" || got.StatusText != "Completed: read failure" ||
		got.Remaining == nil || *got.Remaining != 90 || got.PowerOnHours != 41650 || got.FailingLBA != "123456789" {
		t.Errorf("got %+v", got)
	}
	got, _ = omvSelfTest{Num: "1", Description: "Extended captive", Status: "Completed without error ", Remaining: "00", Lifetime: "7", LBAOfFirstError: "-"}.toAPI()
	if got.Type != "extended" || got.Result != "ok" || got.FailingLBA != "" || *got.Remaining != 0 {
		t.Errorf("got %+v", got)
	}
	if _, ok := (omvSelfTest{Num: "x"}).toAPI(); ok {
		t.Error("bad number accepted")
	}
	for in, want := range map[string]string{
		"Completed without error":           "ok",
		"Interrupted (host reset)":          "aborted",
		"Aborted by host":                   "aborted",
		"Aborted: Controller Reset":         "aborted",
		"Self-test routine in progress 90%": "running",
		"Completed: electrical failure":     "failed",
		"Completed: failed segments":        "failed",
		"Fatal or unknown test error":       "failed",
		"Unknown result (0xc)":              "other",
	} {
		if got := selfTestResult(in); got != want {
			t.Errorf("%q → %s, want %s", in, got, want)
		}
	}
}

func nvmeLog(lines ...string) string {
	return "=== START OF SMART DATA SECTION ===\nTemperature: 38 Celsius\n\nSelf-test Log (NVMe Log 0x06)\n" +
		strings.Join(lines, "\n") + "\n\nSomething else\n 9   Short  Completed without error  1 - - - - -\n"
}

// nvmeRow는 smartmontools 7.4 nvmeprint.cpp의 줄 형식이다.
func nvmeRow(i int, test, status string, poh int, lba, nsid, seg, sct, code string) string {
	return fmt.Sprintf("%2d   %-17s %-33s %9d %12s %5s %3s %3s %4s", i, test, status, poh, lba, nsid, seg, sct, code)
}

const nvmeHeader = "Num  Test_Description  Status                       Power_on_Hours  Failing_LBA  NSID Seg SCT Code"

func TestParseNVMeSelfTests(t *testing.T) {
	// 가짜 OMV의 원문(기록 4줄)
	tests, status, ok := parseNVMeSelfTests(omvfake.DefaultSmartDetails()["nvme0n1"].Extended)
	if !ok || status != nvmeNoSelfTest || len(tests) != 4 {
		t.Fatalf("tests=%+v status=%q ok=%v", tests, status, ok)
	}
	if a, d := tests[0], tests[3]; a.Num != 0 || a.Type != "short" || a.Result != "ok" || a.PowerOnHours != 8210 || a.FailingLBA != "" ||
		d.Type != "extended" || d.Result != "failed" || d.StatusText != "Completed: failed segments" || d.FailingLBA != "123456" || d.Remaining != nil {
		t.Errorf("tests %+v", tests)
	}
	if tests[2].Result != "aborted" || tests[2].StatusText != "Aborted: Controller Reset" {
		t.Errorf("aborted %+v", tests[2])
	}

	// 진행 중, 제조사 검사, 알 수 없는 결과. 구역 뒤의 줄은 읽지 않는다.
	tests, status, ok = parseNVMeSelfTests(nvmeLog(
		"Self-test status: Short self-test in progress (35% completed)", nvmeHeader,
		nvmeRow(0, "Vendor specific", "Unknown result (0xc)", 120, "-", "*", "-", "-", "-"),
		nvmeRow(1, "Unknown (0x3)", "Fatal or unknown test error", 99, "77", "1", "-", "0x0", "0x02"),
	))
	if !ok || status != "Short self-test in progress (35% completed)" || len(tests) != 2 ||
		tests[0].Type != "vendor" || tests[0].Result != "other" || tests[1].Type != "other" || tests[1].TypeText != "Unknown (0x3)" ||
		tests[1].Result != "failed" || tests[1].FailingLBA != "77" {
		t.Errorf("tests=%+v status=%q ok=%v", tests, status, ok)
	}

	// 기록 없음은 읽은 것이다.
	tests, _, ok = parseNVMeSelfTests(nvmeLog("Self-test status: No self-test in progress", "No Self-tests Logged"))
	if !ok || tests == nil || len(tests) != 0 {
		t.Errorf("none: %+v %v", tests, ok)
	}
	// 구역이 없거나 줄 형식이 다르면 읽지 못한다.
	for name, raw := range map[string]string{
		"no section": "=== START OF SMART DATA SECTION ===\nPercentage Used: 1%\n",
		"broken row": nvmeLog("Self-test status: No self-test in progress", nvmeHeader, " 0   Short   ???"),
		"no header":  nvmeLog("Self-test status: No self-test in progress"),
	} {
		if tests, _, ok := parseNVMeSelfTests(raw); ok || tests != nil {
			t.Errorf("%s: %+v %v", name, tests, ok)
		}
	}
}

func TestTruncate(t *testing.T) {
	if s, cut := truncate("abc", 3); s != "abc" || cut {
		t.Errorf("%q %v", s, cut)
	}
	if s, cut := truncate("line one\nline two\nline three", 15); s != "line one" || !cut {
		t.Errorf("%q %v", s, cut)
	}
}

// detailRPC는 상세 RPC를 기록하고 가짜 OMV의 기본 상세 값으로 답한다.
type detailRPC struct {
	calls   []string
	params  []string
	err     error
	details map[string]omvfake.SmartDetail
}

func (f *detailRPC) Call(_ context.Context, service, method string, params any) (json.RawMessage, error) {
	p, _ := json.Marshal(params)
	f.calls = append(f.calls, service+"."+method)
	f.params = append(f.params, string(p))
	if service == "Smart" && method == "getList" {
		return json.RawMessage(`{"total": 3, "data": [
			{"devicename": "nvme0n1", "devicefile": "/dev/disk/by-id/fake-nvme0n1", "canonicaldevicefile": "/dev/nvme0n1", "overallstatus": "GOOD"},
			{"devicename": "sda", "canonicaldevicefile": "/dev/sda", "overallstatus": "GOOD"},
			{"devicename": "sdx", "overallstatus": "GOOD"}]}`), nil
	}
	if f.err != nil {
		return nil, f.err
	}
	var dp struct {
		DeviceFile string `json:"devicefile"`
	}
	json.Unmarshal(p, &dp)
	d := f.details[strings.TrimPrefix(strings.TrimPrefix(dp.DeviceFile, "/dev/disk/by-id/fake-"), "/dev/")]
	var out any
	switch method {
	case "getInformation":
		out = d.Information
	case "getAttributes":
		out = d.Attributes
	case "getSelfTestLogs":
		out = d.SelfTests
	case "getExtendedInformation":
		out = d.Extended
	}
	return json.Marshal(out)
}

func TestDetailQuery(t *testing.T) {
	c, _ := newTestCache(at(10, 9, 0))
	details := omvfake.DefaultSmartDetails()
	details["sdx"] = omvfake.SmartDetail{Information: map[string]string{"devicemodel": "Fake SAS", "smarthealthstatus": "OK"}}
	rpc := &detailRPC{details: details}
	ctx := context.Background()
	query := detailQuery(c)

	// 목록을 받기 전에는 어떤 디스크도 없다.
	if _, err := query(ctx, addons.ActionRequest{Target: "sda", RPC: rpc}); !isActionCode(err, api.ErrNotFound) {
		t.Fatalf("before list: %v", err)
	}
	if _, err := c.Get(ctx, rpc, false); err != nil {
		t.Fatal(err)
	}
	if _, err := query(ctx, addons.ActionRequest{Target: "", RPC: rpc}); !errors.Is(err, addons.ErrInvalidTarget) {
		t.Errorf("empty target: %v", err)
	}
	if _, err := query(ctx, addons.ActionRequest{Target: "sdz", RPC: rpc}); !isActionCode(err, api.ErrNotFound) {
		t.Errorf("unknown target: %v", err)
	}

	run := func(target string) (api.SmartDetail, []string, []string) {
		t.Helper()
		rpc.calls, rpc.params = nil, nil
		out, err := query(ctx, addons.ActionRequest{Target: target, RPC: rpc})
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		return out.(api.SmartDetail), rpc.calls, rpc.params
	}

	// NVMe: 정보와 원문만 부르고, OMV 장치 파일(predictable 경로)을 넘긴다.
	d, calls, params := run("nvme0n1")
	if !slices.Equal(calls, []string{"Smart.getInformation", "Smart.getExtendedInformation"}) ||
		params[0] != `{"devicefile":"/dev/disk/by-id/fake-nvme0n1"}` {
		t.Errorf("nvme calls %v %v", calls, params)
	}
	if d.Kind != kindNVMe || d.DeviceFile != "/dev/disk/by-id/fake-nvme0n1" || len(d.SelfTests) != 4 || d.SelfTestsUnreadable ||
		d.SelfTestStatus != "" || d.Raw == "" || len(d.Health) == 0 || len(d.Attributes) != 0 || d.ReadAt != at(10, 9, 0).Format(time.RFC3339) ||
		d.Celsius == nil || *d.Celsius != 38 || d.PowerOnHours == nil || *d.PowerOnHours != 8214 || *d.PowerCycles != 412 {
		t.Errorf("nvme %+v", d)
	}

	// ATA: 정보·속성·자가 진단. predictable 경로가 없으면 정규 경로다.
	d, calls, params = run("sda")
	if !slices.Equal(calls, []string{"Smart.getInformation", "Smart.getAttributes", "Smart.getSelfTestLogs"}) ||
		params[0] != `{"devicefile":"/dev/sda"}` {
		t.Errorf("ata calls %v %v", calls, params)
	}
	if d.Kind != kindATA || len(d.Attributes) == 0 || len(d.SelfTests) != 2 || d.Raw != "" || len(d.Health) != 0 ||
		*d.PowerOnHours != 21456 || *d.Celsius != 31 {
		t.Errorf("ata %+v", d)
	}

	// 속성이 없는 장치(SAS 등)는 자가 진단을 묻지 않는다. 장치 파일이 없으면 /dev/<이름>이다.
	d, calls, params = run("sdx")
	if !slices.Equal(calls, []string{"Smart.getInformation", "Smart.getAttributes"}) || params[0] != `{"devicefile":"/dev/sdx"}` ||
		d.Kind != kindOther || d.Info[0].Key != "model" || d.Info[1].Key != "selfAssessment" {
		t.Errorf("other %v %+v", calls, d)
	}

	// 원문
	rpc.calls = nil
	out, err := rawQuery(c)(ctx, addons.ActionRequest{Target: "nvme0n1", RPC: rpc})
	if raw, _ := out.(api.SmartRaw); err != nil || raw.Device != "nvme0n1" || !strings.Contains(raw.Text, "Self-test Log") || raw.Truncated ||
		!slices.Equal(rpc.calls, []string{"Smart.getExtendedInformation"}) {
		t.Errorf("raw %+v %v %v", out, err, rpc.calls)
	}
}

func isActionCode(err error, code string) bool {
	var ae *addons.ActionError
	return errors.As(err, &ae) && ae.Code == code
}

// omvCaller는 가짜 OMV에 실제 클라이언트로 부르는 RPCCaller다(서버의 rpcCaller처럼 updatelastaccess=false).
type omvCaller struct {
	c   *omv.Client
	sid string
}

func (c omvCaller) Call(ctx context.Context, service, method string, params any) (json.RawMessage, error) {
	res, err := c.c.Call(ctx, omv.Request{SessionID: c.sid, Service: service, Method: method, Params: params, NoTouch: true})
	return res.Result, err
}

// TestDetailFromOMV는 가짜 OMV로 디스크마다의 상세와 오류 분류를 확인한다.
func TestDetailFromOMV(t *testing.T) {
	fake := omvfake.New(omvfake.DefaultUsers(), 0)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	client := omv.NewClient(srv.URL, "maru-test")
	lr, err := client.Login(context.Background(), "admin", "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	rpc := omvCaller{client, lr.SessionID}
	ctx := context.Background()
	c := NewCache(0, 0)
	if _, err := c.Get(ctx, rpc, false); err != nil {
		t.Fatal(err)
	}
	query := detailQuery(c)
	get := func(target string) (api.SmartDetail, error) {
		out, err := query(ctx, addons.ActionRequest{Target: target, RPC: rpc})
		d, _ := out.(api.SmartDetail)
		return d, err
	}

	d, err := get("sdb")
	if err != nil || d.Kind != kindATA || len(d.SelfTests) != 3 || d.SelfTests[1].Result != "failed" {
		t.Fatalf("sdb %+v %v", d, err)
	}
	i := slices.IndexFunc(d.Attributes, func(a api.SmartAttribute) bool { return a.ID == 3 })
	if i < 0 || d.Attributes[i].Status != api.SmartFailing {
		t.Errorf("spin up %+v", d.Attributes)
	}
	if out, _ := json.Marshal(d); strings.Contains(string(out), "0x0000000000000000") || strings.Contains(string(out), "luwwn") ||
		!strings.Contains(string(out), "FAKE-SERIAL-sdb") || strings.Contains(string(out), "Number of remapped sectors") {
		t.Errorf("sdb json %s", out)
	}

	d, err = get("nvme0n1")
	if err != nil || d.Kind != kindNVMe || len(d.SelfTests) != 4 || !strings.HasPrefix(d.Raw, "smartctl 7.4") {
		t.Errorf("nvme %+v %v", d, err)
	}

	if _, err := get("sdc"); !isActionCode(err, api.ErrSmartUnsupported) {
		t.Errorf("sdc: %v", err)
	}
	// OMV가 장치를 찾지 못하면(목록 뒤에 빠진 디스크) 404다.
	data := omvfake.DefaultPluginData()
	data.Smart = data.Smart[:1]
	fake.SetPluginData(data)
	if _, err := get("sda"); !isActionCode(err, api.ErrNotFound) {
		t.Errorf("gone: %v", err)
	}

	// 관리자가 아니면 OMV가 403으로 거절하고 그대로 넘긴다(서버가 처리한다).
	ur, err := client.Login(ctx, "user", "user", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = query(ctx, addons.ActionRequest{Target: "nvme0n1", RPC: omvCaller{client, ur.SessionID}})
	if omv.HTTPStatus(err) != http.StatusForbidden {
		t.Errorf("user: %v", err)
	}
}
