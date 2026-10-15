package prefs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"onmaru/backend/internal/api"
)

// ImageStore는 배경 이미지 저장소다. 자리(ImageKey)마다 이미지 하나를 둔다.
// 내용 검사(형식 판별, 크기 제한)는 호출하는 쪽이 한다.
type ImageStore interface {
	// PutImage는 자리의 이미지를 바꾼다. contentType은 ImageTypes 중 하나여야 한다.
	PutImage(key ImageKey, contentType string, data []byte) (api.ImageInfo, error)
	// OpenImage는 자리의 이미지를 연다. 없으면 ErrNoImage. 다 쓰면 Close한다.
	OpenImage(key ImageKey) (*Image, error)
	// ImageInfo는 자리의 이미지 정보다. 없으면 nil.
	ImageInfo(key ImageKey) (*api.ImageInfo, error)
	// DeleteImage는 자리의 이미지를 지운다. 없어도 오류가 아니다.
	DeleteImage(key ImageKey) error
}

// ErrNoImage는 자리에 이미지가 없다는 뜻이다.
var ErrNoImage = errors.New("prefs: no image")

// 전역 배경 이미지 자리 이름.
const (
	SiteLoginBackground  = "login-background"
	SiteDefaultWallpaper = "default-wallpaper"
)

// ImageKey는 배경 이미지 하나의 자리다. SiteImage, UserWallpaper로만 만들어 저장 경로가 정해진 모양을 벗어나지 않게 한다.
type ImageKey struct {
	// dir, name은 images 디렉터리 아래의 상대 경로('/' 구분)와 파일 이름 앞부분이다.
	dir, name string
}

// SiteImage는 전역 배경 이미지 자리다. 알려진 이름(SiteLoginBackground, SiteDefaultWallpaper)이 아니면 false.
func SiteImage(name string) (ImageKey, bool) {
	if name != SiteLoginBackground && name != SiteDefaultWallpaper {
		return ImageKey{}, false
	}
	return ImageKey{dir: "site", name: name}, true
}

// UserWallpaper는 사용자가 올린 바탕화면 이미지 자리다. 사용자 이름은 fileName으로 인코딩한다.
func UserWallpaper(username string) (ImageKey, error) {
	if username == "" {
		return ImageKey{}, errors.New("preferences: empty username")
	}
	return ImageKey{dir: "users/" + fileName(username), name: "wallpaper"}, nil
}

// imageExts는 받는 이미지 형식과 저장 파일 확장자다. SVG는 스크립트를 품을 수 있어 받지 않는다.
var imageExts = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
}

// IsImageType은 저장할 수 있는 이미지 형식(MIME)인지다.
func IsImageType(contentType string) bool {
	_, ok := imageExts[contentType]
	return ok
}

// Image는 연 이미지다.
type Image struct {
	ContentType string
	ModTime     time.Time
	Info        api.ImageInfo
	Content     io.ReadSeeker
	close       func() error
}

func (i *Image) Close() error {
	if i.close == nil {
		return nil
	}
	return i.close()
}

