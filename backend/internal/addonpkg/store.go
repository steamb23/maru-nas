package addonpkg

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"maru/backend/internal/addons"
)

// indexFile은 설치 기록 파일 이름이다.
const indexFile = "installed.json"

// indexVersion은 설치 기록 형식 번호다.
const indexVersion = 1

// stagingPrefix는 설치 중인 패키지를 푸는 디렉터리 이름의 앞부분이다.
const stagingPrefix = ".staging-"

// currentLink는 지금 rev를 가리키는 심볼릭 링크 이름이다.
const currentLink = "current"

// revPattern은 저장 디렉터리 이름이다(<version>-<sha256 앞 8자>, 개발 패키지는 dev-<해시>).
var revPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.-]{0,40}-[0-9a-f]{8}$`)

// ValidRev는 rev가 저장 디렉터리 이름 형식인지다.
func ValidRev(rev string) bool { return revPattern.MatchString(rev) && !strings.Contains(rev, "..") }

// Record는 설치한 패키지 하나의 기록이다.
type Record struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Rev     string `json:"rev"`
	SHA256  string `json:"sha256"`
	Author  string `json:"author"`
	Enabled bool   `json:"enabled"`
	// InstalledBy는 처음 설치하거나 마지막으로 업데이트한 관리자다.
	InstalledBy string    `json:"installedBy"`
	InstalledAt time.Time `json:"installedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	// InstallUID는 처음 설치할 때 만들고 업데이트해도 유지하는 값이다. 사용자별 저장값을 여기에 묶어,
	// 지웠다 다시 설치한(다른 작성자일 수 있는) 패키지가 이전 값을 읽지 못하게 한다.
	InstallUID string `json:"installUID"`
	// Grant는 승인한 권한 요약의 digest다(Summary.Digest).
	Grant string `json:"grant"`
}

type index struct {
	Version  int      `json:"version"`
	Packages []Record `json:"packages"`
}

// Store는 상태 디렉터리의 addons/ 디렉터리다(installed.json, <id>/<rev>/, <id>/current).
// 동시 쓰기는 호출한 쪽(Manager)이 하나의 잠금으로 막는다.
type Store struct {
	dir string
}

// NewStore는 dir(<state>/addons)을 쓰는 저장소를 반환한다. Init으로 디렉터리를 만든다.
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Dir은 저장소 디렉터리다.
func (s *Store) Dir() string { return s.dir }

// Init은 디렉터리를 0755로 만들고 이전 실행이 남긴 스테이징 디렉터리를 지운다.
func (s *Store) Init() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(s.dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), stagingPrefix) {
			if err := os.RemoveAll(filepath.Join(s.dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// ErrIndex는 installed.json을 읽을 수 없다는 뜻이다. 손상된 기록을 옮기거나 비우지 않는다
// (패키지가 조용히 사라지면 안 된다). 관리자가 파일을 고치거나 지워야 한다.
var ErrIndex = errors.New("cannot read the addon package index")

// Load는 설치 기록을 읽는다. 파일이 없으면 빈 목록이다.
func (s *Store) Load() ([]Record, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, indexFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIndex, err)
	}
	var idx index
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIndex, err)
	}
	if idx.Version != indexVersion {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrIndex, idx.Version)
	}
	seen := map[string]bool{}
	for _, r := range idx.Packages {
		if !addons.ValidID(r.ID) || !ValidRev(r.Rev) || seen[r.ID] {
			return nil, fmt.Errorf("%w: invalid record %q", ErrIndex, r.ID)
		}
		seen[r.ID] = true
	}
	return idx.Packages, nil
}

// Save는 설치 기록 전체를 바꿔 넣는다.
func (s *Store) Save(list []Record) error {
	if list == nil {
		list = []Record{}
	}
	b, err := json.MarshalIndent(index{Version: indexVersion, Packages: list}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.dir, indexFile), append(b, '\n'), 0o644)
}

