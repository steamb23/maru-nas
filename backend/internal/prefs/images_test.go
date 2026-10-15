package prefs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 형식 판별은 호출하는 쪽이 하므로 저장소 테스트에는 내용이 아무 바이트여도 된다.
var (
	pngData  = []byte("\x89PNG\r\n\x1a\nfake png")
	jpegData = []byte("\xff\xd8\xff fake jpeg")
)

func readImage(t *testing.T, s ImageStore, key ImageKey) (string, string) {
	t.Helper()
	img, err := s.OpenImage(key)
	if err != nil {
		t.Fatal(err)
	}
	defer img.Close()
	b, err := io.ReadAll(img.Content)
	if err != nil {
		t.Fatal(err)
	}
	return img.ContentType, string(b)
}

func TestImageRoundTrip(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			images := s.(ImageStore)
			key, _ := SiteImage(SiteLoginBackground)
			if info, err := images.ImageInfo(key); err != nil || info != nil {
				t.Fatalf("empty: %+v, %v", info, err)
			}
			if _, err := images.OpenImage(key); !errors.Is(err, ErrNoImage) {
				t.Fatalf("open empty: %v", err)
			}
			first, err := images.PutImage(key, "image/png", pngData)
			if err != nil {
				t.Fatal(err)
			}
			if ct, body := readImage(t, images, key); ct != "image/png" || body != string(pngData) {
				t.Fatalf("read %q %q", ct, body)
			}
			// 바꾸면 리비전이 바뀌고 형식도 새 것을 따른다.
			second, err := images.PutImage(key, "image/jpeg", jpegData)
			if err != nil || second.Rev == first.Rev {
				t.Fatalf("replace: %+v %+v %v", first, second, err)
			}
			if ct, body := readImage(t, images, key); ct != "image/jpeg" || body != string(jpegData) {
				t.Fatalf("after replace %q %q", ct, body)
			}
			if info, _ := images.ImageInfo(key); info == nil || info.Rev != second.Rev {
				t.Errorf("info %+v", info)
			}
			// 자리마다 따로다.
			other, _ := SiteImage(SiteDefaultWallpaper)
			if info, _ := images.ImageInfo(other); info != nil {
				t.Errorf("other slot %+v", info)
			}
			if err := images.DeleteImage(key); err != nil {
				t.Fatal(err)
			}
			if info, _ := images.ImageInfo(key); info != nil {
				t.Errorf("after delete %+v", info)
			}
			if err := images.DeleteImage(key); err != nil {
				t.Errorf("delete twice: %v", err)
			}
		})
	}
}

func TestImageRejectsUnknownType(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			key, _ := UserWallpaper("alice")
			if _, err := s.(ImageStore).PutImage(key, "image/svg+xml", []byte("<svg/>")); err == nil {
				t.Error("svg accepted")
			}
		})
	}
}

func TestImageKeys(t *testing.T) {
	if _, ok := SiteImage("../users"); ok {
		t.Error("unknown site image accepted")
	}
	if _, err := UserWallpaper(""); err == nil {
		t.Error("empty username accepted")
	}
}

func TestImageFileLayout(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStore(dir)
	for _, name := range []string{"alice", "../x"} {
		key, _ := UserWallpaper(name)
		if _, err := s.PutImage(key, "image/png", pngData); err != nil {
			t.Fatal(err)
		}
	}
	key, _ := UserWallpaper("alice")
	info, _ := s.PutImage(key, "image/jpeg", jpegData)
	entries, _ := os.ReadDir(filepath.Join(dir, "images", "users", "alice"))
	if len(entries) != 1 || entries[0].Name() != "wallpaper."+info.Rev+".jpg" {
		t.Errorf("alice dir: %v", entries)
	}
	// 사용자 이름은 인코딩되어 images 밖으로 나가지 않는다.
	if _, err := os.Stat(filepath.Join(dir, "images", "users", "%2E%2E%2Fx")); err != nil {
		t.Errorf("encoded dir: %v", err)
	}
	top, _ := os.ReadDir(dir)
	var names []string
	for _, e := range top {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "images,users" {
		t.Errorf("state dir entries: %v", names)
	}
	// 다시 열어도 그대로 읽힌다.
	s2, _ := NewFileStore(dir)
	if got, _ := s2.ImageInfo(key); got == nil || got.Rev != info.Rev {
		t.Errorf("reopen %+v", got)
	}
}
