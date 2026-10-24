// Package aptindex는 OMV가 만드는 APT 업데이트 목록과 재부팅 권장 파일을 읽는다.
//
// 업데이트 목록은 /var/lib/openmediavault/apt/upgradeindex.json(OMV omv-mkaptidx가 apt 후크로 다시 만들고
// apt-daily가 매일 갱신, 0644), 재부팅 권장은 /run/reboot-required가 있는지와 /run/reboot-required.pkgs다.
// OMV System.getInformation의 availablePkgUpdates·rebootRequired와 같은 출처이며 root 없이 읽힌다.
// 시스템 업데이트 애드온과 알림 출처가 함께 쓴다.
package aptindex

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"
)

// 루트(fs.FS) 기준 경로다.
const (
	IndexPath      = "var/lib/openmediavault/apt/upgradeindex.json"
	RebootPath     = "run/reboot-required"
	RebootPkgsPath = "run/reboot-required.pkgs"
)

// 읽는 크기의 상한. 인덱스는 패키지마다 2KB 안팎이다.
const (
	maxIndexBytes   = 32 << 20
	maxPkgsBytes    = 64 << 10
	maxRebootPkgs   = 200
	maxFieldBytes   = 512
	maxIndexEntries = 5000
)

// ErrNoIndex는 업데이트 목록 파일이 없다는 뜻이다(OMV가 아직 만들지 않음). 업데이트가 없다는 뜻이 아니다.
var ErrNoIndex = errors.New("apt upgrade index does not exist")

// Package는 업데이트할 패키지 하나다.
type Package struct {
	Name string
	// Version은 설치될 버전이다.
	Version string
	// OldVersion은 지금 설치된 버전이다. 새로 설치되는 패키지는 빈 값이다.
	OldVersion string
	// Repository는 "라벨/아카이브"다. 로컬 저장소는 "/"이거나 라벨이 비어 있을 수 있다.
	Repository string
	Section    string
	Summary    string
	// Filename은 저장소 안 .deb 경로다(워크벤치의 변경 기록 화면이 쓴다).
	Filename string
	// Size는 내려받을 크기(바이트)다.
	Size int64
}

// indexEntry는 upgradeindex.json 항목 중 읽는 필드다. 나머지 필드는 버린다.
type indexEntry struct {
	Name       string       `json:"name"`
	Package    string       `json:"package"`
	Version    string       `json:"version"`
	OldVersion string       `json:"oldversion"`
	Repository string       `json:"repository"`
	Section    string       `json:"section"`
	Summary    string       `json:"summary"`
	Filename   string       `json:"filename"`
	Size       *json.Number `json:"size"`
}

// ParseIndex는 upgradeindex.json(JSON 배열)을 읽는다. 이름이 없는 항목은 버리고 이름순으로 정렬한다.
func ParseIndex(r io.Reader) ([]Package, error) {
	var entries []indexEntry
	if err := json.NewDecoder(io.LimitReader(r, maxIndexBytes)).Decode(&entries); err != nil {
		return nil, fmt.Errorf("apt index: %w", err)
	}
	if len(entries) > maxIndexEntries {
		entries = entries[:maxIndexEntries]
	}
	out := make([]Package, 0, len(entries))
	for _, e := range entries {
		name := field(e.Name)
		if name == "" {
			name = field(e.Package)
		}
		if name == "" {
			continue
		}
		p := Package{
			Name:       name,
			Version:    field(e.Version),
			OldVersion: field(e.OldVersion),
			Repository: field(e.Repository),
			Section:    field(e.Section),
			Summary:    field(e.Summary),
			Filename:   field(e.Filename),
		}
		if e.Size != nil {
			if n, err := e.Size.Int64(); err == nil && n > 0 {
				p.Size = n
			}
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Package) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// field는 문자열 필드를 다듬는다. 앞뒤 공백을 지우고 상한을 넘으면 자른다(UTF-8 경계를 지킨다).
func field(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxFieldBytes {
		return s
	}
	cut := maxFieldBytes
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut]
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// ParseRebootPkgs는 reboot-required.pkgs(줄마다 패키지 이름)를 읽는다. 빈 줄과 중복은 버리고 처음 순서를 지킨다.
func ParseRebootPkgs(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(io.LimitReader(r, maxPkgsBytes))
	for sc.Scan() {
		name := field(sc.Text())
		if name == "" || slices.Contains(out, name) {
			continue
		}
		out = append(out, name)
		if len(out) == maxRebootPkgs {
			break
		}
	}
	return out, sc.Err()
}

// Index는 업데이트 목록과 파일을 마지막으로 쓴 시각이다.
type Index struct {
	Packages []Package
	// ModTime은 OMV가 목록을 마지막으로 다시 만든 시각이다(내용이 같아도 매일 바뀐다).
	ModTime time.Time
}

// Reboot은 재부팅 권장 상태다.
type Reboot struct {
	Required bool
	// Since는 재부팅 권장 파일을 마지막으로 쓴 시각이다. Required가 아니면 0이다.
	Since time.Time
	// Packages는 재부팅을 요청한 패키지 이름이다(파일이 없으면 비어 있다).
	Packages []string
}

// Reader는 루트 파일시스템에서 업데이트 목록과 재부팅 권장 파일을 읽는다.
type Reader struct {
	root fs.FS
}

// NewReader는 호스트의 /를 읽는 Reader를 반환한다.
func NewReader() *Reader { return NewReaderFS(os.DirFS("/")) }

// NewReaderFS는 root를 /로 보고 읽는 Reader를 반환한다(테스트, 개발 서버의 가짜 루트).
func NewReaderFS(root fs.FS) *Reader { return &Reader{root: root} }

// Index는 업데이트 목록을 읽는다. 파일이 없으면 ErrNoIndex다.
func (r *Reader) Index() (Index, error) {
	f, err := r.root.Open(IndexPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Index{}, ErrNoIndex
	}
	if err != nil {
		return Index{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Index{}, err
	}
	if !st.Mode().IsRegular() {
		return Index{}, fmt.Errorf("apt index: not a regular file")
	}
	pkgs, err := ParseIndex(f)
	if err != nil {
		return Index{}, err
	}
	return Index{Packages: pkgs, ModTime: st.ModTime()}, nil
}

// IndexModTime은 업데이트 목록 파일의 수정 시각이다. 파일이 없으면 ErrNoIndex다.
func (r *Reader) IndexModTime() (time.Time, error) {
	st, err := fs.Stat(r.root, IndexPath)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, ErrNoIndex
	}
	if err != nil {
		return time.Time{}, err
	}
	return st.ModTime(), nil
}

// Reboot은 재부팅 권장 상태를 읽는다. 패키지 목록 파일을 읽지 못해도 권장 여부는 돌려준다.
func (r *Reader) Reboot() (Reboot, error) {
	st, err := fs.Stat(r.root, RebootPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Reboot{}, nil
	}
	if err != nil {
		return Reboot{}, err
	}
	out := Reboot{Required: true, Since: st.ModTime()}
	f, err := r.root.Open(RebootPkgsPath)
	if err != nil {
		return out, nil
	}
	defer f.Close()
	if pkgs, err := ParseRebootPkgs(f); err == nil {
		out.Packages = pkgs
	}
	return out, nil
}

// Keys는 패키지마다 "이름@버전"이다. 알림 리비전과 설치 전후 비교에 쓴다.
func Keys(pkgs []Package) []string {
	out := make([]string, len(pkgs))
	for i, p := range pkgs {
		out[i] = p.Name + "@" + p.Version
	}
	return out
}
