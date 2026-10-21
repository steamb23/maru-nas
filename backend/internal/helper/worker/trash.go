package worker

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"onmaru/backend/internal/helper/proto"
)

// 휴지통: FreeDesktop 휴지통 규격을 trash-cli의 trash-put과 같은 곳·형식으로 쓴다. 그래서 서버의 trash-list,
// trash-restore, trash-empty가 같은 휴지통을 다룬다.
//   - 홈 기준 안의 항목은 홈과 같은 파일시스템이면 홈 휴지통(~/.local/share/Trash)에 넣고 Path는 절대 경로다
//   - 그 밖의 항목은 그 항목이 든 가장 깊은 마운트 지점(topdir)의 .Trash/<uid>(.Trash가 심볼릭 링크가 아닌 sticky
//     폴더일 때), 아니면 topdir/.Trash-<uid>에 넣고 Path는 topdir 기준 상대 경로다
//   - /srv 아래 디스크의 항목은 홈과 같은 파일시스템이어도 디스크 휴지통을 쓴다. 기준 디렉터리마다 os.Root로 가두므로
//     기준 사이로는 옮기지 않기 때문이다(trash-put과 다른 점)
//
// 휴지통 안의 이름은 info/<이름>.trashinfo를 O_EXCL로 먼저 만들어 정하고(겹치면 _1, _2 …), 그 이름으로 항목을 옮긴다.

const (
	trashInfoSuffix = ".trashinfo"
	// homeTrashRel은 홈 안의 홈 휴지통 경로다(XDG_DATA_HOME은 쓰지 않는다. worker는 빈 환경 변수로 돈다).
	homeTrashRel = ".local/share/Trash"
	// trashDateLayout은 trashinfo의 DeletionDate 형식이다(지역 시각, 시간대 없음).
	trashDateLayout = "2006-01-02T15:04:05"
	// maxTrashSuffix는 같은 이름이 휴지통에 있을 때 붙여 볼 번호(_1, _2 …)의 최대다.
	maxTrashSuffix = 1000
	// maxTrashInfo는 읽는 trashinfo 파일의 크기 상한이다.
	maxTrashInfo = 64 << 10
	// maxTrashListBytes는 fs.trashlist 결과의 대략적인 크기 상한이다(응답 머리 8MiB 안).
	maxTrashListBytes = 6 << 20
)

// MaxTrashItems는 fs.trashlist가 담는 최대 항목 수다. 넘치면 Truncated다(테스트가 줄인다).
var MaxTrashItems = 10000

// trashDir는 휴지통 디렉터리 하나다.
type trashDir struct {
	// base는 휴지통이 든 기준 디렉터리(os.Root), rel은 그 안의 휴지통 경로다.
	base string
	rel  string
	// top은 trashinfo Path의 기준이다. 홈 휴지통은 빈 값(절대 경로)이다.
	top  string
	kind string
}

func (d trashDir) abs() string { return path.Join(d.base, d.rel) }

// relIn은 p를 base 안의 상대 경로로 바꾼다(p가 base 자신이면 ".").
func relIn(base, p string) string {
	if p == base {
		return "."
	}
	return strings.TrimPrefix(p, base+"/")
}

// within은 p가 root이거나 그 아래인지다.
func within(p, root string) bool {
	return p == root || strings.HasPrefix(p, root+"/")
}

// allMounts는 모든 마운트 지점이다(/srv 밖 포함).
func allMounts(cfg Config) ([]string, error) {
	rc, err := cfg.MountInfo()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return ParseMountInfo(rc)
}

