// Package addonpkg는 애드온 패키지(.maru-addon)를 검사하고 풀고 기록하며, 설치한 패키지를 레지스트리에 더한다.
// 설계는 .claude/docs/addon-packages.md, 규칙은 .claude/rules/addon-packages.md.
package addonpkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	"maru/backend/internal/addonpkg/format"
)

// 패키지를 받지 않은 이유(Error.Reason). 관리자 페이지가 이유별 문구를 보인다.
const (
	// ReasonArchive는 압축 파일이 형식에 맞지 않는다는 뜻이다(gzip·tar가 아님, 허용하지 않는 항목·경로).
	ReasonArchive = "archive_invalid"
	// ReasonTooLarge는 크기·개수 상한을 넘었다는 뜻이다.
	ReasonTooLarge = "too_large"
	// ReasonManifest는 manifest·번역·아이콘이 형식에 맞지 않는다는 뜻이다.
	ReasonManifest = "manifest_invalid"
	// ReasonIncompatible은 이 대시보드가 지원하지 않는 SDK 수준이라는 뜻이다.
	ReasonIncompatible = "incompatible"
	// ReasonArchMissing은 실행 중인 아키텍처의 백엔드가 없다는 뜻이다.
	ReasonArchMissing = "arch_missing"
	// ReasonBackend는 백엔드 실행 파일이 이름과 맞는 ELF가 아니라는 뜻이다.
	ReasonBackend = "backend_invalid"
)

// Error는 패키지를 받지 않은 이유다. Detail은 로그·관리자 화면용 영어 문장이다.
type Error struct {
	Reason string
	Detail string
}

func (e *Error) Error() string { return e.Reason + ": " + e.Detail }

