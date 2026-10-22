package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"maru/backend/internal/omv/omvfake"
)

// 파일 탐색기 개발용 합성 데이터. 가짜 도우미의 가짜 디스크(helper-root/srv/disk1) 아래에 공유 폴더 몇 개와
// 합성 파일을 만들고, 가짜 OMV의 ShareMgmt.enumerateSharedFolders가 그 폴더를 가리키게 한다. 이름·크기·날짜는
// 모두 가짜 값이다. 이미 있는 파일은 건드리지 않으므로 개발 중에 올리거나 지운 파일은 남는다.
//
// FAKEFILES_LARGE=<n>이면 사진 공유 폴더에 이름만 다른 빈 파일 n개짜리 큰 폴더(휴대폰 백업)를 만든다.

// devShares는 가짜 OMV의 공유 폴더다. 백업은 폴더가 없는 공유 폴더(목록에서 빠짐), kvm은 루트 파일시스템의
// 공유 폴더(도우미가 닿지 않음)를 흉내 낸다. 공유 폴더 권한 따르기를 켜면 user에게 사진은 읽기 전용, 관리는 보이지
// 않는다(미디어는 정한 권한이 없어 모두에게 열린다).
func devShares(disk string) []omvfake.SharedFolder {
	rw := func(names ...string) []omvfake.Privilege {
		var out []omvfake.Privilege
		for _, n := range names {
			out = append(out, omvfake.Privilege{Type: "user", Name: n, Perms: 7})
		}
		return out
	}
	return []omvfake.SharedFolder{
		{UUID: "fa4e0001-0000-4000-8000-000000000001", Name: "문서", MountDir: disk, RelDirPath: "docs/",
			Privileges: []omvfake.Privilege{{Type: "group", Name: "users", Perms: 7}}},
		{UUID: "fa4e0001-0000-4000-8000-000000000002", Name: "미디어", MountDir: disk, RelDirPath: "media/"},
		{UUID: "fa4e0001-0000-4000-8000-000000000003", Name: "사진", MountDir: disk, RelDirPath: "photos/",
			Privileges: append(rw("admin", "totp"), omvfake.Privilege{Type: "user", Name: "user", Perms: 5})},
		{UUID: "fa4e0001-0000-4000-8000-000000000006", Name: "관리", MountDir: disk, RelDirPath: "manage/", Privileges: rw("admin", "totp")},
		{UUID: "fa4e0001-0000-4000-8000-000000000004", Name: "백업", MountDir: disk, RelDirPath: "backup/", Missing: true},
		{UUID: "fa4e0001-0000-4000-8000-000000000005", Name: "kvm", MountDir: "/", RelDirPath: "kvm/"},
	}
}

// seedFiles는 disk 아래에 합성 파일을 만든다.
func seedFiles(disk string) {
	files := map[string]int{
		"docs/README.md":                        1200,
		"manage/점검 계획.txt":                      900,
		"docs/회의록 2026-10.txt":                  3400,
		"docs/예산.xlsx":                          18600,
		"docs/보고서/2026 상반기.pdf":                 512000,
		"docs/보고서/2026 하반기 초안.docx":             48000,
		"media/영화/영화 목록.xlsx":                   18200,
		"media/영화/여름 휴가 2024.mkv":               2 << 20,
		"media/영화/여름 휴가 2024 메이킹.mp4":           1 << 20,
		"media/영화/자막 모음.zip":                    420000,
		"media/영화/포스터.png":                      240000,
		"media/영화/설치 디스크.iso":                   300000,
		"media/영화/2023/a.mkv":                   1000,
		"media/영화/2024/b.mkv":                   1000,
		"media/영화/2025/c.mkv":                   1000,
		"media/영화/다큐멘터리/d.mkv":                  1000,
		"media/음악/노래 1.flac":                    300000,
		"media/음악/노래 2.mp3":                     120000,
		"media/음악/노래 10.mp3":                    130000,
		"photos/가족/IMG_0001.jpg":                200000,
		"photos/가족/IMG_0002.jpg":                210000,
		"photos/가족/IMG_0010.heic":               180000,
		"photos/스크린샷/Screenshot 2026-10-01.png": 90000,
	}
	for rel, size := range files {
		p := filepath.Join(disk, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			log.Printf("fake files: %v", err)
			return
		}
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			log.Printf("fake files: %v", err)
			return
		}
	}
	// 들어갈 수 없는 폴더(자물쇠 표시)
	locked := filepath.Join(disk, "media", "영화", "관리")
	if _, err := os.Stat(locked); err != nil {
		if err := os.Mkdir(locked, 0o000); err != nil {
			log.Printf("fake files: %v", err)
		}
	}
	// 읽기만 되는 파일
	ro := filepath.Join(disk, "docs", "읽기 전용.txt")
	if _, err := os.Stat(ro); err != nil {
		if err := os.WriteFile(ro, []byte("read only\n"), 0o444); err != nil {
			log.Printf("fake files: %v", err)
		}
	}
	if n, err := strconv.Atoi(os.Getenv("FAKEFILES_LARGE")); err == nil && n > 0 {
		seedLarge(filepath.Join(disk, "photos", "휴대폰 백업"), n)
	}
}

// seedLarge는 dir에 빈 파일 n개를 만든다(이미 있는 이름은 건너뛴다).
func seedLarge(dir string, n int) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("fake files: %v", err)
		return
	}
	made := 0
	for i := range n {
		name := fmt.Sprintf("IMG_%08d_%06d.jpg", 20190101+i/400, i%400*137%240000)
		if i%25 == 0 {
			name = fmt.Sprintf("VID_%08d_%06d.mp4", 20190101+i/400, i%400*131%240000)
		}
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(p); err == nil {
			continue
		}
		f, err := os.Create(p)
		if err != nil {
			log.Printf("fake files: %v", err)
			return
		}
		f.Close()
		made++
	}
	log.Printf("fake files: large folder %s (%d new of %d)", dir, made, n)
}
