package worker

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"maru/backend/config"
	"maru/backend/internal/helper/proto"
)

// upgradeApply는 대시보드에서 업그레이드의 적용 요청을 요청 디렉터리(cfg.UpgradeDir)에 쓴다(root 범위). 그러면
// maru-upgrade.path가 업그레이드 서비스를 실행한다. 대상 커밋과 설정 이름·값의 꼴만 검사하고 명령은 받지 않는다.
// 요청한 사용자는 티켓에서 정한다. 진행할 수 있는지(마지막 확인과 같은 대상인지, 저장소 상태, 설정 값)는 업그레이드
// 서비스가 모두 다시 확인하므로 여기서는 상태 파일을 읽지 않는다. 처리하지 않은 요청이 있으면 새 요청으로 바꾼다.
func upgradeApply(job Job, cfg Config) error {
	var p proto.UpgradeApplyParams
	if err := proto.DecodeParams(job.Params, &p); err != nil {
		return err
	}
	if !config.ValidUpgradeTarget(p.Target) || len(p.Settings) > config.MaxUpgradeSettings {
		return proto.NewError(proto.CodeBadRequest)
	}
	settings := map[string]string{}
	for k, v := range p.Settings {
		if !config.ValidUpgradeSetting(k, v) {
			return proto.NewError(proto.CodeBadRequest)
		}
		settings[k] = v
	}
	// 디렉터리는 대시보드에서 업그레이드를 켠 도우미 유닛만 만든다(RuntimeDirectory=).
	if fi, err := os.Lstat(cfg.UpgradeDir); errors.Is(err, fs.ErrNotExist) || (err == nil && !fi.IsDir()) {
		return proto.NewError(proto.CodeUpgradeDisabled)
	} else if err != nil {
		return err
	}
	data, err := json.Marshal(config.UpgradeApplyRequest{
		Format: config.UpgradeRequestFormat, Target: p.Target, Settings: settings,
		User: job.User, RequestedAt: cfg.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	// 경로 유닛은 점으로 시작하는 이름을 보지 않으므로 다 쓴 뒤 이름을 바꿀 때 한 번만 실행된다.
	tmp, err := os.CreateTemp(cfg.UpgradeDir, ".apply-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// worker의 umask(0002)와 상관없이 root만 읽게 한다.
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(cfg.UpgradeDir, config.UpgradeApplyRequestName))
}