func fail(reason, format string, args ...any) *Error {
	return &Error{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// Limits는 압축 파일 상한이다.
type Limits struct {
	Compressed int64 // 압축 파일 전체
	Total      int64 // 풀린 파일 합
	Entries    int   // 항목 수(디렉터리 포함)
	UIFile     int64 // ui/ 아래 파일 하나
	Backend    int64 // 백엔드 실행 파일 하나
	I18n       int64 // 번역 파일 하나
	Icon       int64
	Text       int64 // LICENSE, README.md
}

// DefaultLimits는 설치에 쓰는 상한이다(.claude/docs/addon-packages.md의 "패키지 파일").
var DefaultLimits = Limits{
	Compressed: 64 << 20,
	Total:      160 << 20,
	Entries:    2000,
	UIFile:     8 << 20,
	Backend:    64 << 20,
	I18n:       128 << 10,
	Icon:       128 << 10,
	Text:       256 << 10,
}

// maxDepth는 경로 깊이(조각 수)의 상한이다.
const maxDepth = 16

// segmentPattern은 경로 조각 하나다. 숨김 이름(.으로 시작)과 제어 문자는 받지 않는다.
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// reservedUI는 ui/ 아래에 둘 수 없는 이름이다. 호스트가 프레임 경로의 _maru/로 SDK를 준다.
const reservedUI = "_maru"

// 백엔드 파일 이름(backend/<이름>)과 그 ELF 아키텍처.
var backendArchs = map[string]string{"linux-amd64": "amd64", "linux-arm64": "arm64"}

// i18n 파일의 키·값 상한.
const (
	maxI18nKeys     = 2000
	maxI18nKeyLen   = 100
	maxI18nValueLen = 2000
)

var i18nKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

// Package는 검사를 통과해 스테이징 디렉터리에 풀린 패키지다.
type Package struct {
	Manifest *format.Manifest
	// SHA256은 압축 파일 전체의 sha256(16진수)이다.
	SHA256 string
	// Languages는 번역 파일이 있는 언어다(정렬됨).
	Languages []string
	// Backends는 들어 있는 백엔드 아키텍처(amd64, arm64)다.
	Backends []string
	// Size는 풀린 파일 크기의 합이다.
	Size int64
}

// Rev는 저장 디렉터리 이름이다(<version>-<sha256 앞 8자>).
func (p *Package) Rev() string { return p.Manifest.Version + "-" + p.SHA256[:8] }

// Read는 압축 파일 r을 검사하며 dst(빈 스테이징 디렉터리)에 푼다. 실패하면 dst에 일부 파일이 남을 수 있으므로
// 호출한 쪽이 스테이징 디렉터리를 지운다. 권한은 받은 값을 쓰지 않고 디렉터리 0755, 파일 0644로 맞춘다.
// arch는 실행 중인 아키텍처(runtime.GOARCH)이고, 백엔드 패키지는 그 아키텍처의 실행 파일이 있어야 한다.
func Read(r io.Reader, lim Limits, dst *os.Root, arch string) (*Package, error) {
	h := sha256.New()
	limited := &io.LimitedReader{R: r, N: lim.Compressed + 1}
	tee := io.TeeReader(limited, h)
	zr, err := gzip.NewReader(tee)
	if err != nil {
		return nil, fail(ReasonArchive, "not a gzip file: %v", err)
	}
	tr := tar.NewReader(zr)

	var (
		pkg       = &Package{}
		manifest  []byte
		files     = map[string]int64{}
		dirs      = map[string]bool{}
		entries   int
		languages []string
	)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if limited.N <= 0 {
				return nil, fail(ReasonTooLarge, "archive is larger than %d bytes", lim.Compressed)
			}
			return nil, fail(ReasonArchive, "read tar: %v", err)
		}
		if entries++; entries > lim.Entries {
			return nil, fail(ReasonTooLarge, "more than %d entries", lim.Entries)
		}
		name, err := cleanName(hdr.Name)
		if err != nil {
			return nil, err
		}
		switch hdr.Typeflag {
		case tar.TypeXGlobalHeader:
			// pax 전역 머리(메타데이터)는 파일이 아니다.
			continue
		case tar.TypeDir:
			if err := mkdirs(dst, name, dirs, files); err != nil {
				return nil, err
			}
			continue
		case tar.TypeReg:
		default:
			return nil, fail(ReasonArchive, "%s: only regular files and directories are allowed", name)
		}
		if _, dup := files[name]; dup || dirs[name] {
			return nil, fail(ReasonArchive, "%s: listed twice", name)
		}
		max, err := fileLimit(name, lim)
		if err != nil {
			return nil, err
		}
		if hdr.Size < 0 || hdr.Size > max {
			return nil, fail(ReasonTooLarge, "%s: larger than %d bytes", name, max)
		}
		if pkg.Size += hdr.Size; pkg.Size > lim.Total {
			return nil, fail(ReasonTooLarge, "unpacked size is larger than %d bytes", lim.Total)
		}
		if dir := path.Dir(name); dir != "." {
			if err := mkdirs(dst, dir, dirs, files); err != nil {
				return nil, err
			}
		}
		files[name] = hdr.Size
		var keep *bytes.Buffer
		if name == "manifest.json" {
			keep = &bytes.Buffer{}
		}
		if err := writeFile(dst, name, tr, hdr.Size, keep); err != nil {
			return nil, err
		}
		if keep != nil {
			manifest = keep.Bytes()
		}
		if lang, ok := strings.CutPrefix(name, "i18n/"); ok {
			languages = append(languages, strings.TrimSuffix(lang, ".json"))
		}
	}
	// 압축 파일 끝까지 읽어 sha256을 마친다(tar 끝 뒤에 남은 값도 상한 안이어야 한다).
	if _, err := io.Copy(io.Discard, zr); err != nil {
		return nil, fail(ReasonArchive, "read gzip: %v", err)
	}
	if _, err := io.Copy(io.Discard, tee); err != nil {
		return nil, fail(ReasonArchive, "read archive: %v", err)
	}
	if limited.N <= 0 {
		return nil, fail(ReasonTooLarge, "archive is larger than %d bytes", lim.Compressed)
	}
	pkg.SHA256 = hex.EncodeToString(h.Sum(nil))

	if err := inspect(pkg, dst, manifest, files, languages, arch, true); err != nil {
		return nil, err
	}
	return pkg, nil
}

