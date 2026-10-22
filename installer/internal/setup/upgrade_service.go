package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"maru/backend/config"
	"maru/installer/internal/prompt"
)

// 업그레이드 서비스(--upgrade-service). maru-upgrade.path가 확인 요청(maru의 상태 디렉터리) 또는 적용 요청(도우미의
// upgrade.apply)을 보면 maru-upgrade.service가 root 소유 설치기 복사본으로 이것을 실행한다. 한 번에 요청 하나를
// 처리하고(적용이 먼저), 진행과 결과는 상태 파일(config.UpgradeStatusFile)에 쓴다. 설계는 .claude/docs/upgrade.md,
// 명령 실행·파일 처리는 upgrade_exec.go에 있다.
//
//   - 확인: 저장소 상태를 보고(원격이면 git fetch 뒤 추적 브랜치, 로컬이면 HEAD) 대상 커밋을 정한 뒤, 대상 커밋의 설치기만
//     소유자로 임시 빌드해 --upgrade-plan을 실행한다. 저장소 코드는 root로 실행하지 않는다
//   - 적용: 마지막 확인과 저장소 상태를 다시 확인하고, 입력한 설정 값을 새 버전의 계획으로 검사한 뒤 fast-forward(원격),
//     소유자로 빌드, 새 설치기를 root로 --service-install --upgrade로 실행한다
//
// 작업 실패는 상태 파일에 남기고 0으로 끝낸다(경로 유닛이 계속 지켜보게). 상태 파일을 쓰지 못하면 오류다.

// runUpgradeService는 --upgrade-service다.
func runUpgradeService(c *Context) error {
	if !c.IsRoot {
		return fail(c.L("--upgrade-service must run as root; maru-upgrade.service runs it",
			"--upgrade-service는 root로만 실행할 수 있습니다. maru-upgrade.service가 실행합니다"))
	}
	s := &upgradeService{out: c.Out, now: time.Now, dropPrivileges: true}
	return s.serve()
}

// upgradeService는 업그레이드 서비스 한 번의 실행이다.
type upgradeService struct {
	// root는 상태·요청·설정 파일 앞에 붙이는 가짜 루트다(테스트). 실제는 빈 값이다.
	root string
	// tmp는 임시 빌드 디렉터리를 만들 곳이다. 비우면 os.TempDir()(유닛의 PrivateTmp=)다.
	tmp string
	// dropPrivileges는 소유자 명령을 소유자 uid로 강등하는지다. 테스트는 거짓이다(지금 계정이 소유자).
	dropPrivileges bool
	// extraEnv는 모든 명령에 더할 환경 변수다(테스트).
	extraEnv []string
	now      func() time.Time
	// out은 로그(journal)다. log는 out과 실패했을 때 상태 파일에 남길 끝부분(tail)에 함께 쓴다.
	out  io.Writer
	log  io.Writer
	tail tailBuffer

	cfg        upgradeConfig
	owner      ownerAccount
	sshCommand string
	status     config.UpgradeStatus
}

// upgradeError는 상태 파일에 남길 두 언어 실패 이유다.
type upgradeError struct{ text Text }

func (e *upgradeError) Error() string { return e.text.En }

func uerr(en, ko string) error { return &upgradeError{text: L(en, ko)} }

// serve는 요청 하나를 처리한다.
func (s *upgradeService) serve() error {
	s.log = io.MultiWriter(s.out, &s.tail)
	kind, data, rerr := s.takeRequest()
	if kind == "" {
		if rerr != nil {
			fmt.Fprintln(s.log, "upgrade request:", rerr)
		}
		return nil
	}
	cfgData, err := readNoFollow(s.path(config.UpgradeEnvFile), maxRootFile)
	if errors.Is(err, fs.ErrNotExist) {
		// 설치기가 대시보드에서 업그레이드를 끄면서 지운 경우다. 상태 파일도 지웠으므로 다시 만들지 않는다.
		fmt.Fprintln(s.log, "upgrading from the dashboard is not enabled ("+config.UpgradeEnvFile+" is missing); request ignored")
		return nil
	}
	s.status = s.readStatus()
	if err == nil {
		s.cfg, err = parseUpgradeConfig(cfgData)
	}
	var cfgErr error
	if err != nil {
		cfgErr = uerr("cannot read "+config.UpgradeEnvFile+" ("+err.Error()+"). Run ./install.sh again from the repository as its owner.",
			config.UpgradeEnvFile+" 을(를) 읽을 수 없습니다("+err.Error()+"). 저장소 소유자 계정으로 저장소에서 ./install.sh를 다시 실행하세요.")
	}
	if kind == "apply" {
		return s.runApply(data, rerr, cfgErr)
	}
	return s.runCheck(data, rerr, cfgErr)
}

