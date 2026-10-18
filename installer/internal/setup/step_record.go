package setup

import (
	"encoding/json"
	"runtime/debug"
	"time"

	"onmaru/backend/config"
)

// stepRecord는 설치 기록(config.InstallRecordFile)을 쓴다. 대시보드에서 업그레이드는 이 기록으로 지금 설치된 버전과
// 커밋을 안다. 수동 설치도 매번 쓴다. 업그레이드 계획(--upgrade-plan)을 저장소 소유자가 읽으므로 0644로 쓴다
// (디렉터리는 WriteFile이 0755로 만든다). 내용이 매번 바뀌므로(설치 시각) 백업하지 않는다.
func stepRecord(c *Context) error {
	data, err := json.MarshalIndent(c.installRecord(), "", "  ")
	if err != nil {
		return err
	}
	if err := c.Sys.WriteFile(config.InstallRecordFile, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if !c.Sys.DryRun() {
		c.Info(L("Wrote "+config.InstallRecordFile, config.InstallRecordFile+" 작성"))
	}
	return nil
}

// installRecord는 설치하는 실행 파일의 기록이다. 커밋은 설치기가 아니라 설치하는 bin/onmaru에서 읽는다(--no-build면
// 설치기만 새로 빌드되고 bin/onmaru는 예전 것일 수 있다). 버전은 이 설치기의 버전이므로, 설치기와 bin/onmaru가 같은
// 커밋에서 빌드됐는지 확인했을 때만 적는다. 확인하지 못한 값은 빈 값으로 두고 경고한다(업그레이드가 틀린 기록을 믿고
// 수동 재설치가 필요한 버전 경계를 건너뛰지 않게).
func (c *Context) installRecord() config.InstallRecord {
	rec := config.InstallRecord{
		Format:      config.InstallRecordFormat,
		InstalledAt: c.Sys.Now().UTC().Format(time.RFC3339),
	}
	bin := c.binSrc()
	bi, err := c.Sys.ReadBuildInfo(bin)
	if err != nil {
		c.Warn(L("install record: cannot read the build information of "+bin+" ("+err.Error()+"); the version and commit are left empty",
			"설치 기록: "+bin+" 의 빌드 정보를 읽을 수 없어("+err.Error()+") 버전과 커밋을 비워 둡니다"))
		return rec
	}
	rec.Revision, rec.RevisionTime, rec.Modified = vcsInfo(bi)
	self, _, _ := vcsInfo(c.SelfBuild)
	switch {
	case rec.Revision == "":
		c.Warn(L("install record: "+bin+" has no git commit information (built outside a git checkout?); the version is left empty",
			"설치 기록: "+bin+" 에 git 커밋 정보가 없어(git 저장소 밖에서 빌드?) 버전을 비워 둡니다"))
	case self == "":
		c.Warn(L("install record: the installer has no git commit information, so the version of "+bin+" cannot be confirmed and is left empty",
			"설치 기록: 설치기에 git 커밋 정보가 없어 "+bin+" 의 버전을 확인할 수 없으므로 비워 둡니다"))
	case self != rec.Revision:
		c.Warn(L("install record: "+bin+" was built from a different commit than the installer (an older build with --no-build?); the version is left empty. Run "+c.entry()+" without --no-build to record it.",
			"설치 기록: "+bin+" 이(가) 설치기와 다른 커밋에서 빌드되어(--no-build로 예전 빌드를 설치?) 버전을 비워 둡니다. 기록하려면 --no-build 없이 "+c.entry()+" 를 실행하세요."))
	default:
		rec.Version = config.Version
	}
	return rec
}

// vcsInfo는 빌드 정보의 git 커밋, 커밋 시각, 커밋하지 않은 변경이 있었는지다. bi가 nil이어도 된다.
func vcsInfo(bi *debug.BuildInfo) (revision, revTime string, modified bool) {
	if bi == nil {
		return "", "", false
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			revTime = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return revision, revTime, modified
}