// inspect는 풀린 파일(files: 경로 → 크기)을 manifest와 맞춰 검사하고 pkg를 채운다. needBackend가 false면
// (개발 디렉터리) 백엔드 패키지에 실행 파일이 없어도 된다(개발자가 개발 소켓으로 따로 띄운다).
func inspect(pkg *Package, dst *os.Root, manifest []byte, files map[string]int64, languages []string, arch string, needBackend bool) error {
	if manifest == nil {
		return fail(ReasonManifest, "manifest.json is missing")
	}
	m, err := format.Decode(manifest)
	if err != nil {
		return fail(ReasonManifest, "%v", err)
	}
	if err := format.Validate(m); err != nil {
		var p *format.Problem
		if errors.As(err, &p) && p.Incompatible {
			return fail(ReasonIncompatible, "%v", err)
		}
		return fail(ReasonManifest, "%v", err)
	}
	pkg.Manifest = m
	if err := checkContents(dst, m, files); err != nil {
		return err
	}
	slices.Sort(languages)
	pkg.Languages = languages
	for _, lang := range languages {
		if err := checkI18n(dst, "i18n/"+lang+".json"); err != nil {
			return err
		}
	}
	pkg.Backends = nil
	for file, a := range backendArchs {
		if _, ok := files["backend/"+file]; ok {
			if err := checkELF(dst, "backend/"+file, a); err != nil {
				return err
			}
			pkg.Backends = append(pkg.Backends, a)
		}
	}
	slices.Sort(pkg.Backends)
	if m.HasBackend() {
		if needBackend && !slices.Contains(pkg.Backends, arch) {
			return fail(ReasonArchMissing, "no backend for %s (backend/linux-%s)", arch, arch)
		}
	} else if len(pkg.Backends) > 0 {
		return fail(ReasonManifest, "backend files are present but the manifest declares no backend")
	}
	return nil
}

// cleanName은 tar 항목 이름을 검사한다. 끝의 /(디렉터리)와 앞의 ./는 떼어 낸다.
func cleanName(name string) (string, error) {
	name = strings.TrimPrefix(name, "./")
	name = strings.TrimSuffix(name, "/")
	if name == "" || name == "." {
		return "", fail(ReasonArchive, "empty entry name")
	}
	if strings.ContainsAny(name, "\\\x00") || strings.HasPrefix(name, "/") {
		return "", fail(ReasonArchive, "%q: invalid path", name)
	}
	parts := strings.Split(name, "/")
	if len(parts) > maxDepth {
		return "", fail(ReasonArchive, "%q: deeper than %d levels", name, maxDepth)
	}
	for _, p := range parts {
		if !segmentPattern.MatchString(p) {
			return "", fail(ReasonArchive, "%q: invalid path segment %q", name, p)
		}
	}
	return name, nil
}

// allowedDir는 최상위에 둘 수 있는 디렉터리인지 본다.
func allowedDir(name string) error {
	top, rest, _ := strings.Cut(name, "/")
	switch top {
	case "ui":
		if first, _, _ := strings.Cut(rest, "/"); first == reservedUI {
			return fail(ReasonArchive, "%s: ui/%s is reserved", name, reservedUI)
		}
		return nil
	case "i18n", "backend":
		if rest != "" {
			return fail(ReasonArchive, "%s: no subdirectories allowed", name)
		}
		return nil
	}
	return fail(ReasonArchive, "%s: not allowed at the top level", name)
}

// fileLimit은 파일 이름으로 허용 여부와 크기 상한을 정한다.
func fileLimit(name string, lim Limits) (int64, error) {
	switch {
	case name == "manifest.json":
		return format.MaxManifestBytes, nil
	case name == format.IconFile:
		return lim.Icon, nil
	case name == "LICENSE" || name == "README.md":
		return lim.Text, nil
	case strings.HasPrefix(name, "ui/"):
		if first, _, _ := strings.Cut(strings.TrimPrefix(name, "ui/"), "/"); first == reservedUI {
			return 0, fail(ReasonArchive, "%s: ui/%s is reserved", name, reservedUI)
		}
		return lim.UIFile, nil
	case strings.HasPrefix(name, "i18n/"):
		lang, ok := strings.CutSuffix(strings.TrimPrefix(name, "i18n/"), ".json")
		if !ok || strings.Contains(lang, "/") || !slices.Contains(format.Languages, lang) {
			return 0, fail(ReasonArchive, "%s: must be i18n/<supported language>.json", name)
		}
		return lim.I18n, nil
	case strings.HasPrefix(name, "backend/"):
		if _, ok := backendArchs[strings.TrimPrefix(name, "backend/")]; !ok {
			return 0, fail(ReasonArchive, "%s: must be backend/linux-amd64 or backend/linux-arm64", name)
		}
		return lim.Backend, nil
	}
	return 0, fail(ReasonArchive, "%s: not allowed at the top level", name)
}