// imageRev는 이미지 내용의 리비전이다. 내용이 같으면 같다.
func imageRev(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// memImage는 MemoryStore의 이미지 하나다.
type memImage struct {
	contentType string
	data        []byte
	rev         string
	modTime     time.Time
}

func (m *MemoryStore) PutImage(key ImageKey, contentType string, data []byte) (api.ImageInfo, error) {
	if !IsImageType(contentType) {
		return api.ImageInfo{}, errors.New("preferences: unsupported image type " + contentType)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	img := memImage{contentType: contentType, data: bytes.Clone(data), rev: imageRev(data), modTime: time.Now()}
	m.images[key] = img
	return api.ImageInfo{Rev: img.rev}, nil
}

func (m *MemoryStore) OpenImage(key ImageKey) (*Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	img, ok := m.images[key]
	if !ok {
		return nil, ErrNoImage
	}
	return &Image{
		ContentType: img.contentType,
		ModTime:     img.modTime,
		Info:        api.ImageInfo{Rev: img.rev},
		Content:     bytes.NewReader(img.data),
	}, nil
}

func (m *MemoryStore) ImageInfo(key ImageKey) (*api.ImageInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	img, ok := m.images[key]
	if !ok {
		return nil, nil
	}
	return &api.ImageInfo{Rev: img.rev}, nil
}

func (m *MemoryStore) DeleteImage(key ImageKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.images, key)
	return nil
}

// FileStore는 이미지를 <root>/images/<dir>/<name>.<리비전>.<확장자>에 둔다. 리비전을 파일 이름에 넣어
// 파일 시스템의 수정 시각 정밀도와 무관하게 내용이 바뀌면 주소(?v=)가 바뀌게 한다.

func (s *FileStore) PutImage(key ImageKey, contentType string, data []byte) (api.ImageInfo, error) {
	ext, ok := imageExts[contentType]
	if !ok {
		return api.ImageInfo{}, errors.New("preferences: unsupported image type " + contentType)
	}
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	dir := s.imageDir(key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return api.ImageInfo{}, err
	}
	rev := imageRev(data)
	name := key.name + "." + rev + ext
	if err := writeFileAtomic(filepath.Join(dir, name), data); err != nil {
		return api.ImageInfo{}, err
	}
	// 새 파일을 넣은 뒤에 이전 파일을 지운다. 중간에 멈추면 둘 다 남을 수 있으나 읽을 때는 하나만 쓴다.
	old, err := s.imageFiles(key)
	if err != nil {
		return api.ImageInfo{}, err
	}
	for _, f := range old {
		if f != name {
			if err := os.Remove(filepath.Join(dir, f)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return api.ImageInfo{}, err
			}
		}
	}
	return api.ImageInfo{Rev: rev}, nil
}

func (s *FileStore) OpenImage(key ImageKey) (*Image, error) {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	name, rev, contentType, err := s.findImage(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(s.imageDir(key), name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNoImage
	}
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Image{
		ContentType: contentType,
		ModTime:     fi.ModTime(),
		Info:        api.ImageInfo{Rev: rev},
		Content:     f,
		close:       f.Close,
	}, nil
}

func (s *FileStore) ImageInfo(key ImageKey) (*api.ImageInfo, error) {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	_, rev, _, err := s.findImage(key)
	if errors.Is(err, ErrNoImage) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &api.ImageInfo{Rev: rev}, nil
}

func (s *FileStore) DeleteImage(key ImageKey) error {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	files, err := s.imageFiles(key)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(filepath.Join(s.imageDir(key), f)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *FileStore) imageDir(key ImageKey) string {
	return filepath.Join(s.root, "images", filepath.FromSlash(key.dir))
}

// imageFiles는 자리의 이미지 파일 이름들이다(정상이면 하나). 디렉터리가 없으면 빈 목록이다.
func (s *FileStore) imageFiles(key ImageKey) ([]string, error) {
	entries, err := os.ReadDir(s.imageDir(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if _, _, ok := parseImageName(key, e.Name()); ok && e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// findImage는 자리의 이미지 파일 이름, 리비전, 형식이다. 여러 개 남아 있으면(넣는 중에 멈춘 경우) 첫 번째를 쓴다.
func (s *FileStore) findImage(key ImageKey) (name, rev, contentType string, err error) {
	files, err := s.imageFiles(key)
	if err != nil {
		return "", "", "", err
	}
	if len(files) == 0 {
		return "", "", "", ErrNoImage
	}
	rev, contentType, _ = parseImageName(key, files[0])
	return files[0], rev, contentType, nil
}

// parseImageName은 <name>.<리비전>.<확장자> 모양의 파일 이름을 읽는다. 임시 파일(.tmp-*)은 맞지 않는다.
func parseImageName(key ImageKey, file string) (rev, contentType string, ok bool) {
	rest, found := strings.CutPrefix(file, key.name+".")
	if !found {
		return "", "", false
	}
	rev, ext, found := strings.Cut(rest, ".")
	if !found || rev == "" {
		return "", "", false
	}
	for ct, e := range imageExts {
		if e == "."+ext {
			return rev, ct, true
		}
	}
	return "", "", false
}
