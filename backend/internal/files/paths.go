// Package files는 파일 탐색기의 서버 쪽 계산이다: 경로 검사, 공유 폴더 스냅숏 읽기, 이름 정렬, 폴더 목록 캐시.
//
// 파일 자체는 권한 도우미가 로그인한 사용자의 권한으로 다룬다(fs.names, fs.statmany). 이 패키지는 도우미가 준
// 이름 목록을 세션별로 잠시 두고, 정렬·찾기를 적용한 보기에서 브라우저가 보는 범위만 꺼낸다.
package files

import (
	"cmp"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxPath는 받는 경로의 최대 길이(PATH_MAX)다. 권한 도우미와 같다.
const maxPath = 4096

// ValidPath는 절대 경로이고 정리되어 있으며(.., 겹친 /, 끝의 / 없음) NUL이 없는지다.
func ValidPath(p string) bool {
	return len(p) <= maxPath && strings.HasPrefix(p, "/") && !strings.ContainsRune(p, 0) && path.Clean(p) == p
}

// Within은 p가 root이거나 그 아래인지다. 둘 다 ValidPath여야 한다.
func Within(p, root string) bool {
	return p == root || root == "/" || strings.HasPrefix(p, root+"/")
}

// Reachable은 권한 도우미가 닿을 수 있는 곳(srvDir 아래 마운트)인지 경로 모양으로만 본다. 홈 폴더는 따로 본다.
// 루트 파일시스템의 공유 폴더(openmediavault-sharerootfs, 예: /kvm)는 도우미의 기준 디렉터리 밖이다.
func Reachable(p, srvDir string) bool {
	return strings.HasPrefix(p, strings.TrimSuffix(srvDir, "/")+"/")
}

// CompareNames는 파일 이름 순서다. 숫자 묶음은 값으로, 나머지는 소문자로 바꾼 코드포인트로 비교하고, 같으면
// 바이트 순서로 정한다(표준 라이브러리만 쓰므로 로캘 정렬이 아니다. 한글 음절은 코드포인트가 가나다순이다).
func CompareNames(a, b string) int {
	if c := compareNatural(a, b); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func compareNatural(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			si, sj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na, nb := strings.TrimLeft(a[si:i], "0"), strings.TrimLeft(b[sj:j], "0")
			if len(na) != len(nb) {
				return cmp.Compare(len(na), len(nb))
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			continue
		}
		ra, wa := utf8.DecodeRuneInString(a[i:])
		rb, wb := utf8.DecodeRuneInString(b[j:])
		if fa, fb := unicode.ToLower(ra), unicode.ToLower(rb); fa != fb {
			return cmp.Compare(fa, fb)
		}
		i += wa
		j += wb
	}
	return cmp.Compare(len(a)-i, len(b)-j)
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }
