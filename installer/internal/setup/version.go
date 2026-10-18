package setup

import (
	"errors"
	"slices"
	"strconv"
	"strings"
)

// 빌드에 필요한 도구 버전. 업그레이드 계획(--upgrade-plan)이 내고, Node.js는 PATH의 node와 비교한다.
// 계획은 저장소 파일을 읽지 않으므로 상수로 두고, 원본과 같은지는 테스트가 본다.
const (
	// requireGo는 installer/go.mod와 backend/go.mod의 go 지시어다(TestRequireGo).
	requireGo = "1.27.1"
	// requireNode는 frontend/package.json의 engines.node다(TestRequireNode). ^X.Y.Z와 >=X.Y.Z를 ||로 이은 꼴만 쓴다.
	requireNode = "^20.19.0 || >=22.12.0"
)

// version은 X.Y.Z 버전이다(온마루 0.y.z, Node.js).
type version [3]int

// parseVersion은 X.Y.Z(앞의 v 허용)를 읽는다. 숫자 세 개가 아니면(꼬리표 포함) false다.
func parseVersion(s string) (version, bool) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return version{}, false
	}
	var v version
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return version{}, false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return version{}, false
		}
		v[i] = n
	}
	return v, true
}

// cmp는 a < b면 음수, 같으면 0, a > b면 양수다.
func (a version) cmp(b version) int { return slices.Compare(a[:], b[:]) }

// versionRange는 requireNode 꼴의 범위를 읽는다. 각 항목은 ^X.Y.Z(같은 X에서 X.Y.Z 이상) 또는 >=X.Y.Z다.
type versionRange []struct {
	caret bool
	min   version
}

func parseVersionRange(s string) (versionRange, error) {
	var r versionRange
	for _, alt := range strings.Split(s, "||") {
		alt = strings.TrimSpace(alt)
		caret := strings.HasPrefix(alt, "^")
		rest, ok := strings.CutPrefix(alt, "^")
		if !caret {
			if rest, ok = strings.CutPrefix(alt, ">="); !ok {
				return nil, errors.New("unsupported version range: " + alt)
			}
		}
		v, ok := parseVersion(rest)
		if !ok || (caret && v[0] == 0) {
			return nil, errors.New("unsupported version range: " + alt)
		}
		r = append(r, struct {
			caret bool
			min   version
		}{caret, v})
	}
	return r, nil
}

// contains는 v가 범위 안인지다.
func (r versionRange) contains(v version) bool {
	for _, alt := range r {
		if v.cmp(alt.min) >= 0 && (!alt.caret || v[0] == alt.min[0]) {
			return true
		}
	}
	return false
}
