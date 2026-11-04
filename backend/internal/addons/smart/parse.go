package smart

import (
	"cmp"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"maru/backend/internal/api"
)

// listResponse는 Smart.getList 응답이다(rpc.common.getlist 형식의 {total, data}).
// 형식은 OMV 8 engined/rpc/smart.inc의 enumerateDevices를 따른다.
type listResponse struct {
	Data []struct {
		DeviceName string `json:"devicename"`
		// DeviceFile은 OMV의 predictable 장치 파일(/dev/disk/by-id/...)이다. 상세 RPC의 파라미터와 워크벤치
		// 링크에 쓰고, 목록 응답에는 넣지 않는다(시리얼 번호가 들어 있다).
		DeviceFile          string          `json:"devicefile"`
		CanonicalDeviceFile string          `json:"canonicaldevicefile"`
		Model               string          `json:"model"`
		Size                json.RawMessage `json:"size"`
		Temperature         json.RawMessage `json:"temperature"`
		OverallStatus       string          `json:"overallstatus"`
		Monitor             bool            `json:"monitor"`
	} `json:"data"`
}

// ParseList는 Smart.getList 응답을 읽는다. 시리얼 번호·WWN 같은 식별 정보는 버린다.
func ParseList(raw json.RawMessage) ([]api.SmartDisk, error) {
	disks, _, err := parseList(raw)
	return disks, err
}

// parseList는 ParseList에 더해 장치 이름 → OMV 장치 파일을 돌려준다(상세 RPC의 파라미터). 장치 파일은
// predictable 경로, 없으면 정규 경로, 그것도 없으면 /dev/<이름>이다.
func parseList(raw json.RawMessage) ([]api.SmartDisk, map[string]string, error) {
	var res listResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, nil, fmt.Errorf("smart list: %w", err)
	}
	disks := make([]api.SmartDisk, 0, len(res.Data))
	files := make(map[string]string, len(res.Data))
	for _, d := range res.Data {
		files[d.DeviceName] = cmp.Or(d.DeviceFile, d.CanonicalDeviceFile, "/dev/"+d.DeviceName)
		disk := api.SmartDisk{
			Device:        d.DeviceName,
			Model:         d.Model,
			SizeBytes:     uint64(number(d.Size)),
			Status:        Summarize(d.OverallStatus),
			OverallStatus: d.OverallStatus,
			Monitored:     d.Monitor,
		}
		// 온도를 모르면 빈 문자열이다.
		if t := number(d.Temperature); t > 0 {
			disk.Celsius = &t
		}
		disks = append(disks, disk)
	}
	return disks, files, nil
}

// number는 숫자나 숫자 문자열을 읽는다. OMV는 큰 정수(디스크 크기)를 문자열로 준다. 읽지 못하면 0이다.
func number(raw json.RawMessage) float64 {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0
	}
	return f
}

// Summarize는 OMV의 SMART 평가(SmartInformation::SMART_ASSESSMENT_*)를 줄인다. 색 구분은 OMV
// workbench의 SMART 상태 위젯(dashboard.d/smart_status.yaml)을 따른다: BAD_SECTOR만 경고,
// BAD_ATTRIBUTE_NOW·BAD_ATTRIBUTE_IN_THE_PAST·BAD_SECTOR_MANY는 오류, BAD_STATUS는 알 수 없음(회색).
func Summarize(overall string) string {
	switch overall {
	case "GOOD":
		return api.SmartGood
	case "BAD_SECTOR":
		return api.SmartWarning
	case "BAD_ATTRIBUTE_NOW", "BAD_ATTRIBUTE_IN_THE_PAST", "BAD_SECTOR_MANY":
		return api.SmartFailing
	}
	return api.SmartUnknown
}
