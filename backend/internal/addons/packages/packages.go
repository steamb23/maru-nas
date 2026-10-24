// Package packages는 시스템 업데이트 애드온이다. OMV가 만드는 APT 업데이트 목록과 재부팅 권장 파일을 직접 읽고,
// 업데이트 확인(Apt.update)과 모두 설치(Apt.upgrade, dist-upgrade)를 서버 백그라운드 작업(internal/jobs)으로 시작한다.
// 작업 진행·출력·결과는 /api/jobs가 준다. 골라서 설치(Apt.install)는 하지 않는다(OMV 워크벤치와 같다).
package packages

import (
	"context"
	"errors"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/aptindex"
	"maru/backend/internal/jobs"
)

// ID는 시스템 업데이트 애드온의 id다.
const ID = "packages"

// Source는 provider 이름이다.
const Source = "apt-index"

// Manifest는 시스템 업데이트 애드온의 선언이다. 목록 파일은 누구나 읽을 수 있지만, OMV가 업데이트 RPC와 목록을
// 관리자에게만 주므로 애드온도 관리자 전용이다. Exec.getOutput은 작업 감시자(omvjob)가 이 애드온의 허용 목록으로 부른다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.packages",
	Icon:  "software-update-available",
	Views: api.AddonViews{
		// 크기는 브라우저 실측으로 정한다. 내용을 바꾸면 다시 잰다.
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 760, Height: 560},
			MinSize:     &api.Size{Width: 420, Height: 360},
		},
	},
	Requires: api.Requirements{Admin: true},
	RPC: api.RPCMethods{
		Read:  []string{"Exec.getOutput"},
		Write: []string{"Apt.update", "Apt.upgrade"},
	},
	Actions: []api.ActionDecl{
		{Name: "check", Admin: true},
		{Name: "upgrade", Admin: true, Confirm: true},
	},
}

// actionTimeout은 작업을 시작하는 데 주는 시간이다. 작업 자체는 서버가 따로 끝까지 지켜본다.
const actionTimeout = 30 * time.Second

type provider struct{}

func (provider) Name() string               { return Source }
func (provider) Requires() api.Requirements { return api.Requirements{Admin: true} }

// New는 시스템 업데이트 애드온을 반환한다.
func New(r *aptindex.Reader) addons.Addon {
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{provider{}},
		Fetch: func(context.Context, addons.Request) (any, error) {
			return Read(r)
		},
		Actions: map[string]addons.Action{
			"check":   {Timeout: actionTimeout, Run: start(jobs.KindAptUpdate)},
			"upgrade": {Timeout: actionTimeout, Run: start(jobs.KindAptUpgrade)},
		},
	}
}

// start는 작업을 시작하는 조작이다. 대상은 받지 않는다.
func start(kind jobs.Kind) func(context.Context, addons.ActionRequest) error {
	return func(ctx context.Context, req addons.ActionRequest) error {
		if req.Target != "" {
			return addons.ErrInvalidTarget
		}
		if req.Jobs == nil {
			return errors.New("packages: no job starter")
		}
		_, err := req.Jobs.Start(ctx, string(kind))
		return err
	}
}

// Read는 업데이트 목록과 재부팅 권장 상태를 읽는다. 목록 파일이 없으면 Indexed가 false다.
func Read(r *aptindex.Reader) (api.PackagesResponse, error) {
	res := api.PackagesResponse{
		AddonSources: api.AddonSources{Sources: []string{Source}},
		Packages:     []api.PackageUpdate{},
	}
	idx, err := r.Index()
	switch {
	case errors.Is(err, aptindex.ErrNoIndex):
	case err != nil:
		return api.PackagesResponse{}, err
	default:
		res.Indexed = true
		res.IndexedAt = idx.ModTime.UTC().Format(time.RFC3339)
		for _, p := range idx.Packages {
			res.Packages = append(res.Packages, api.PackageUpdate{
				Name:       p.Name,
				Version:    p.Version,
				OldVersion: p.OldVersion,
				Repository: p.Repository,
				Section:    p.Section,
				Summary:    p.Summary,
				Filename:   p.Filename,
				Size:       p.Size,
			})
		}
	}
	rb, err := r.Reboot()
	if err != nil {
		return api.PackagesResponse{}, err
	}
	res.Reboot = api.RebootInfo{Required: rb.Required, Packages: rb.Packages}
	if rb.Required && !rb.Since.IsZero() {
		res.Reboot.Since = rb.Since.UTC().Format(time.RFC3339)
	}
	return res, nil
}
