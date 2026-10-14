// Package storage는 디스크 용량 애드온이다. /proc/mounts의 로컬 파일시스템과 statfs 값을 읽는다.
package storage

import (
	"bufio"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"
)

// Mount는 /proc/mounts의 한 줄이다.
type Mount struct {
	Device     string
	MountPoint string
	Type       string
	Options    []string
}

// ParseMounts는 /proc/mounts(fstab 형식)를 읽는다. 공백 등은 \040 같은 8진수로 이스케이프되어 있다.
func ParseMounts(r io.Reader) ([]Mount, error) {
	var out []Mount
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 4 {
			continue
		}
		out = append(out, Mount{
			Device:     unescape(f[0]),
			MountPoint: unescape(f[1]),
			Type:       f[2],
			Options:    strings.Split(f[3], ","),
		})
	}
	return out, sc.Err()
}

// unescape는 \NNN(8진수 세 자리)을 그 바이트로 바꾼다.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// 장치가 /dev/ 밖이어도 로컬 디스크 공간을 나타내는 파일시스템.
var poolTypes = []string{"fuse.mergerfs", "zfs"}

// 장치가 /dev/ 아래여도 보여 주지 않는 파일시스템(읽기 전용 이미지, 스냅 패키지 등).
var skipTypes = []string{"squashfs", "iso9660", "udf", "erofs"}

// SelectFilesystems는 사용자에게 보여 줄 로컬 파일시스템만 고른다.
//
//   - 장치가 /dev/로 시작하거나 mergerfs·zfs 풀이어야 한다. overlay, tmpfs, proc, 네트워크
//     파일시스템(nfs, cifs)은 뺀다. 네트워크 파일시스템은 statfs가 오래 걸릴 수 있다
//   - zfs는 최상위 데이터셋만(같은 풀 공간을 나눠 쓰므로)
//   - 같은 장치가 여러 곳에 마운트되어 있으면(bind mount, 컨테이너) 처음 나온 것 하나만.
//     /proc/mounts는 마운트한 순서이므로 원래 마운트가 bind mount보다 앞에 있다
//
// 결과는 마운트 경로 순서다.
func SelectFilesystems(mounts []Mount) []Mount {
	byDevice := make(map[string]Mount)
	for _, m := range mounts {
		if slices.Contains(skipTypes, m.Type) || !strings.HasPrefix(m.MountPoint, "/") {
			continue
		}
		switch {
		case m.Type == "zfs":
			if strings.Contains(m.Device, "/") {
				continue
			}
		case slices.Contains(poolTypes, m.Type):
		case strings.HasPrefix(m.Device, "/dev/"):
		default:
			continue
		}
		key := m.Type + "\x00" + m.Device
		if _, ok := byDevice[key]; ok {
			continue
		}
		byDevice[key] = m
	}
	out := make([]Mount, 0, len(byDevice))
	for _, m := range byDevice {
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b Mount) int { return strings.Compare(a.MountPoint, b.MountPoint) })
	return out
}

// decodeLabel은 udev가 /dev/disk/by-label 이름에 쓰는 \xHH 이스케이프를 푼다.
func decodeLabel(name string) string {
	if !strings.Contains(name, `\x`) {
		return name
	}
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if name[i] == '\\' && i+3 < len(name) && name[i+1] == 'x' {
			if n, err := strconv.ParseUint(name[i+2:i+4], 16, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(name[i])
	}
	return b.String()
}

// deviceName은 /dev/ 아래 링크 대상(../../sda1 등)이나 장치 경로에서 커널 장치 이름을 얻는다.
func deviceName(p string) string { return path.Base(p) }
