package worker

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"syscall"
	"time"

	"onmaru/backend/internal/helper/proto"
)

// fs.copy는 한 폴더 안의 항목을 다른 폴더(다른 기준 디렉터리여도 된다)로 복사한다. 먼저 복사할 파일 수와 크기를
// 세고, 항목마다 대상 폴더의 임시 이름(uploadPrefix…part)에 다 복사한 뒤 이름을 바꾼다. 그래서 실패하거나 취소해도
// 반쯤 복사한 항목이 보이는 이름으로 남지 않는다(임시 이름은 지운다). 진행은 Partial 응답으로 보내며, 받는 쪽이
// 연결을 끊으면 다음 진행 응답을 쓰지 못해 멈춘다. 이미 다 복사한 항목은 남는다.
//
// 권한 비트·소유자·ACL·확장 속성은 복사하지 않는다. 올리기(fs.write)처럼 새로 만든 파일은 0666(원본에 실행 비트가
// 있으면 0777), 폴더는 0777에서 umask와 대상 폴더의 기본 ACL·setgid를 따른다. 수정 시각은 파일과 폴더 모두
// 원본을 따른다. 심볼릭 링크는 따라가지 않고 링크 자체를 만든다.

// copyReadDir는 폴더 항목을 한 번에 읽는 수다.
const copyReadDir = 256

// 테스트가 바꾼다.
var (
	// copyReportEvery는 진행 응답을 보내는 최소 간격이다.
	copyReportEvery = 250 * time.Millisecond
	// copyChunk는 파일 내용을 한 번에 복사하는 양이다. 큰 파일에서도 진행 응답을 보낼 수 있게 나눈다.
	copyChunk int64 = 32 << 20
)

// connError는 진행 응답을 쓰지 못한 오류다(받는 쪽이 끊었다). 복사를 멈추고 결과를 보내지 않는다.
type connError struct{ err error }

func (e connError) Error() string { return "connection: " + e.err.Error() }
func (e connError) Unwrap() error { return e.err }

// copier는 fs.copy 하나의 상태다.
type copier struct {
	c        idleConn
	src, dst *os.Root
	// dir은 원본 폴더의 기준 안 상대 경로다(진행 응답의 Current를 그 안의 상대 경로로 보낸다).
	dir     string
	prog    proto.CopyProgress
	last    time.Time
	skipped int
}

