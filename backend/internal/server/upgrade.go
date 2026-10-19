package server

import (
	"errors"
	"log"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"onmaru/backend/config"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/upgrade"
)

// 대시보드에서 업그레이드(.claude/docs/upgrade.md). onmaru는 업그레이드 서비스의 상태 파일을 읽고 확인 요청을 상태 디렉터리에 쓰며,
// 업그레이드(적용)는 권한 도우미의 root 연산 upgrade.apply로 요청한다. 진행할 수 있는지는 업그레이드 서비스가 저장소와
// 설치 상태로 다시 확인하므로, 여기서 하는 검사는 빠른 거절과 안내를 위한 것이다.

// maxUpgradeBody는 적용 요청 본문의 상한이다(설정 32개 × 값 512바이트).
const maxUpgradeBody = 64 << 10

// upgradeInfo는 상태를 응답으로 바꾼다. 쓸 수 없으면 이유를 담는다.
func (h *handler) upgradeInfo(s upgrade.Snapshot) api.UpgradeInfo {
	info := api.UpgradeInfo{
		Enabled: true, Repo: s.Repo, User: s.User,
		Pending: s.Pending, PendingStalled: s.PendingStalled, Interrupted: s.Interrupted,
		Version: h.about.Version, Revision: h.about.Revision, Installed: s.Installed, Status: s.Status,
	}
	switch {
	case !s.Enabled:
		info.Enabled, info.DisabledReason = false, api.UpgradeNotEnabled
	case h.Helper == nil:
		info.Enabled, info.DisabledReason = false, api.UpgradeNoHelper
	case !h.Upgrade.CanRequest():
		info.Enabled, info.DisabledReason = false, api.UpgradeStateDir
	}
	return info
}

func (h *handler) upgradeState(w http.ResponseWriter, _ *http.Request, _ auth.Session) {
	writeJSON(w, http.StatusOK, h.upgradeInfo(h.Upgrade.Read()))
}

// upgradeCheck는 확인 요청을 남긴다. 확인은 저장소 코드를 root로 실행하지 않으므로 상승을 요구하지 않는다(.claude/docs/upgrade.md).
func (h *handler) upgradeCheck(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.StartUpgradeCheckRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Mode != config.UpgradeModeRemote && req.Mode != config.UpgradeModeLocal {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "mode must be remote or local")
		return
	}
	if info := h.upgradeInfo(h.Upgrade.Read()); !info.Enabled {
		writeError(w, http.StatusConflict, api.ErrUpgradeDisabled, "upgrading from the dashboard is not available: "+info.DisabledReason)
		return
	}
	var rl *upgrade.RateLimitedError
	switch err := h.Upgrade.RequestCheck(req.Mode); {
	case errors.Is(err, upgrade.ErrDisabled):
		writeError(w, http.StatusConflict, api.ErrUpgradeDisabled, "upgrading from the dashboard is not available")
		return
	case errors.Is(err, upgrade.ErrBusy):
		writeError(w, http.StatusConflict, api.ErrUpgradeBusy, "an upgrade check or upgrade is in progress")
		return
	case errors.As(err, &rl):
		secs := int(math.Ceil(rl.RetryAfter.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, api.ErrorResponse{
			Error: "upgrade check requested too often", Code: api.ErrRateLimited, RetryAfterSeconds: secs,
		})
		return
	case err != nil:
		log.Printf("upgrade check request: %v", err)
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "cannot write the check request"})
		return
	}
	log.Printf("upgrade check requested: user=%q mode=%s ip=%s", s.Username, req.Mode, clientIP(r))
	writeJSON(w, http.StatusOK, h.upgradeInfo(h.Upgrade.Read()))
}

// upgradeApply는 마지막 확인이 정한 대상으로 업그레이드를 요청한다. root 권한 상승이 필요하며(도우미가 확인한다),
// 상승하지 않았으면 elevation_required로 끝나 화면이 상승한 뒤 다시 시도한다.
func (h *handler) upgradeApply(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.StartUpgradeRequest
	if !decodeJSONLimit(w, r, &req, maxUpgradeBody) {
		return
	}
	// 도우미도 꼴을 검사하지만, 그 오류(bad_request)를 기다리지 않고 여기서 400으로 끝낸다.
	if !config.ValidUpgradeTarget(req.Target) || len(req.Settings) > config.MaxUpgradeSettings {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid target or too many settings")
		return
	}
	for k, v := range req.Settings {
		if !config.ValidUpgradeSetting(k, v) {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid setting")
			return
		}
	}
	snap := h.Upgrade.Read()
	if info := h.upgradeInfo(snap); !info.Enabled {
		writeError(w, http.StatusConflict, api.ErrUpgradeDisabled, "upgrading from the dashboard is not available: "+info.DisabledReason)
		return
	}
	if snap.Busy() {
		writeError(w, http.StatusConflict, api.ErrUpgradeBusy, "an upgrade check or upgrade is in progress")
		return
	}
	st := snap.Status
	if st == nil || st.State != config.UpgradeStateChecked || st.Check == nil || !st.Check.Web || st.Check.Target != req.Target {
		writeError(w, http.StatusConflict, api.ErrUpgradeOutdated, "no finished check allows upgrading to this commit, check again")
		return
	}
	ticket, ok := h.helperTicket(w, s)
	if !ok {
		return
	}
	names := strings.Join(slices.Sorted(maps.Keys(req.Settings)), ",")
	if err := h.Helper.UpgradeApply(r.Context(), ticket, req.Target, req.Settings); err != nil {
		log.Printf("upgrade request failed: user=%q target=%s settings=%q ip=%s: %s", s.Username, req.Target, names, clientIP(r), helperReason(err))
		h.writeHelperError(w, s, err)
		return
	}
	// 값은 남기지 않는다(도우미 감사 로그와 같다).
	log.Printf("upgrade requested: user=%q target=%s settings=%q ip=%s", s.Username, req.Target, names, clientIP(r))
	writeJSON(w, http.StatusAccepted, h.upgradeInfo(h.Upgrade.Read()))
}