// takeRequest는 처리할 요청을 읽고 지운다. 적용 요청이 있으면 그것만 처리하고 확인 요청은 다음 실행에 둔다.
func (s *upgradeService) takeRequest() (kind string, data []byte, err error) {
	for _, r := range []struct {
		kind, path string
		max        int64
	}{{"apply", upgradeApplyRequest, maxRootFile}, {"check", upgradeCheckRequest, maxCheckRequest}} {
		data, err := takeFile(s.path(r.path), r.max)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return r.kind, data, err
	}
	return "", nil, nil
}

// stage는 단계를 바꾸고 상태 파일에 쓴다. 로그 끝부분은 단계마다 새로 모은다.
func (s *upgradeService) stage(name string) error {
	s.status.Stage = name
	s.tail.reset()
	fmt.Fprintf(s.log, "==> %s\n", name)
	return s.writeStatus()
}

// finishFailed는 실패를 상태 파일에 남긴다.
func (s *upgradeService) finishFailed(state string, err error) error {
	t := L("unexpected error: "+err.Error(), "예상하지 못한 오류: "+err.Error())
	var ue *upgradeError
	if errors.As(err, &ue) {
		t = ue.text
	}
	fmt.Fprintf(s.out, "%s failed at %s: %s\n", state, s.status.Stage, t.En)
	s.status.State = state
	s.status.Error = &config.UpgradeError{Text: t.localized(), Log: s.tail.String()}
	if s.status.Apply != nil && state == config.UpgradeStateApplyFailed {
		s.status.Apply.FinishedAt = s.stamp()
	}
	return s.writeStatus()
}

// runCheck는 확인 요청을 처리한다.
func (s *upgradeService) runCheck(data []byte, rerr, cfgErr error) error {
	s.status.State, s.status.Check, s.status.Error = config.UpgradeStateChecking, nil, nil
	if err := s.stage(config.UpgradeStageInspect); err != nil {
		return err
	}
	if cfgErr != nil {
		return s.finishFailed(config.UpgradeStateCheckFailed, cfgErr)
	}
	var req config.UpgradeCheckRequest
	if rerr != nil || json.Unmarshal(data, &req) != nil || req.Format != config.UpgradeRequestFormat ||
		(req.Mode != config.UpgradeModeRemote && req.Mode != config.UpgradeModeLocal) {
		return s.finishFailed(config.UpgradeStateCheckFailed, uerr("the check request from the dashboard is not valid",
			"대시보드의 확인 요청이 올바르지 않습니다"))
	}
	chk, err := s.check(req.Mode)
	if err != nil {
		return s.finishFailed(config.UpgradeStateCheckFailed, err)
	}
	s.status.State, s.status.Stage, s.status.Check = config.UpgradeStateChecked, "", chk
	fmt.Fprintf(s.out, "check finished: target %s, web %t, manual reasons %d\n", chk.Target, chk.Web, len(chk.Manual))
	return s.writeStatus()
}