func copyFiles(job Job, cfg Config, c idleConn) error {
	k := &copier{c: c}
	var p proto.CopyParams
	var from, to target
	err := func() error {
		if err := proto.DecodeParams(job.Params, &p); err != nil {
			return err
		}
		if len(p.Names) == 0 || len(p.Names) > proto.MaxCopy {
			return proto.NewError(proto.CodeBadRequest)
		}
		bs, err := bases(job, cfg)
		if err != nil {
			return err
		}
		if from, err = resolve(p.Dir, bs); err != nil {
			return err
		}
		if to, err = resolve(p.To, bs); err != nil {
			return err
		}
		k.dir = from.rel
		for _, n := range p.Names {
			// 폴더를 자기 안으로 복사하면 끝나지 않는다.
			if !proto.ValidName(n) || within(p.To, path.Join(p.Dir, n)) {
				return proto.NewError(proto.CodeBadRequest)
			}
		}
		if k.src, err = os.OpenRoot(from.base); err != nil {
			return err
		}
		if k.dst, err = os.OpenRoot(to.base); err != nil {
			return err
		}
		for _, d := range []struct {
			r   *os.Root
			rel string
		}{{k.src, from.rel}, {k.dst, to.rel}} {
			st, err := d.r.Stat(d.rel)
			if err != nil {
				return err
			}
			if !st.IsDir() {
				return proto.NewError(proto.CodeNotDir)
			}
		}
		return nil
	}()
	if k.src != nil {
		defer k.src.Close()
	}
	if k.dst != nil {
		defer k.dst.Close()
	}
	if err != nil {
		e := toError(err)
		c.respond(proto.Fail(e))
		return e
	}

	// 먼저 세면서 첫 진행 응답을 바로 보낸다. 그 전의 오류만 오류 응답이 된다.
	k.prog.Scanning = true
	if err := k.report(true); err != nil {
		return err
	}
	failed := map[string]string{}
	for _, n := range p.Names {
		// 대상에 같은 이름이 있으면 세지 않는다(복사하지 않을 양이 전체에 들어가지 않게).
		if _, err := k.dst.Lstat(path.Join(to.rel, n)); err == nil {
			failed[n] = proto.CodeExists
			continue
		}
		if err := k.scan(path.Join(from.rel, n)); err != nil {
			var ce connError
			if errors.As(err, &ce) {
				return err
			}
			failed[n] = toError(err).Code
		}
	}
	k.prog.Scanning = false
	if err := k.report(true); err != nil {
		return err
	}

	res := proto.CopyResult{Copied: []string{}}
	// 공간이 없거나 읽기 전용이면 남은 항목도 복사할 수 없다.
	stop := ""
	for _, n := range p.Names {
		code, ok := failed[n]
		if !ok && stop != "" {
			code, ok = stop, true
		}
		if !ok {
			err := k.copyTop(path.Join(from.rel, n), path.Join(to.rel, n))
			if err == nil {
				res.Copied = append(res.Copied, n)
				continue
			}
			var ce connError
			if errors.As(err, &ce) {
				return err
			}
			code = toError(err).Code
			if code == proto.CodeNoSpace || code == proto.CodeReadOnly {
				stop = code
			}
		}
		res.Failed = append(res.Failed, proto.CopyFailure{Name: n, Code: code})
	}
	res.Skipped = k.skipped
	// 마지막 진행(다 복사한 양)을 보낸 뒤 결과를 보낸다.
	k.prog.Current = ""
	if err := k.report(true); err != nil {
		return err
	}
	return reply(c, func() (any, error) { return res, nil })
}

// report는 진행 응답을 보낸다. force가 아니면 copyReportEvery마다 한 번만 보낸다.
func (k *copier) report(force bool) error {
	now := time.Now()
	if !force && now.Sub(k.last) < copyReportEvery {
		return nil
	}
	k.last = now
	b, err := json.Marshal(k.prog)
	if err != nil {
		return err
	}
	if err := k.c.respond(proto.Response{OK: true, Partial: true, Result: b}); err != nil {
		return connError{err}
	}
	return nil
}

// scan은 rel(원본 기준 상대 경로)을 복사할 파일 수와 크기를 센다. 안의 폴더를 읽지 못하면 그 항목 전체가 실패다.
func (k *copier) scan(rel string) error {
	info, err := k.src.Lstat(rel)
	if err != nil {
		return err
	}
	return k.scanEntry(rel, info)
}

func (k *copier) scanEntry(rel string, info fs.FileInfo) error {
	switch m := info.Mode(); {
	case m.IsRegular():
		k.prog.TotalFiles++
		k.prog.TotalBytes += info.Size()
	case m&fs.ModeSymlink != 0:
		k.prog.TotalFiles++
	case m.IsDir():
		if err := k.eachChild(rel, k.scanEntry); err != nil {
			return err
		}
	}
	return k.report(false)
}

