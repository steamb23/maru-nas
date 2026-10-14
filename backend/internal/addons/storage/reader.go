package storage

import (
	"io/fs"
	"log"
	"os"
	"slices"
	"strings"
	"syscall"

	"onmaru/backend/internal/api"
)

// Usage는 statfs 결과(바이트)다.
type Usage struct {
	Total uint64
	// Free는 root 예약분을 포함한 남은 공간, Avail은 일반 사용자가 쓸 수 있는 남은 공간이다.
	Free  uint64
	Avail uint64
}

// StatfsFunc는 마운트 경로의 용량을 읽는다. 테스트에서 합성 값으로 바꿔 끼운다.
type StatfsFunc func(path string) (Usage, error)

// Statfs는 statfs(2)로 용량을 읽는다. 마운트 경로를 볼 권한만 있으면 root 없이 된다.
func Statfs(path string) (Usage, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Usage{}, err
	}
	bs := uint64(st.Bsize)
	return Usage{Total: st.Blocks * bs, Free: st.Bfree * bs, Avail: st.Bavail * bs}, nil
}

// Reader는 마운트 목록과 용량을 읽는 provider다(이름 proc).
type Reader struct {
	proc   fs.FS
	dev    fs.FS
	statfs StatfsFunc
}

// NewReader는 호스트의 /proc, /dev를 읽는 Reader를 반환한다.
func NewReader() *Reader {
	return NewReaderFS(os.DirFS("/proc"), os.DirFS("/dev"), Statfs)
}

// NewReaderFS는 proc의 "mounts", dev의 "disk/by-label/*"(장치로의 심볼릭 링크)를 읽는 Reader를 반환한다.
func NewReaderFS(proc, dev fs.FS, statfs StatfsFunc) *Reader {
	return &Reader{proc: proc, dev: dev, statfs: statfs}
}

// Storage는 로컬 파일시스템의 용량을 반환한다. 용량을 읽지 못한 파일시스템은 뺀다.
func (r *Reader) Storage() (api.StorageResponse, error) {
	f, err := r.proc.Open("mounts")
	if err != nil {
		return api.StorageResponse{}, err
	}
	mounts, err := ParseMounts(f)
	f.Close()
	if err != nil {
		return api.StorageResponse{}, err
	}
	labels := r.labels()
	res := api.StorageResponse{
		AddonSources: api.AddonSources{Sources: []string{r.Name()}},
		Filesystems:  []api.Filesystem{},
	}
	for _, m := range SelectFilesystems(mounts) {
		u, err := r.statfs(m.MountPoint)
		if err != nil {
			log.Printf("storage: statfs %s: %v", m.MountPoint, err)
			continue
		}
		res.Filesystems = append(res.Filesystems, api.Filesystem{
			Device:         m.Device,
			Label:          labels[r.kernelName(m.Device)],
			MountPoint:     m.MountPoint,
			Type:           m.Type,
			TotalBytes:     u.Total,
			UsedBytes:      u.Total - min(u.Free, u.Total),
			AvailableBytes: u.Avail,
			ReadOnly:       slices.Contains(m.Options, "ro"),
		})
	}
	return res, nil
}

// labels는 커널 장치 이름(sda1 등) → 파일시스템 라벨이다. 읽지 못하면 빈 표다.
func (r *Reader) labels() map[string]string {
	out := make(map[string]string)
	entries, err := fs.ReadDir(r.dev, "disk/by-label")
	if err != nil {
		return out
	}
	for _, e := range entries {
		target, err := fs.ReadLink(r.dev, "disk/by-label/"+e.Name())
		if err != nil {
			continue
		}
		out[deviceName(target)] = decodeLabel(e.Name())
	}
	return out
}

// kernelName은 /dev/ 아래 장치 경로의 커널 장치 이름이다. /dev/mapper/x 같은 링크는 따라간다.
func (r *Reader) kernelName(dev string) string {
	rel, ok := strings.CutPrefix(dev, "/dev/")
	if !ok {
		return ""
	}
	if target, err := fs.ReadLink(r.dev, rel); err == nil {
		return deviceName(target)
	}
	return deviceName(dev)
}
