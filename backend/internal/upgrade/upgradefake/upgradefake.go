// Package upgradefake는 `make dev-fake`와 테스트에서 쓰는 가짜 업그레이드 서비스다(설치기의 --upgrade-service 대신).
//
// maru가 상태 디렉터리에 남긴 확인 요청과 가짜 권한 도우미가 쓴 적용 요청을 짧은 주기로 보고, 단계마다 조금씩
// 기다리며 가짜 루트의 상태 파일(config.UpgradeStatusFile)에 진행과 결과를 쓴다. 저장소·빌드·설치는 하지 않고
// maru를 다시 시작하지도 않는다(재시작 화면은 시험 VM에서 확인한다). 값은 모두 합성한 가짜 값이다.
// 운영 바이너리(cmd/maru, cmd/maru-helper)에서 import하지 않는다.
package upgradefake

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"maru/backend/config"
)

// 시나리오(Options.Scenario, 개발 서버의 FAKEUPGRADE_SCENARIO).
const (
	// ScenarioWeb은 대시보드에서 업그레이드할 수 있는 새 버전이다(커밋 목록, 새 설정 질문, 업그레이드 뒤 할 일).
	ScenarioWeb = "web"
	// ScenarioManual은 수동 재설치가 필요한 새 버전이다.
	ScenarioManual = "manual"
	// ScenarioLatest는 이미 최신이다.
	ScenarioLatest = "latest"
	// ScenarioCheckFail은 확인이 실패한다(원격이면 fetch, 로컬이면 계획 단계).
	ScenarioCheckFail = "check-fail"
	// ScenarioApplyFail은 확인은 web과 같고 업그레이드가 빌드 단계에서 실패한다.
	ScenarioApplyFail = "apply-fail"
	// ScenarioDisabled는 대시보드에서 업그레이드가 꺼져 있다(root 쪽 설정 파일이 없다).
	ScenarioDisabled = "disabled"
)

// Scenarios는 받는 시나리오다.
var Scenarios = []string{ScenarioWeb, ScenarioManual, ScenarioLatest, ScenarioCheckFail, ScenarioApplyFail, ScenarioDisabled}

// 가짜 커밋. 설치된 커밋에서 대상까지 커밋 다섯 개가 있다. 가짜 저장소는 stable 브랜치에서 시작하고, main 채널로
// 옮기면 대상까지 간다. main에서 stable 채널로 돌아오면 stable은 그보다 앞의 커밋(stableRevision)이라 덮어써서 설치한다.
const (
	installedRevision = "1111111111111111111111111111111111111111"
	targetRevision    = "2222222222222222222222222222222222222222"
	stableRevision    = "3333333333333333333333333333333333333333"
)

// Options는 Start의 설정이다.
type Options struct {
	// Root는 root 쪽 설정 파일과 상태 파일 앞에 붙는 가짜 루트다(maru의 MARU_DEV_UPGRADE_ROOT와 같다).
	Root string
	// CheckRequest는 maru가 남기는 확인 요청 파일이다(<상태 디렉터리>/upgrade-check.json).
	CheckRequest string
	// ApplyDir는 가짜 도우미의 upgrade.apply가 적용 요청을 쓰는 디렉터리다(helperfake.Server.UpgradeDir).
	ApplyDir string
	// Scenario는 시나리오다. 비우면 ScenarioWeb이다.
	Scenario string
	// Step은 단계 사이의 지연이다. 0이면 기다리지 않는다(테스트).
	Step time.Duration
	// Poll은 요청을 보는 주기다. 0이면 300ms다.
	Poll time.Duration
	Now  func() time.Time
	Logf func(format string, args ...any)
}

// Service는 돌고 있는 가짜 업그레이드 서비스다.
type Service struct {
	o        Options
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	status   config.UpgradeStatus
	revision string
	// branch는 가짜 저장소의 지금 브랜치다.
	branch string
}

