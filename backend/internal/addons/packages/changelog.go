package packages

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/aptindex"
	"maru/backend/internal/omv"
)

// 변경 기록은 OMV Apt.getChangeLog로 가져온다(관리자 전용, 백그라운드 작업). OMV는 .deb가 apt 캐시에 없으면
// apt-get --download-only로 받고(받은 파일은 캐시에 남아 설치 때 다시 쓴다) usr/share/doc/<이름>/changelog*.gz를
// 풀어 작업 출력으로 준다. 이 메서드는 파일 이름에서 뗀 패키지 이름을 이스케이프하지 않고 셸 명령에 넣으므로
// 쓰기로 선언해 /api/rpc가 중계하지 못하게 하고, 브라우저가 보낸 값 대신 업데이트 목록에서 찾은 파일 이름을
// 형식 검사한 뒤에만 넘긴다.
const (
	// changelogTimeout은 변경 기록을 가져오는 데 주는 시간이다. 커널처럼 큰 패키지는 내려받기가 오래 걸린다.
	changelogTimeout = 5 * time.Minute
	// changelogReadLimit은 받아 두는 변경 기록의 상한이다(Debian 변경 기록은 최신 항목이 위에 있다).
	changelogReadLimit = 1 << 20
	// earlierLimit은 설치된 버전과 그 이전 항목을 돌려주는 상한이다.
	earlierLimit = 64 << 10
	// changelogCacheSize는 기억해 두는 변경 기록 수다(파일 이름, 곧 버전마다 내용이 같다).
	changelogCacheSize = 8
)

