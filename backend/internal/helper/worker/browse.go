package worker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"onmaru/backend/internal/helper/proto"
)

// 파일 탐색기가 쓰는 연산: fs.names(이름과 종류만), fs.statmany(이름 여러 개 lstat), fs.info(정보 패널).

const (
	// oPath는 O_PATH다(syscall 패키지에 상수가 없다). 읽기 권한 없이 *at 호출의 기준 fd로만 쓴다.
	oPath = 0x200000
	// faccessat 모드(R_OK, W_OK, X_OK)
	accessRead  = 4
	accessWrite = 2
	accessExec  = 1
	// direntHeader는 linux_dirent64에서 이름 앞까지의 크기(d_ino 8 + d_off 8 + d_reclen 2 + d_type 1)다.
	direntHeader = 19
)

// namesLimit과 namesBytesLimit은 fs.names가 담는 항목 수와 본문 크기의 상한이다(테스트가 줄인다).
var (
	namesLimit      = proto.MaxNames
	namesBytesLimit = proto.MaxNamesBytes
)

// unknownTypeBudget은 fs.names가 종류를 주지 않은 항목(DT_UNKNOWN)을 lstat하는 데 쓰는 최대 시간이다.
// 넘으면 나머지는 NameUnknown으로 두어 연산 시간 제한(30초)을 넘지 않게 한다.
var unknownTypeBudget = 15 * time.Second

var errBadDirent = errors.New("malformed dirent buffer")

// names는 폴더의 이름과 종류를 getdents로 직접 읽는다. os.Root로 연 폴더의 ReadDir는 항목마다 lstat을
// 하므로(os/file_unix.go newUnixDirent) 큰 폴더에서 느리다. 결과는 응답 머리 뒤 본문으로 보낸다.
func names(job Job, cfg Config, c idleConn) error {
	var res proto.NamesResult
	var body []byte
	err := func() error {
		var p proto.PathParams
		if err := proto.DecodeParams(job.Params, &p); err != nil {
			return err
		}
		r, t, err := open(job, cfg, p.Path)
		if err != nil {
			return err
		}
		defer r.Close()
		f, err := r.Open(t.rel)
		if err != nil {
			return err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return err
		}
		if !st.IsDir() {
			return proto.NewError(proto.CodeNotDir)
		}
		res.DirModTimeNs = st.ModTime().UnixNano()
		list, truncated, err := readNames(f)
		if err != nil {
			return err
		}
		res.Truncated = truncated
		res.Writable = accessible(f, ".", accessWrite|accessExec)
		list = resolveUnknown(r, t.rel, list, time.Now().Add(unknownTypeBudget))
		res.Count = len(list)
		for _, e := range list {
			body = proto.AppendName(body, e.Type, e.Name)
		}
		return nil
	}()
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
	resp.Size = int64(len(body))
	if err := c.respond(resp); err != nil {
		return err
	}
	if _, err := c.Write(body); err != nil {
		return toError(err)
	}
	return nil
}

// readNames는 열린 폴더의 항목을 디렉터리 순서대로 읽는다. MaxNames·MaxNamesBytes를 넘으면 그 앞까지만
// 담고 truncated다.
func readNames(f *os.File) (list []proto.NameEntry, truncated bool, err error) {
	rc, err := f.SyscallConn()
	if err != nil {
		return nil, false, err
	}
	size := 0
	buf := make([]byte, 64<<10)
	var readErr error
	cerr := rc.Control(func(fd uintptr) {
		for {
			n, err := syscall.ReadDirent(int(fd), buf)
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if err != nil {
				readErr = err
				return
			}
			if n <= 0 {
				return
			}
			more, err := parseDirents(buf[:n], func(name string, typ byte) bool {
				if len(list) >= namesLimit || size+proto.NameSize(name) > namesBytesLimit {
					truncated = true
					return false
				}
				list = append(list, proto.NameEntry{Type: typ, Name: name})
				size += proto.NameSize(name)
				return true
			})
			if err != nil {
				readErr = err
				return
			}
			if !more {
				return
			}
		}
	})
	if cerr != nil {
		return nil, false, cerr
	}
	return list, truncated, readErr
}

