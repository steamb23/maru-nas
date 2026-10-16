package worker

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"syscall"

	"onmaru/backend/internal/helper/proto"
)

const (
	// maxPath는 받는 경로의 최대 길이(PATH_MAX)다.
	maxPath = 4096
	// MaxList는 fs.list가 담는 최대 항목 수다. 넘치면 Truncated다.
	MaxList = 10000
	// uploadPrefix는 올리는 중인 임시 파일 이름의 접두사다.
	uploadPrefix = ".onmaru-upload-"
)

// bases는 사용자 범위 파일 연산이 닿을 수 있는 기준 디렉터리다: 사용자 홈 디렉터리와
// SrvDir 아래에 마운트된 파일시스템(OMV 데이터 디스크·mergerfs 등). SrvDir 자체와 그 안의
// 루트 파일시스템 디렉터리(/srv/salt, /srv/pillar)는 넣지 않는다.
// 실제로 읽고 쓸 수 있는지는 worker가 가진 사용자 권한으로 커널이 정한다.
func bases(job Job, cfg Config) ([]string, error) {
	var out []string
	if usableBase(job.Home) {
		out = append(out, job.Home)
	}
	rc, err := cfg.MountInfo()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	mounts, err := ParseMountInfo(rc)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(cfg.SrvDir, "/") + "/"
	for _, m := range mounts {
		if strings.HasPrefix(m, prefix) && usableBase(m) && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out, nil
}

// usableBase는 기준 디렉터리로 쓸 수 있는 경로인지다. 루트(/)는 쓰지 않는다.
func usableBase(p string) bool {
	return validPath(p) && p != "/"
}

// validPath는 받은 경로가 절대 경로이고 정리되어 있는지다(.., 겹친 /, 끝의 / 없음).
func validPath(p string) bool {
	return len(p) <= maxPath && strings.HasPrefix(p, "/") && !strings.ContainsRune(p, 0) && path.Clean(p) == p
}

// target은 경로를 가장 긴 기준 디렉터리와 그 안의 상대 경로로 나눈 것이다.
type target struct {
	base string
	// rel은 base 안의 상대 경로다. base 자신이면 "."이다.
	rel string
	// path는 받은 절대 경로다.
	path string
}

func resolve(p string, bs []string) (target, error) {
	if !validPath(p) {
		return target{}, proto.NewError(proto.CodeBadRequest)
	}
	best := ""
	for _, b := range bs {
		if (p == b || strings.HasPrefix(p, b+"/")) && len(b) > len(best) {
			best = b
		}
	}
	if best == "" {
		return target{}, proto.NewError(proto.CodeOutsideRoots)
	}
	rel := "."
	if p != best {
		rel = p[len(best)+1:]
	}
	return target{base: best, rel: rel, path: p}, nil
}

// open은 경로의 기준 디렉터리를 os.Root로 연다. os.Root는 ..와 심볼릭 링크로 기준 밖에 나가는 것을 막는다.
func open(job Job, cfg Config, p string) (*os.Root, target, error) {
	bs, err := bases(job, cfg)
	if err != nil {
		return nil, target{}, err
	}
	t, err := resolve(p, bs)
	if err != nil {
		return nil, target{}, err
	}
	r, err := os.OpenRoot(t.base)
	if err != nil {
		return nil, target{}, err
	}
	return r, t, nil
}

func list(job Job, cfg Config, p proto.PathParams) (proto.ListResult, error) {
	r, t, err := open(job, cfg, p.Path)
	if err != nil {
		return proto.ListResult{}, err
	}
	defer r.Close()
	f, err := r.Open(t.rel)
	if err != nil {
		return proto.ListResult{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return proto.ListResult{}, err
	}
	if !st.IsDir() {
		return proto.ListResult{}, proto.NewError(proto.CodeNotDir)
	}
	res := proto.ListResult{Entries: []proto.Entry{}}
	for len(res.Entries) < MaxList {
		des, err := f.ReadDir(min(256, MaxList-len(res.Entries)))
		for _, de := range des {
			info, err := de.Info()
			if err != nil {
				// 읽는 사이에 지워진 항목은 건너뛴다.
				continue
			}
			e := entryOf(de.Name(), info)
			if e.Type == proto.TypeSymlink {
				fillLink(r, path.Join(t.rel, de.Name()), &e)
			}
			res.Entries = append(res.Entries, e)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return proto.ListResult{}, err
		}
	}
	if len(res.Entries) >= MaxList {
		// 다음 항목이 있는지 하나 더 읽어 본다.
		if more, _ := f.ReadDir(1); len(more) > 0 {
			res.Truncated = true
		}
	}
	slices.SortFunc(res.Entries, func(a, b proto.Entry) int { return strings.Compare(a.Name, b.Name) })
	return res, nil
}

func stat(job Job, cfg Config, p proto.PathParams) (proto.Entry, error) {
	r, t, err := open(job, cfg, p.Path)
	if err != nil {
		return proto.Entry{}, err
	}
	defer r.Close()
	return statAt(r, t)
}

func statAt(r *os.Root, t target) (proto.Entry, error) {
	info, err := r.Lstat(t.rel)
	if err != nil {
		return proto.Entry{}, err
	}
	e := entryOf(path.Base(t.path), info)
	if e.Type == proto.TypeSymlink {
		fillLink(r, t.rel, &e)
	}
	return e, nil
}

func mkdir(job Job, cfg Config, p proto.PathParams) (proto.Entry, error) {
	r, t, err := open(job, cfg, p.Path)
	if err != nil {
		return proto.Entry{}, err
	}
	defer r.Close()
	if t.rel == "." {
		return proto.Entry{}, proto.NewError(proto.CodeExists)
	}
	if err := r.Mkdir(t.rel, 0o777); err != nil {
		return proto.Entry{}, err
	}
	return statAt(r, t)
}

func read(job Job, cfg Config, c idleConn) error {
	var p proto.ReadParams
	var r *os.Root
	var f *os.File
	var n int64
	var res proto.ReadResult
	err := func() error {
		if err := proto.DecodeParams(job.Params, &p); err != nil {
			return err
		}
		if p.Offset < 0 || p.Length < 0 {
			return proto.NewError(proto.CodeBadRequest)
		}
		var t target
		var err error
		if r, t, err = open(job, cfg, p.Path); err != nil {
			return err
		}
		// O_NONBLOCK: FIFO를 열 때 쓰는 쪽을 기다리며 멈추지 않게 한다(일반 파일에는 영향 없음).
		if f, err = r.OpenFile(t.rel, os.O_RDONLY|syscall.O_NONBLOCK, 0); err != nil {
			return err
		}
		st, err := f.Stat()
		if err != nil {
			return err
		}
		if st.IsDir() {
			return proto.NewError(proto.CodeIsDir)
		}
		if !st.Mode().IsRegular() {
			return proto.NewError(proto.CodeNotRegular)
		}
		size := st.Size()
		off := min(p.Offset, size)
		n = size - off
		if p.Length > 0 && p.Length < n {
			n = p.Length
		}
		if _, err := f.Seek(off, io.SeekStart); err != nil {
			return err
		}
		res = proto.ReadResult{FileSize: size, ModTime: st.ModTime().Unix()}
		return nil
	}()
	if r != nil {
		defer r.Close()
	}
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		e := toError(err)
		c.respond(proto.Fail(e))
		return e
	}
	resp, err := proto.Success(res)
	if err != nil {
		c.respond(proto.Fail(err))
		return err
	}
	resp.Size = n
	if err := c.respond(resp); err != nil {
		return err
	}
	// 읽는 도중 파일이 줄어들면 n보다 적게 보내고 연결이 끊긴다. 받는 쪽은 Size와 비교해 알 수 있다.
	if _, err := io.CopyN(c, f, n); err != nil {
		return toError(err)
	}
	return nil
}

func write(job Job, cfg Config, c idleConn) error {
	var p proto.WriteParams
	e, err := func() (proto.Entry, error) {
		if err := proto.DecodeParams(job.Params, &p); err != nil {
			return proto.Entry{}, err
		}
		if job.Size < 0 {
			return proto.Entry{}, proto.NewError(proto.CodeBadRequest)
		}
		r, t, err := open(job, cfg, p.Path)
		if err != nil {
			return proto.Entry{}, err
		}
		defer r.Close()
		if t.rel == "." || strings.HasPrefix(path.Base(t.rel), uploadPrefix) {
			return proto.Entry{}, proto.NewError(proto.CodeBadRequest)
		}
		old, err := r.Lstat(t.rel)
		switch {
		case err == nil && !p.Overwrite:
			return proto.Entry{}, proto.NewError(proto.CodeExists)
		case err == nil && old.IsDir():
			return proto.Entry{}, proto.NewError(proto.CodeIsDir)
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return proto.Entry{}, err
		}
		existed := err == nil
		// 같은 폴더의 임시 파일에 다 받은 뒤 이름을 바꿔, 실패해도 기존 파일이 반쯤 쓰인 채 남지 않게 한다.
		tmp := path.Join(path.Dir(t.rel), uploadPrefix+randomSuffix()+".part")
		f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err != nil {
			return proto.Entry{}, err
		}
		if _, err := io.CopyN(f, c, job.Size); err != nil {
			f.Close()
			r.Remove(tmp)
			if errors.Is(err, io.EOF) {
				return proto.Entry{}, proto.NewError(proto.CodeBadRequest)
			}
			return proto.Entry{}, err
		}
		if err := f.Close(); err != nil {
			r.Remove(tmp)
			return proto.Entry{}, err
		}
		if err := commit(r, tmp, t.rel, p.Overwrite); err != nil {
			r.Remove(tmp)
			return proto.Entry{}, err
		}
		if existed && old.Mode().IsRegular() {
			// 덮어쓴 파일의 권한 비트를 되도록 유지한다(소유자가 아니면 바꾸지 못한다).
			_ = r.Chmod(t.rel, old.Mode().Perm())
		}
		return statAt(r, t)
	}()
	if err != nil {
		pe := toError(err)
		c.respond(proto.Fail(pe))
		return pe
	}
	return reply(c, func() (any, error) { return e, nil })
}

// commit은 다 받은 임시 파일을 대상 이름으로 옮긴다. 덮어쓰지 않을 때는 하드 링크로 같은 이름이
// 생기지 않았는지 원자적으로 확인하고, 하드 링크를 쓸 수 없는 파일시스템이면 다시 확인한 뒤 옮긴다.
func commit(r *os.Root, tmp, dst string, overwrite bool) error {
	if overwrite {
		return r.Rename(tmp, dst)
	}
	err := r.Link(tmp, dst)
	if err == nil {
		return r.Remove(tmp)
	}
	if errors.Is(err, fs.ErrExist) {
		return proto.NewError(proto.CodeExists)
	}
	if _, err := r.Lstat(dst); err == nil {
		return proto.NewError(proto.CodeExists)
	}
	return r.Rename(tmp, dst)
}

func rename(job Job, cfg Config, c idleConn) error {
	return reply(c, func() (any, error) {
		var p proto.RenameParams
		if err := proto.DecodeParams(job.Params, &p); err != nil {
			return nil, err
		}
		bs, err := bases(job, cfg)
		if err != nil {
			return nil, err
		}
		from, err := resolve(p.From, bs)
		if err != nil {
			return nil, err
		}
		to, err := resolve(p.To, bs)
		if err != nil {
			return nil, err
		}
		// 기준 디렉터리 자체는 옮기지 않고, 기준 디렉터리 사이로도 옮기지 않는다.
		if from.base != to.base || from.rel == "." || to.rel == "." {
			return nil, proto.NewError(proto.CodeBadRequest)
		}
		r, err := os.OpenRoot(from.base)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		if _, err := r.Lstat(to.rel); err == nil {
			return nil, proto.NewError(proto.CodeExists)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err := r.Rename(from.rel, to.rel); err != nil {
			return nil, err
		}
		return statAt(r, to)
	})
}

func remove(job Job, cfg Config, c idleConn) error {
	return reply(c, func() (any, error) {
		var p proto.RemoveParams
		if err := proto.DecodeParams(job.Params, &p); err != nil {
			return nil, err
		}
		r, t, err := open(job, cfg, p.Path)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		if t.rel == "." {
			return nil, proto.NewError(proto.CodeBadRequest)
		}
		if _, err := r.Lstat(t.rel); err != nil {
			return nil, err
		}
		if p.Recursive {
			// os.Root.RemoveAll은 심볼릭 링크를 따라가지 않고 링크 자체만 지운다.
			err = r.RemoveAll(t.rel)
		} else {
			err = r.Remove(t.rel)
		}
		return nil, err
	})
}

// entryOf는 Lstat 결과를 Entry로 바꾼다.
func entryOf(name string, info fs.FileInfo) proto.Entry {
	m := info.Mode()
	e := proto.Entry{Name: name, Size: info.Size(), ModTime: info.ModTime().Unix(), Mode: uint32(m.Perm())}
	if m&fs.ModeSetuid != 0 {
		e.Mode |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		e.Mode |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		e.Mode |= 0o1000
	}
	e.Type = typeOf(m)
	if e.Type != proto.TypeFile {
		e.Size = 0
	}
	return e
}

func typeOf(m fs.FileMode) string {
	switch {
	case m.IsRegular():
		return proto.TypeFile
	case m.IsDir():
		return proto.TypeDir
	case m&fs.ModeSymlink != 0:
		return proto.TypeSymlink
	default:
		return proto.TypeOther
	}
}

// fillLink는 심볼릭 링크의 대상과, 기준 안에서 따라간 대상의 종류를 채운다.
func fillLink(r *os.Root, rel string, e *proto.Entry) {
	if target, err := r.Readlink(rel); err == nil {
		e.Target = target
	}
	if st, err := r.Stat(rel); err == nil {
		e.LinkType = typeOf(st.Mode())
	}
}

func randomSuffix() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
