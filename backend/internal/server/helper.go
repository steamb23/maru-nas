package server

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/helper/client"
	"onmaru/backend/internal/helper/proto"
)

// Helper는 권한 도우미(onmaru-helper) 클라이언트다. *client.Client가 구현한다.
// 티켓은 도우미가 비밀번호를 직접 확인하고 발급하며, 이 백엔드는 세션에 보관해 넘기기만 한다.
type Helper interface {
	Open(ctx context.Context, user, password string, pending bool) (proto.OpenResult, error)
	Activate(ctx context.Context, ticket string) error
	Close(ctx context.Context, ticket string) error
	Status(ctx context.Context, ticket string) (proto.StatusResult, error)
	Elevate(ctx context.Context, ticket, password string) (proto.ElevateResult, error)
	Drop(ctx context.Context, ticket string) error
	Identity(ctx context.Context, ticket, scope string) (proto.Identity, error)
	// SSHAuthorize, SSHRevoke는 웹 터미널이 쓰는 짧은 수명의 SSH 키를 사용자의 authorized_keys에 넣고 지운다.
	SSHAuthorize(ctx context.Context, ticket, publicKey string) error
	SSHRevoke(ctx context.Context, ticket, publicKey string) error
	// UpgradeApply는 대시보드에서 업그레이드의 적용 요청을 업그레이드 서비스에 넘긴다(root 연산, 상승한 티켓).
	UpgradeApply(ctx context.Context, ticket, target string, settings map[string]string) error
	// 파일 탐색기가 쓰는 사용자 권한 파일 연산이다(files.go).
	Stat(ctx context.Context, ticket, path string) (proto.Entry, error)
	Names(ctx context.Context, ticket, path string) (proto.NamesResult, []proto.NameEntry, error)
	StatMany(ctx context.Context, ticket, dir string, names []string) (proto.StatManyResult, error)
	Info(ctx context.Context, ticket, path string) (proto.InfoResult, error)
	Read(ctx context.Context, ticket, path string, offset, length int64) (proto.ReadResult, *client.Body, error)
	Write(ctx context.Context, ticket, path string, overwrite bool, r io.Reader, size int64) (proto.Entry, error)
	Mkdir(ctx context.Context, ticket, path string) (proto.Entry, error)
	Rename(ctx context.Context, ticket, from, to string) (proto.Entry, error)
	Remove(ctx context.Context, ticket, path string, recursive bool) error
	// 휴지통(files_trash.go). TrashPrepare만 root 연산이다(상승한 티켓).
	Trash(ctx context.Context, ticket, path string) (proto.TrashResult, error)
	TrashDirs(ctx context.Context, ticket string) (proto.TrashDirsResult, error)
	TrashList(ctx context.Context, ticket string) (proto.TrashListResult, error)
	TrashRestore(ctx context.Context, ticket, trash, name, to string) (proto.Entry, error)
	TrashPurge(ctx context.Context, ticket, trash, name string) error
	TrashEmpty(ctx context.Context, ticket string) error
	TrashPrepare(ctx context.Context, ticket, disk string) error
}

const (
	// helperOpenTimeout은 로그인할 때 도우미 티켓을 기다리는 시간이다. 넘으면 티켓 없이 로그인한다.
	helperOpenTimeout = 5 * time.Second
	// helperCloseTimeout은 세션이 지워질 때 티켓을 폐기하는 요청의 시간 제한이다.
	helperCloseTimeout = 5 * time.Second
)

// openHelper는 로그인에 성공한 사용자의 비밀번호로 도우미 티켓을 받는다. 도우미가 없거나
// 실패하면 빈 값이며 로그인은 그대로 진행한다(도우미 기능만 쓸 수 없다).
func (h *handler) openHelper(ctx context.Context, user, password string, pending bool) string {
	if h.Helper == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, helperOpenTimeout)
	defer cancel()
	res, err := h.Helper.Open(ctx, user, password, pending)
	if err != nil {
		log.Printf("helper: no ticket for user=%q: %s", user, helperReason(err))
		return ""
	}
	return res.Ticket
}