// mkdirs는 dir과 그 위 디렉터리를 0755로 만든다. 같은 이름의 파일이 있으면 거부한다.
func mkdirs(dst *os.Root, dir string, dirs map[string]bool, files map[string]int64) error {
	parts := strings.Split(dir, "/")
	for i := range parts {
		d := strings.Join(parts[:i+1], "/")
		if dirs[d] {
			continue
		}
		if _, isFile := files[d]; isFile {
			return fail(ReasonArchive, "%s: listed as both a file and a directory", d)
		}
		if err := allowedDir(d); err != nil {
			return err
		}
		if err := dst.Mkdir(d, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
		if err := dst.Chmod(d, 0o755); err != nil {
			return fmt.Errorf("chmod %s: %w", d, err)
		}
		dirs[d] = true
	}
	return nil
}

// writeFile은 r의 size 바이트를 dst의 name에 새로 쓴다(있으면 실패). keep이 있으면 내용도 담는다.
func writeFile(dst *os.Root, name string, r io.Reader, size int64, keep *bytes.Buffer) error {
	f, err := dst.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	var w io.Writer = f
	if keep != nil {
		w = io.MultiWriter(f, keep)
	}
	n, err := io.Copy(w, io.LimitReader(r, size))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fail(ReasonArchive, "%s: %v", name, err)
	}
	if n != size {
		return fail(ReasonArchive, "%s: truncated", name)
	}
	return dst.Chmod(name, 0o644)
}

// checkContents는 manifest가 가리키는 파일이 모두 있는지 본다.
func checkContents(dst *os.Root, m *format.Manifest, files map[string]int64) error {
	need := []string{format.IconFile, "i18n/" + format.FallbackLanguage + ".json"}
	if w := m.Views.Window; w != nil {
		need = append(need, "ui/"+w.Entry)
	}
	if w := m.Views.Widget; w != nil {
		need = append(need, "ui/"+w.Entry)
	}
	for _, f := range need {
		if _, ok := files[f]; !ok {
			return fail(ReasonManifest, "%s is missing", f)
		}
	}
	icon, err := readSmall(dst, format.IconFile, DefaultLimits.Icon)
	if err != nil {
		return err
	}
	if !looksLikeSVG(icon) {
		return fail(ReasonManifest, "%s is not an SVG image", format.IconFile)
	}
	return nil
}

// looksLikeSVG는 내용이 SVG 문서로 보이는지다. 아이콘은 <img>로만 그리므로 스크립트가 있어도 실행되지 않지만,
// 다른 형식을 svg로 내보내지 않게 앞부분만 본다.
func looksLikeSVG(b []byte) bool {
	head := strings.ToLower(string(b[:min(len(b), 1024)]))
	return strings.Contains(head, "<svg")
}

// checkI18n은 번역 파일이 평평한 문자열 표인지 본다.
func checkI18n(dst *os.Root, name string) error {
	b, err := readSmall(dst, name, DefaultLimits.I18n)
	if err != nil {
		return err
	}
	var table map[string]string
	if err := json.Unmarshal(b, &table); err != nil {
		return fail(ReasonManifest, "%s: must be a JSON object of strings: %v", name, err)
	}
	if len(table) > maxI18nKeys {
		return fail(ReasonManifest, "%s: more than %d keys", name, maxI18nKeys)
	}
	for k, v := range table {
		if !i18nKeyPattern.MatchString(k) || len(k) > maxI18nKeyLen {
			return fail(ReasonManifest, "%s: invalid key %q", name, k)
		}
		if len(v) > maxI18nValueLen {
			return fail(ReasonManifest, "%s: value of %q is longer than %d bytes", name, k, maxI18nValueLen)
		}
	}
	return nil
}

func readSmall(dst *os.Root, name string, max int64) ([]byte, error) {
	f, err := dst.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(b)) > max {
		return nil, fail(ReasonTooLarge, "%s: larger than %d bytes", name, max)
	}
	return b, nil
}
