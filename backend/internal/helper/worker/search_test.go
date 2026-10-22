package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"maru/backend/internal/helper/proto"
)

// searchRun은 fs.search를 실행하고 찾은 항목(경로 순), 마지막 진행, 마지막 응답, Serve의 오류를 돌려준다.
func (e *env) searchRun(p proto.SearchParams) ([]proto.SearchHit, proto.SearchProgress, proto.Response, error) {
	e.t.Helper()
	client, server := socketPair(e.t)
	defer client.Close()
	job := e.job
	job.Action = proto.ActionSearch
	job.Params, _ = json.Marshal(p)
	errc := make(chan error, 1)
	go func() {
		errc <- Serve(job, server, e.cfg)
		server.Close()
	}()
	var hits []proto.SearchHit
	var last proto.SearchProgress
	for {
		resp, err := proto.ReadResponse(client)
		if err != nil {
			e.t.Fatalf("read response: %v", err)
		}
		if !resp.Partial {
			slices.SortFunc(hits, func(a, b proto.SearchHit) int { return compareStrings(a.Path, b.Path) })
			return hits, last, resp, <-errc
		}
		if err := json.Unmarshal(resp.Result, &last); err != nil {
			e.t.Fatal(err)
		}
		hits = append(hits, last.Hits...)
	}
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func TestSearch(t *testing.T) {
	e := newEnv(t)
	e.file(filepath.Join(e.disk, "사진", "2024", "Holiday.JPG"), "x")
	e.file(filepath.Join(e.disk, "사진", "holiday-notes.txt"), "x")
	e.file(filepath.Join(e.disk, "문서", "보고서.txt"), "x")
	if err := os.MkdirAll(filepath.Join(e.disk, "사진", "HOLIDAY 앨범"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 링크는 찾지만 따라가지 않는다(기준 밖 폴더를 가리킨다).
	outside := t.TempDir()
	e.file(filepath.Join(outside, "holiday-secret.txt"), "x")
	if err := os.Symlink(outside, filepath.Join(e.disk, "사진", "holiday-link")); err != nil {
		t.Fatal(err)
	}

	hits, last, resp, err := e.searchRun(proto.SearchParams{Path: e.disk, Query: "HoliDay"})
	if err != nil || !resp.OK {
		t.Fatalf("search: %+v, %v", resp, err)
	}
	want := []proto.SearchHit{
		{Path: "사진/2024/Holiday.JPG", Type: proto.TypeFile},
		{Path: "사진/HOLIDAY 앨범", Type: proto.TypeDir},
		{Path: "사진/holiday-link", Type: proto.TypeSymlink},
		{Path: "사진/holiday-notes.txt", Type: proto.TypeFile},
	}
	if !slices.Equal(hits, want) {
		t.Fatalf("hits = %+v", hits)
	}
	var res proto.SearchResult
	json.Unmarshal(resp.Result, &res)
	// 맨 위, 사진, 문서, 2024, 앨범을 읽었다.
	if res.Truncated || res.Dirs != 5 || last.Dirs != 5 {
		t.Fatalf("result = %+v, last = %+v", res, last)
	}

	// 아래 폴더에서 찾으면 그 안의 상대 경로다.
	hits, _, _, _ = e.searchRun(proto.SearchParams{Path: filepath.Join(e.disk, "사진"), Query: "2024"})
	if !slices.Equal(hits, []proto.SearchHit{{Path: "2024", Type: proto.TypeDir}}) {
		t.Fatalf("sub hits = %+v", hits)
	}
}

func TestSearchLimitAndBatches(t *testing.T) {
	batch := searchBatch
	searchBatch = 3
	t.Cleanup(func() { searchBatch = batch })
	e := newEnv(t)
	for i := range 10 {
		e.file(filepath.Join(e.home, fmt.Sprintf("note-%d.txt", i)), "")
	}
	hits, _, resp, err := e.searchRun(proto.SearchParams{Path: e.home, Query: "note", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	var res proto.SearchResult
	json.Unmarshal(resp.Result, &res)
	if len(hits) != 4 || !res.Truncated {
		t.Fatalf("hits %d, result %+v", len(hits), res)
	}
	if hits, _, _, _ = e.searchRun(proto.SearchParams{Path: e.home, Query: "note"}); len(hits) != 10 {
		t.Fatalf("hits without limit = %d", len(hits))
	}
}

func TestSearchRejects(t *testing.T) {
	e := newEnv(t)
	e.file(filepath.Join(e.home, "a.txt"), "")
	for name, c := range map[string]struct {
		p    proto.SearchParams
		code string
	}{
		"empty query": {proto.SearchParams{Path: e.home}, proto.CodeBadRequest},
		"long query":  {proto.SearchParams{Path: e.home, Query: string(make([]byte, proto.MaxSearchQuery+1))}, proto.CodeBadRequest},
		"big limit":   {proto.SearchParams{Path: e.home, Query: "a", Limit: proto.MaxSearch + 1}, proto.CodeBadRequest},
		"file":        {proto.SearchParams{Path: filepath.Join(e.home, "a.txt"), Query: "a"}, proto.CodeNotDir},
		"outside":     {proto.SearchParams{Path: "/etc", Query: "a"}, proto.CodeOutsideRoots},
		"missing":     {proto.SearchParams{Path: filepath.Join(e.home, "none"), Query: "a"}, proto.CodeNotFound},
		"relative":    {proto.SearchParams{Path: "home", Query: "a"}, proto.CodeBadRequest},
	} {
		_, _, resp, err := e.searchRun(c.p)
		if resp.OK || resp.Code != c.code || Code(err) != c.code {
			t.Errorf("%s: %+v, %v", name, resp, err)
		}
	}
}

func TestSearchSkipsUnreadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	e := newEnv(t)
	e.file(filepath.Join(e.home, "locked", "match.txt"), "")
	e.file(filepath.Join(e.home, "open", "match.txt"), "")
	if err := os.Chmod(filepath.Join(e.home, "locked"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(e.home, "locked"), 0o755) })
	hits, _, resp, _ := e.searchRun(proto.SearchParams{Path: e.home, Query: "match"})
	var res proto.SearchResult
	json.Unmarshal(resp.Result, &res)
	if len(hits) != 1 || hits[0].Path != "open/match.txt" || res.Skipped != 1 {
		t.Fatalf("hits %+v, result %+v", hits, res)
	}
}

func TestSearchCancel(t *testing.T) {
	every, batch := searchReportEvery, searchBatch
	searchReportEvery, searchBatch = 0, 1
	t.Cleanup(func() { searchReportEvery, searchBatch = every, batch })
	e := newEnv(t)
	// 진행 응답이 소켓 버퍼(수백 KiB)를 넘어야 검색이 끝나기 전에 쓰기가 막히고, 연결을 닫으면 그 쓰기가 실패한다.
	// 응답이 버퍼에 다 들어가면 받는 쪽이 닫기 전에 검색이 끝나 버린다.
	long := strings.Repeat("h", 200)
	for i := range 4000 {
		e.file(filepath.Join(e.home, fmt.Sprintf("hit-%s-%d", long, i)), "")
	}
	client, server := socketPair(t)
	job := e.job
	job.Action = proto.ActionSearch
	job.Params, _ = json.Marshal(proto.SearchParams{Path: e.home, Query: "hit"})
	errc := make(chan error, 1)
	go func() {
		errc <- Serve(job, server, e.cfg)
		server.Close()
	}()
	if resp, err := proto.ReadResponse(client); err != nil || !resp.Partial {
		t.Fatalf("first response %+v, %v", resp, err)
	}
	client.Close()
	if err := <-errc; Code(err) != proto.CodeCanceled {
		t.Fatalf("search after the connection closed: %v", err)
	}
}
