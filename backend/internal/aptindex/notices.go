package aptindex

import (
	"context"
	"errors"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/notify"
)

// 알림 id. 읽음 상태의 키다.
const (
	UpdatesNoticeID = "apt-updates"
	RebootNoticeID  = "reboot-required"
)

// UpdatesApp은 업데이트 알림을 누르면 여는 앱(시스템 업데이트 애드온)이다.
const UpdatesApp = "packages"

// UpdatesSource는 설치할 수 있는 APT 업데이트를 알리는 출처다(관리자 전용).
type UpdatesSource struct{ Reader *Reader }

func (UpdatesSource) Name() string { return "apt-updates" }
func (UpdatesSource) Admin() bool  { return true }

// Collect는 업데이트 목록을 읽는다. 목록 파일이 없거나 비어 있으면 알릴 것이 없다.
// 리비전은 "이름@버전" 목록의 해시라 OMV가 같은 내용으로 파일을 다시 써도 바뀌지 않는다.
func (s UpdatesSource) Collect(context.Context) ([]api.Notice, error) {
	idx, err := s.Reader.Index()
	if errors.Is(err, ErrNoIndex) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(idx.Packages) == 0 {
		return nil, nil
	}
	count := len(idx.Packages)
	names := make([]string, len(idx.Packages))
	for i, p := range idx.Packages {
		names[i] = p.Name
	}
	return []api.Notice{{
		ID:       UpdatesNoticeID,
		Code:     api.NoticeAptUpdates,
		Level:    api.NoticeInfo,
		Revision: notify.Revision(Keys(idx.Packages)...),
		Count:    &count,
		Items:    names,
		Action:   &api.NoticeAction{Kind: api.NoticeActionApp, App: UpdatesApp},
	}}, nil
}

// RebootSource는 재부팅 권장을 알리는 출처다(관리자 전용).
type RebootSource struct{ Reader *Reader }

func (RebootSource) Name() string { return "reboot-required" }
func (RebootSource) Admin() bool  { return true }

// Collect는 재부팅 권장 파일을 읽는다. 리비전은 재부팅을 요청한 패키지 목록으로 만든다.
func (s RebootSource) Collect(context.Context) ([]api.Notice, error) {
	rb, err := s.Reader.Reboot()
	if err != nil {
		return nil, err
	}
	if !rb.Required {
		return nil, nil
	}
	n := api.Notice{
		ID:       RebootNoticeID,
		Code:     api.NoticeRebootRequired,
		Level:    api.NoticeWarning,
		Revision: notify.Revision(append([]string{"reboot"}, rb.Packages...)...),
		Items:    rb.Packages,
		Action:   &api.NoticeAction{Kind: api.NoticeActionPower},
	}
	if !rb.Since.IsZero() {
		n.Since = rb.Since.UTC().Format(time.RFC3339)
	}
	return []api.Notice{n}, nil
}
