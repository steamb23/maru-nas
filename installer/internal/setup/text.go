package setup

import (
	"strings"

	"maru/backend/config"
)

// Text는 영어·한국어 문구 한 쌍이다. 문구는 쓰는 곳(설정·단계 파일) 옆에 L(en, ko)로 둔다.
// 생성하는 설정 파일(유닛, /etc/default/*)의 주석은 언어와 상관없이 영어로 쓰므로 Text를 쓰지 않는다.
type Text struct{ En, Ko string }

// L은 문구 한 쌍을 만든다.
func L(en, ko string) Text { return Text{En: en, Ko: ko} }

// localized는 JSON으로 내보낼 두 언어 문구다(업그레이드 계획).
func (t Text) localized() config.LocalizedText { return config.LocalizedText{En: t.En, Ko: t.Ko} }

// In은 lang(en, ko)의 문구다.
func (t Text) In(lang string) string {
	if lang == "ko" {
		return t.Ko
	}
	return t.En
}

// DetectLang은 안내 문구 언어를 로캘에서 고른다. LC_ALL, LC_MESSAGES, LANG 중 처음으로 비어 있지 않은 값이
// ko로 시작하면 ko, 아니면 en이다(설치 스크립트가 쓰던 규칙).
func DetectLang(getenv func(string) string) string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			if strings.HasPrefix(v, "ko") {
				return "ko"
			}
			return "en"
		}
	}
	return "en"
}

// displayWidth는 터미널에 보이는 폭이다. 한글은 두 칸으로 센다(요약 표의 열 맞춤용).
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0xA4CF, r >= 0xAC00 && r <= 0xD7A3, r >= 0xF900 && r <= 0xFAFF, r >= 0xFF00 && r <= 0xFF60:
			w += 2
		default:
			w++
		}
	}
	return w
}

// padRight는 s를 보이는 폭 width가 되게 공백으로 채운다.
func padRight(s string, width int) string {
	if n := width - displayWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
