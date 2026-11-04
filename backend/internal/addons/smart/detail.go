package smart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/omv"
)

// 디스크 하나의 SMART 상세는 OMV의 Smart.getInformation·getAttributes·getSelfTestLogs·getExtendedInformation
// (관리자 전용, {devicefile})으로 읽는다. 메서드마다 smartctl을 --nocheck 없이 한 번 실행해 잠든 디스크를 깨우므로,
// 브라우저가 고른 장치 파일로 /api/rpc가 부르지 못하게 쓰기로 선언하고 조작(detail, raw)으로만 부른다.
// OMV에는 브라우저가 보낸 값 대신 목록 스냅숏에 보관한 그 디스크의 장치 파일을 넘긴다.
//
// 형식은 OMV 8 engined/rpc/smart.inc와 php/openmediavault/system/storage/smartinformation.inc를 따른다(8.0부터 같다).

const (
	// queryTimeout은 상세·원문 조회 하나에 주는 시간이다. 잠든 디스크가 깨어나는 시간을 넣어 목록과 같게 둔다.
	queryTimeout = fetchTimeout
	// rawLimit은 원문의 상한이다.
	rawLimit = 256 << 10
	// dataUnitBytes는 NVMe 데이터 단위 하나의 바이트 수다(512바이트 1000개, NVMe 명세).
	dataUnitBytes = 512_000
)

// 장치 종류. api.SmartDetail.Kind에 들어간다.
const (
	kindATA   = "ata"
	kindNVMe  = "nvme"
	kindOther = "other"
)

// 값의 단위. api.SmartField.Unit에 들어간다.
const (
	unitPercent = "percent"
	unitCelsius = "celsius"
	unitBytes   = "bytes"
	unitHours   = "hours"
	unitMinutes = "minutes"
	unitCount   = "count"
	unitHex     = "hex"
)

// fieldSpec은 OMV getInformation의 키를 화면 항목 하나로 바꾸는 규칙이다. omv는 앞의 키부터 보고 빈 값이 아닌
// 첫 값을 쓴다. 이 표에 없는 키(WWN, EUI-64 같은 식별자 포함)는 응답에 넣지 않는다.
type fieldSpec struct {
	key   string
	n     int
	omv   []string
	unit  string
	scale float64
}

// infoFields는 장치 정보 항목이다(ATA·NVMe·SAS의 키를 함께 둔다. 없는 키는 건너뛴다).
var infoFields = []fieldSpec{
	{key: "modelFamily", omv: []string{"modelfamily"}},
	{key: "model", omv: []string{"devicemodel", "modelnumber", "product"}},
	{key: "vendor", omv: []string{"vendor"}},
	{key: "serial", omv: []string{"serialnumber"}},
	{key: "firmware", omv: []string{"firmwareversion", "revision"}},
	// NVMe에는 usercapacity가 없다(OMV가 빈 값으로 둔다).
	{key: "capacity", omv: []string{"usercapacity", "totalnvmcapacity", "namespace1size/capacity"}, unit: unitBytes},
	{key: "sectorSize", omv: []string{"sectorsizes", "sectorsize", "logicalblocksize"}},
	{key: "lbaSize", omv: []string{"namespace1formattedlbasize"}, unit: unitBytes},
	{key: "rotationRate", omv: []string{"rotationrate"}},
	{key: "formFactor", omv: []string{"formfactor"}},
	{key: "ataVersion", omv: []string{"ataversionis"}},
	{key: "sataVersion", omv: []string{"sataversionis"}},
	{key: "nvmeVersion", omv: []string{"nvmeversion"}},
	{key: "namespaces", omv: []string{"numberofnamespaces"}, unit: unitCount},
	{key: "pciVendor", omv: []string{"pcivendor/subsystemid"}},
	{key: "trim", omv: []string{"trimcommand"}},
	{key: "smartSupport", omv: []string{"smartsupportis"}},
	{key: "selfAssessment", omv: []string{"smartoverallhealthselfassessmenttestresult", "smarthealthstatus"}},
}