// Start는 가짜 루트를 처음 상태로 되돌리고(설정 파일을 쓰고 상태 파일을 지운다) 요청을 보기 시작한다.
func Start(o Options) (*Service, error) {
	if o.Scenario == "" {
		o.Scenario = ScenarioWeb
	}
	if !slices.Contains(Scenarios, o.Scenario) {
		return nil, fmt.Errorf("unknown fake upgrade scenario %q (one of %s)", o.Scenario, strings.Join(Scenarios, ", "))
	}
	if o.Root == "" || o.CheckRequest == "" || o.ApplyDir == "" {
		return nil, errors.New("fake upgrade service needs Root, CheckRequest and ApplyDir")
	}
	if o.Poll == 0 {
		o.Poll = 300 * time.Millisecond
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	s := &Service{o: o, stop: make(chan struct{}), done: make(chan struct{}), revision: installedRevision, branch: config.UpgradeChannelStable}
	envFile := s.path(config.UpgradeEnvFile)
	if err := os.MkdirAll(filepath.Dir(envFile), 0o755); err != nil {
		return nil, err
	}
	if o.Scenario == ScenarioDisabled {
		if err := os.Remove(envFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	} else {
		env := "# Fake file written by the fake upgrade service (make dev-fake).\n" +
			config.EnvUpgradeRepo + "=/home/fake-owner/maru-nas\n" +
			config.EnvUpgradeUser + "=fake-owner\n" +
			config.EnvUpgradePath + "=/usr/local/bin:/usr/bin:/bin\n"
		if err := os.WriteFile(envFile, []byte(env), 0o644); err != nil {
			return nil, err
		}
	}
	if err := os.Remove(s.path(config.UpgradeStatusFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := s.writeRecord(config.Version); err != nil {
		return nil, err
	}
	go s.loop()
	return s, nil
}

// Close는 요청 보기를 멈춘다. 진행 중인 처리가 끝날 때까지 기다린다.
func (s *Service) Close() {
	s.once.Do(func() { close(s.stop) })
	<-s.done
}

func (s *Service) path(p string) string { return s.o.Root + p }

func (s *Service) loop() {
	defer close(s.done)
	t := time.NewTicker(s.o.Poll)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
		}
		if err := s.serve(); err != nil {
			s.o.Logf("fake upgrade: %v", err)
		}
	}
}

// serve는 요청 하나를 처리한다(진짜 서비스처럼 적용이 먼저이고, 요청은 내용과 상관없이 먼저 지운다).
func (s *Service) serve() error {
	apply := filepath.Join(s.o.ApplyDir, config.UpgradeApplyRequestName)
	if data, err := take(apply); err == nil {
		if s.o.Scenario == ScenarioDisabled {
			return nil
		}
		return s.apply(data)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if data, err := take(s.o.CheckRequest); err == nil {
		if s.o.Scenario == ScenarioDisabled {
			s.o.Logf("fake upgrade: disabled, check request ignored")
			return nil
		}
		return s.check(data)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func take(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, os.Remove(path)
}

func (s *Service) stamp() string { return s.o.Now().UTC().Format(time.RFC3339) }

// stage는 단계를 바꾸고 상태 파일에 쓴 뒤 Step만큼 기다린다.
func (s *Service) stage(name string) error {
	s.status.Stage = name
	if err := s.write(); err != nil {
		return err
	}
	select {
	case <-time.After(s.o.Step):
	case <-s.stop:
	}
	return nil
}

func (s *Service) write() error {
	s.status.Format = config.UpgradeStatusFormat
	s.status.UpdatedAt = s.stamp()
	data, err := json.MarshalIndent(s.status, "", "  ")
	if err != nil {
		return err
	}
	path := s.path(config.UpgradeStatusFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), ".status.tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeRecord는 설치기처럼 설치 기록을 쓴다(지금 가짜 커밋과 version).
func (s *Service) writeRecord(version string) error {
	rec := s.record()
	rec.Version = version
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	path := s.path(config.InstallRecordFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (s *Service) fail(state string, en, ko, log string) error {
	s.o.Logf("fake upgrade: %s at %s: %s", state, s.status.Stage, en)
	s.status.State = state
	s.status.Error = &config.UpgradeError{Text: config.LocalizedText{En: en, Ko: ko}, Log: log}
	if s.status.Apply != nil {
		s.status.Apply.FinishedAt = s.stamp()
	}
	return s.write()
}

func (s *Service) check(data []byte) error {
	var req config.UpgradeCheckRequest
	s.status.State, s.status.Check, s.status.Error = config.UpgradeStateChecking, nil, nil
	if err := s.stage(config.UpgradeStageInspect); err != nil {
		return err
	}
	if json.Unmarshal(data, &req) != nil || req.Format != config.UpgradeRequestFormat ||
		(req.Mode != config.UpgradeModeRemote && req.Mode != config.UpgradeModeLocal) || !config.ValidUpgradeChannel(req.Channel) {
		return s.fail(config.UpgradeStateCheckFailed, "the check request from the dashboard is not valid",
			"대시보드의 확인 요청이 올바르지 않습니다", "")
	}
	s.o.Logf("fake upgrade: check (%s, channel %q, branch %s, scenario %s)", req.Mode, req.Channel, s.branch, s.o.Scenario)
	chk := &config.UpgradeCheck{Mode: req.Mode, Head: s.revision, Branch: s.branch, Target: targetRevision,
		Installed: s.record(), Commits: []config.UpgradeCommit{}, Manual: []config.UpgradeReason{}}
	s.status.Branch, s.status.Detached = s.branch, false
	if req.Mode == config.UpgradeModeRemote {
		follow := s.branch
		if req.Channel != config.UpgradeChannelCurrent {
			chk.Channel, chk.Switch, follow = req.Channel, req.Channel != s.branch, req.Channel
		}
		chk.Upstream = "origin/" + follow
		if chk.Switch && req.Channel == config.UpgradeChannelStable {
			chk.Target = stableRevision
		}
		if err := s.stage(config.UpgradeStageFetch); err != nil {
			return err
		}
		if s.o.Scenario == ScenarioCheckFail {
			return s.fail(config.UpgradeStateCheckFailed,
				"git fetch failed. The repository owner's git settings and credentials are used without asking for passwords; check the network and that fetching works for that account without a prompt (see the log)",
				"git fetch가 실패했습니다. 저장소 소유자 계정의 git 설정과 자격 증명을 비밀번호를 묻지 않고 씁니다. 네트워크와, 그 계정에서 묻는 것 없이 가져올 수 있는지 확인하세요(로그 참고)",
				"$ git -C /home/fake-owner/maru-nas fetch\nfatal: unable to access 'https://git.example.invalid/maru-nas.git/': Could not resolve host: git.example.invalid\n")
		}
		if err := s.stage(config.UpgradeStageInspect); err != nil {
			return err
		}
	} else {
		chk.Head = targetRevision
	}
	if s.o.Scenario == ScenarioLatest || s.revision == chk.Target {
		chk.Target, chk.UpToDate = s.revision, true
		return s.checked(chk)
	}
	if chk.Target == stableRevision {
		// main에서 stable로 돌아오면 이전 버전을 덮어쓴다. 새 커밋은 없고 가짜 커밋 다섯 개가 빠진다.
		chk.Dropped = len(fakeCommits(s.o.Now()))
	} else {
		chk.Commits = fakeCommits(s.o.Now())
	}
	if err := s.stage(config.UpgradeStagePlan); err != nil {
		return err
	}
	if s.o.Scenario == ScenarioCheckFail {
		return s.fail(config.UpgradeStateCheckFailed,
			"the installer of "+targetRevision+" could not make an upgrade plan (see the log)",
			targetRevision+"의 설치기가 업그레이드 계획을 만들지 못했습니다(로그 참고)",
			"$ go build -trimpath -o bin/maru-install .\ninternal/setup/plan.go:12:2: undefined: fakeSymbol\n")
	}
	chk.Plan = s.plan(chk.Target)
	if s.o.Scenario == ScenarioManual {
		chk.Manual = append(chk.Manual, config.UpgradeReason{ID: config.UpgradeReasonDirty, Command: config.UpgradeCommandInstall,
			Text: config.LocalizedText{
				En: "the work tree has uncommitted changes. Commit or discard them over SSH, then check again",
				Ko: "작업 트리에 커밋하지 않은 변경이 있습니다. SSH로 커밋하거나 되돌린 뒤 다시 확인하세요"}})
		chk.Manual = append(chk.Manual, chk.Plan.Manual...)
	}
	return s.checked(chk)
}

// checked는 확인을 마친다(진짜 서비스의 done과 같은 규칙으로 Web·Command를 정한다).
func (s *Service) checked(chk *config.UpgradeCheck) error {
	if chk.UpToDate {
		chk.Manual = []config.UpgradeReason{}
	}
	chk.CheckedAt = s.stamp()
	chk.Web = !chk.UpToDate && chk.Plan != nil && chk.Plan.Web && len(chk.Manual) == 0
	if len(chk.Manual) > 0 {
		chk.Command = config.UpgradeCommandInstall
		for _, r := range chk.Manual {
			if r.Command == config.UpgradeCommandReinstall {
				chk.Command = config.UpgradeCommandReinstall
			}
		}
	}
	s.status.State, s.status.Stage, s.status.Check = config.UpgradeStateChecked, "", chk
	s.o.Logf("fake upgrade: checked (web %t, manual %d, up to date %t)", chk.Web, len(chk.Manual), chk.UpToDate)
	return s.write()
}

func (s *Service) record() *config.InstallRecord {
	return &config.InstallRecord{Format: config.InstallRecordFormat, Version: config.Version, Revision: s.revision,
		RevisionTime: "2026-10-01T09:00:00+09:00", InstalledAt: "2026-10-01T00:30:00Z"}
}

// nextVersion은 config.Version의 다음 마이너 버전이다.
func nextVersion() string {
	parts := strings.Split(config.Version, ".")
	if len(parts) == 3 {
		if minor, err := strconv.Atoi(parts[1]); err == nil {
			return fmt.Sprintf("%s.%d.0", parts[0], minor+1)
		}
	}
	return config.Version
}

func fakeCommits(now time.Time) []config.UpgradeCommit {
	subjects := []string{
		"업데이트 페이지에 커밋 목록 더하기 (fake)",
		"Fix the wallpaper fade on slow devices (fake)",
		"설치기에 예시 설정 더하기 (fake)",
		"Translate the update page (fake)",
		"README에 대시보드에서 업그레이드 적기 (fake)",
	}
	out := make([]config.UpgradeCommit, len(subjects))
	for i, subj := range subjects {
		out[i] = config.UpgradeCommit{
			Hash:    fmt.Sprintf("%02d%038d", 90-i, i),
			Author:  "Fake Maintainer",
			Time:    now.Add(-time.Duration(i+1) * 26 * time.Hour).Format(time.RFC3339),
			Subject: subj,
		}
	}
	out[0].Hash = targetRevision
	return out
}

// plan은 대상 설치기가 낸 것 같은 업그레이드 계획이다. stable로 돌아오는 대상은 지금과 같은 버전이다.
func (s *Service) plan(target string) *config.UpgradePlan {
	version := nextVersion()
	if target == stableRevision {
		version = config.Version
	}
	p := &config.UpgradePlan{
		Format: config.UpgradePlanFormat, Version: version, Revision: target, Installed: s.record(), Web: true,
		Manual: []config.UpgradeReason{},
		Questions: []config.UpgradeQuestion{
			{
				Name: "public-origin", Kind: config.UpgradeQuestionInput, Dash: true, EmptyOK: true, Reason: config.UpgradeQuestionNew,
				Title: config.LocalizedText{En: "Public dashboard address for passkeys (- to turn off, fake)", Ko: "패스키에 쓸 대시보드 공개 주소(끄려면 -, 가짜)"},
				Help: config.LocalizedText{
					En: "Passkeys need the public HTTPS address browsers use for the dashboard, e.g. https://desk.example.lan\nEnter - to turn passkeys off.",
					Ko: "패스키에는 브라우저가 대시보드에 접속하는 공개 HTTPS 주소가 필요합니다. 예: https://desk.example.lan\n-를 입력하면 패스키를 끕니다."},
			},
			{
				Name: "smart-refresh-at", Kind: config.UpgradeQuestionInput, Default: "00:00", Reason: config.UpgradeQuestionInvalid,
				Title: config.LocalizedText{En: "Time to refresh SMART data (HH:MM, fake)", Ko: "SMART 정보를 다시 가져올 시각(HH:MM, 가짜)"},
				Error: &config.LocalizedText{En: "\"25:00\" is not a time of day (HH:MM)", Ko: "\"25:00\"은(는) 올바른 시각(HH:MM)이 아닙니다"},
			},
			{
				Name: "example-toggle", Kind: config.UpgradeQuestionConfirm, Default: "yes", Reason: config.UpgradeQuestionNew,
				Title: config.LocalizedText{En: "Turn on the example feature? (fake)", Ko: "예시 기능을 켤까요?(가짜)"},
			},
		},
		Notes: []config.LocalizedText{{
			En: "Recreate the reverse proxy container so it picks up the new socket directory (fake note).",
			Ko: "리버스 프록시 컨테이너를 다시 만들어 새 소켓 디렉터리를 반영하세요(가짜 안내).",
		}},
		Requires: config.UpgradeRequires{Go: "1.27.1", Node: "^20.19.0 || >=22.12.0", NodeFound: "22.12.0"},
	}
	if s.o.Scenario == ScenarioManual {
		p.Web, p.Questions, p.Notes = false, []config.UpgradeQuestion{}, []config.LocalizedText{}
		p.Command = config.UpgradeCommandReinstall
		p.Manual = []config.UpgradeReason{{ID: config.UpgradeReasonManualList, Command: config.UpgradeCommandReinstall,
			Text: config.LocalizedText{
				En: "this version moves the service socket; reinstall once with ./install.sh --reinstall (fake)",
				Ko: "이 버전은 서비스 소켓 경로를 바꿉니다. ./install.sh --reinstall로 한 번 다시 설치하세요(가짜)"}}}
	}
	return p
}

func (s *Service) apply(data []byte) error {
	prev := s.status
	var req config.UpgradeApplyRequest
	valid := json.Unmarshal(data, &req) == nil && req.Format == config.UpgradeRequestFormat && config.ValidUpgradeTarget(req.Target)
	s.status.State, s.status.Error = config.UpgradeStateApplying, nil
	s.status.Apply = &config.UpgradeApply{Target: req.Target, User: req.User, RequestedAt: req.RequestedAt, StartedAt: s.stamp()}
	if err := s.stage(config.UpgradeStageInspect); err != nil {
		return err
	}
	if !valid {
		return s.fail(config.UpgradeStateApplyFailed, "the upgrade request is not valid", "업그레이드 요청이 올바르지 않습니다", "")
	}
	s.o.Logf("fake upgrade: apply %s by %s", req.Target, req.User)
	chk := prev.Check
	if prev.State != config.UpgradeStateChecked || chk == nil || !chk.Web || chk.Plan == nil || chk.Target != req.Target {
		return s.fail(config.UpgradeStateApplyFailed, "there is no finished check that allows upgrading to this commit. Check again",
			"이 커밋으로 업그레이드할 수 있다고 끝난 확인이 없습니다. 다시 확인하세요", "")
	}
	s.status.Check = chk
	if err := s.stage(config.UpgradeStagePlan); err != nil {
		return err
	}
	if en, ko, ok := checkSettings(chk.Plan.Questions, req.Settings); !ok {
		return s.fail(config.UpgradeStateApplyFailed, "the new version did not accept the settings: "+en,
			"새 버전이 설정 값을 받지 않았습니다: "+ko, "")
	}
	for _, st := range []string{config.UpgradeStageUpdate, config.UpgradeStageBuild} {
		if err := s.stage(st); err != nil {
			return err
		}
	}
	if s.o.Scenario == ScenarioApplyFail {
		return s.fail(config.UpgradeStateApplyFailed, "the build failed (see the log)", "빌드가 실패했습니다(로그 참고)",
			"$ npm --prefix frontend ci\nnpm error code ETIMEDOUT\nnpm error network request to https://registry.example.invalid/ failed\n")
	}
	if err := s.stage(config.UpgradeStageInstall); err != nil {
		return err
	}
	s.revision = req.Target
	if chk.Switch {
		s.branch = chk.Channel
	}
	s.status.Branch = s.branch
	if err := s.writeRecord(chk.Plan.Version); err != nil {
		return err
	}
	s.status.State, s.status.Stage = config.UpgradeStateApplied, ""
	s.status.Apply.FinishedAt, s.status.Apply.Version = s.stamp(), chk.Plan.Version
	s.o.Logf("fake upgrade: applied %s (maru is not restarted in the fake service)", chk.Plan.Version)
	return s.write()
}

// checkSettings는 진짜 서비스(settingArgs와 새 설치기의 검사)처럼 답을 본다. 계획에 없는 이름, 빠진 답, 비어 있으면
// 안 되는 입력, 시각 꼴이 아닌 smart-refresh-at을 받지 않는다.
func checkSettings(questions []config.UpgradeQuestion, given map[string]string) (en, ko string, ok bool) {
	for name := range given {
		if !slices.ContainsFunc(questions, func(q config.UpgradeQuestion) bool { return q.Name == name }) {
			return "--" + name + ": not asked", "--" + name + ": 묻지 않은 설정입니다", false
		}
	}
	for _, q := range questions {
		v, found := given[q.Name]
		if q.Dash && v == "-" {
			v = ""
		}
		switch {
		case !found || !config.ValidUpgradeSetting(q.Name, v):
			return "--" + q.Name + ": needs a value", "--" + q.Name + ": 값이 필요합니다", false
		case q.Kind == config.UpgradeQuestionConfirm && v != "yes" && v != "no":
			return "--" + q.Name + ": answer yes or no", "--" + q.Name + ": yes 또는 no로 답하세요", false
		case q.Kind == config.UpgradeQuestionInput && v == "" && !q.EmptyOK:
			return "--" + q.Name + ": needs a value", "--" + q.Name + ": 값이 필요합니다", false
		case q.Name == "smart-refresh-at":
			if _, _, err := config.ParseRefreshAt(v); err != nil {
				return "--" + q.Name + ": " + strconv.Quote(v) + " is not a time of day (HH:MM)",
					"--" + q.Name + ": " + strconv.Quote(v) + "은(는) 올바른 시각(HH:MM)이 아닙니다", false
			}
		}
	}
	return "", "", true
}
