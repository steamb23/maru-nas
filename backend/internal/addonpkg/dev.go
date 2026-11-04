package addonpkg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// ReadDir는 풀린 패키지 디렉터리(개발 패키지, MARU_DEV_ADDONS)를 압축 파일과 같은 규칙으로 검사한다. 파일은
// 옮기지 않는다. rev는 "dev-<파일 이름·크기·수정 시각 해시 앞 8자>"라 빌드할 때마다 바뀐다(열린 프레임이 다시 뜬다).
// 심볼릭 링크는 따라가지 않고 거부한다. 백엔드 실행 파일은 없어도 된다(개발 소켓으로 따로 띄운다).
func ReadDir(dir string, lim Limits, arch string) (*Package, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	var (
		pkg       = &Package{}
		files     = map[string]int64{}
		manifest  []byte
		languages []string
		entries   int
		h         = sha256.New()
	)
	err = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		// 개발 디렉터리의 dev/(가짜 OMV 응답 등)는 패키지에 넣지 않는 개발용 파일이다.
		if name == "dev" && d.IsDir() {
			return fs.SkipDir
		}
		if entries++; entries > lim.Entries {
			return fail(ReasonTooLarge, "more than %d entries", lim.Entries)
		}
		if _, err := cleanName(name); err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return allowedDir(name)
		case !d.Type().IsRegular():
			return fail(ReasonArchive, "%s: only regular files and directories are allowed", name)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		max, err := fileLimit(name, lim)
		if err != nil {
			return err
		}
		if info.Size() > max {
			return fail(ReasonTooLarge, "%s: larger than %d bytes", name, max)
		}
		if pkg.Size += info.Size(); pkg.Size > lim.Total {
			return fail(ReasonTooLarge, "unpacked size is larger than %d bytes", lim.Total)
		}
		files[name] = info.Size()
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", name, info.Size(), info.ModTime().UnixNano())
		if name == "manifest.json" {
			if manifest, err = readSmall(root, name, max); err != nil {
				return err
			}
		}
		if lang, ok := strings.CutPrefix(name, "i18n/"); ok {
			languages = append(languages, strings.TrimSuffix(lang, ".json"))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	pkg.SHA256 = hex.EncodeToString(h.Sum(nil))
	if err := inspect(pkg, root, manifest, files, languages, arch, false); err != nil {
		return nil, err
	}
	return pkg, nil
}

// DevRev는 개발 패키지의 rev다.
func (p *Package) DevRev() string { return "dev-" + p.SHA256[:8] }