// topdirsIn은 base 안(base 자신 포함)의 마운트 지점이다. 홈 기준은 홈 자신을 빼고 그 아래 마운트만 본다.
func topdirsIn(base string, mounts []string, home bool) []string {
	var out []string
	if !home {
		out = append(out, base)
	}
	for _, m := range mounts {
		if m != base && within(m, base) && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// topdirOf는 p가 든 가장 깊은 마운트 지점이다(base 안에서). 없으면 base다.
func topdirOf(p, base string, mounts []string) string {
	best := base
	for _, m := range mounts {
		if within(p, m) && within(m, base) && len(m) > len(best) {
			best = m
		}
	}
	return best
}

func ownedDir(fi fs.FileInfo, uid int) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 && int(st.Uid) == uid
}

// sharedUsable은 topRel/.Trash가 모든 사용자가 쓰는 휴지통(심볼릭 링크가 아닌 sticky 폴더)인지다.
func sharedUsable(r *os.Root, topRel string) bool {
	fi, err := r.Lstat(path.Join(topRel, ".Trash"))
	return err == nil && fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 && fi.Mode()&fs.ModeSticky != 0
}

// volumeTrash는 topdir의 휴지통이다. create면 없는 사용자 폴더를 만든다(0700). 쓸 수 있는 휴지통이 없으면 false다.
func volumeTrash(r *os.Root, base, top string, uid int, create bool) (trashDir, bool) {
	topRel := relIn(base, top)
	if sharedUsable(r, topRel) {
		rel := path.Join(topRel, ".Trash", strconv.Itoa(uid))
		fi, err := r.Lstat(rel)
		if errors.Is(err, fs.ErrNotExist) && create && r.Mkdir(rel, 0o700) == nil {
			fi, err = r.Lstat(rel)
		}
		if err == nil && ownedDir(fi, uid) {
			return trashDir{base: base, rel: rel, top: top, kind: proto.TrashShared}, true
		}
	}
	rel := path.Join(topRel, ".Trash-"+strconv.Itoa(uid))
	fi, err := r.Lstat(rel)
	if errors.Is(err, fs.ErrNotExist) && create && r.Mkdir(rel, 0o700) == nil {
		fi, err = r.Lstat(rel)
	}
	if err == nil && ownedDir(fi, uid) {
		return trashDir{base: base, rel: rel, top: top, kind: proto.TrashOwn}, true
	}
	return trashDir{}, false
}

func homeTrash(home string) trashDir {
	return trashDir{base: home, rel: homeTrashRel, kind: proto.TrashHome}
}

func sameDevice(a, b fs.FileInfo) bool {
	sa, ok1 := a.Sys().(*syscall.Stat_t)
	sb, ok2 := b.Sys().(*syscall.Stat_t)
	return ok1 && ok2 && sa.Dev == sb.Dev
}

// chooseTrash는 t(기준 안의 항목, 정보 fi)를 넣을 휴지통을 정하고 없으면 만든다.
func chooseTrash(job Job, cfg Config, r *os.Root, t target, fi fs.FileInfo) (trashDir, error) {
	if job.Home != "" && t.base == job.Home {
		if hfi, err := r.Lstat("."); err == nil && sameDevice(fi, hfi) {
			d := homeTrash(job.Home)
			if err := r.MkdirAll(d.rel, 0o700); err != nil {
				return trashDir{}, err
			}
			return d, nil
		}
	}
	mounts, err := allMounts(cfg)
	if err != nil {
		return trashDir{}, err
	}
	top := topdirOf(t.path, t.base, mounts)
	d, ok := volumeTrash(r, t.base, top, os.Getuid(), true)
	if !ok {
		return trashDir{}, proto.NewError(proto.CodeNoTrash)
	}
	return d, nil
}

// trashQuote는 trashinfo의 Path 값이다: 비예약 문자(A-Z a-z 0-9 - _ . ~)와 /만 그대로 두고 나머지 바이트는 %XX다
// (trash-cli의 urllib.parse.quote(path, '/')와 같다).
func trashQuote(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || strings.IndexByte("-_.~/", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// trashUnquote는 trashQuote를 되돌린다. 잘못된 %XX는 그대로 둔다.
func trashUnquote(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// trashInfo는 trashinfo 파일 내용이다.
func trashInfo(p string, at time.Time) []byte {
	return []byte("[Trash Info]\nPath=" + trashQuote(p) + "\nDeletionDate=" + at.Format(trashDateLayout) + "\n")
}

// parseTrashInfo는 trashinfo의 Path(풀어 쓴 값)와 DeletionDate다. [Trash Info] 절의 값만 본다.
func parseTrashInfo(data []byte) (p string, deleted time.Time, ok bool) {
	section := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.HasPrefix(line, "[") {
			section = line == "[Trash Info]"
			continue
		}
		if !section {
			continue
		}
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		switch k {
		case "Path":
			p, ok = trashUnquote(v), v != ""
		case "DeletionDate":
			if t, err := time.ParseInLocation(trashDateLayout, v, time.Local); err == nil {
				deleted = t
			}
		}
	}
	return p, deleted, ok
}

// reserveTrashName은 휴지통 안에서 쓸 이름을 정하고 그 이름의 trashinfo를 만든다(O_EXCL이 자리 잡기다).
func reserveTrashName(r *os.Root, d trashDir, name string, content []byte) (string, error) {
	for i := 0; i <= maxTrashSuffix; i++ {
		n := name
		if i > 0 {
			n = name + "_" + strconv.Itoa(i)
		}
		if len(n)+len(trashInfoSuffix) > 255 {
			return "", proto.NewError(proto.CodeBadRequest)
		}
		if _, err := r.Lstat(path.Join(d.rel, "files", n)); err == nil {
			continue
		}
		info := path.Join(d.rel, "info", n+trashInfoSuffix)
		f, err := r.OpenFile(info, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.Write(content)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			r.Remove(info)
			return "", werr
		}
		return n, nil
	}
	return "", proto.NewError(proto.CodeExists)
}

// trash는 항목 하나를 휴지통으로 옮긴다(fs.trash).
func trash(job Job, cfg Config, p proto.PathParams) (proto.TrashResult, error) {
	bs, err := bases(job, cfg)
	if err != nil {
		return proto.TrashResult{}, err
	}
	t, err := resolve(p.Path, bs)
	if err != nil {
		return proto.TrashResult{}, err
	}
	if t.rel == "." {
		return proto.TrashResult{}, proto.NewError(proto.CodeBadRequest)
	}
	r, err := os.OpenRoot(t.base)
	if err != nil {
		return proto.TrashResult{}, err
	}
	defer r.Close()
	fi, err := r.Lstat(t.rel)
	if err != nil {
		return proto.TrashResult{}, err
	}
	d, err := chooseTrash(job, cfg, r, t, fi)
	if err != nil {
		return proto.TrashResult{}, err
	}
	// 휴지통 자신이나 그것을 품은 폴더(~/.local 등)는 휴지통에 넣지 않는다.
	if within(d.abs(), t.path) {
		return proto.TrashResult{}, proto.NewError(proto.CodeBadRequest)
	}
	for _, sub := range []string{"files", "info"} {
		if err := r.MkdirAll(path.Join(d.rel, sub), 0o700); err != nil {
			return proto.TrashResult{}, err
		}
	}
	orig := t.path
	if d.top != "" {
		orig = relIn(d.top, t.path)
	}
	name, err := reserveTrashName(r, d, path.Base(t.rel), trashInfo(orig, cfg.Now()))
	if err != nil {
		return proto.TrashResult{}, err
	}
	if err := r.Rename(t.rel, path.Join(d.rel, "files", name)); err != nil {
		r.Remove(path.Join(d.rel, "info", name+trashInfoSuffix))
		return proto.TrashResult{}, err
	}
	return proto.TrashResult{Trash: d.abs(), Name: name}, nil
}

// trashVolumes는 기준 디렉터리마다 휴지통을 쓸 수 있는지다(fs.trashdirs). 만들지 않고 보기만 한다.
func trashVolumes(job Job, cfg Config) (proto.TrashDirsResult, error) {
	bs, err := bases(job, cfg)
	if err != nil {
		return proto.TrashDirsResult{}, err
	}
	uid := os.Getuid()
	out := proto.TrashDirsResult{Volumes: []proto.TrashVolume{}}
	for _, b := range bs {
		if b == job.Home {
			out.Volumes = append(out.Volumes, proto.TrashVolume{Top: b, Kind: proto.TrashHome})
			continue
		}
		v := proto.TrashVolume{Top: b, Kind: proto.TrashNone}
		if r, err := os.OpenRoot(b); err == nil {
			v.Shared = sharedUsable(r, ".")
			if v.Shared {
				// 사용자 폴더(.Trash/<uid>)는 처음 넣을 때 만든다.
				v.Kind = proto.TrashShared
			} else if d, ok := volumeTrash(r, b, b, uid, false); ok {
				v.Kind = d.kind
			} else if top, err := r.Open("."); err == nil {
				// 아직 없지만 만들 수 있다(맨 위에 쓸 수 있다).
				if accessible(top, ".", accessWrite|accessExec) {
					v.Kind = proto.TrashOwn
				}
				top.Close()
			}
			r.Close()
		}
		out.Volumes = append(out.Volumes, v)
	}
	return out, nil
}

// userTrashDirs는 사용자의 휴지통 디렉터리 가운데 지금 있는 것이다: 홈 휴지통, 기준 안 마운트 지점마다 .Trash/<uid>와
// .Trash-<uid>.
func userTrashDirs(job Job, cfg Config) ([]trashDir, error) {
	bs, err := bases(job, cfg)
	if err != nil {
		return nil, err
	}
	mounts, err := allMounts(cfg)
	if err != nil {
		return nil, err
	}
	uid := os.Getuid()
	var out []trashDir
	for _, b := range bs {
		r, err := os.OpenRoot(b)
		if err != nil {
			continue
		}
		home := b == job.Home
		if home {
			if fi, err := r.Lstat(homeTrashRel); err == nil && ownedDir(fi, uid) {
				out = append(out, homeTrash(b))
			}
		}
		for _, top := range topdirsIn(b, mounts, home) {
			topRel := relIn(b, top)
			if sharedUsable(r, topRel) {
				rel := path.Join(topRel, ".Trash", strconv.Itoa(uid))
				if fi, err := r.Lstat(rel); err == nil && ownedDir(fi, uid) {
					out = append(out, trashDir{base: b, rel: rel, top: top, kind: proto.TrashShared})
				}
			}
			rel := path.Join(topRel, ".Trash-"+strconv.Itoa(uid))
			if fi, err := r.Lstat(rel); err == nil && ownedDir(fi, uid) {
				out = append(out, trashDir{base: b, rel: rel, top: top, kind: proto.TrashOwn})
			}
		}
		r.Close()
	}
	return out, nil
}

// findTrashDir는 abs가 사용자의 휴지통 디렉터리인지 확인한다. onmaru가 보낸 경로는 믿지 않고 직접 찾은 목록과 비교한다.
func findTrashDir(job Job, cfg Config, abs string) (trashDir, error) {
	dirs, err := userTrashDirs(job, cfg)
	if err != nil {
		return trashDir{}, err
	}
	for _, d := range dirs {
		if d.abs() == abs {
			return d, nil
		}
	}
	return trashDir{}, proto.NewError(proto.CodeNotFound)
}

// trashList는 사용자의 모든 휴지통 항목이다(fs.trashlist). 짝이 없는 trashinfo나 파일은 뺀다.
func trashList(job Job, cfg Config) (proto.TrashListResult, error) {
	dirs, err := userTrashDirs(job, cfg)
	if err != nil {
		return proto.TrashListResult{}, err
	}
	res := proto.TrashListResult{Items: []proto.TrashItem{}}
	size := 0
	for _, d := range dirs {
		r, err := os.OpenRoot(d.base)
		if err != nil {
			continue
		}
		f, err := r.Open(path.Join(d.rel, "info"))
		if err != nil {
			r.Close()
			continue
		}
		ents, _ := f.ReadDir(-1)
		f.Close()
		for _, e := range ents {
			name, ok := strings.CutSuffix(e.Name(), trashInfoSuffix)
			if !ok || name == "" || !e.Type().IsRegular() {
				continue
			}
			if len(res.Items) >= MaxTrashItems || size >= maxTrashListBytes {
				res.Truncated = true
				break
			}
			data, err := readSmall(r, path.Join(d.rel, "info", e.Name()))
			if err != nil {
				continue
			}
			orig, deleted, ok := parseTrashInfo(data)
			if !ok {
				continue
			}
			fi, err := r.Lstat(path.Join(d.rel, "files", name))
			if err != nil {
				continue
			}
			if !path.IsAbs(orig) {
				orig = path.Join(d.top, orig)
			}
			en := entryOf(name, fi)
			it := proto.TrashItem{Trash: d.abs(), Name: name, Path: orig, Type: en.Type, Size: en.Size, ModTime: en.ModTime}
			if !deleted.IsZero() {
				it.DeletedAt = deleted.Unix()
			}
			if !utf8.ValidString(name) || !utf8.ValidString(orig) || !utf8.ValidString(it.Trash) {
				it.BadName = true
				it.Name, it.Path, it.Trash = strings.ToValidUTF8(name, "�"), strings.ToValidUTF8(orig, "�"),
					strings.ToValidUTF8(it.Trash, "�")
			}
			size += len(it.Trash) + len(it.Name) + len(it.Path) + 120
			res.Items = append(res.Items, it)
		}
		r.Close()
	}
	slices.SortStableFunc(res.Items, func(a, b proto.TrashItem) int {
		if a.DeletedAt != b.DeletedAt {
			if a.DeletedAt > b.DeletedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return res, nil
}

// readSmall은 maxTrashInfo 바이트까지의 일반 파일을 읽는다(FIFO에서 멈추지 않게 O_NONBLOCK).
func readSmall(r *os.Root, rel string) ([]byte, error) {
	f, err := r.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() || fi.Size() > maxTrashInfo {
		return nil, proto.NewError(proto.CodeNotRegular)
	}
	return io.ReadAll(io.LimitReader(f, maxTrashInfo))
}

// trashRestore는 휴지통 항목 하나를 To로 되돌린다(fs.trashrestore). To의 폴더가 없으면 만든다(trash-restore와 같다).
func trashRestore(job Job, cfg Config, p proto.TrashRestoreParams) (proto.Entry, error) {
	if !proto.ValidName(p.Name) {
		return proto.Entry{}, proto.NewError(proto.CodeBadRequest)
	}
	d, err := findTrashDir(job, cfg, p.Trash)
	if err != nil {
		return proto.Entry{}, err
	}
	bs, err := bases(job, cfg)
	if err != nil {
		return proto.Entry{}, err
	}
	to, err := resolve(p.To, bs)
	if err != nil {
		return proto.Entry{}, err
	}
	if to.rel == "." || within(to.path, d.abs()) {
		return proto.Entry{}, proto.NewError(proto.CodeBadRequest)
	}
	if to.base != d.base {
		return proto.Entry{}, proto.NewError(proto.CodeCrossDevice)
	}
	r, err := os.OpenRoot(d.base)
	if err != nil {
		return proto.Entry{}, err
	}
	defer r.Close()
	src := path.Join(d.rel, "files", p.Name)
	if _, err := r.Lstat(src); err != nil {
		return proto.Entry{}, err
	}
	if _, err := r.Lstat(to.rel); err == nil {
		return proto.Entry{}, proto.NewError(proto.CodeExists)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return proto.Entry{}, err
	}
	if dir := path.Dir(to.rel); dir != "." {
		if err := r.MkdirAll(dir, 0o777); err != nil {
			return proto.Entry{}, err
		}
	}
	if err := r.Rename(src, to.rel); err != nil {
		return proto.Entry{}, err
	}
	if err := r.Remove(path.Join(d.rel, "info", p.Name+trashInfoSuffix)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return proto.Entry{}, err
	}
	return statAt(r, to)
}

// trashPurge는 휴지통 항목 하나를 영구 삭제한다(fs.trashpurge).
func trashPurge(job Job, cfg Config, p proto.TrashItemParams) error {
	if !proto.ValidName(p.Name) {
		return proto.NewError(proto.CodeBadRequest)
	}
	d, err := findTrashDir(job, cfg, p.Trash)
	if err != nil {
		return err
	}
	r, err := os.OpenRoot(d.base)
	if err != nil {
		return err
	}
	defer r.Close()
	file := path.Join(d.rel, "files", p.Name)
	info := path.Join(d.rel, "info", p.Name+trashInfoSuffix)
	_, ferr := r.Lstat(file)
	_, ierr := r.Lstat(info)
	if errors.Is(ferr, fs.ErrNotExist) && errors.Is(ierr, fs.ErrNotExist) {
		return proto.NewError(proto.CodeNotFound)
	}
	// os.Root.RemoveAll은 심볼릭 링크를 따라가지 않고 링크 자체만 지운다. 파일을 먼저 지우고 정보를 지운다(중간에
	// 끊기면 trash-list에 남아 다시 지울 수 있다).
	if err := r.RemoveAll(file); err != nil {
		return err
	}
	if err := r.Remove(info); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// trashEmpty는 사용자의 모든 휴지통을 비운다(fs.trashempty). 휴지통 폴더 자신은 둔다.
func trashEmpty(job Job, cfg Config) error {
	dirs, err := userTrashDirs(job, cfg)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		r, err := os.OpenRoot(d.base)
		if err != nil {
			return err
		}
		for _, sub := range []string{"files", "info"} {
			f, err := r.Open(path.Join(d.rel, sub))
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				r.Close()
				return err
			}
			names, _ := f.Readdirnames(-1)
			f.Close()
			for _, n := range names {
				if err := r.RemoveAll(path.Join(d.rel, sub, n)); err != nil {
					r.Close()
					return err
				}
			}
		}
		r.Close()
	}
	return nil
}

// trashPrepare는 데이터 디스크 맨 위에 모든 사용자가 쓰는 휴지통 .Trash(1777)를 만든다(trash.prepare, root 범위).
// worker에는 CAP_DAC_OVERRIDE·CAP_FOWNER가 없어 root도 소유자로서만 할 수 있다. 사용자가 미리 둔 .Trash가
// 심볼릭 링크면 따라가 다른 폴더의 권한을 바꾸게 되므로, 심볼릭 링크·폴더가 아닌 것·root 소유가 아닌 것은 거절하고
// O_NOFOLLOW로 연 fd에 fchmod한다. 이미 있으면 권한만 고친다.
func trashPrepare(job Job, cfg Config) error {
	var p proto.TrashPrepareParams
	if err := proto.DecodeParams(job.Params, &p); err != nil {
		return err
	}
	bs, err := bases(job, cfg)
	if err != nil {
		return err
	}
	prefix := strings.TrimSuffix(cfg.SrvDir, "/") + "/"
	ok := false
	for _, b := range bs {
		if b == p.Disk && b != job.Home && strings.HasPrefix(b, prefix) {
			ok = true
		}
	}
	if !ok {
		return proto.NewError(proto.CodeOutsideRoots)
	}
	r, err := os.OpenRoot(p.Disk)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.Mkdir(".Trash", 0o777); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	if fi, err := r.Lstat(".Trash"); err != nil {
		return err
	} else if !fi.IsDir() || fi.Mode()&fs.ModeSymlink != 0 {
		return proto.NewError(proto.CodeNotDir)
	}
	// 확인한 뒤 바뀌어도 링크를 따라가지 않게 O_NOFOLLOW로 열고 그 fd를 다시 확인한다.
	f, err := r.OpenFile(".Trash", os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.ENOTDIR) {
			return proto.NewError(proto.CodeNotDir)
		}
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if !ownedDir(fi, os.Getuid()) {
		return proto.NewError(proto.CodePermissionDenied)
	}
	return f.Chmod(fs.ModeSticky | 0o777)
}
