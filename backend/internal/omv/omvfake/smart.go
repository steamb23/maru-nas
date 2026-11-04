package omvfake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

// 디스크 하나의 SMART 상세 RPC 합성 응답. 형식은 OMV 8 engined/rpc/smart.inc와
// php/openmediavault/system/storage/smartinformation.inc를 따른다(8.0부터 같다).
//
//   - 네 메서드 모두 관리자 전용이고 파라미터는 rpc.common.devicefile({devicefile})이다
//   - 검사 순서: 역할 → 파라미터 형식 → 장치가 있는지(assertGetStorageDevice) → SMART 지원(assertHasSmartSupport)
//   - Smart.getInformation: INFORMATION·SMART DATA 구역의 "키: 값" 줄을 키를 소문자로, 공백·하이픈을 뺀 사전으로
//     준다. RPC가 temperature(숫자만)·powercycles·poweronhours를 덧붙인다
//   - Smart.getAttributes: ATA 속성 표(brief 형식) 배열. NVMe·SAS는 빈 배열이다
//   - Smart.getSelfTestLogs: ATA 자가 진단 기록 배열(값은 모두 문자열, status 뒤에 공백이 남는다). NVMe는 빈 배열이다
//   - Smart.getExtendedInformation: smartctl --xall 출력 그대로(줄을 \n으로 이은 문자열)
//
// 값은 모두 가짜다(모델명, 시리얼, 식별자).

// SmartDetail은 디스크 하나의 상세 RPC 값이다. PluginData.SmartDetails에 장치 이름으로 둔다.
type SmartDetail struct {
	// Unsupported면 네 메서드 모두 "does not support S.M.A.R.T."로 실패한다.
	Unsupported bool
	Information map[string]string
	Attributes  []SmartAttribute
	SelfTests   []SmartSelfTest
	Extended    string
}

// SmartAttribute는 Smart.getAttributes의 한 행이다.
type SmartAttribute struct {
	ID          int    `json:"id"`
	AttrName    string `json:"attrname"`
	Flags       string `json:"flags"`
	Value       int    `json:"value"`
	Worst       int    `json:"worst"`
	Threshold   int    `json:"threshold"`
	WhenFailed  string `json:"whenfailed"`
	RawValue    string `json:"rawvalue"`
	Description string `json:"description"`
	Prefailure  bool   `json:"prefailure"`
	Assessment  string `json:"assessment"`
}