// check는 확인 하나다. 대시보드에서 업그레이드할 수 없는 상태는 수동 재설치 사유로 담고, 확인 자체를 못 하면 오류다.
func (s *upgradeService) check(mode string) (*config.UpgradeCheck, error) {
	chk := &config.UpgradeCheck{Mode: mode, Commits: []config.UpgradeCommit{}, Manual: []config.UpgradeReason{}}
	reason := func(id string, t Text) {
		chk.Manual = append(chk.Manual, config.UpgradeReason{ID: id, Text: t.localized(), Command: config.UpgradeCommandInstall})
	}
	done := func() (*config.UpgradeCheck, error) {
		if chk.UpToDate {
			chk.Manual = []config.UpgradeReason{}
		}
		chk.CheckedAt = s.stamp()
		chk.Web = chk.Target != "" && !chk.UpToDate && chk.Plan != nil && chk.Plan.Web && len(chk.Manual) == 0
		if len(chk.Manual) > 0 {
			chk.Command = config.UpgradeCommandInstall
			for _, r := range chk.Manual {
				if r.Command == config.UpgradeCommandReinstall {
					chk.Command = config.UpgradeCommandReinstall
				}
			}
		}
		return chk, nil
	}

	if t, ok := s.prepareOwner(); !ok {
		reason(config.UpgradeReasonRepository, t)
		return done()
	}
	if missing := s.missingTools(); len(missing) > 0 {
		reason(config.UpgradeReasonTools, L(
			"not found in the build PATH ("+s.cfg.Path+"): "+strings.Join(missing, ", ")+". Install them, or run ./install.sh again as the repository owner to record the current PATH",
			"빌드 PATH("+s.cfg.Path+")에서 찾을 수 없습니다: "+strings.Join(missing, ", ")+". 설치하거나, 저장소 소유자 계정으로 ./install.sh를 다시 실행해 지금 PATH를 기록하세요"))
		if slices.Contains(missing, "git") {
			return done()
		}
	}
	s.prepareSSH()
	head, err := s.git("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, uerr("cannot read the current commit of the repository", "저장소의 지금 커밋을 읽을 수 없습니다")
	}
	chk.Head = head
	target := head
	if mode == config.UpgradeModeRemote {
		up, err := s.git("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
		if err != nil || up == "" {
			reason(config.UpgradeReasonNoUpstream, L(
				"the checkout has no upstream branch to fetch (a tag or a commit is checked out, or the branch tracks nothing). Use \"Check the local repository\" after updating it over SSH, or check out a branch such as stable",
				"체크아웃에 가져올 추적 브랜치가 없습니다(태그나 커밋을 체크아웃했거나 브랜치가 아무것도 추적하지 않습니다). SSH로 저장소를 갱신한 뒤 \"로컬 저장소만 확인\"을 쓰거나 stable 같은 브랜치를 체크아웃하세요"))
			return done()
		}
		chk.Upstream = up
		if err := s.stage(config.UpgradeStageFetch); err != nil {
			return nil, err
		}
		if err := s.run(cmdOpts{owner: true, dir: s.cfg.Repo, timeout: fetchTimeout}, "git", "-C", s.cfg.Repo, "fetch"); err != nil {
			return nil, uerr("git fetch failed. The repository owner's git settings and credentials are used without asking for passwords; check the network and that fetching works for that account without a prompt (see the log)",
				"git fetch가 실패했습니다. 저장소 소유자 계정의 git 설정과 자격 증명을 비밀번호를 묻지 않고 씁니다. 네트워크와, 그 계정에서 묻는 것 없이 가져올 수 있는지 확인하세요(로그 참고)")
		}
		if err := s.stage(config.UpgradeStageInspect); err != nil {
			return nil, err
		}
		if target, err = s.git("rev-parse", "--verify", "@{upstream}^{commit}"); err != nil {
			return nil, uerr("cannot read the upstream commit "+up, "추적 브랜치 "+up+"의 커밋을 읽을 수 없습니다")
		}
	}
	if !config.ValidUpgradeTarget(target) || !config.ValidUpgradeTarget(head) {
		return nil, uerr("unsupported commit id "+target+" (only SHA-1 repositories are supported)",
			"지원하지 않는 커밋 ID입니다: "+target+"(SHA-1 저장소만 지원합니다)")
	}
	chk.Target = target
	if dirty, err := s.dirty(); err != nil {
		return nil, err
	} else if dirty {
		reason(config.UpgradeReasonDirty, L(
			"the work tree has uncommitted changes. Commit or discard them over SSH, then check again",
			"작업 트리에 커밋하지 않은 변경이 있습니다. SSH로 커밋하거나 되돌린 뒤 다시 확인하세요"))
	}
	if mode == config.UpgradeModeRemote {
		ok, err := s.gitTest("merge-base", "--is-ancestor", head, target)
		if err != nil {
			return nil, err
		}
		if !ok {
			reason(config.UpgradeReasonDiverged, L(
				"the checkout cannot be fast-forwarded to "+chk.Upstream+" (it has local commits). Use \"Check the local repository\" to install the checkout as it is, or reconcile it over SSH",
				"체크아웃을 "+chk.Upstream+"(으)로 fast-forward할 수 없습니다(로컬 커밋이 있습니다). 체크아웃을 그대로 설치하려면 \"로컬 저장소만 확인\"을 쓰거나 SSH로 정리하세요"))
		}
	}

	rec := s.readRecord()
	chk.Installed = rec
	if rec == nil {
		reason(config.UpgradeReasonNoRecord, L(
			"there is no usable install record ("+config.InstallRecordFile+"), so the installed commit is unknown. Install once with ./install.sh",
			"쓸 수 있는 설치 기록("+config.InstallRecordFile+")이 없어 설치된 커밋을 알 수 없습니다. ./install.sh로 한 번 설치하세요"))
		return done()
	}
	exists, err := s.gitTest("rev-parse", "--verify", "--quiet", rec.Revision+"^{commit}")
	if err != nil {
		return nil, err
	}
	switch {
	case !exists:
		reason(config.UpgradeReasonCommitMissing, L(
			"the installed commit "+rec.Revision+" is not in the repository (installed from another clone?). Install from this repository once with ./install.sh",
			"설치된 커밋 "+rec.Revision+"이(가) 저장소에 없습니다(다른 클론에서 설치?). 이 저장소에서 ./install.sh로 한 번 설치하세요"))
	case rec.Revision == target:
		chk.UpToDate = true
		return done()
	default:
		ok, err := s.gitTest("merge-base", "--is-ancestor", rec.Revision, target)
		if err != nil {
			return nil, err
		}
		if !ok {
			reason(config.UpgradeReasonNotDescendant, L(
				"the target "+target+" does not follow the installed commit "+rec.Revision+" (it would go back or sideways). Install it with ./install.sh if that is intended",
				"대상 "+target+"이(가) 설치된 커밋 "+rec.Revision+"에서 이어지지 않습니다(되돌아가거나 다른 갈래입니다). 의도한 것이라면 ./install.sh로 설치하세요"))
		} else if chk.Commits, chk.CommitsTruncated, err = s.commits(rec.Revision, target); err != nil {
			return nil, err
		}
	}

	if s.lookPath("go") == "" {
		return done()
	}
	if err := s.stage(config.UpgradeStagePlan); err != nil {
		return nil, err
	}
	plan, goErr, err := s.plan(target, nil)
	if err != nil {
		return nil, err
	}
	if goErr != "" {
		reason(config.UpgradeReasonGo, L(
			"Go cannot build this version: "+goErr+". Update Go (see docs/install.md), then check again",
			"Go로 이 버전을 빌드할 수 없습니다: "+goErr+". Go를 업데이트한 뒤(docs/install.md 참고) 다시 확인하세요"))
		return done()
	}
	if plan.Format != config.UpgradePlanFormat {
		reason(config.UpgradeReasonPlanFormat, L(
			fmt.Sprintf("the new version's upgrade plan (format %d) is not understood by this upgrade service; upgrade with ./install.sh once", plan.Format),
			fmt.Sprintf("새 버전의 업그레이드 계획(형식 %d)을 이 업그레이드 서비스가 모릅니다. ./install.sh로 한 번 업그레이드하세요", plan.Format)))
		return done()
	}
	chk.Plan = plan
	for _, r := range plan.Manual {
		if !slices.ContainsFunc(chk.Manual, func(m config.UpgradeReason) bool { return m.ID == r.ID && m.Setting == r.Setting }) {
			chk.Manual = append(chk.Manual, r)
		}
	}
	return done()
}

// prepareOwner는 설정한 빌드 계정을 찾고 저장소가 그 계정 소유의 디렉터리인지 본다(매 실행). 저장소 경로가 심볼릭
// 링크이거나, 소유자가 바뀌었거나, root면 쓰지 않는다.
func (s *upgradeService) prepareOwner() (Text, bool) {
	a, err := lookupOwner(s.cfg.User)
	if err != nil {
		return L("the build account "+s.cfg.User+" in "+config.UpgradeEnvFile+" cannot be used ("+err.Error()+"). Run ./install.sh again from the repository as its owner",
			config.UpgradeEnvFile+"의 빌드 계정 "+s.cfg.User+"을(를) 쓸 수 없습니다("+err.Error()+"). 저장소 소유자 계정으로 저장소에서 ./install.sh를 다시 실행하세요"), false
	}
	if !s.dropPrivileges && a.UID != os.Getuid() {
		return L("cannot run as "+a.Name+" without root", a.Name+" 계정으로 실행하려면 root가 필요합니다"), false
	}
	fi, err := os.Lstat(s.cfg.Repo)
	if err != nil || !fi.IsDir() {
		return L("the repository "+s.cfg.Repo+" in "+config.UpgradeEnvFile+" does not exist or is not a directory. Run ./install.sh again from the repository",
			config.UpgradeEnvFile+"의 저장소 "+s.cfg.Repo+"이(가) 없거나 디렉터리가 아닙니다. 저장소에서 ./install.sh를 다시 실행하세요"), false
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != a.UID {
		return L("the repository "+s.cfg.Repo+" is no longer owned by "+a.Name+". Run ./install.sh again from the repository as its owner",
			"저장소 "+s.cfg.Repo+"의 소유자가 더 이상 "+a.Name+"이(가) 아닙니다. 저장소 소유자 계정으로 저장소에서 ./install.sh를 다시 실행하세요"), false
	}
	s.owner = a
	return Text{}, true
}

// missingTools는 빌드 PATH에 없는 확인·빌드 도구다(Node.js는 계획이 범위까지 본다).
func (s *upgradeService) missingTools() []string {
	var missing []string
	for _, tool := range []string{"git", "go", "npm", "make"} {
		if s.lookPath(tool) == "" {
			missing = append(missing, tool)
		}
	}
	return missing
}

// prepareSSH는 git의 SSH 명령에 BatchMode를 더한다(비밀번호·호스트 키를 묻지 않고 실패하게). 소유자가 정한
// core.sshCommand가 있으면 그 뒤에 붙인다.
func (s *upgradeService) prepareSSH() {
	base := "ssh"
	if v, err := s.git("config", "--get", "core.sshCommand"); err == nil && v != "" {
		base = v
	}
	s.sshCommand = base + " -o BatchMode=yes"
}

// dirty는 작업 트리에 커밋하지 않은 변경(추적하는 파일, 서브모듈)이 있는지다. 추적하지 않는 파일은 보지 않는다.
func (s *upgradeService) dirty() (bool, error) {
	out, err := s.git("status", "--porcelain", "--untracked-files=no")
	return out != "", err
}

// commits는 from..to의 커밋이다(새것부터, 최대 config.MaxUpgradeCommits개).
func (s *upgradeService) commits(from, to string) ([]config.UpgradeCommit, bool, error) {
	out, err := s.git("log", "--no-color", "--format=%H%x1f%an%x1f%cI%x1f%s", "-n", fmt.Sprint(config.MaxUpgradeCommits+1), from+".."+to, "--")
	if err != nil {
		return nil, false, err
	}
	commits := []config.UpgradeCommit{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 4 {
			continue
		}
		commits = append(commits, config.UpgradeCommit{Hash: f[0], Author: clip(f[1], 100), Time: f[2], Subject: clip(f[3], 200)})
	}
	if len(commits) > config.MaxUpgradeCommits {
		return commits[:config.MaxUpgradeCommits], true, nil
	}
	return commits, false, nil
}

// clip은 s를 n글자까지 자른다(올바르지 않은 UTF-8은 바꾼다).
func clip(s string, n int) string {
	r := []rune(strings.ToValidUTF8(s, "?"))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}

// plan은 대상 커밋의 설치기를 소유자로 임시 빌드해 --upgrade-plan(설정 옵션 opts)을 실행한다. 저장소 작업 트리를 바꾸지
// 않게 git archive로 installer와 backend만 꺼낸다. Go가 이 버전을 빌드할 수 없으면(go env가 실패) goErr에 그 이유를 준다.
// 임시 디렉터리는 소유자의 것이다(계획이 저장소 소유자를 확인하는 설정이 있다).
func (s *upgradeService) plan(target string, opts []string) (*config.UpgradePlan, string, error) {
	dir, err := os.MkdirTemp(s.tmp, "maru-upgrade-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(dir)
	if s.dropPrivileges {
		if err := os.Chown(dir, s.owner.UID, s.owner.GID); err != nil {
			return nil, "", err
		}
	}
	owner := func(dir string, timeout time.Duration) cmdOpts {
		return cmdOpts{owner: true, dir: dir, timeout: timeout}
	}
	archive := filepath.Join(dir, "source.tar")
	if err := s.run(owner(s.cfg.Repo, gitTimeout), "git", "-C", s.cfg.Repo, "archive", "--format=tar", "-o", archive, target, "--", "installer", "backend"); err != nil {
		return nil, "", uerr("cannot extract the installer of "+target+" from the repository", "저장소에서 "+target+"의 설치기를 꺼낼 수 없습니다")
	}
	if err := s.run(owner(dir, gitTimeout), "tar", "-x", "-f", archive, "-C", dir); err != nil {
		return nil, "", uerr("cannot extract the installer of "+target, target+"의 설치기를 풀 수 없습니다")
	}
	src := filepath.Join(dir, "installer")
	var goStderr limitedWriter
	goStderr.max = maxOutput
	o := owner(src, planTimeout)
	o.stdout, o.stderr = io.Discard, &goStderr
	if err := s.run(o, "go", "env", "GOVERSION"); err != nil {
		line, _, _ := strings.Cut(strings.TrimSpace(goStderr.String()), "\n")
		if line == "" {
			line = err.Error()
		}
		return nil, clip(line, 300), nil
	}
	bin := filepath.Join(dir, "bin", "maru-install")
	if err := s.run(owner(src, planTimeout), "go", "build", "-trimpath", "-o", bin, "."); err != nil {
		return nil, "", uerr("cannot build the installer of "+target+" (see the log)", target+"의 설치기를 빌드할 수 없습니다(로그 참고)")
	}
	var out limitedWriter
	out.max = maxOutput
	o = owner(dir, planTimeout)
	o.stdout = &out
	if err := s.run(o, bin, append([]string{"--upgrade-plan"}, opts...)...); err != nil {
		return nil, "", uerr("the installer of "+target+" could not make an upgrade plan (see the log)",
			target+"의 설치기가 업그레이드 계획을 만들지 못했습니다(로그 참고)")
	}
	var plan config.UpgradePlan
	if err := json.Unmarshal([]byte(out.String()), &plan); err != nil {
		return nil, "", uerr("the upgrade plan of "+target+" is not valid JSON", target+"의 업그레이드 계획이 올바른 JSON이 아닙니다")
	}
	return &plan, "", nil
}

// runApply는 적용 요청을 처리한다.
func (s *upgradeService) runApply(data []byte, rerr, cfgErr error) error {
	prev := s.status
	var req config.UpgradeApplyRequest
	valid := rerr == nil && json.Unmarshal(data, &req) == nil && validApplyRequest(req)
	s.status.State, s.status.Error = config.UpgradeStateApplying, nil
	// 요청이 올바르지 않을 수 있으므로 상태 파일에 옮기는 값은 길이를 자른다.
	s.status.Apply = &config.UpgradeApply{Target: clip(req.Target, 64), User: clip(req.User, 64), RequestedAt: clip(req.RequestedAt, 64), StartedAt: s.stamp()}
	if err := s.stage(config.UpgradeStageInspect); err != nil {
		return err
	}
	switch {
	case cfgErr != nil:
		return s.finishFailed(config.UpgradeStateApplyFailed, cfgErr)
	case !valid:
		return s.finishFailed(config.UpgradeStateApplyFailed, uerr("the upgrade request is not valid", "업그레이드 요청이 올바르지 않습니다"))
	}
	version, err := s.apply(prev, req)
	if err != nil {
		return s.finishFailed(config.UpgradeStateApplyFailed, err)
	}
	s.status.State, s.status.Stage = config.UpgradeStateApplied, ""
	s.status.Apply.FinishedAt, s.status.Apply.Version = s.stamp(), version
	fmt.Fprintf(s.out, "upgrade finished: %s (%s)\n", version, req.Target)
	return s.writeStatus()
}

// validApplyRequest는 적용 요청의 꼴을 본다. 도우미가 이미 검사했지만 다시 본다.
func validApplyRequest(req config.UpgradeApplyRequest) bool {
	if req.Format != config.UpgradeRequestFormat || !config.ValidUpgradeTarget(req.Target) || len(req.Settings) > config.MaxUpgradeSettings {
		return false
	}
	for k, v := range req.Settings {
		if !config.ValidUpgradeSetting(k, v) {
			return false
		}
	}
	return true
}

// apply는 적용 하나다. 마지막 확인을 믿지 않고 저장소와 설치 상태를 다시 확인하며, 빌드하기 전에 입력한 설정 값을 새
// 버전의 계획으로 검사한다. 성공하면 설치한 버전이다.
func (s *upgradeService) apply(prev config.UpgradeStatus, req config.UpgradeApplyRequest) (string, error) {
	chk := prev.Check
	if prev.State != config.UpgradeStateChecked || chk == nil || !chk.Web || chk.Plan == nil || chk.Target != req.Target {
		return "", uerr("there is no finished check that allows upgrading to this commit. Check again",
			"이 커밋으로 업그레이드할 수 있다고 끝난 확인이 없습니다. 다시 확인하세요")
	}
	s.status.Check = chk
	if t, ok := s.prepareOwner(); !ok {
		return "", &upgradeError{text: t}
	}
	if missing := s.missingTools(); len(missing) > 0 {
		return "", uerr("not found in the build PATH: "+strings.Join(missing, ", "), "빌드 PATH에서 찾을 수 없습니다: "+strings.Join(missing, ", "))
	}
	s.prepareSSH()
	rec := s.readRecord()
	if rec == nil || chk.Installed == nil || rec.Revision != chk.Installed.Revision {
		return "", uerr("the installed version changed since the check. Check again", "확인한 뒤 설치된 버전이 바뀌었습니다. 다시 확인하세요")
	}
	args, err := settingArgs(chk.Plan.Questions, req.Settings)
	if err != nil {
		return "", err
	}
	moved := uerr("the repository changed since the check. Check again", "확인한 뒤 저장소가 바뀌었습니다. 다시 확인하세요")
	if dirty, err := s.dirty(); err != nil {
		return "", err
	} else if dirty {
		return "", uerr("the work tree has uncommitted changes. Commit or discard them over SSH, then check again",
			"작업 트리에 커밋하지 않은 변경이 있습니다. SSH로 커밋하거나 되돌린 뒤 다시 확인하세요")
	}
	head, err := s.git("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", err
	}
	switch chk.Mode {
	case config.UpgradeModeLocal:
		if head != req.Target {
			return "", moved
		}
	case config.UpgradeModeRemote:
		if ok, err := s.gitTest("merge-base", "--is-ancestor", head, req.Target); err != nil || !ok {
			return "", moved
		}
	default:
		return "", moved
	}
	if ok, err := s.gitTest("merge-base", "--is-ancestor", rec.Revision, req.Target); err != nil || !ok {
		return "", moved
	}

	// 설정 값은 빌드하기 전에 새 버전의 검사로 확인한다.
	if err := s.stage(config.UpgradeStagePlan); err != nil {
		return "", err
	}
	plan, goErr, err := s.plan(req.Target, args)
	switch {
	case err != nil:
		return "", err
	case goErr != "":
		return "", uerr("Go cannot build this version: "+goErr, "Go로 이 버전을 빌드할 수 없습니다: "+goErr)
	case plan.Format != config.UpgradePlanFormat || !plan.Web:
		return "", uerr("the new version cannot be upgraded from the dashboard any more. Check again", "새 버전을 더 이상 대시보드에서 업그레이드할 수 없습니다. 다시 확인하세요")
	case len(plan.Questions) > 0:
		var en, ko []string
		for _, q := range plan.Questions {
			e, k := "--"+q.Name+": needs a value", "--"+q.Name+": 값이 필요합니다"
			if q.Error != nil {
				e, k = "--"+q.Name+": "+q.Error.En, "--"+q.Name+": "+q.Error.Ko
			}
			en, ko = append(en, e), append(ko, k)
		}
		return "", uerr("the new version did not accept the settings: "+strings.Join(en, "; "), "새 버전이 설정 값을 받지 않았습니다: "+strings.Join(ko, "; "))
	}

	if err := s.stage(config.UpgradeStageUpdate); err != nil {
		return "", err
	}
	if head != req.Target {
		if err := s.run(cmdOpts{owner: true, dir: s.cfg.Repo, timeout: fetchTimeout}, "git", "-C", s.cfg.Repo, "merge", "--ff-only", "--quiet", req.Target); err != nil {
			return "", uerr("cannot fast-forward the repository to "+req.Target+" (see the log)", "저장소를 "+req.Target+"(으)로 fast-forward할 수 없습니다(로그 참고)")
		}
	}
	// fast-forward는 서브모듈(아이콘)을 옮기지 않는다. 옮긴 커밋이 가리키는 서브모듈로 맞춘다.
	if err := s.run(cmdOpts{owner: true, dir: s.cfg.Repo, timeout: fetchTimeout}, "git", "-C", s.cfg.Repo, "submodule", "update", "--init", "--depth", "1"); err != nil {
		return "", uerr("cannot update the git submodules (see the log)", "git 서브모듈을 갱신할 수 없습니다(로그 참고)")
	}
	if head, err = s.git("rev-parse", "--verify", "HEAD^{commit}"); err != nil || head != req.Target {
		return "", moved
	}

	if err := s.stage(config.UpgradeStageBuild); err != nil {
		return "", err
	}
	bc := &Context{
		Lang: "en", Sys: &ownerBuildSystem{System: NewRealSystem(s.log, s.log), s: s}, P: prompt.Fail{},
		Out: s.log, Err: s.log, RepoDir: s.cfg.Repo,
	}
	if err := stepBuild(bc); err != nil {
		return "", uerr("the build failed (see the log)", "빌드가 실패했습니다(로그 참고)")
	}
	for _, rel := range []string{binSrcRel, helperBinSrcRel, installerBinSrcRel} {
		rev, modified, err := readBuildRevision(filepath.Join(s.cfg.Repo, rel))
		if err != nil || rev != req.Target {
			return "", uerr(rel+" was not built from "+req.Target, rel+" 이(가) "+req.Target+"에서 빌드되지 않았습니다")
		}
		if modified {
			fmt.Fprintf(s.log, "note: %s was built with untracked files in the work tree (vcs.modified)\n", rel)
		}
	}

	if err := s.stage(config.UpgradeStageInstall); err != nil {
		return "", err
	}
	install := append([]string{"--service-install", "--upgrade", "--plain", "--lang", "en"}, args...)
	if err := s.run(cmdOpts{dir: s.cfg.Repo, timeout: installTimeout}, filepath.Join(s.cfg.Repo, installerBinSrcRel), install...); err != nil {
		return "", uerr("the new installer failed. maru may not be running; run ./install.sh from the repository over SSH to recover (see the log and docs/upgrade.md)",
			"새 설치기가 실패했습니다. maru가 돌고 있지 않을 수 있습니다. SSH로 저장소에서 ./install.sh를 실행해 복구하세요(로그와 docs/upgrade.md 참고)")
	}
	version := ""
	if rec := s.readRecord(); rec != nil && rec.Revision == req.Target {
		version = rec.Version
	}
	return version, nil
}

// settingArgs는 계획이 물은 설정의 답을 새 설치기 옵션으로 바꾼다. 계획에 없는 이름이나 빠진 답은 받지 않는다.
// confirm은 --이름 / --no-이름, input은 --이름=값 인자 하나다(셸을 거치지 않는다). Dash 설정의 -는 빈 값이다.
func settingArgs(questions []config.UpgradeQuestion, given map[string]string) ([]string, error) {
	for name := range given {
		if !slices.ContainsFunc(questions, func(q config.UpgradeQuestion) bool { return q.Name == name }) {
			return nil, uerr("the upgrade does not ask for --"+name, "업그레이드가 --"+name+"을(를) 묻지 않습니다")
		}
	}
	var args []string
	for _, q := range questions {
		v, ok := given[q.Name]
		if q.Dash && v == "-" {
			v = ""
		}
		switch {
		case !ok || !config.ValidUpgradeSetting(q.Name, v):
			return nil, uerr("--"+q.Name+" needs an answer", "--"+q.Name+"에 답이 필요합니다")
		case q.Kind == config.UpgradeQuestionConfirm && v == "yes":
			args = append(args, "--"+q.Name)
		case q.Kind == config.UpgradeQuestionConfirm && v == "no":
			args = append(args, "--no-"+q.Name)
		case q.Kind == config.UpgradeQuestionInput && (v != "" || q.EmptyOK):
			args = append(args, "--"+q.Name+"="+v)
		default:
			return nil, uerr("--"+q.Name+" needs an answer", "--"+q.Name+"에 답이 필요합니다")
		}
	}
	return args, nil
}

// ownerBuildSystem은 stepBuild를 저장소 소유자로 실행하는 System이다. 읽기는 실제 시스템에서 하고, 빌드 명령은 소유자로
// 강등해 빌드 PATH에서 찾는다.
type ownerBuildSystem struct {
	System
	s *upgradeService
}

func (o *ownerBuildSystem) RunUser(dir, name string, args ...string) error {
	return o.s.run(cmdOpts{owner: true, dir: dir, timeout: buildTimeout}, name, args...)
}

func (o *ownerBuildSystem) LookPath(name string) bool { return o.s.lookPath(name) != "" }
func (o *ownerBuildSystem) CurrentUser() string       { return o.s.owner.Name }
