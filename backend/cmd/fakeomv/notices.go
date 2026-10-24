package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"maru/backend/internal/api"
)

// 가짜 알림 시나리오. FAKENOTICES로 고른다.
const (
	// noticesSample은 관리자 알림 둘(동작: 워크벤치, 전원)과 모두가 보는 알림 하나(동작: 설정의 정보 페이지)다.
	noticesSample = "sample"
	// noticesNone은 알림이 없다.
	noticesNone = "none"
	// noticesMany는 모두가 보는 알림 12개다(배지의 9+).
	noticesMany = "many"
	// noticesRotate는 sample에서 시작해 FAKENOTICES_STEP마다 개수(곧 리비전)를 바꾼다(토스트 확인).
	noticesRotate = "rotate"
)

// startNotices는 maru의 개발용 알림 출처(MARU_DEV_NOTICES)가 읽는 가짜 알림 파일을 쓴다.
// rotate면 step마다 다시 쓰는 고루틴을 띄운다. 리비전은 비워 두어 maru가 내용으로 채운다.
func startNotices(path, scenario string, step time.Duration) error {
	since := time.Now().UTC().Truncate(time.Second)
	switch scenario {
	case noticesSample, noticesNone, noticesMany:
		return writeNotices(path, fakeNotices(scenario, since, 0))
	case noticesRotate:
		if err := writeNotices(path, fakeNotices(noticesSample, since, 0)); err != nil {
			return err
		}
		go func() {
			for n := 1; ; n++ {
				time.Sleep(step)
				if err := writeNotices(path, fakeNotices(noticesSample, since, n)); err != nil {
					log.Printf("fake notices: %v", err)
				}
			}
		}()
		return nil
	}
	return fmt.Errorf("unknown FAKENOTICES scenario %q (sample, none, many, rotate)", scenario)
}

// fakeNotices는 시나리오의 알림 목록이다. bump만큼 개수를 늘린다. 값은 모두 합성이다.
func fakeNotices(scenario string, since time.Time, bump int) map[string][]api.Notice {
	out := map[string][]api.Notice{"admin": {}, "everyone": {}}
	sinceText := since.Format(time.RFC3339)
	count := func(n int) *int { return &n }
	switch scenario {
	case noticesSample:
		out["admin"] = []api.Notice{
			{
				ID: "sample-workbench", Code: api.NoticeSample, Level: api.NoticeWarning, Since: sinceText,
				Count: count(3 + bump), Items: []string{"fake-item-alpha", "fake-item-beta", "fake-item-gamma"},
				Action: &api.NoticeAction{Kind: api.NoticeActionWorkbench, Route: "storage/disks"},
			},
			{
				ID: "sample-power", Code: api.NoticeSample, Level: api.NoticeError, Since: sinceText,
				Action: &api.NoticeAction{Kind: api.NoticeActionPower},
			},
		}
		out["everyone"] = []api.Notice{{
			ID: "sample-app", Code: api.NoticeSample, Level: api.NoticeInfo, Since: sinceText, Count: count(1 + bump),
			Action: &api.NoticeAction{Kind: api.NoticeActionApp, App: "settings", Page: "about"},
		}}
	case noticesMany:
		for i := 1; i <= 12; i++ {
			out["everyone"] = append(out["everyone"], api.Notice{
				ID: fmt.Sprintf("sample-%02d", i), Code: api.NoticeSample, Level: api.NoticeInfo, Count: count(i),
			})
		}
	}
	return out
}

// writeNotices는 파일을 같은 디렉터리의 임시 파일에 쓴 뒤 이름을 바꾼다. maru가 반쯤 쓴 파일을 읽지 않게 한다.
func writeNotices(path string, notices map[string][]api.Notice) error {
	b, err := json.MarshalIndent(notices, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".notices-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
