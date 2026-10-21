package proto

import (
	"bytes"
	"errors"
	"strings"
)

const (
	// MaxNames는 fs.names가 담는 최대 항목 수다. 넘치면 Truncated다.
	MaxNames = 300000
	// MaxNamesBytes는 fs.names 본문의 최대 바이트 수다. 넘치면 Truncated다.
	MaxNamesBytes = 32 << 20
	// MaxStatMany는 fs.statmany가 한 번에 받는 이름 수다.
	MaxStatMany = 512
	// MaxCopy는 fs.copy가 한 번에 받는 이름 수다. MaxCopyParams는 그 인자(JSON)의 최대 바이트 수로, 요청 머리
	// (MaxRequest) 안에 들어가야 한다.
	MaxCopy       = 512
	MaxCopyParams = 56 << 10
	// MaxSearch는 fs.search가 찾는 최대 항목 수다. 넘치면 Truncated다. MaxSearchQuery는 찾기어의 최대 바이트 수다.
	MaxSearch      = 2000
	MaxSearchQuery = 255
	// MaxNameBytes는 이름 하나의 최대 바이트 수(NAME_MAX)다.
	MaxNameBytes = 255
)

// fs.names 본문의 항목 종류(1바이트)
const (
	NameDir     byte = 'd'
	NameFile    byte = 'f'
	NameSymlink byte = 'l'
	NameOther   byte = 'o'
	// NameUnknown은 파일시스템이 종류를 주지 않았고 시간 안에 lstat하지 못한 항목이다.
	NameUnknown byte = '?'
)

// NameEntry는 fs.names의 항목 하나다.
type NameEntry struct {
	Type byte
	Name string
}

// AppendName은 fs.names 본문에 항목 하나(종류 1바이트 + 이름 + NUL)를 붙인다.
func AppendName(buf []byte, typ byte, name string) []byte {
	buf = append(buf, typ)
	buf = append(buf, name...)
	return append(buf, 0)
}

// NameSize는 AppendName이 붙이는 바이트 수다.
func NameSize(name string) int { return len(name) + 2 }

// ErrBadNames는 fs.names 본문의 형식이 잘못됐다는 오류다.
var ErrBadNames = errors.New("malformed names body")

// ParseNames는 fs.names 본문을 읽는다. 항목 수가 count와 다르면 ErrBadNames다.
func ParseNames(b []byte, count int) ([]NameEntry, error) {
	if count < 0 {
		return nil, ErrBadNames
	}
	out := make([]NameEntry, 0, count)
	for len(b) > 0 {
		i := bytes.IndexByte(b, 0)
		if i < 2 {
			return nil, ErrBadNames
		}
		switch b[0] {
		case NameDir, NameFile, NameSymlink, NameOther, NameUnknown:
		default:
			return nil, ErrBadNames
		}
		out = append(out, NameEntry{Type: b[0], Name: string(b[1:i])})
		b = b[i+1:]
	}
	if len(out) != count {
		return nil, ErrBadNames
	}
	return out, nil
}

// ValidName은 폴더 바로 안의 이름으로 쓸 수 있는지다: 비어 있지 않고, ".", ".."가 아니며,
// 경로 구분자·NUL이 없고 MaxNameBytes 이하다.
func ValidName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= MaxNameBytes &&
		!strings.ContainsAny(name, "/\x00")
}