// Staging은 압축 파일을 풀 빈 디렉터리다. 다 쓰면 Commit하거나 Discard한다.
type Staging struct {
	Dir  string
	Root *os.Root
}

// Discard는 스테이징 디렉터리를 지운다.
func (st *Staging) Discard() {
	if st == nil {
		return
	}
	st.Root.Close()
	os.RemoveAll(st.Dir)
}

// NewStaging은 저장소 안에 빈 스테이징 디렉터리(0755)를 만든다.
func (s *Store) NewStaging() (*Staging, error) {
	dir, err := os.MkdirTemp(s.dir, stagingPrefix+"*")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &Staging{Dir: dir, Root: root}, nil
}

// Commit은 스테이징 디렉터리를 <id>/<rev>로 옮기고 current를 그 rev로 바꾼다. 백엔드 패키지는 arch의 실행 파일을
// backend/run(0755)으로 두고 다른 아키텍처의 실행 파일은 지운다. 같은 rev가 이미 있으면(같은 파일을 다시 설치)
// 스테이징을 버리고 있는 것을 쓴다.
func (s *Store) Commit(st *Staging, id, rev, arch string) error {
	if !addons.ValidID(id) || !ValidRev(rev) {
		return fmt.Errorf("invalid id or rev %q/%q", id, rev)
	}
	defer st.Root.Close()
	if err := placeBackend(st.Root, arch); err != nil {
		return err
	}
	idDir := filepath.Join(s.dir, id)
	if err := os.MkdirAll(idDir, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(idDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(idDir, rev)
	if _, err := os.Lstat(target); err == nil {
		os.RemoveAll(st.Dir)
	} else if err := os.Rename(st.Dir, target); err != nil {
		return err
	}
	return s.setCurrent(id, rev)
}

// placeBackend는 arch의 실행 파일을 backend/run으로 옮기고 나머지 실행 파일을 지운다. 백엔드가 없으면 그대로다.
func placeBackend(root *os.Root, arch string) error {
	entries, err := fs.ReadDir(root.FS(), "backend")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := "backend/" + e.Name()
		if e.Name() == "linux-"+arch {
			if err := root.Rename(name, "backend/run"); err != nil {
				return err
			}
			if err := root.Chmod("backend/run", 0o755); err != nil {
				return err
			}
			continue
		}
		if err := root.Remove(name); err != nil {
			return err
		}
	}
	return nil
}

// setCurrent는 <id>/current를 rev로 바꾼다(임시 링크를 만들어 rename).
func (s *Store) setCurrent(id, rev string) error {
	idDir := filepath.Join(s.dir, id)
	tmp := filepath.Join(idDir, ".current-"+randomHex(8))
	if err := os.Symlink(rev, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(idDir, currentLink)); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Prune은 <id> 아래에서 keep 밖의 rev를 지운다. 새 rev로 바꾼 뒤 부른다.
func (s *Store) Prune(id, keep string) error {
	if !addons.ValidID(id) {
		return fmt.Errorf("invalid id %q", id)
	}
	idDir := filepath.Join(s.dir, id)
	entries, err := os.ReadDir(idDir)
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if e.Name() == keep || e.Name() == currentLink {
			continue
		}
		errs = append(errs, os.RemoveAll(filepath.Join(idDir, e.Name())))
	}
	return errors.Join(errs...)
}

// Remove는 패키지의 파일을 모두 지운다.
func (s *Store) Remove(id string) error {
	if !addons.ValidID(id) {
		return fmt.Errorf("invalid id %q", id)
	}
	return os.RemoveAll(filepath.Join(s.dir, id))
}

// Path는 <id>/<rev> 디렉터리다.
func (s *Store) Path(id, rev string) string { return filepath.Join(s.dir, id, rev) }

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// writeFileAtomic은 data를 같은 디렉터리의 임시 파일에 쓰고 fsync한 뒤 path로 바꿔 넣는다.
func writeFileAtomic(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
