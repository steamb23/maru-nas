package worker

import (
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"syscall"
	"time"

	"maru/backend/internal/helper/proto"
)

const (
	// SSHKeyMarker는 maru가 넣은 키 줄의 주석이다. 이 주석과 아래 옵션을 가진 줄만 고치거나 지운다.
	SSHKeyMarker = "maru-ephemeral"
	// SSHKeyTTL은 키 줄의 수명이다. 터미널 접속의 인증 한 번에만 쓰므로 짧게 둔다. maru가 지우지 못해도
	// 이 시간이 지나면 sshd가 받지 않고, 다음 ssh.authorize·ssh.revoke가 지운다.
	SSHKeyTTL = 2 * time.Minute
	// sshKeyOptions는 키 줄의 옵션이다. restrict로 포트·에이전트·X11 전달과 ~/.ssh/rc를 막고 pty만 다시
	// 허용하며, 루프백에서 온 접속만 받는다(maru는 같은 호스트의 sshd에 붙는다).
	sshKeyOptions = `restrict,pty,from="127.0.0.1,::1"`
	// sshExpiryLayout은 expiry-time 옵션의 시각 형식이다(OpenSSH 8.2+, 끝의 Z는 UTC).
	sshExpiryLayout = "20060102150405"
	// maxAuthorizedKeys는 읽는 authorized_keys의 최대 크기다.
	maxAuthorizedKeys = 1 << 20
	sshDir            = ".ssh"
	authorizedKeys    = ".ssh/authorized_keys"
	keysTmpPrefix     = ".maru-keys-"
)

// sshKey는 ssh.authorize·ssh.revoke를 실행한다. 키 줄은 worker가 직접 만들고 maru에게서는 공개 키만 받는다.
func sshKey(job Job, cfg Config, c idleConn) error {
	return reply(c, func() (any, error) {
		var p proto.SSHKeyParams
		if err := proto.DecodeParams(job.Params, &p); err != nil {
			return nil, err
		}
		blob, err := proto.ParseSSHPublicKey(p.PublicKey)
		if err != nil {
			return nil, err
		}
		return nil, editAuthorizedKeys(job, cfg.Now(), blob, job.Action == proto.ActionSSHAuthorize)
	})
}

// sshKeyLine은 blob을 now부터 SSHKeyTTL 동안만 받는 키 줄이다.
func sshKeyLine(blob []byte, now time.Time) string {
	expiry := now.Add(SSHKeyTTL).UTC().Format(sshExpiryLayout) + "Z"
	return sshKeyOptions + `,expiry-time="` + expiry + `" ` + proto.SSHKeyType + " " + base64.StdEncoding.EncodeToString(blob) + " " + SSHKeyMarker
}

// maruKey는 line이 maru가 넣은 키 줄이면 그 키의 base64와 만료 시각을 돌려준다.
// 만료 시각을 읽지 못하면 이미 만료된 것으로 본다.
func maruKey(line string) (key string, expiry time.Time, ok bool) {
	f := strings.Fields(line)
	if len(f) != 4 || f[1] != proto.SSHKeyType || f[3] != SSHKeyMarker || !strings.HasPrefix(f[0], sshKeyOptions+",") {
		return "", time.Time{}, false
	}
	opt := strings.TrimPrefix(f[0], sshKeyOptions+",")
	v, found := strings.CutPrefix(opt, `expiry-time="`)
	if found {
		v, found = strings.CutSuffix(v, `Z"`)
	}
	if found {
		if t, err := time.Parse(sshExpiryLayout, v); err == nil {
			expiry = t
		}
	}
	return f[2], expiry, true
}

