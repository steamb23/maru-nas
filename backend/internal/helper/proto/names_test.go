package proto

import (
	"strings"
	"testing"
)

func TestNamesBody(t *testing.T) {
	var b []byte
	b = AppendName(b, NameDir, "사진")
	b = AppendName(b, NameFile, "a b.txt")
	b = AppendName(b, NameUnknown, "x")
	if len(b) != NameSize("사진")+NameSize("a b.txt")+NameSize("x") {
		t.Fatalf("size = %d", len(b))
	}
	got, err := ParseNames(b, 3)
	if err != nil || len(got) != 3 || got[0] != (NameEntry{NameDir, "사진"}) || got[1] != (NameEntry{NameFile, "a b.txt"}) || got[2].Type != NameUnknown {
		t.Fatalf("parse = %v %v", got, err)
	}
	if got, err := ParseNames(nil, 0); err != nil || len(got) != 0 {
		t.Fatalf("empty = %v %v", got, err)
	}
	for _, bad := range []struct {
		b     string
		count int
	}{
		{"fa\x00", 2},   // 개수가 다름
		{"fa", 1},       // NUL 없음
		{"f\x00", 1},    // 빈 이름
		{"za\x00", 1},   // 모르는 종류
		{"fa\x00fb", 2}, // 마지막 NUL 없음
		{"fa\x00", -1},  // 음수 개수
	} {
		if _, err := ParseNames([]byte(bad.b), bad.count); err == nil {
			t.Errorf("ParseNames(%q, %d) accepted", bad.b, bad.count)
		}
	}
}

func TestValidName(t *testing.T) {
	for _, ok := range []string{"a", "사진.jpg", ".hidden", "...", strings.Repeat("a", 255)} {
		if !ValidName(ok) {
			t.Errorf("ValidName(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", ".", "..", "a/b", "/", "a\x00", strings.Repeat("a", 256)} {
		if ValidName(bad) {
			t.Errorf("ValidName(%q) = true", bad)
		}
	}
}