// activateHelper는 2차 인증을 마친 대기 티켓을 쓸 수 있게 한다. 실패하면 티켓을 버린다.
func (h *handler) activateHelper(ctx context.Context, p auth.Pending) string {
	if h.Helper == nil || p.HelperTicket == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, helperOpenTimeout)
	defer cancel()
	if err := h.Helper.Activate(ctx, p.HelperTicket); err != nil {
		log.Printf("helper: activate failed for user=%q: %s", p.Username, helperReason(err))
		h.closeHelper(p.HelperTicket)
		return ""
	}
	return p.HelperTicket
}

// closeHelper는 티켓을 비동기로 폐기한다. 세션 저장소의 삭제 훅에서 부른다.
func (h *handler) closeHelper(ticket string) {
	if h.Helper == nil || ticket == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), helperCloseTimeout)
		defer cancel()
		if err := h.Helper.Close(ctx, ticket); err != nil {
			log.Printf("helper: close ticket: %s", helperReason(err))
		}
	}()
}

// helperReason은 도우미 오류의 로그용 이유다(코드 또는 unavailable). 티켓·비밀번호는 들어가지 않는다.
func helperReason(err error) string {
	if code := proto.CodeOf(err); code != "" {
		return code
	}
	return "unavailable"
}

// markHelperLost는 도우미에서 사라진 티켓을 세션에서 지우고 다시 로그인이 필요하다고 표시한다.
func (h *handler) markHelperLost(s auth.Session) {
	h.Sessions.Update(s.ID, func(ss *auth.Session) {
		if ss.HelperTicket == s.HelperTicket {
			ss.HelperTicket, ss.HelperLost = "", true
		}
	})
}

// helperState는 이 세션의 도우미 상태다. worker를 실행하지 않는다.
func (h *handler) helperState(ctx context.Context, s auth.Session) api.HelperStatus {
	switch {
	case h.Helper == nil:
		return api.HelperStatus{State: api.HelperDisabled}
	case s.HelperTicket == "" && s.HelperLost:
		return api.HelperStatus{State: api.HelperReauth}
	case s.HelperTicket == "":
		return api.HelperStatus{State: api.HelperNone}
	}
	st, err := h.Helper.Status(ctx, s.HelperTicket)
	if proto.CodeOf(err) == proto.CodeTicketInvalid {
		h.markHelperLost(s)
		return api.HelperStatus{State: api.HelperReauth}
	}
	if err != nil {
		log.Printf("helper: status for user=%q: %s", s.Username, helperReason(err))
		return api.HelperStatus{State: api.HelperUnavailable}
	}
	return api.HelperStatus{State: api.HelperActive, CanElevate: st.CanElevate, ElevatedSeconds: st.ElevatedSeconds}
}

func (h *handler) helperStatus(w http.ResponseWriter, r *http.Request, s auth.Session) {
	writeJSON(w, http.StatusOK, h.helperState(r.Context(), s))
}

// helperTicket은 이 세션의 티켓이다. 없으면 오류 응답을 쓰고 false다.
func (h *handler) helperTicket(w http.ResponseWriter, s auth.Session) (string, bool) {
	switch {
	case h.Helper == nil:
		writeError(w, http.StatusServiceUnavailable, api.ErrHelperUnavailable, "privileged helper is not configured")
		return "", false
	case s.HelperTicket == "":
		writeError(w, http.StatusConflict, api.ErrHelperReauth, "no helper session, log in again")
		return "", false
	}
	return s.HelperTicket, true
}

