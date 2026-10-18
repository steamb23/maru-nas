package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"onmaru/backend/config"
	"onmaru/backend/internal/helper/proto"
)

// upgrade.apply는 대상 커밋과 설정 이름·값의 꼴만 검사해 요청 디렉터리에 root만 읽는 적용 요청을 쓴다. 요청한 사용자는
// 작업(티켓)에서 정한다. 대시보드에서 업그레이드가 꺼져 있으면(디렉터리 없음) upgrade_disabled다.
func TestUpgradeApply(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(t.TempDir(), "onmaru-upgrade")
	e.cfg.UpgradeDir = dir
	e.cfg.Now = func() time.Time { return time.Date(2026, 10, 19, 1, 2, 3, 0, time.FixedZone("KST", 9*3600)) }
	e.job.User = "bob"
	target := strings.Repeat("0123456789", 4)
	good := proto.UpgradeApplyParams{Target: target, Settings: map[string]string{"public-origin": "https://desk.example.lan", "workbench-url": ""}}
	run := func(params any) (proto.Response, error) {
		resp, _, err := e.run(proto.ScopeRoot, proto.ActionUpgradeApply, params, nil)
		return resp, err
	}
	if resp, err := run(good); resp.Code != proto.CodeUpgradeDisabled || proto.CodeOf(err) != proto.CodeUpgradeDisabled {
		t.Fatalf("disabled: %+v %v", resp, err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	many := map[string]string{}
	for i := range config.MaxUpgradeSettings + 1 {
		many["s"+strings.Repeat("x", i)] = "v"
	}
	for _, bad := range []any{
		proto.UpgradeApplyParams{Target: target[:39]},
		proto.UpgradeApplyParams{Target: strings.ToUpper(target[:10]) + target[10:] + "x"},
		proto.UpgradeApplyParams{Target: target, Settings: map[string]string{"Bad": "x"}},
		proto.UpgradeApplyParams{Target: target, Settings: map[string]string{"public-origin": "a b"}},
		proto.UpgradeApplyParams{Target: target, Settings: map[string]string{"no-helper": "yes"}},
		proto.UpgradeApplyParams{Target: target, Settings: many},
		map[string]any{"target": target, "command": "rm -rf /"},
	} {
		if resp, _ := run(bad); resp.OK || resp.Code != proto.CodeBadRequest {
			t.Errorf("%+v: %+v", bad, resp)
		}
	}
	// 처리하지 않은 요청은 새 요청으로 바꾼다.
	if err := os.WriteFile(filepath.Join(dir, config.UpgradeApplyRequestName), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if resp, err := run(good); !resp.OK || err != nil {
		t.Fatalf("apply: %+v %v", resp, err)
	}
	path := filepath.Join(dir, config.UpgradeApplyRequestName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var req config.UpgradeApplyRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	if req.Format != config.UpgradeRequestFormat || req.Target != target || req.User != "bob" || req.RequestedAt != "2026-10-18T16:02:03Z" ||
		len(req.Settings) != 2 || req.Settings["public-origin"] != "https://desk.example.lan" {
		t.Errorf("request %+v", req)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left behind %v", entries)
	}
	// 사용자 범위에는 없다.
	if resp, _, _ := e.run(proto.ScopeUser, proto.ActionUpgradeApply, good, nil); resp.Code != proto.CodeBadRequest {
		t.Errorf("user scope: %+v", resp)
	}
}