// parseDirents는 getdents64가 채운 linux_dirent64 레코드를 차례로 fn에 넘긴다. ".", ".."와 지워진 항목
// (d_ino 0)은 건너뛴다. fn이 false를 돌려주면 멈추고 false를 돌려준다.
func parseDirents(buf []byte, fn func(name string, typ byte) bool) (bool, error) {
	for len(buf) > 0 {
		if len(buf) < direntHeader+1 {
			return true, errBadDirent
		}
		reclen := int(binary.NativeEndian.Uint16(buf[16:18]))
		if reclen < direntHeader+1 || reclen > len(buf) {
			return true, errBadDirent
		}
		ino := binary.NativeEndian.Uint64(buf[0:8])
		typ := buf[18]
		name := buf[direntHeader:reclen]
		if i := bytes.IndexByte(name, 0); i >= 0 {
			name = name[:i]
		}
		buf = buf[reclen:]
		if ino == 0 || len(name) == 0 || string(name) == "." || string(name) == ".." {
			continue
		}
		if !fn(string(name), direntType(typ)) {
			return false, nil
		}
	}
	return true, nil
}

func direntType(t uint8) byte {
	switch t {
	case syscall.DT_DIR:
		return proto.NameDir
	case syscall.DT_REG:
		return proto.NameFile
	case syscall.DT_LNK:
		return proto.NameSymlink
	case syscall.DT_UNKNOWN:
		return proto.NameUnknown
	default:
		return proto.NameOther
	}
}

func nameType(m fs.FileMode) byte {
	switch typeOf(m) {
	case proto.TypeDir:
		return proto.NameDir
	case proto.TypeFile:
		return proto.NameFile
	case proto.TypeSymlink:
		return proto.NameSymlink
	default:
		return proto.NameOther
	}
}

// resolveUnknown은 파일시스템이 종류를 주지 않은 항목만 lstat해 종류를 채운다. deadline이 지나면 나머지는
// 그대로 둔다. 읽는 사이에 지워진 항목은 뺀다.
func resolveUnknown(r *os.Root, dir string, list []proto.NameEntry, deadline time.Time) []proto.NameEntry {
	out := list[:0]
	for _, e := range list {
		if e.Type == proto.NameUnknown && time.Now().Before(deadline) {
			info, err := r.Lstat(path.Join(dir, e.Name))
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err == nil {
				e.Type = nameType(info.Mode())
			}
		}
		out = append(out, e)
	}
	return out
}

// statMany는 폴더 하나 안의 이름 여러 개를 lstat한다. 이름에는 경로 구분자·".", ".."를 받지 않으므로
// 모두 그 폴더 바로 안이다. 폴더인 항목은 들어갈 수 있는지(R_OK|X_OK)도 본다.
func statMany(job Job, cfg Config) (proto.StatManyResult, error) {
	var p proto.StatManyParams
	if err := proto.DecodeParams(job.Params, &p); err != nil {
		return proto.StatManyResult{}, err
	}
	if len(p.Names) > proto.MaxStatMany {
		return proto.StatManyResult{}, proto.NewError(proto.CodeBadRequest)
	}
	for _, n := range p.Names {
		if !proto.ValidName(n) {
			return proto.StatManyResult{}, proto.NewError(proto.CodeBadRequest)
		}
	}
	r, t, err := open(job, cfg, p.Dir)
	if err != nil {
		return proto.StatManyResult{}, err
	}
	defer r.Close()
	// O_DIRECTORY로 먼저 열어 폴더가 아니면 ENOTDIR(not_dir)로 거절한다(OpenRoot는 오류 번호 없이 알린다).
	dir, err := r.OpenFile(t.rel, oPath|syscall.O_DIRECTORY, 0)
	if err != nil {
		return proto.StatManyResult{}, err
	}
	defer dir.Close()
	// 폴더를 Root로 한 번 열어 이름마다 경로를 처음부터 다시 따라가지 않게 한다(이름마다 fstatat 한 번).
	sub, err := r.OpenRoot(t.rel)
	if err != nil {
		return proto.StatManyResult{}, err
	}
	defer sub.Close()
	items := make([]proto.StatItem, len(p.Names))
	for i, name := range p.Names {
		rel := path.Join(t.rel, name)
		info, err := sub.Lstat(name)
		if err != nil {
			items[i] = proto.StatItem{Entry: proto.Entry{Name: name}, Code: toError(err).Code}
			continue
		}
		e := entryOf(name, info)
		if e.Type == proto.TypeSymlink {
			// 링크 대상은 폴더 밖(같은 기준 안)일 수 있으므로 기준 Root로 따라간다.
			fillLink(r, rel, &e)
		}
		it := proto.StatItem{Entry: e}
		if e.Type == proto.TypeDir {
			// Lstat이 폴더라고 한 항목만 본다. faccessat은 마지막 구성 요소가 심볼릭 링크면 따라간다.
			it.Locked = !accessible(dir, name, accessRead|accessExec)
		}
		items[i] = it
	}
	return proto.StatManyResult{Items: items}, nil
}

