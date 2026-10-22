package worker

import (
	"encoding/json"
	"os"
	"path"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"maru/backend/internal/helper/proto"
)

// fs.search는 폴더와 그 아래 모든 폴더를 너비 우선으로 읽으며 이름에 찾기어가 든 항목을 찾는다(대소문자 무시,
// 파일 탐색기의 지금 폴더 찾기와 같은 규칙). 폴더는 fs.names처럼 getdents로 이름과 종류만 읽고 항목마다 lstat하지
// 않는다. 심볼릭 링크는 따라가지 않고, 읽지 못한 폴더는 건너뛰어 센다. 찾은 항목은 Partial 응답으로 보내며 받는
// 쪽이 연결을 끊으면 다음 응답을 쓰지 못해 멈춘다. 기준 디렉터리 밖으로는 os.Root가 막는다.

// 테스트가 바꾼다.
var (
	// searchReportEvery는 진행 응답을 보내는 최소 간격이다.
	searchReportEvery = 250 * time.Millisecond
	// searchBatch는 진행 응답 하나에 담는 최대 항목 수다(응답 머리 8MiB 안에 들게).
	searchBatch = 256
)

// searcher는 fs.search 하나의 상태다.
type searcher struct {
	c     idleConn
	r     *os.Root
	query string
	limit int
	found int
	prog  proto.SearchProgress
	last  time.Time
}

func search(job Job, cfg Config, c idleConn) error {
	var p proto.SearchParams
	k := &searcher{c: c}
	var start string
	err := func() error {
		if err := proto.DecodeParams(job.Params, &p); err != nil {
			return err
		}
		if p.Query == "" || len(p.Query) > proto.MaxSearchQuery || !utf8.ValidString(p.Query) || p.Limit < 0 || p.Limit > proto.MaxSearch {
			return proto.NewError(proto.CodeBadRequest)
		}
		k.query = strings.ToLower(p.Query)
		k.limit = p.Limit
		if k.limit == 0 {
			k.limit = proto.MaxSearch
		}
		r, t, err := open(job, cfg, p.Path)
		if err != nil {
			return err
		}
		k.r = r
		start = t.rel
		st, err := r.Stat(start)
		if err != nil {
			return err
		}
		if !st.IsDir() {
			return proto.NewError(proto.CodeNotDir)
		}
		return nil
	}()
	if k.r != nil {
		defer k.r.Close()
	}
	if err != nil {
		e := toError(err)
		c.respond(proto.Fail(e))
		return e
	}

	// 첫 진행 응답을 바로 보낸다. 그 전의 오류만 오류 응답이 된다.
	if err := k.report(true); err != nil {
		return err
	}
	truncated, err := k.walk(start)
	if err != nil {
		return err
	}
	if err := k.report(true); err != nil {
		return err
	}
	res := proto.SearchResult{Dirs: k.prog.Dirs, Skipped: k.prog.Skipped, Truncated: truncated}
	return reply(c, func() (any, error) { return res, nil })
}

// walk는 start(기준 안 상대 경로)부터 너비 우선으로 찾는다. 상한에 닿으면 truncated다. 진행 응답을 쓰지 못하면
// 오류(connError)다.
func (k *searcher) walk(start string) (truncated bool, err error) {
	queue := []string{start}
	for len(queue) > 0 {
		dir := queue[0]
		queue[0] = ""
		queue = queue[1:]
		list, err := k.readDir(dir)
		if err != nil {
			k.prog.Skipped++
			if err := k.report(false); err != nil {
				return false, err
			}
			continue
		}
		k.prog.Dirs++
		for _, e := range list {
			rel := path.Join(dir, e.Name)
			if e.Type == proto.NameDir {
				queue = append(queue, rel)
			}
			if !strings.Contains(strings.ToLower(e.Name), k.query) {
				continue
			}
			// UTF-8이 아닌 이름(그 아래 경로 포함)은 JSON으로 옮길 수 없으므로 찾은 항목에 넣지 않는다.
			hit := rel
			if start != "." {
				hit = rel[len(start)+1:]
			}
			if !utf8.ValidString(hit) {
				continue
			}
			if k.found >= k.limit {
				return true, nil
			}
			k.found++
			k.prog.Hits = append(k.prog.Hits, proto.SearchHit{Path: hit, Type: hitType(e.Type)})
			if len(k.prog.Hits) >= searchBatch {
				if err := k.report(true); err != nil {
					return false, err
				}
			}
		}
		if err := k.report(false); err != nil {
			return false, err
		}
	}
	return false, nil
}

// readDir는 폴더 하나의 이름과 종류다. 종류를 주지 않는 파일시스템의 항목은 짧은 시간 안에서만 lstat한다.
func (k *searcher) readDir(rel string) ([]proto.NameEntry, error) {
	f, err := k.r.OpenFile(rel, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	list, _, err := readNames(f)
	if err != nil {
		return nil, err
	}
	return resolveUnknown(k.r, rel, list, time.Now().Add(time.Second)), nil
}

// report는 진행 응답을 보낸다. force가 아니면 searchReportEvery마다 한 번만 보낸다. 보낸 항목은 비운다.
func (k *searcher) report(force bool) error {
	now := time.Now()
	if !force && now.Sub(k.last) < searchReportEvery {
		return nil
	}
	k.last = now
	b, err := json.Marshal(k.prog)
	if err != nil {
		return err
	}
	k.prog.Hits = nil
	if err := k.c.respond(proto.Response{OK: true, Partial: true, Result: b}); err != nil {
		return connError{err}
	}
	return nil
}

func hitType(t byte) string {
	switch t {
	case proto.NameDir:
		return proto.TypeDir
	case proto.NameFile:
		return proto.TypeFile
	case proto.NameSymlink:
		return proto.TypeSymlink
	case proto.NameOther:
		return proto.TypeOther
	}
	return ""
}