// healthFields는 NVMe SMART/건강 정보 로그(NVMe Log 0x02) 항목이다. OMV getInformation이 SMART DATA 구역의
// "키: 값" 줄로 준다.
var healthFields = func() []fieldSpec {
	f := []fieldSpec{
		{key: "criticalWarning", omv: []string{"criticalwarning"}, unit: unitHex},
		{key: "availableSpare", omv: []string{"availablespare"}, unit: unitPercent},
		{key: "availableSpareThreshold", omv: []string{"availablesparethreshold"}, unit: unitPercent},
		{key: "percentageUsed", omv: []string{"percentageused"}, unit: unitPercent},
		{key: "dataUnitsRead", omv: []string{"dataunitsread"}, unit: unitBytes, scale: dataUnitBytes},
		{key: "dataUnitsWritten", omv: []string{"dataunitswritten"}, unit: unitBytes, scale: dataUnitBytes},
		{key: "hostReadCommands", omv: []string{"hostreadcommands"}, unit: unitCount},
		{key: "hostWriteCommands", omv: []string{"hostwritecommands"}, unit: unitCount},
		{key: "controllerBusyTime", omv: []string{"controllerbusytime"}, unit: unitMinutes},
		{key: "powerCycles", omv: []string{"powercycles"}, unit: unitCount},
		{key: "powerOnHours", omv: []string{"poweronhours"}, unit: unitHours},
		{key: "unsafeShutdowns", omv: []string{"unsafeshutdowns"}, unit: unitCount},
		{key: "mediaErrors", omv: []string{"mediaanddataintegrityerrors"}, unit: unitCount},
		{key: "errorLogEntries", omv: []string{"errorinformationlogentries"}, unit: unitCount},
		{key: "temperature", omv: []string{"temperature"}, unit: unitCelsius},
		{key: "warningTempTime", omv: []string{"warningcomp.temperaturetime"}, unit: unitMinutes},
		{key: "criticalTempTime", omv: []string{"criticalcomp.temperaturetime"}, unit: unitMinutes},
	}
	// NVMe는 온도 센서를 8개까지 알린다.
	for n := 1; n <= 8; n++ {
		f = append(f, fieldSpec{key: "temperatureSensor", n: n, omv: []string{fmt.Sprint("temperaturesensor", n)}, unit: unitCelsius})
	}
	return f
}()

// detailQuery는 디스크 하나의 상세(detail 조작)다.
func detailQuery(c *Cache) func(context.Context, addons.ActionRequest) (any, error) {
	return func(ctx context.Context, req addons.ActionRequest) (any, error) {
		file, err := c.target(req.Target)
		if err != nil {
			return nil, err
		}
		d, err := readDetail(ctx, req.RPC, req.Target, file)
		if err != nil {
			return nil, err
		}
		d.ReadAt = c.Now().Format(time.RFC3339)
		return d, nil
	}
}

// rawQuery는 디스크 하나의 smartctl --xall 원문(raw 조작)이다.
func rawQuery(c *Cache) func(context.Context, addons.ActionRequest) (any, error) {
	return func(ctx context.Context, req addons.ActionRequest) (any, error) {
		file, err := c.target(req.Target)
		if err != nil {
			return nil, err
		}
		var raw string
		if err := addons.CallInto(ctx, req.RPC, "Smart", "getExtendedInformation", devicefileParams(file), &raw); err != nil {
			return nil, smartError(req.Target, file, err)
		}
		text, cut := truncate(raw, rawLimit)
		return api.SmartRaw{Device: req.Target, ReadAt: c.Now().Format(time.RFC3339), Text: text, Truncated: cut}, nil
	}
}

// target은 조작 대상(장치 이름)의 OMV 장치 파일이다. 목록 스냅숏에 없는 디스크는 404 not_found다.
func (c *Cache) target(name string) (string, error) {
	if name == "" {
		return "", addons.ErrInvalidTarget
	}
	if snap, ok := c.Peek(); ok {
		if file, ok := snap.DeviceFile(name); ok {
			return file, nil
		}
	}
	return "", &addons.ActionError{Status: http.StatusNotFound, Code: api.ErrNotFound, Message: "the disk is not in the SMART list"}
}

func devicefileParams(file string) map[string]string { return map[string]string{"devicefile": file} }

