package storage

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"maru/backend/internal/api"
)

func readMounts(t *testing.T) []Mount {
	t.Helper()
	f, err := os.Open("testdata/mounts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m, err := ParseMounts(f)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestParseMountsUnescapes(t *testing.T) {
	m, err := ParseMounts(strings.NewReader("/dev/sdx1 /a\\040b\\011c ext4 rw 0 0\nshort line\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m[0].MountPoint != "/a b\tc" || m[0].Options[0] != "rw" {
		t.Fatalf("got %+v", m)
	}
	// 잘린 이스케이프는 그대로 둔다.
	if got := unescape(`/x\04`); got != `/x\04` {
		t.Errorf("unescape = %q", got)
	}
}

func TestSelectFilesystems(t *testing.T) {
	got := SelectFilesystems(readMounts(t))
	var points []string
	for _, m := range got {
		points = append(points, m.MountPoint)
	}
	want := []string{
		"/",
		"/boot/efi",
		// /dev/sda1은 처음 나온 마운트(뒤의 /export/media 등은 bind mount)
		"/srv/dev-disk-by-uuid-00000000-0000-4000-8000-000000000001",
		"/srv/dev-disk-by-uuid-00000000-0000-4000-8000-000000000002",
		"/srv/mergerfs/pool",
		"/srv/my vault",
		"/tank",
	}
	if strings.Join(points, "|") != strings.Join(want, "|") {
		t.Errorf("got %q\nwant %q", points, want)
	}
}

func TestDecodeLabel(t *testing.T) {
	if got := decodeLabel(`my\x20data`); got != "my data" {
		t.Errorf("got %q", got)
	}
	if got := decodeLabel(`odd\x2`); got != `odd\x2` {
		t.Errorf("got %q", got)
	}
}

func TestStorage(t *testing.T) {
	mounts, _ := os.ReadFile("testdata/mounts")
	proc := fstest.MapFS{"mounts": {Data: mounts}}
	dev := fstest.MapFS{
		"disk/by-label/system":     {Data: []byte("../../nvme0n1p2"), Mode: os.ModeSymlink},
		`disk/by-label/my\x20data`: {Data: []byte("../../sda1"), Mode: os.ModeSymlink},
		"disk/by-label/vault":      {Data: []byte("../../dm-0"), Mode: os.ModeSymlink},
		"mapper/vault":             {Data: []byte("../dm-0"), Mode: os.ModeSymlink},
	}
	const gib = 1 << 30
	statfs := func(p string) (Usage, error) {
		switch p {
		case "/boot/efi":
			return Usage{}, errors.New("permission denied")
		case "/":
			return Usage{Total: 100 * gib, Free: 60 * gib, Avail: 55 * gib}, nil
		}
		return Usage{Total: 10 * gib, Free: 4 * gib, Avail: 4 * gib}, nil
	}
	res, err := NewReaderFS(proc, dev, statfs).Storage()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sources) != 1 || res.Sources[0] != "proc" {
		t.Errorf("sources %v", res.Sources)
	}
	byPoint := map[string]api.Filesystem{}
	for _, f := range res.Filesystems {
		byPoint[f.MountPoint] = f
	}
	if _, ok := byPoint["/boot/efi"]; ok {
		t.Error("filesystem with failed statfs included")
	}
	root := byPoint["/"]
	if root.Label != "system" || root.TotalBytes != 100*gib || root.UsedBytes != 40*gib || root.AvailableBytes != 55*gib {
		t.Errorf("root %+v", root)
	}
	if got := byPoint["/srv/dev-disk-by-uuid-00000000-0000-4000-8000-000000000001"].Label; got != "my data" {
		t.Errorf("sda1 label %q", got)
	}
	if got := byPoint["/srv/my vault"].Label; got != "vault" {
		t.Errorf("mapper label %q", got)
	}
	if !byPoint["/srv/dev-disk-by-uuid-00000000-0000-4000-8000-000000000002"].ReadOnly {
		t.Error("ro not detected")
	}
	if len(res.Filesystems) != 6 {
		t.Errorf("got %d filesystems", len(res.Filesystems))
	}
}

func TestStorageWithoutMounts(t *testing.T) {
	if _, err := NewReaderFS(fstest.MapFS{}, fstest.MapFS{}, Statfs).Storage(); err == nil {
		t.Error("want error")
	}
}

func TestMountsUnder(t *testing.T) {
	mounts, _ := os.ReadFile("testdata/mounts")
	proc := fstest.MapFS{"mounts": {Data: mounts}}
	dev := fstest.MapFS{
		`disk/by-label/my\x20data`: {Data: []byte("../../sda1"), Mode: os.ModeSymlink},
		"disk/by-label/vault":      {Data: []byte("../../dm-0"), Mode: os.ModeSymlink},
		"mapper/vault":             {Data: []byte("../dm-0"), Mode: os.ModeSymlink},
	}
	got, err := NewReaderFS(proc, dev, Statfs).MountsUnder("/srv")
	if err != nil {
		t.Fatal(err)
	}
	want := []MountLabel{
		{MountPoint: "/srv/dev-disk-by-uuid-00000000-0000-4000-8000-000000000001", Label: "my data", Device: "/dev/sda1"},
		{MountPoint: "/srv/dev-disk-by-uuid-00000000-0000-4000-8000-000000000002", Device: "/dev/sdb1"},
		{MountPoint: "/srv/mergerfs/pool", Device: "pool:a:b"},
		{MountPoint: "/srv/my vault", Label: "vault", Device: "/dev/mapper/vault"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("mounts under /srv = %+v", got)
	}
}