// info는 파일 하나의 정보와 소유자·그룹 이름, ACL 유무, 내 권한이다. 심볼릭 링크는 ACL·권한을 읽지 않는다
// (faccessat·getxattr 경로가 링크를 따라가 기준 밖 대상을 볼 수 있다).
func info(job Job, cfg Config, p proto.PathParams) (proto.InfoResult, error) {
	r, t, err := open(job, cfg, p.Path)
	if err != nil {
		return proto.InfoResult{}, err
	}
	defer r.Close()
	fi, err := r.Lstat(t.rel)
	if err != nil {
		return proto.InfoResult{}, err
	}
	e := entryOf(path.Base(t.path), fi)
	if e.Type == proto.TypeSymlink {
		fillLink(r, t.rel, &e)
	}
	res := proto.InfoResult{Entry: e}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		res.UID, res.GID = int(st.Uid), int(st.Gid)
		if u, err := user.LookupId(strconv.Itoa(res.UID)); err == nil {
			res.Owner = u.Username
		}
		if g, err := user.LookupGroupId(strconv.Itoa(res.GID)); err == nil {
			res.Group = g.Name
		}
	}
	if e.Type == proto.TypeSymlink {
		return res, nil
	}
	dirRel, name := path.Dir(t.rel), path.Base(t.rel)
	if t.rel == "." {
		dirRel, name = ".", "."
	}
	dir, err := r.OpenFile(dirRel, oPath|syscall.O_DIRECTORY, 0)
	if err != nil {
		// 부모 폴더를 열지 못하면 권한·ACL 없이 돌려준다.
		return res, nil
	}
	defer dir.Close()
	readMode, writeMode := uint32(accessRead), uint32(accessWrite)
	if e.Type == proto.TypeDir {
		readMode, writeMode = accessRead|accessExec, accessWrite|accessExec
	}
	res.Access = &proto.Access{Read: accessible(dir, name, readMode), Write: accessible(dir, name, writeMode)}
	res.ACL = hasACL(dir, name)
	return res, nil
}

// accessible은 이 프로세스의 실제 uid·gid로 dir 안의 name에 mode 권한이 있는지(faccessat)다.
func accessible(dir *os.File, name string, mode uint32) bool {
	rc, err := dir.SyscallConn()
	if err != nil {
		return false
	}
	ok := false
	rc.Control(func(fd uintptr) {
		ok = syscall.Faccessat(int(fd), name, mode, 0) == nil
	})
	return ok
}

// hasACL은 dir 안의 name에 POSIX ACL(접근 또는 기본)이 있는지다. 부모 fd를 /proc/self/fd로 거쳐 경로를
// 만들고 lgetxattr(마지막 구성 요소를 따라가지 않음)로 본다. 확인하지 못하면 nil이다.
func hasACL(dir *os.File, name string) *bool {
	rc, err := dir.SyscallConn()
	if err != nil {
		return nil
	}
	var res *bool
	rc.Control(func(fd uintptr) {
		p := fmt.Sprintf("/proc/self/fd/%d/%s", fd, name)
		found := false
		for _, attr := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
			switch err := lgetxattrSize(p, attr); {
			case err == nil:
				found = true
			case errors.Is(err, syscall.ENODATA), errors.Is(err, syscall.ENOTSUP):
			default:
				return
			}
		}
		res = &found
	})
	return res
}

// lgetxattrSize는 속성 값의 크기만 묻는다(버퍼 없이). 속성이 있으면 nil이다.
// syscall 패키지에 Lgetxattr 래퍼가 없어 시스템 호출을 직접 부른다.
func lgetxattrSize(p, attr string) error {
	pp, err := syscall.BytePtrFromString(p)
	if err != nil {
		return err
	}
	ap, err := syscall.BytePtrFromString(attr)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(syscall.SYS_LGETXATTR, uintptr(unsafe.Pointer(pp)), uintptr(unsafe.Pointer(ap)), 0, 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