// readDetail은 장치 종류에 맞는 RPC만 부른다(smartctl 실행 횟수를 줄이려고).
//   - NVMe: getInformation + getExtendedInformation(자가 진단 기록은 OMV가 표로 주지 않아 원문에서 읽는다)
//   - 그 밖: getInformation + getAttributes, 속성이 있으면(ATA) getSelfTestLogs
func readDetail(ctx context.Context, rpc addons.RPCCaller, name, file string) (api.SmartDetail, error) {
	params := devicefileParams(file)
	call := func(method string, out any) error {
		if err := addons.CallInto(ctx, rpc, "Smart", method, params, out); err != nil {
			return smartError(name, file, err)
		}
		return nil
	}
	var rawInfo map[string]json.RawMessage
	if err := call("getInformation", &rawInfo); err != nil {
		return api.SmartDetail{}, err
	}
	info := stringValues(rawInfo)
	d := api.SmartDetail{
		Device:       name,
		DeviceFile:   file,
		Info:         fields(info, infoFields),
		Health:       []api.SmartField{},
		Attributes:   []api.SmartAttribute{},
		SelfTests:    []api.SmartSelfTest{},
		Celsius:      infoNumber(info, "temperature"),
		PowerOnHours: infoInteger(info, "poweronhours"),
		PowerCycles:  infoInteger(info, "powercycles"),
	}

	if strings.HasPrefix(name, "nvme") {
		d.Kind = kindNVMe
		d.Health = fields(info, healthFields)
		var raw string
		if err := call("getExtendedInformation", &raw); err != nil {
			return api.SmartDetail{}, err
		}
		d.Raw, d.RawTruncated = truncate(raw, rawLimit)
		tests, status, ok := parseNVMeSelfTests(raw)
		if ok {
			d.SelfTests = tests
		}
		d.SelfTestsUnreadable = !ok
		if status != "" && status != nvmeNoSelfTest {
			d.SelfTestStatus = status
		}
		return d, nil
	}

	var attrs []omvAttribute
	if err := call("getAttributes", &attrs); err != nil {
		return api.SmartDetail{}, err
	}
	if len(attrs) == 0 {
		d.Kind = kindOther
		return d, nil
	}
	d.Kind = kindATA
	for _, a := range attrs {
		d.Attributes = append(d.Attributes, a.toAPI())
	}
	var tests []omvSelfTest
	if err := call("getSelfTestLogs", &tests); err != nil {
		return api.SmartDetail{}, err
	}
	for _, t := range tests {
		if st, ok := t.toAPI(); ok {
			d.SelfTests = append(d.SelfTests, st)
		}
	}
	return d, nil
}

// stringValues는 getInformation 값을 문자열로 바꾼다. OMV는 대부분 문자열로 주지만 덧붙인 값은 숫자일 수 있다.
func stringValues(raw map[string]json.RawMessage) map[string]string {
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		var s string
		if json.Unmarshal(v, &s) != nil {
			s = strings.TrimSpace(string(v))
			if s == "null" {
				continue
			}
		}
		out[k] = strings.TrimSpace(s)
	}
	return out
}

// fields는 specs 순서대로 info에 있는 항목만 만든다.
func fields(info map[string]string, specs []fieldSpec) []api.SmartField {
	out := []api.SmartField{}
	for _, spec := range specs {
		var value string
		for _, k := range spec.omv {
			if v := info[k]; v != "" {
				value = v
				break
			}
		}
		if value == "" {
			continue
		}
		f := api.SmartField{Key: spec.key, N: spec.n, Value: value, Unit: spec.unit}
		if spec.unit != "" {
			if v, ok := parseValue(value, spec.unit); ok {
				if spec.scale != 0 {
					v *= spec.scale
				}
				f.Num = &v
			}
		}
		out = append(out, f)
	}
	return out
}

// leadingNum은 값 앞의 숫자다. 천 단위 쉼표를 허용한다(8,214, 1,000,204,886,016 [1.00 TB], 31482h+12m+34.567s,
// 34 (Min/Max 19/46)).
var leadingNum = regexp.MustCompile(`^\s*([0-9][0-9,]*(?:\.[0-9]+)?)`)

// leadingNumber는 s 앞의 숫자를 읽는다.
func leadingNumber(s string) (float64, bool) {
	m := leadingNum.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	return f, err == nil
}

// parseValue는 단위에 맞게 값을 읽는다. 16진수(0x00)는 hex 단위만 읽는다.
func parseValue(s, unit string) (float64, bool) {
	if unit == unitHex {
		h, ok := strings.CutPrefix(strings.TrimSpace(s), "0x")
		if !ok {
			return 0, false
		}
		n, err := strconv.ParseUint(h, 16, 64)
		return float64(n), err == nil
	}
	return leadingNumber(s)
}

func infoNumber(info map[string]string, key string) *float64 {
	if v, ok := leadingNumber(info[key]); ok {
		return &v
	}
	return nil
}

func infoInteger(info map[string]string, key string) *int64 {
	if v, ok := leadingNumber(info[key]); ok {
		n := int64(v)
		return &n
	}
	return nil
}

// omvAttribute는 Smart.getAttributes의 한 행이다. 영어 설명(description)은 화면이 번역하지 못하므로 버린다.
type omvAttribute struct {
	ID         int    `json:"id"`
	AttrName   string `json:"attrname"`
	Flags      string `json:"flags"`
	Value      int    `json:"value"`
	Worst      int    `json:"worst"`
	Threshold  int    `json:"threshold"`
	WhenFailed string `json:"whenfailed"`
	RawValue   string `json:"rawvalue"`
	Prefailure bool   `json:"prefailure"`
	Assessment string `json:"assessment"`
}