// eachChild는 원본 폴더 rel 바로 안의 항목마다 fn을 부른다(Lstat 정보). 읽는 사이에 지워진 항목은 건너뛴다.
func (k *copier) eachChild(rel string, fn func(rel string, info fs.FileInfo) error) error {
	d, err := k.src.OpenFile(rel, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer d.Close()
	for {
		des, err := d.ReadDir(copyReadDir)
		for _, de := range des {
			info, ierr := de.Info()
			if errors.Is(ierr, fs.ErrNotExist) {
				continue
			}
			if ierr != nil {
				return ierr
			}
			if err := fn(path.Join(rel, de.Name()), info); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// copyTop은 맨 위 항목 하나를 대상 폴더의 임시 이름에 복사한 뒤 dst로 옮긴다. 실패하면 임시 이름을 지운다.
func (k *copier) copyTop(src, dst string) error {
	info, err := k.src.Lstat(src)
	if err != nil {
		return err
	}
	if !copyable(info.Mode()) {
		return proto.NewError(proto.CodeNotRegular)
	}
	if _, err := k.dst.Lstat(dst); err == nil {
		return proto.NewError(proto.CodeExists)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	tmp := path.Join(path.Dir(dst), uploadPrefix+randomSuffix()+".part")
	if err := k.copyEntry(src, tmp, info); err != nil {
		k.dst.RemoveAll(tmp)
		return err
	}
	if err := k.commit(tmp, dst, info); err != nil {
		k.dst.RemoveAll(tmp)
		return err
	}
	return nil
}

// commit은 다 복사한 임시 항목을 dst로 옮긴다. 파일은 올리기와 같게 하드 링크로 같은 이름이 생기지 않았는지
// 원자적으로 확인한다. 폴더·링크는 바로 앞에서 다시 확인한 뒤 옮긴다(빈 폴더를 덮어쓰지 않게).
func (k *copier) commit(tmp, dst string, info fs.FileInfo) error {
	if info.Mode().IsRegular() {
		return commit(k.dst, tmp, dst, false)
	}
	if _, err := k.dst.Lstat(dst); err == nil {
		return proto.NewError(proto.CodeExists)
	}
	return k.dst.Rename(tmp, dst)
}

// copyable은 복사하는 종류(일반 파일, 폴더, 심볼릭 링크)인지다.
func copyable(m fs.FileMode) bool {
	return m.IsRegular() || m.IsDir() || m&fs.ModeSymlink != 0
}

// copyEntry는 원본 src를 대상 dst에 만든다. 폴더 안의 특수 파일은 건너뛰고 센다.
func (k *copier) copyEntry(src, dst string, info fs.FileInfo) error {
	k.prog.Current = relIn(k.dir, src)
	switch m := info.Mode(); {
	case m.IsRegular():
		return k.copyFile(src, dst, info)
	case m&fs.ModeSymlink != 0:
		target, err := k.src.Readlink(src)
		if err != nil {
			return err
		}
		if err := k.dst.Symlink(target, dst); err != nil {
			return err
		}
		k.prog.Files++
		return k.report(false)
	case m.IsDir():
		if err := k.dst.Mkdir(dst, 0o777); err != nil {
			return err
		}
		err := k.eachChild(src, func(rel string, ci fs.FileInfo) error {
			if !copyable(ci.Mode()) {
				k.skipped++
				return nil
			}
			return k.copyEntry(rel, path.Join(dst, path.Base(rel)), ci)
		})
		if err != nil {
			return err
		}
		// 안의 항목을 다 만든 뒤에 시각을 맞춘다(항목을 만들면 폴더 시각이 바뀐다).
		_ = k.dst.Chtimes(dst, info.ModTime(), info.ModTime())
		return k.report(false)
	}
	k.skipped++
	return nil
}

func (k *copier) copyFile(src, dst string, info fs.FileInfo) error {
	// O_NONBLOCK: 세는 사이에 FIFO로 바뀌었어도 쓰는 쪽을 기다리며 멈추지 않는다.
	in, err := k.src.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return proto.NewError(proto.CodeNotRegular)
	}
	perm := os.FileMode(0o666)
	if info.Mode()&0o111 != 0 {
		perm = 0o777
	}
	out, err := k.dst.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	for {
		n, err := io.CopyN(out, in, copyChunk)
		k.prog.Bytes += n
		if rerr := k.report(false); rerr != nil {
			out.Close()
			return rerr
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			out.Close()
			return err
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = k.dst.Chtimes(dst, st.ModTime(), st.ModTime())
	k.prog.Files++
	return k.report(false)
}