// writeHelperError는 도우미 오류를 HTTP 응답으로 바꾼다.
func (h *handler) writeHelperError(w http.ResponseWriter, s auth.Session, err error) {
	var pe *proto.Error
	if !errors.As(err, &pe) {
		log.Printf("helper: user=%q: %v", s.Username, err)
		writeError(w, http.StatusServiceUnavailable, api.ErrHelperUnavailable, "privileged helper is unavailable")
		return
	}
	switch pe.Code {
	case proto.CodeTicketInvalid:
		h.markHelperLost(s)
		writeError(w, http.StatusConflict, api.ErrHelperReauth, "helper session expired, log in again")
	case proto.CodeAuthFailed:
		writeError(w, http.StatusBadRequest, api.ErrElevationFailed, "incorrect password")
	case proto.CodeRateLimited:
		writeRateLimited(w, time.Duration(pe.RetryAfter)*time.Second)
	case proto.CodeElevationNotAllowed, proto.CodeAccountNotAllowed:
		writeError(w, http.StatusForbidden, api.ErrElevationNotAllowed, "this account cannot elevate")
	case proto.CodeElevationRequired:
		writeError(w, http.StatusForbidden, api.ErrElevationRequired, "elevation required")
	case proto.CodeUpgradeDisabled:
		writeError(w, http.StatusConflict, api.ErrUpgradeDisabled, "upgrading from the dashboard is not enabled in the privileged helper")
	case proto.CodeBadRequest:
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "the privileged helper rejected the request")
	default:
		log.Printf("helper: user=%q: %s", s.Username, pe.Code)
		writeError(w, http.StatusServiceUnavailable, api.ErrHelperUnavailable, "privileged helper failed")
	}
}

// helperIdentity는 사용자 권한(상승 중이면 root 권한도)으로 실행한 worker의 실제 권한이다.
// 도우미가 권한을 제대로 바꾸는지 설정 화면에서 확인하는 데 쓴다.
func (h *handler) helperIdentity(w http.ResponseWriter, r *http.Request, s auth.Session) {
	ticket, ok := h.helperTicket(w, s)
	if !ok {
		return
	}
	id, err := h.Helper.Identity(r.Context(), ticket, proto.ScopeUser)
	if err != nil {
		h.writeHelperError(w, s, err)
		return
	}
	resp := api.HelperIdentityResponse{User: identityOf(id)}
	if st, err := h.Helper.Status(r.Context(), ticket); err == nil && st.ElevatedSeconds > 0 {
		if root, err := h.Helper.Identity(r.Context(), ticket, proto.ScopeRoot); err == nil {
			ri := identityOf(root)
			resp.Root = &ri
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func identityOf(id proto.Identity) api.HelperIdentity {
	out := api.HelperIdentity{UID: id.UID, GID: id.GID, User: id.User, Groups: []api.HelperGroup{}}
	for _, g := range id.Groups {
		out.Groups = append(out.Groups, api.HelperGroup{ID: g.ID, Name: g.Name})
	}
	return out
}

// elevate는 관리자가 비밀번호를 다시 입력해 이 세션에 root 권한 상승을 짧게 붙인다.
// 비밀번호는 도우미가 직접 확인하고, 그룹 조건(OMV 관리자 + 상승 그룹)도 도우미가 판단한다.
func (h *handler) elevate(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.ElevationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// 보안 강화 정책의 확인을 먼저 거친다(비밀번호를 묻기 전에).
	if lvl := h.securityPolicy().Elevation; !satisfies(s, lvl) {
		writeSecurityRequired(w, lvl)
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "password is required")
		return
	}
	ticket, ok := h.helperTicket(w, s)
	if !ok {
		return
	}
	res, err := h.Helper.Elevate(r.Context(), ticket, req.Password)
	if err != nil {
		log.Printf("helper elevate failed: user=%q ip=%s: %s", s.Username, clientIP(r), helperReason(err))
		h.writeHelperError(w, s, err)
		return
	}
	log.Printf("helper elevate ok: user=%q ip=%s", s.Username, clientIP(r))
	writeJSON(w, http.StatusOK, api.HelperStatus{State: api.HelperActive, CanElevate: true, ElevatedSeconds: res.ElevatedSeconds})
}

// dropElevation은 상승을 바로 끝낸다.
func (h *handler) dropElevation(w http.ResponseWriter, r *http.Request, s auth.Session) {
	ticket, ok := h.helperTicket(w, s)
	if !ok {
		return
	}
	if err := h.Helper.Drop(r.Context(), ticket); err != nil {
		h.writeHelperError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, h.helperState(r.Context(), s))
}