func (a omvAttribute) toAPI() api.SmartAttribute {
	out := api.SmartAttribute{
		ID: a.ID, Name: a.AttrName, Flags: a.Flags, Prefailure: a.Prefailure,
		Value: a.Value, Worst: a.Worst, Threshold: a.Threshold,
		WhenFailed: strings.TrimSpace(a.WhenFailed), Raw: strings.TrimSpace(a.RawValue),
	}
	if v, ok := leadingNumber(a.RawValue); ok {
		out.RawNum = &v
	}
	// OMV는 고장 예고가 아닌 속성(187 말고)과 임계값이 의미 없는 속성을 BAD_STATUS로 둔다. 평가가 없다는 뜻이다.
	if a.Assessment != "" && a.Assessment != "BAD_STATUS" {
		out.Status = Summarize(a.Assessment)
	}
	return out
}

// omvSelfTest는 Smart.getSelfTestLogs의 한 행이다(값은 모두 문자열). OMV 정규식이 status를 탐욕적으로 잡아 뒤에
// 공백이 남는다.
type omvSelfTest struct {
	Num             string `json:"num"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	Remaining       string `json:"remaining"`
	Lifetime        string `json:"lifetime"`
	LBAOfFirstError string `json:"lbaoffirsterror"`
}

func (t omvSelfTest) toAPI() (api.SmartSelfTest, bool) {
	num, err := strconv.Atoi(strings.TrimSpace(t.Num))
	if err != nil {
		return api.SmartSelfTest{}, false
	}
	desc := strings.TrimSpace(t.Description)
	status := strings.TrimSpace(t.Status)
	out := api.SmartSelfTest{
		Num:        num,
		Type:       selfTestType(desc),
		TypeText:   desc,
		Result:     selfTestResult(status),
		StatusText: status,
		FailingLBA: lba(t.LBAOfFirstError),
	}
	if r, err := strconv.Atoi(strings.TrimSpace(t.Remaining)); err == nil {
		out.Remaining = &r
	}
	out.PowerOnHours, _ = strconv.ParseInt(strings.TrimSpace(t.Lifetime), 10, 64)
	return out, true
}

// selfTestType은 smartctl의 자가 진단 종류(ATA: Short offline, Extended captive / NVMe: Short, Extended,
// Vendor specific)를 줄인다.
func selfTestType(desc string) string {
	switch {
	case strings.HasPrefix(desc, "Short"):
		return "short"
	case strings.HasPrefix(desc, "Extended"):
		return "extended"
	case strings.HasPrefix(desc, "Vendor specific"):
		return "vendor"
	}
	return "other"
}

// selfTestResult는 smartctl의 자가 진단 결과 문구를 줄인다(ATA: ataprint.cpp, NVMe: nvmeprint.cpp).
func selfTestResult(status string) string {
	switch {
	case status == "Completed without error":
		return "ok"
	case strings.HasPrefix(status, "Self-test routine in progress"):
		return "running"
	case strings.HasPrefix(status, "Aborted"), strings.HasPrefix(status, "Interrupted"):
		return "aborted"
	case strings.HasPrefix(status, "Completed:"), strings.HasPrefix(status, "Fatal"):
		return "failed"
	}
	return "other"
}

func lba(s string) string {
	s = strings.TrimSpace(s)
	if s == "-" {
		return ""
	}
	return s
}

// truncate는 s가 limit 바이트를 넘으면 마지막 줄바꿈에서 자른다.
func truncate(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	cut := s[:limit]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i]
	}
	return strings.ToValidUTF8(cut, ""), true
}

// smartError는 OMV 오류를 화면이 가릴 수 있는 오류로 바꾼다. OMV 문구는 장치 파일(시리얼 번호가 들어 있다)을
// 지운 뒤 로그에만 남긴다.
func smartError(name, file string, err error) error {
	if errors.Is(err, addons.ErrActionTimeout) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	var oe *omv.Error
	if !errors.As(err, &oe) || !oe.FromRPC() || oe.HTTPStatus == http.StatusUnauthorized || oe.HTTPStatus == http.StatusForbidden {
		// 네트워크·세션·권한 문제는 서버가 omv_error나 로그아웃으로 처리한다.
		return err
	}
	log.Printf("smart: detail of %s: %s", name, strings.ReplaceAll(oe.Message, file, "<devicefile>"))
	switch {
	case strings.Contains(oe.Message, "does not support S.M.A.R.T."):
		return &addons.ActionError{Status: http.StatusConflict, Code: api.ErrSmartUnsupported, Message: "the device does not report SMART data"}
	case strings.HasPrefix(oe.Message, "Device '") && strings.HasSuffix(oe.Message, "does not exist."):
		return &addons.ActionError{Status: http.StatusNotFound, Code: api.ErrNotFound, Message: "the disk no longer exists"}
	}
	return fmt.Errorf("%w: smart rpc failed", addons.ErrActionFailed)
}
