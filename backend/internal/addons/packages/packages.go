// Package packages는 시스템 업데이트 애드온이다. OMV가 만드는 APT 업데이트 목록과 재부팅 권장 파일을 직접 읽고,
// 업데이트 확인(Apt.update)과 모두 설치(Apt.upgrade, dist-upgrade)를 서버 백그라운드 작업(internal/jobs)으로 시작한다.
// 작업 진행·출력·결과는 /api/jobs가 준다. 골라서 설치(Apt.install)는 하지 않는다(OMV 워크벤치와 같다).
//
// 패키지 하나의 긴 설명은 ?detail=<이름>(업데이트 목록과 dpkg 상태를 직접 읽음), 변경 기록은 결과를 돌려주는
// changelog 조작(Apt.getChangeLog, changelog.go)으로 준다.
package packages

import (
	"context"
	"errors"
	"log"
	"slices"
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
		Read: []string{"Exec.getOutput"},
		// Apt.getChangeLog는 데이터를 읽지만 파라미터로 셸 명령을 만들므로 쓰기로 선언해 조작으로만 부른다.
		Write: []string{"Apt.update", "Apt.upgrade", "Apt.getChangeLog"},
	},
	Actions: []api.ActionDecl{
		{Name: "check", Admin: true},
		{Name: "upgrade", Admin: true, Confirm: true},
		{Name: "changelog", Admin: true},
	},
}

// SourceDpkg는 설치된 버전의 긴 설명을 읽는 dpkg 상태 파일이다. 읽지 못하면 응답의 failedSources에 담는다.
const SourceDpkg = "dpkg-status"

// actionTimeout은 작업을 시작하는 데 주는 시간이다. 작업 자체는 서버가 따로 끝까지 지켜본다.
const actionTimeout = 30 * time.Second

type provider struct{}

func (provider) Name() string               { return Source }
func (provider) Requires() api.Requirements { return api.Requirements{Admin: true} }

// New는 시스템 업데이트 애드온을 반환한다.
func New(r *aptindex.Reader) addons.Addon {
	cl := newChangelogs()
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{provider{}},
		Fetch: func(_ context.Context, req addons.Request) (any, error) {
			res, pkgs, err := read(r)
			if err != nil {
				return nil, err
			}
			// 설명은 시스템 업데이트 창이 행의 자세히 보기를 열 때 detail=<이름>으로 묻는다.
			if name := req.Query.Get("detail"); name != "" {
				d, err := Detail(r, pkgs, name)
				if err != nil {
					log.Printf("packages: read installed description of %q: %v", name, err)
					res.FailedSources = append(res.FailedSources, SourceDpkg)
				}
				res.Detail = d
			}
			return res, nil
		},
		Actions: map[string]addons.Action{
			"check":     {Timeout: actionTimeout, Run: start(jobs.KindAptUpdate)},
			"upgrade":   {Timeout: actionTimeout, Run: start(jobs.KindAptUpgrade)},
			"changelog": {Timeout: changelogTimeout, Query: cl.query(r)},
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
	res, _, err := read(r)
	return res, err
}

// read는 Read와 같고, 응답에 넣지 않는 긴 설명을 담은 목록도 함께 돌려준다.
func read(r *aptindex.Reader) (api.PackagesResponse, []aptindex.Package, error) {
	var pkgs []aptindex.Package
	res := api.PackagesResponse{
		AddonSources: api.AddonSources{Sources: []string{Source}},
		Packages:     []api.PackageUpdate{},
	}
	idx, err := r.Index()
	switch {
	case errors.Is(err, aptindex.ErrNoIndex):
	case err != nil:
		return api.PackagesResponse{}, nil, err
	default:
		pkgs = idx.Packages
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
		return api.PackagesResponse{}, nil, err
	}
	res.Reboot = api.RebootInfo{Required: rb.Required, Packages: rb.Packages}
	if rb.Required && !rb.Since.IsZero() {
		res.Reboot.Since = rb.Since.UTC().Format(time.RFC3339)
	}
	return res, pkgs, nil
}

// Detail은 pkgs(업데이트 목록) 중 name 패키지의 설명이다. 목록에 없으면 nil이다. 긴 설명은 설치될 버전의
// 것을 먼저 쓰고, 없으면(Debian 공식 저장소) 지금 설치된 버전의 것을 dpkg 상태에서 읽는다. dpkg 상태를
// 읽지 못하면 요약만 담고 오류를 함께 돌려준다.
func Detail(r *aptindex.Reader, pkgs []aptindex.Package, name string) (*api.PackageDetail, error) {
	i := slices.IndexFunc(pkgs, func(p aptindex.Package) bool { return p.Name == name })
	if i < 0 {
		return nil, nil
	}
	p := pkgs[i]
	d := &api.PackageDetail{Name: p.Name, Summary: p.Summary}
	if p.Description != "" {
		d.Description, d.DescriptionFrom = p.Description, api.DescriptionCandidate
		return d, nil
	}
	if p.OldVersion == "" {
		return d, nil
	}
	desc, err := r.InstalledDescription(p.Name)
	if err != nil {
		return d, err
	}
	if desc != "" {
		d.Description, d.DescriptionFrom = desc, api.DescriptionInstalled
	}
	return d, nil
}
