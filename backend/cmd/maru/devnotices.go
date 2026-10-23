package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"maru/backend/internal/api"
	"maru/backend/internal/notify"
)

// envDevNotices는 개발 서버(make dev-fake) 전용 환경 변수다. 가짜 알림 파일의 경로이며, 가짜 OMV 프로세스
// (cmd/fakeomv)가 FAKENOTICES 시나리오대로 쓴다. 운영에는 알림 출처를 고르는 설정이 없으므로 플래그로 두지 않는다
// (가동 설정 목록·설치기 레지스트리에도 넣지 않는다).
const envDevNotices = "MARU_DEV_NOTICES"

// maxDevNoticeBytes는 가짜 알림 파일 크기의 상한이다.
const maxDevNoticeBytes = 1 << 20

// devNoticeFile은 가짜 알림 파일의 형식이다. admin은 관리자에게만, everyone은 모두에게 보인다.
type devNoticeFile struct {
	Admin    []api.Notice `json:"admin"`
	Everyone []api.Notice `json:"everyone"`
}

// parseDevNotices는 가짜 알림 파일을 읽는다. 리비전이 비어 있는 알림은 내용으로 리비전을 채워,
// 파일을 손으로 고치면 리비전도 바뀌게 한다.
func parseDevNotices(r io.Reader) (devNoticeFile, error) {
	var f devNoticeFile
	if err := json.NewDecoder(io.LimitReader(r, maxDevNoticeBytes)).Decode(&f); err != nil {
		return devNoticeFile{}, fmt.Errorf("dev notices: %w", err)
	}
	for _, list := range [][]api.Notice{f.Admin, f.Everyone} {
		for i := range list {
			if list[i].Revision == "" {
				b, _ := json.Marshal(list[i])
				list[i].Revision = notify.Revision(string(b))
			}
		}
	}
	return f, nil
}

// devNotices는 가짜 알림 파일을 읽는 알림 출처다. admin이면 파일의 admin 목록을, 아니면 everyone 목록을 준다.
type devNotices struct {
	path  string
	admin bool
}

func (d devNotices) Name() string {
	if d.admin {
		return "dev-admin"
	}
	return "dev"
}

func (d devNotices) Admin() bool { return d.admin }

// Collect는 파일을 읽는다. 아직 없으면(가짜 OMV가 쓰기 전) 알릴 것이 없다.
func (d devNotices) Collect(context.Context) ([]api.Notice, error) {
	f, err := os.Open(d.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	file, err := parseDevNotices(f)
	if err != nil {
		return nil, err
	}
	if d.admin {
		return file.Admin, nil
	}
	return file.Everyone, nil
}
