package server

import (
	"net/http"
	"path"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/helper/proto"
)

// filesSearch는 폴더와 그 아래 모든 폴더에서 이름에 찾기어가 든 항목을 찾는다(대소문자 무시). 찾기는 도우미가 사용자
// 권한으로 하고, 찾은 항목은 요청이 열려 있는 동안 text/event-stream으로 보낸다(서버에 백그라운드 작업은 없다).
// 브라우저가 요청을 끊으면 도우미 연결을 닫고 도우미도 멈춘다. 결과는 proto.MaxSearch개까지다.
//
// 시작하기 전의 실패(보안 정책, 범위, 폴더가 아님 등)는 보통의 오류 응답이고, 첫 진행을 받은 뒤의 실패는 마지막
// 사건(Error)이다.
func (h *handler) filesSearch(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	p := r.URL.Query().Get("path")
	query := r.URL.Query().Get("q")
	if query == "" || len(query) > maxQuery {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "q must have 1 to 255 bytes")
		return
	}
	roots, ok := h.allowedPaths(w, r, s, ticket, p)
	if !ok {
		return
	}
	ev := newEventStream(w)
	res, err := h.Helper.Search(r.Context(), ticket, proto.SearchParams{Path: p, Query: query}, func(pr proto.SearchProgress) {
		e := api.FilesSearchEvent{Dirs: pr.Dirs, Skipped: pr.Skipped}
		for _, hit := range pr.Hits {
			full := path.Join(p, hit.Path)
			// 보이는 폴더 안이어도 그 아래에 보이지 않는 공유 폴더가 있을 수 있다(디스크 최상위 보기).
			if !withinRoots(roots, full) {
				continue
			}
			typ := hit.Type
			if typ == "" {
				typ = "unknown"
			}
			e.Hits = append(e.Hits, api.FilesSearchHit{Path: full, Type: typ})
		}
		ev.send(e)
	})
	if err != nil {
		if r.Context().Err() != nil {
			// 브라우저가 검색을 멈췄다.
			return
		}
		if !ev.started {
			h.writeFilesError(w, s, err)
			return
		}
		e, status := h.filesErrorEvent(s, err)
		ev.send(api.FilesSearchEvent{Error: &e, Status: status})
		return
	}
	ev.send(api.FilesSearchEvent{Dirs: res.Dirs, Skipped: res.Skipped,
		Result: &api.FilesSearchResult{Dirs: res.Dirs, Skipped: res.Skipped, Truncated: res.Truncated}})
}