var (
	// bgStatusPattern은 OMV execBgProc가 돌려주는 상태 파일 이름이다(jobs/omvjob과 같다).
	bgStatusPattern = regexp.MustCompile(`^/tmp/bgstatus[A-Za-z0-9]+$`)
	// debFilePattern은 저장소 안 .deb 경로다(pool/main/o/openssl/openssl_3.5.4-1~deb13u1_amd64.deb,
	// 에포크는 %3a). 로컬 저장소는 ./로 시작할 수 있다.
	debFilePattern = regexp.MustCompile(`^(\./)?[A-Za-z0-9][A-Za-z0-9.+~%_/-]*\.deb$`)
	// debNamePattern은 Debian 패키지 이름이다(정책 5.6.1). OMV가 이 값을 셸 명령에 그대로 넣는다.
	debNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]+$`)
	// changelogHeader는 Debian 변경 기록 항목의 첫 줄 "패키지 (버전) 배포판; 속성"이다.
	changelogHeader = regexp.MustCompile(`(?m)^[a-z0-9][a-z0-9.+-]+ \(([^()\s]+)\) [^;\n]*;`)
	// binNMU는 소스 변경 기록에 없는 바이너리 재빌드 접미사(+b1)다.
	binNMU = regexp.MustCompile(`\+b[0-9]+$`)
)

// validDebFile은 OMV에 넘길 .deb 경로가 안전한 형식인지다. OMV는 기본 이름을 "_"로 나눠 첫 조각(패키지
// 이름)을 셸 명령에, 둘째·셋째 조각(버전, 아키텍처)을 glob에 넣는다.
func validDebFile(filename string) bool {
	if !debFilePattern.MatchString(filename) || strings.Contains(filename, "..") {
		return false
	}
	base := filename[strings.LastIndexByte(filename, '/')+1:]
	parts := strings.Split(strings.TrimSuffix(base, ".deb"), "_")
	return len(parts) >= 3 && debNamePattern.MatchString(parts[0]) && parts[1] != "" && parts[2] != ""
}

// changelogs는 변경 기록을 가져와 기억해 둔다. OMV 호출은 한 번에 하나만 한다(동시에 apt-get을 돌리면
// apt 캐시 잠금 때문에 내려받기가 실패한다). 브라우저가 끊어도 조작은 끝까지 돌므로 결과를 기억해 두면
// 다시 열 때 바로 보인다.
type changelogs struct {
	sem   chan struct{}
	cache *changelogCache
	// interval은 작업이 끝났는지 묻는 간격이다(테스트는 줄인다).
	interval time.Duration
}

func newChangelogs() *changelogs {
	return &changelogs{sem: make(chan struct{}, 1), cache: &changelogCache{}, interval: addons.BgPollInterval}
}

// query는 changelog 조작이다. 대상은 업데이트 목록의 패키지 이름이다.
func (c *changelogs) query(r *aptindex.Reader) func(context.Context, addons.ActionRequest) (any, error) {
	return func(ctx context.Context, req addons.ActionRequest) (any, error) {
		pkg, err := findPackage(r, req.Target)
		if err != nil {
			return nil, err
		}
		if !validDebFile(pkg.Filename) {
			log.Printf("packages: changelog %q: unexpected filename %q", pkg.Name, pkg.Filename)
			return nil, addons.ErrInvalidTarget
		}
		if req.Jobs != nil && req.Jobs.Busy() {
			return nil, &addons.ActionError{Status: http.StatusConflict, Code: api.ErrJobBusy, Message: "a job is running"}
		}
		text, cut, err := c.get(ctx, req.RPC, pkg.Filename)
		if err != nil {
			return nil, err
		}
		return SplitChangelog(pkg, text, cut), nil
	}
}

// findPackage는 업데이트 목록에서 name 패키지를 찾는다. 없으면 404 not_found다(목록이 바뀌었다).
func findPackage(r *aptindex.Reader, name string) (aptindex.Package, error) {
	notFound := &addons.ActionError{Status: http.StatusNotFound, Code: api.ErrNotFound, Message: "package is not in the update list"}
	if name == "" {
		return aptindex.Package{}, addons.ErrInvalidTarget
	}
	idx, err := r.Index()
	if errors.Is(err, aptindex.ErrNoIndex) {
		return aptindex.Package{}, notFound
	}
	if err != nil {
		return aptindex.Package{}, err
	}
	i := slices.IndexFunc(idx.Packages, func(p aptindex.Package) bool { return p.Name == name })
	if i < 0 {
		return aptindex.Package{}, notFound
	}
	return idx.Packages[i], nil
}

// get은 filename의 변경 기록 원문을 돌려준다. 기억해 둔 것이 없으면 OMV에서 가져온다.
func (c *changelogs) get(ctx context.Context, rpc addons.RPCCaller, filename string) (string, bool, error) {
	if e, ok := c.cache.get(filename); ok {
		return e.text, e.cut, nil
	}
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return "", false, addons.ErrActionTimeout
	}
	defer func() { <-c.sem }()
	// 기다리는 동안 다른 요청이 같은 파일을 가져왔을 수 있다.
	if e, ok := c.cache.get(filename); ok {
		return e.text, e.cut, nil
	}
	var status string
	if err := addons.CallInto(ctx, rpc, "Apt", "getChangeLog", map[string]string{"filename": filename}, &status); err != nil {
		return "", false, changelogError(filename, err)
	}
	if !bgStatusPattern.MatchString(status) {
		return "", false, fmt.Errorf("%w: unexpected background status file %q", addons.ErrActionFailed, status)
	}
	text, cut, err := addons.ReadBg(ctx, rpc, status, c.interval, changelogReadLimit)
	if err != nil {
		return "", false, changelogError(filename, err)
	}
	text = cleanChangelog(text, cut)
	c.cache.put(cachedChangelog{filename: filename, text: text, cut: cut})
	return text, cut, nil
}

// changelogError는 OMV 오류를 화면이 가릴 수 있는 오류로 바꾼다. OMV 문구는 로그에만 남긴다.
func changelogError(filename string, err error) error {
	if errors.Is(err, addons.ErrActionTimeout) {
		return err
	}
	var oe *omv.Error
	if !errors.As(err, &oe) || !oe.FromRPC() || oe.HTTPStatus == http.StatusUnauthorized || oe.HTTPStatus == http.StatusForbidden {
		// 네트워크·세션·권한 문제는 서버가 omv_error나 로그아웃으로 처리한다.
		return err
	}
	log.Printf("packages: changelog %q: %v", filename, err)
	switch {
	case strings.HasPrefix(oe.Message, "No changelog found"):
		return &addons.ActionError{Status: http.StatusNotFound, Code: api.ErrNoChangelog, Message: "the package has no changelog"}
	case strings.HasPrefix(oe.Message, "Package file '"):
		return &addons.ActionError{Status: http.StatusBadGateway, Code: api.ErrDownloadFailed, Message: "the package file could not be downloaded"}
	}
	return fmt.Errorf("%w: %w", addons.ErrActionFailed, err)
}

// cleanChangelog은 받은 원문을 화면에 보일 수 있게 다듬는다. 잘렸으면 끝의 반쪽 줄을 버리고, UTF-8이 아닌
// 바이트와 줄바꿈·탭이 아닌 제어 문자를 지운다.
func cleanChangelog(text string, cut bool) string {
	if cut {
		if i := strings.LastIndexByte(text, '\n'); i >= 0 {
			text = text[:i+1]
		}
	}
	text = strings.ToValidUTF8(text, "�")
	text = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return -1
		}
		return r
	}, text)
	return strings.TrimRight(text, " \t\n")
}

// SplitChangelog은 변경 기록을 설치된 버전보다 새 항목(Recent)과 설치된 버전부터의 항목(Earlier)으로
// 나눈다. 설치된 버전의 머리 줄은 정확히 같은 버전을, 없으면 바이너리 재빌드 접미사(+bN)를 뗀 버전을 찾는다.
// 찾지 못하면(새로 설치, Debian 형식이 아님) 전체가 Recent다. Earlier는 earlierLimit에서 항목 경계로 자른다.
func SplitChangelog(pkg aptindex.Package, text string, cut bool) api.PackageChangelog {
	out := api.PackageChangelog{Name: pkg.Name, Version: pkg.Version, Recent: text, Truncated: cut}
	if pkg.OldVersion == "" {
		return out
	}
	want := []string{pkg.OldVersion}
	if v := binNMU.ReplaceAllString(pkg.OldVersion, ""); v != pkg.OldVersion {
		want = append(want, v)
	}
	headers := changelogHeader.FindAllStringSubmatchIndex(text, -1)
	at := slices.IndexFunc(headers, func(m []int) bool { return slices.Contains(want, text[m[2]:m[3]]) })
	if at < 0 {
		return out
	}
	start := headers[at][0]
	out.Matched = true
	out.Recent = strings.TrimRight(text[:start], " \t\n")
	earlier := text[start:]
	if len(earlier) > earlierLimit {
		end := 0
		// 한도 안에서 마지막으로 시작하는 다음 항목 앞에서 자른다.
		for _, m := range headers[at+1:] {
			if m[0]-start > earlierLimit {
				break
			}
			end = m[0] - start
		}
		if end == 0 {
			// 설치된 버전의 항목 하나가 한도보다 길면 줄 경계에서 자른다.
			end = max(strings.LastIndexByte(earlier[:earlierLimit], '\n'), 0)
		}
		earlier = earlier[:end]
		out.Truncated = true
	}
	out.Earlier = strings.TrimRight(earlier, " \t\n")
	return out
}

// changelogCache는 최근에 가져온 변경 기록이다(가장 최근 것이 뒤).
type changelogCache struct {
	mu      sync.Mutex
	entries []cachedChangelog
}

type cachedChangelog struct {
	filename string
	text     string
	cut      bool
}

func (c *changelogCache) get(filename string) (cachedChangelog, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	i := slices.IndexFunc(c.entries, func(e cachedChangelog) bool { return e.filename == filename })
	if i < 0 {
		return cachedChangelog{}, false
	}
	e := c.entries[i]
	c.entries = append(slices.Delete(c.entries, i, i+1), e)
	return e, true
}

func (c *changelogCache) put(e cachedChangelog) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = slices.DeleteFunc(c.entries, func(x cachedChangelog) bool { return x.filename == e.filename })
	c.entries = append(c.entries, e)
	if len(c.entries) > changelogCacheSize {
		c.entries = slices.Delete(c.entries, 0, len(c.entries)-changelogCacheSize)
	}
}