// SmartSelfTest는 Smart.getSelfTestLogs의 한 행이다. OMV는 모두 문자열로 준다.
type SmartSelfTest struct {
	Num             string `json:"num"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	Remaining       string `json:"remaining"`
	Lifetime        string `json:"lifetime"`
	LBAOfFirstError string `json:"lbaoffirsterror"`
}

// smartDetailMethods는 이 파일이 처리하는 Smart 메서드다.
var smartDetailMethods = []string{"getInformation", "getAttributes", "getSelfTestLogs", "getExtendedInformation"}

// devicefileFormat은 OMV의 devicefile 형식(json schema format "devicefile")이다.
var devicefileFormat = regexp.MustCompile(`(?i)^/dev(/disk/by-id)?/[a-z0-9/.:_+-]+$`)

// SetSmartDelay는 SMART 상세 RPC가 응답하기 전에 기다리는 시간이다(잠든 디스크를 깨우는 시간 흉내).
// 서버 잠금을 잡기 전에 기다리므로 다른 호출은 막지 않는다.
func (s *Server) SetSmartDelay(d time.Duration) {
	s.smartDelay.Store(int64(d))
}

func isSmartDetail(service, method string) bool {
	return service == "Smart" && slices.Contains(smartDetailMethods, method)
}

// handleSmartDetail은 SMART 상세 RPC를 처리한다. 다른 메서드면 ok가 false다.
func (s *Server) handleSmartDetail(sess *session, method string, params json.RawMessage) (any, *rpcError, bool) {
	if !slices.Contains(smartDetailMethods, method) {
		return nil, nil, false
	}
	if !sess.admin {
		return nil, errStatus(http.StatusForbidden, "Invalid context role."), true
	}
	var p struct {
		DeviceFile *string `json:"devicefile"`
	}
	if json.Unmarshal(params, &p) != nil || p.DeviceFile == nil {
		return nil, errStatus(http.StatusInternalServerError, "Invalid RPC parameters."), true
	}
	file := *p.DeviceFile
	if !devicefileFormat.MatchString(file) {
		return nil, errStatus(http.StatusInternalServerError,
			fmt.Sprintf("devicefile: The value '%s' is not a device file.", file)), true
	}
	name, ok := s.smartDeviceName(file)
	if !ok {
		return nil, errStatus(http.StatusInternalServerError, fmt.Sprintf("Device '%s' does not exist.", file)), true
	}
	detail, ok := s.plugin.SmartDetails[name]
	if !ok || detail.Unsupported {
		return nil, errStatus(http.StatusInternalServerError,
			fmt.Sprintf("Device '%s' does not support S.M.A.R.T.", file)), true
	}
	switch method {
	case "getInformation":
		info := make(map[string]string, len(detail.Information))
		for k, v := range detail.Information {
			info[k] = v
		}
		return info, nil, true
	case "getAttributes":
		return nonNil(detail.Attributes), nil, true
	case "getSelfTestLogs":
		return nonNil(detail.SelfTests), nil, true
	default:
		return detail.Extended, nil, true
	}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return slices.Clone(s)
}

// smartDeviceName은 장치 파일(정규 경로, predictable 경로, 링크 중 하나)에 맞는 Smart.getList 장치 이름이다.
func (s *Server) smartDeviceName(file string) (string, bool) {
	for _, d := range s.plugin.Smart {
		if file == d.CanonicalDeviceFile || file == d.DeviceFile || slices.Contains(d.DeviceLinks, file) {
			return d.DeviceName, true
		}
	}
	return "", false
}

// DefaultSmartDetails는 DefaultPluginData의 디스크(nvme0n1, sda, sdb, sdc)에 맞춘 상세 합성 값이다.
func DefaultSmartDetails() map[string]SmartDetail {
	return map[string]SmartDetail{
		"nvme0n1": fakeNVMeDetail(),
		"sda":     fakeSSDDetail(),
		"sdb":     fakeHDDDetail(),
		// sdc는 USB 어댑터를 거친 디스크라 SMART를 읽을 수 없다.
		"sdc": {Unsupported: true},
	}
}

func fakeNVMeDetail() SmartDetail {
	info := map[string]string{
		"devicefile":                 "/dev/nvme0n1",
		"devicemodel":                "Fake NVMe SSD 1TB",
		"serialnumber":               "FAKE-SERIAL-nvme0n1",
		"wwn":                        "eui.fa4e000000000001",
		"firmwareversion":            "FAKE10",
		"usercapacity":               "",
		"modelnumber":                "Fake NVMe SSD 1TB",
		"pcivendor/subsystemid":      "0xfa4e",
		"ieeeouiidentifier":          "0x000000",
		"totalnvmcapacity":           "1,000,204,886,016 [1.00 TB]",
		"unallocatednvmcapacity":     "0",
		"controllerid":               "1",
		"nvmeversion":                "1.4",
		"numberofnamespaces":         "1",
		"namespace1size/capacity":    "1,000,204,886,016 [1.00 TB]",
		"namespace1utilization":      "412,345,678,848 [412 GB]",
		"namespace1formattedlbasize": "512",
		"namespace1ieeeeui64":        "fa4e00 0000000001",
		"localtimeis":                "Wed Nov  4 14:33:10 2026 UTC",
		"smartoverallhealthselfassessmenttestresult": "PASSED",
		"criticalwarning":              "0x00",
		"temperature":                  "38",
		"availablespare":               "100%",
		"availablesparethreshold":      "10%",
		"percentageused":               "3%",
		"dataunitsread":                "48,213,904 [24.6 TB]",
		"dataunitswritten":             "31,902,551 [16.3 TB]",
		"hostreadcommands":             "512,394,201",
		"hostwritecommands":            "402,118,934",
		"controllerbusytime":           "1,204",
		"powercycles":                  "412",
		"poweronhours":                 "8,214",
		"unsafeshutdowns":              "23",
		"mediaanddataintegrityerrors":  "0",
		"errorinformationlogentries":   "0",
		"warningcomp.temperaturetime":  "0",
		"criticalcomp.temperaturetime": "0",
		"temperaturesensor1":           "38 Celsius",
		"temperaturesensor2":           "45 Celsius",
		"selfteststatus":               "No self-test in progress",
	}
	var b strings.Builder
	b.WriteString(smartctlBanner)
	b.WriteString(`=== START OF INFORMATION SECTION ===
Model Number:                       Fake NVMe SSD 1TB
Serial Number:                      FAKE-SERIAL-nvme0n1
Firmware Version:                   FAKE10
PCI Vendor/Subsystem ID:            0xfa4e
IEEE OUI Identifier:                0x000000
Total NVM Capacity:                 1,000,204,886,016 [1.00 TB]
Unallocated NVM Capacity:           0
Controller ID:                      1
NVMe Version:                       1.4
Number of Namespaces:               1
Namespace 1 Size/Capacity:          1,000,204,886,016 [1.00 TB]
Namespace 1 Utilization:            412,345,678,848 [412 GB]
Namespace 1 Formatted LBA Size:     512
Namespace 1 IEEE EUI-64:            fa4e00 0000000001
Local Time is:                      Wed Nov  4 14:33:10 2026 UTC

=== START OF SMART DATA SECTION ===
SMART overall-health self-assessment test result: PASSED

SMART/Health Information (NVMe Log 0x02)
Critical Warning:                   0x00
Temperature:                        38 Celsius
Available Spare:                    100%
Available Spare Threshold:          10%
Percentage Used:                    3%
Data Units Read:                    48,213,904 [24.6 TB]
Data Units Written:                 31,902,551 [16.3 TB]
Host Read Commands:                 512,394,201
Host Write Commands:                402,118,934
Controller Busy Time:               1,204
Power Cycles:                       412
Power On Hours:                     8,214
Unsafe Shutdowns:                   23
Media and Data Integrity Errors:    0
Error Information Log Entries:      0
Warning  Comp. Temperature Time:    0
Critical Comp. Temperature Time:    0
Temperature Sensor 1:               38 Celsius
Temperature Sensor 2:               45 Celsius

Error Information (NVMe Log 0x01, 16 of 64 entries)
No Errors Logged

Self-test Log (NVMe Log 0x06)
Self-test status: No self-test in progress
Num  Test_Description  Status                       Power_on_Hours  Failing_LBA  NSID Seg SCT Code
`)
	// smartmontools 7.4 nvmeprint.cpp의 줄 형식과 같다.
	row := func(i int, test, status string, poh int, lba, nsid, seg, sct, code string) {
		fmt.Fprintf(&b, "%2d   %-17s %-33s %9d %12s %5s %3s %3s %4s\n", i, test, status, poh, lba, nsid, seg, sct, code)
	}
	row(0, "Short", "Completed without error", 8210, "-", "-", "-", "-", "-")
	row(1, "Extended", "Completed without error", 8102, "-", "-", "-", "-", "-")
	row(2, "Short", "Aborted: Controller Reset", 7990, "-", "-", "-", "-", "-")
	row(3, "Extended", "Completed: failed segments", 7801, "123456", "1", "2", "0x2", "0x81")
	return SmartDetail{Information: info, Extended: strings.TrimSuffix(b.String(), "\n")}
}

func fakeSSDDetail() SmartDetail {
	info := ataInfo("sda", "Fake SATA SSD 2TB", "2,000,398,934,016 bytes [2.00 TB]", "Solid State Device", "2.5 inches",
		"512 bytes logical/physical", "sectorsize")
	attrs := []SmartAttribute{
		ataAttr(5, "Reallocated_Sector_Ct", "PO--CK", 100, 100, 10, "0"),
		ataAttr(9, "Power_On_Hours", "-O--CK", 95, 95, 0, "21456h+05m+12.345s"),
		ataAttr(12, "Power_Cycle_Count", "-O--CK", 99, 99, 0, "318"),
		ataAttr(177, "Wear_Leveling_Count", "PO--C-", 97, 97, 0, "42"),
		ataAttr(179, "Used_Rsvd_Blk_Cnt_Tot", "PO--C-", 100, 100, 10, "0"),
		ataAttr(181, "Program_Fail_Cnt_Total", "-O--CK", 100, 100, 10, "0"),
		ataAttr(182, "Erase_Fail_Count_Total", "-O--CK", 100, 100, 10, "0"),
		ataAttr(183, "Runtime_Bad_Block", "PO--C-", 100, 100, 10, "0"),
		ataAttr(187, "Reported_Uncorrect", "-O--CK", 100, 100, 0, "0"),
		ataAttr(194, "Temperature_Celsius", "-O---K", 69, 52, 0, "31 (Min/Max 18/48)"),
		ataAttr(195, "Hardware_ECC_Recovered", "-O-RC-", 200, 200, 0, "0"),
		ataAttr(199, "UDMA_CRC_Error_Count", "-OSRCK", 100, 100, 0, "0"),
		ataAttr(235, "Unknown_Attribute", "-O--C-", 99, 99, 0, "21"),
		ataAttr(241, "Total_LBAs_Written", "-O--CK", 99, 99, 0, "41234567890"),
	}
	tests := []SmartSelfTest{
		ataTest(1, "Short offline", "Completed without error", 0, 21440, "-"),
		ataTest(2, "Extended offline", "Completed without error", 0, 21300, "-"),
	}
	return ataDetail(info, attrs, tests)
}

func fakeHDDDetail() SmartDetail {
	info := ataInfo("sdb", "Fake HDD 8TB", "8,001,563,222,016 bytes [8.00 TB]", "7200 rpm", "3.5 inches",
		"512 bytes logical, 4096 bytes physical", "sectorsizes")
	info["modelfamily"] = "Fake NAS HDD"
	attrs := []SmartAttribute{
		ataAttr(1, "Raw_Read_Error_Rate", "PO-R--", 100, 100, 16, "0"),
		ataAttr(2, "Throughput_Performance", "P-S---", 132, 132, 54, "96"),
		// worst가 threshold 이하라 OMV가 BAD_ATTRIBUTE_IN_THE_PAST로 본다(Smart.getList의 overallstatus와 맞춘다).
		ataAttr(3, "Spin_Up_Time", "POS---", 150, 21, 21, "412 (Average 398)"),
		ataAttr(4, "Start_Stop_Count", "-O--C-", 100, 100, 0, "1032"),
		ataAttr(5, "Reallocated_Sector_Ct", "PO--CK", 100, 100, 5, "0"),
		ataAttr(7, "Seek_Error_Rate", "PO-R--", 100, 100, 67, "0"),
		ataAttr(9, "Power_On_Hours", "-O--C-", 95, 95, 0, "41820"),
		ataAttr(10, "Spin_Retry_Count", "PO--C-", 100, 100, 60, "0"),
		ataAttr(12, "Power_Cycle_Count", "-O--CK", 100, 100, 0, "214"),
		ataAttr(22, "Helium_Level", "PO---K", 100, 100, 25, "100"),
		ataAttr(192, "Power-Off_Retract_Count", "-O--CK", 99, 99, 0, "1210"),
		ataAttr(193, "Load_Cycle_Count", "-O--C-", 99, 99, 0, "1210"),
		ataAttr(194, "Temperature_Celsius", "-O----", 166, 125, 0, "36 (Min/Max 19/52)"),
		ataAttr(196, "Reallocated_Event_Count", "-O--CK", 100, 100, 0, "0"),
		ataAttr(197, "Current_Pending_Sector", "-O---K", 100, 100, 0, "0"),
		ataAttr(198, "Offline_Uncorrectable", "---R--", 100, 100, 0, "0"),
		ataAttr(199, "UDMA_CRC_Error_Count", "-O-R--", 200, 200, 0, "0"),
	}
	tests := []SmartSelfTest{
		ataTest(1, "Extended offline", "Completed without error", 0, 41800, "-"),
		ataTest(2, "Short offline", "Completed: read failure", 90, 41650, "123456789"),
		ataTest(3, "Short offline", "Interrupted (host reset)", 80, 41500, "-"),
	}
	return ataDetail(info, attrs, tests)
}

// ataInfo는 ATA 디스크의 Smart.getInformation 기본 값이다. sectorKey는 smartctl이 섹터 크기를 쓰는 줄의 키
// (논리·물리가 같으면 sectorsize, 다르면 sectorsizes)다.
func ataInfo(name, model, capacity, rotation, formFactor, sector, sectorKey string) map[string]string {
	return map[string]string{
		"devicefile":      "/dev/" + name,
		"devicemodel":     model,
		"serialnumber":    "FAKE-SERIAL-" + name,
		"wwn":             "0x0000000000000000",
		"luwwndeviceid":   "0 000000 000000000",
		"firmwareversion": "FAKE01",
		"usercapacity":    capacity,
		sectorKey:         sector,
		"rotationrate":    rotation,
		"formfactor":      formFactor,
		"deviceis":        "Not in smartctl database 7.3/5528",
		"ataversionis":    "ACS-4 T13/BSR INCITS 529 revision 5",
		"sataversionis":   "SATA 3.3, 6.0 Gb/s (current: 6.0 Gb/s)",
		"localtimeis":     "Wed Nov  4 14:32:08 2026 UTC",
		"smartsupportis":  "Enabled",
		"smartoverallhealthselfassessmenttestresult": "PASSED",
		"smartattributesdatastructurerevisionnumber": "16",
	}
}

// ataDetail은 정보·속성·자가 진단으로 RPC 값을 채운다. temperature·powercycles·poweronhours는 OMV RPC처럼
// 속성 194·12·9에서 덧붙이고, --xall 원문은 같은 값으로 만든다.
func ataDetail(info map[string]string, attrs []SmartAttribute, tests []SmartSelfTest) SmartDetail {
	for _, a := range attrs {
		switch a.ID {
		case 194:
			info["temperature"], _, _ = strings.Cut(a.RawValue, " ")
		case 12:
			info["powercycles"] = a.RawValue
		case 9:
			info["poweronhours"] = a.RawValue
		}
	}
	var b strings.Builder
	b.WriteString(smartctlBanner)
	b.WriteString("=== START OF INFORMATION SECTION ===\n")
	if v := info["modelfamily"]; v != "" {
		fmt.Fprintf(&b, "Model Family:     %s\n", v)
	}
	fmt.Fprintf(&b, "Device Model:     %s\nSerial Number:    %s\nLU WWN Device Id: %s\nFirmware Version: %s\n",
		info["devicemodel"], info["serialnumber"], info["luwwndeviceid"], info["firmwareversion"])
	fmt.Fprintf(&b, "User Capacity:    %s\n", info["usercapacity"])
	if v := info["sectorsize"]; v != "" {
		fmt.Fprintf(&b, "Sector Size:      %s\n", v)
	} else {
		fmt.Fprintf(&b, "Sector Sizes:     %s\n", info["sectorsizes"])
	}
	fmt.Fprintf(&b, "Rotation Rate:    %s\nForm Factor:      %s\nDevice is:        %s\nATA Version is:   %s\nSATA Version is:  %s\n",
		info["rotationrate"], info["formfactor"], info["deviceis"], info["ataversionis"], info["sataversionis"])
	fmt.Fprintf(&b, "Local Time is:    %s\n", info["localtimeis"])
	b.WriteString("SMART support is: Available - device has SMART capability.\nSMART support is: Enabled\n\n")
	b.WriteString("=== START OF READ SMART DATA SECTION ===\nSMART overall-health self-assessment test result: PASSED\n\n")
	b.WriteString("SMART Attributes Data Structure revision number: 16\nVendor Specific SMART Attributes with Thresholds:\n")
	b.WriteString("ID# ATTRIBUTE_NAME          FLAGS    VALUE WORST THRESH FAIL RAW_VALUE\n")
	for _, a := range attrs {
		fmt.Fprintf(&b, "%3d %-23s %-8s %03d   %03d   %03d    %s    %s\n",
			a.ID, a.AttrName, a.Flags, a.Value, a.Worst, a.Threshold, a.WhenFailed, a.RawValue)
	}
	b.WriteString("\nSMART Extended Self-test Log Version: 1 (1 sectors)\n")
	b.WriteString("Num  Test_Description    Status                  Remaining  LifeTime(hours)  LBA_of_first_error\n")
	for _, t := range tests {
		fmt.Fprintf(&b, "# %s  %-19s %-29s %s%%  %8s         %s\n",
			t.Num, t.Description, strings.TrimSpace(t.Status), t.Remaining, t.Lifetime, t.LBAOfFirstError)
	}
	return SmartDetail{Information: info, Attributes: attrs, SelfTests: tests, Extended: strings.TrimSuffix(b.String(), "\n")}
}

const smartctlBanner = "smartctl 7.4 2023-08-01 r5530 [x86_64-linux-6.12.0-fake] (local build)\n" +
	"Copyright (C) 2002-23, Bruce Allen, Christian Franke, www.smartmontools.org\n\n"

// ataAttr는 속성 한 행이다. prefailure·assessment는 OMV의 SmartInformation::assessAttribute를 그대로 따른다.
func ataAttr(id int, name, flags string, value, worst, threshold int, raw string) SmartAttribute {
	a := SmartAttribute{
		ID: id, AttrName: name, Flags: flags, Value: value, Worst: worst, Threshold: threshold,
		WhenFailed: "-", RawValue: raw, Prefailure: strings.Contains(flags, "P"), Assessment: "BAD_STATUS",
	}
	if a.Prefailure {
		a.Assessment = "GOOD"
		switch {
		case threshold < 1 || threshold > 0xFD:
			a.Assessment = "BAD_STATUS"
		case value <= threshold:
			a.Assessment = "BAD_ATTRIBUTE_NOW"
		case worst <= threshold:
			a.Assessment = "BAD_ATTRIBUTE_IN_THE_PAST"
		case (id == 5 || id == 197) && raw != "0":
			a.Assessment = "BAD_SECTOR"
		}
	} else if id == 187 && raw != "0" {
		a.Assessment = "BAD_ATTRIBUTE_NOW"
	}
	if id == 5 {
		a.Description = "Number of remapped sectors"
	}
	return a
}

// ataTest는 자가 진단 기록 한 행이다. OMV 정규식이 status를 탐욕적으로 잡으므로 뒤에 공백이 남는다.
func ataTest(num int, desc, status string, remaining, lifetime int, lba string) SmartSelfTest {
	return SmartSelfTest{
		Num:             fmt.Sprint(num),
		Description:     desc,
		Status:          fmt.Sprintf("%-29s", status),
		Remaining:       fmt.Sprintf("%02d", remaining),
		Lifetime:        fmt.Sprint(lifetime),
		LBAOfFirstError: lba,
	}
}
