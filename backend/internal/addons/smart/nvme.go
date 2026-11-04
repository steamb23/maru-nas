package smart

import (
	"regexp"
	"strconv"
	"strings"

	"maru/backend/internal/api"
)

// NVMe 자가 진단 기록은 OMV getSelfTestLogs가 ATA 형식만 읽어 빈 배열로 주므로 smartctl --xall 원문에서 읽는다
// (smartctl 7.4에서 -x는 NVMe 자가 진단 기록을 켠다). 형식은 smartmontools 7.4 nvmeprint.cpp의
// print_self_test_log를 따른다.
//
//	Self-test Log (NVMe Log 0x06)
//	Self-test status: No self-test in progress
//	Num  Test_Description  Status                       Power_on_Hours  Failing_LBA  NSID Seg SCT Code
//	 0   Short             Completed without error               8210            -     -   -   -    -
//
// 줄은 "%2u   %-17s %-33s %9u %12s %5s %3s %3s %4s"이고 빈 칸은 "-"다. 기록이 없으면 열 머리 없이
// "No Self-tests Logged"만 쓴다. 진행 중이면 상태 줄 끝에 " (35% completed)"가 붙는다.
const (
	nvmeSelfTestSection = "Self-test Log (NVMe Log 0x06"
	nvmeSelfTestStatus  = "Self-test status:"
	nvmeNoSelfTests     = "No Self-tests Logged"
	nvmeNoSelfTest      = "No self-test in progress"
)

// nvmeSelfTestRow는 기록 한 줄이다. 종류는 Short, Extended, Vendor specific, Unknown (0x..)이고, 결과 문구에는
// 공백이 들어가지만 숫자로 끝나지 않으므로 사용 시간 앞에서 끊는다.
var nvmeSelfTestRow = regexp.MustCompile(
	`^\s*(\d+)\s+(Short|Extended|Vendor specific|Unknown \(0x[0-9A-Fa-f]+\))\s+(.+?)\s+(\d+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s*$`)

// parseNVMeSelfTests는 원문의 NVMe 자가 진단 기록을 읽는다. status는 상태 줄의 값이다. ok가 false면 구역을 찾지
// 못했거나 줄을 읽지 못했다(화면이 원문을 보라고 안내한다).
func parseNVMeSelfTests(raw string) (tests []api.SmartSelfTest, status string, ok bool) {
	in, header, none := false, false, false
	tests = []api.SmartSelfTest{}
	for line := range strings.Lines(raw) {
		line = strings.TrimRight(line, "\r\n")
		if !in {
			in = strings.HasPrefix(line, nvmeSelfTestSection)
			continue
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		switch {
		case strings.HasPrefix(line, nvmeSelfTestStatus):
			status = strings.TrimSpace(strings.TrimPrefix(line, nvmeSelfTestStatus))
		case strings.HasPrefix(line, nvmeNoSelfTests):
			none = true
		case strings.HasPrefix(line, "Num "):
			header = true
		case header:
			m := nvmeSelfTestRow.FindStringSubmatch(line)
			if m == nil {
				return nil, status, false
			}
			num, _ := strconv.Atoi(m[1])
			poh, _ := strconv.ParseInt(m[4], 10, 64)
			result := strings.TrimSpace(m[3])
			tests = append(tests, api.SmartSelfTest{
				Num:          num,
				Type:         selfTestType(m[2]),
				TypeText:     m[2],
				Result:       selfTestResult(result),
				StatusText:   result,
				PowerOnHours: poh,
				FailingLBA:   lba(m[5]),
			})
		}
	}
	switch {
	case !in:
		return nil, "", false
	case none:
		return []api.SmartSelfTest{}, status, true
	case !header:
		return nil, status, false
	}
	return tests, status, true
}