// editAuthorizedKeys는 사용자 홈의 ~/.ssh/authorized_keys에서 만료된 maru 키 줄과 blob의 maru 키 줄을
// 지우고, add면 blob의 새 키 줄을 더한다. 다른 줄은 바이트 그대로 둔다.
//
// sshd(StrictModes)는 홈·~/.ssh·파일이 사용자나 root 소유이고 그룹·다른 사용자 쓰기가 없어야 파일을 읽는다.
// 홈이 이를 어기면 고치지 않고 ssh_strict_modes로 거절하고(공유 폴더 위 홈의 권한은 사용자가 정한다),
// ~/.ssh와 파일은 사용자 소유일 때 그룹·다른 사용자 쓰기만 뺀다. worker의 umask가 0002라 권한을 직접 정한다.
func editAuthorizedKeys(job Job, now time.Time, blob []byte, add bool) error {
	if !usableBase(job.Home) {
		return proto.NewError(proto.CodeNoHome)
	}
	// os.OpenRoot는 디렉터리가 아닐 때 ENOTDIR이 아닌 오류 문구를 주므로 먼저 확인한다.
	if st, err := os.Stat(job.Home); errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || (err == nil && !st.IsDir()) {
		return proto.NewError(proto.CodeNoHome)
	}
	r, err := os.OpenRoot(job.Home)
	if err != nil {
		return err
	}
	defer r.Close()
	uid := os.Getuid()
	if err := strictModes(r, ".", uid, false); err != nil {
		return err
	}
	if add {
		if err := r.Mkdir(sshDir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	} else if _, err := r.Stat(sshDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := strictModes(r, sshDir, uid, true); err != nil {
		return err
	}
	// 같은 사용자의 터미널을 동시에 여는 경우(다른 worker)와 겹쳐 쓰지 않게 ~/.ssh를 잠근다.
	dir, err := r.Open(sshDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}

	old, mode, err := readAuthorizedKeys(r, uid)
	if err != nil {
		return err
	}
	if old == "" && !add {
		return nil
	}
	var b strings.Builder
	for _, piece := range strings.SplitAfter(old, "\n") {
		if key, expiry, ok := maruKey(strings.TrimRight(piece, "\r\n")); ok {
			if !now.Before(expiry) || proto.SameSSHKey(key, blob) {
				continue
			}
		}
		b.WriteString(piece)
	}
	if add {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
		b.WriteString(sshKeyLine(blob, now) + "\n")
	}
	if b.String() == old {
		return nil
	}
	return replaceAuthorizedKeys(r, b.String(), mode)
}

// strictModes는 name이 sshd의 StrictModes 검사를 통과하는지 확인한다. fix면 사용자 소유일 때
// 그룹·다른 사용자 쓰기 비트를 빼서 통과시킨다.
func strictModes(r *os.Root, name string, uid int, fix bool) error {
	st, err := r.Stat(name)
	if err != nil {
		if name == "." && errors.Is(err, fs.ErrNotExist) {
			return proto.NewError(proto.CodeNoHome)
		}
		return err
	}
	if !st.IsDir() {
		if name == "." {
			return proto.NewError(proto.CodeNoHome)
		}
		return proto.NewError(proto.CodeNotDir)
	}
	owner := ownerOf(st)
	if owner != uid && owner != 0 {
		return proto.NewError(proto.CodeSSHStrictModes)
	}
	if st.Mode().Perm()&0o022 == 0 {
		return nil
	}
	if !fix || owner != uid {
		return proto.NewError(proto.CodeSSHStrictModes)
	}
	return r.Chmod(name, st.Mode().Perm()&^0o022)
}

// readAuthorizedKeys는 authorized_keys의 내용과 새 파일에 줄 권한을 읽는다. 없으면 빈 내용과 0600이다.
// 일반 파일이 아니면(심볼릭 링크 등) 사용자가 꾸민 구성을 바꾸지 않도록 거절한다.
func readAuthorizedKeys(r *os.Root, uid int) (string, fs.FileMode, error) {
	st, err := r.Lstat(authorizedKeys)
	if errors.Is(err, fs.ErrNotExist) {
		return "", 0o600, nil
	}
	if err != nil {
		return "", 0, err
	}
	if !st.Mode().IsRegular() {
		return "", 0, proto.NewError(proto.CodeNotRegular)
	}
	if owner := ownerOf(st); owner != uid && owner != 0 {
		return "", 0, proto.NewError(proto.CodeSSHStrictModes)
	}
	f, err := r.OpenFile(authorizedKeys, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxAuthorizedKeys+1))
	if err != nil {
		return "", 0, err
	}
	if len(b) > maxAuthorizedKeys {
		return "", 0, proto.NewError(proto.CodeBadRequest)
	}
	return string(b), st.Mode().Perm() &^ 0o022, nil
}

// replaceAuthorizedKeys는 같은 폴더의 임시 파일에 content를 다 쓴 뒤 이름을 바꿔, 실패해도 기존 파일이
// 반쯤 쓰인 채 남지 않게 한다.
func replaceAuthorizedKeys(r *os.Root, content string, mode fs.FileMode) error {
	tmp := sshDir + "/" + keysTmpPrefix + randomSuffix()
	f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	err = func() error {
		if _, err := f.WriteString(content); err != nil {
			f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		if err := r.Chmod(tmp, mode); err != nil {
			return err
		}
		return r.Rename(tmp, authorizedKeys)
	}()
	if err != nil {
		r.Remove(tmp)
	}
	return err
}

func ownerOf(st fs.FileInfo) int {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return int(s.Uid)
	}
	return -1
}
