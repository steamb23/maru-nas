package addons

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"onmaru/backend/internal/api"
)

// applyCaller는 ApplyChange가 부르는 Config·Exec RPC를 흉내 내고 순서를 기록한다.
type applyCaller struct {
	dirty    bool
	applyErr error
	calls    []string
}

func (c *applyCaller) Call(_ context.Context, service, method string, _ any) (json.RawMessage, error) {
	name := service + "." + method
	c.calls = append(c.calls, name)
	switch name {
	case "Config.isDirty":
		return json.Marshal(c.dirty)
	case "Config.applyChangesBg":
		if c.applyErr != nil {
			return nil, c.applyErr
		}
		return json.Marshal("bgstatus123")
	case "Exec.isRunning":
		return json.Marshal(map[string]any{"filename": "bgstatus123", "running": false})
	}
	return nil, errors.New("unexpected rpc " + name)
}

func TestApplyChange(t *testing.T) {
	ctx := context.Background()
	changed := func(context.Context) (bool, error) { return true, nil }

	c := &applyCaller{}
	if err := ApplyChange(ctx, c, changed); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.calls, " "); got != "Config.isDirty Config.applyChangesBg Exec.isRunning" {
		t.Errorf("calls %s", got)
	}

	// 적용하지 않은 변경이 있으면 바꾸지 않고 409 config_dirty다.
	c = &applyCaller{dirty: true}
	ran := false
	err := ApplyChange(ctx, c, func(context.Context) (bool, error) { ran = true; return true, nil })
	var ae *ActionError
	if !errors.As(err, &ae) || ae.Status != http.StatusConflict || ae.Code != api.ErrConfigDirty || ran {
		t.Errorf("dirty: err=%v ran=%v", err, ran)
	}

	// 바꿀 것이 없으면 적용하지 않는다.
	c = &applyCaller{}
	if err := ApplyChange(ctx, c, func(context.Context) (bool, error) { return false, nil }); err != nil || len(c.calls) != 1 {
		t.Errorf("unchanged: err=%v calls=%v", err, c.calls)
	}

	// 변경이 실패하면 그 오류이고 적용하지 않는다.
	c = &applyCaller{}
	boom := errors.New("boom")
	if err := ApplyChange(ctx, c, func(context.Context) (bool, error) { return false, boom }); !errors.Is(err, boom) || len(c.calls) != 1 {
		t.Errorf("change failed: err=%v calls=%v", err, c.calls)
	}

	// 적용 시작이 실패하면 ErrActionFailed다.
	c = &applyCaller{applyErr: errors.New("salt failed")}
	if err := ApplyChange(ctx, c, changed); !errors.Is(err, ErrActionFailed) {
		t.Errorf("apply failed: err=%v", err)
	}
}

func TestApplyChangeWaitsForLock(t *testing.T) {
	// 다른 적용이 진행 중이면 기다리고, 그동안 제한 시간이 지나면 ErrActionTimeout이다.
	applyLock <- struct{}{}
	defer func() { <-applyLock }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ApplyChange(ctx, &applyCaller{}, func(context.Context) (bool, error) { return true, nil }); !errors.Is(err, ErrActionTimeout) {
		t.Errorf("err=%v", err)
	}
}
