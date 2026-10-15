// 한글 검색 보조. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 완성된 음절을 호환 자모(ㄱ~ㅎ, ㅏ~ㅣ)로 풀어 비교한다. 그러면 초성만 입력해도(ㄴㅌ → 네트워크),
// 입력 중인 글자가 다음 음절의 첫소리를 받침으로 먼저 붙이고 있어도(넽 → 네트워크) 찾는다.
// 한글이 아닌 글자는 그대로 둔다.

const SYLLABLE_FIRST = 0xac00
const SYLLABLE_LAST = 0xd7a3
const INITIALS = 'ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ'
const MEDIALS = 'ㅏㅐㅑㅒㅓㅔㅕㅖㅗㅘㅙㅚㅛㅜㅝㅞㅟㅠㅡㅢㅣ'
// 첫 칸은 받침 없음
const FINALS = ['', ...'ㄱㄲㄳㄴㄵㄶㄷㄹㄺㄻㄼㄽㄾㄿㅀㅁㅂㅄㅅㅆㅇㅈㅊㅋㅌㅍㅎ']

// 겹모음·겹받침은 낱자로 나눈다(ㅘ → ㅗㅏ, ㄳ → ㄱㅅ). 입력할 때는 낱자가 차례로 들어오기 때문이다.
// 된소리(ㄲ, ㅆ 등)는 한 번에 입력하므로 나누지 않는다.
const SPLIT: Readonly<Record<string, string>> = {
  ㅘ: 'ㅗㅏ',
  ㅙ: 'ㅗㅐ',
  ㅚ: 'ㅗㅣ',
  ㅝ: 'ㅜㅓ',
  ㅞ: 'ㅜㅔ',
  ㅟ: 'ㅜㅣ',
  ㅢ: 'ㅡㅣ',
  ㄳ: 'ㄱㅅ',
  ㄵ: 'ㄴㅈ',
  ㄶ: 'ㄴㅎ',
  ㄺ: 'ㄹㄱ',
  ㄻ: 'ㄹㅁ',
  ㄼ: 'ㄹㅂ',
  ㄽ: 'ㄹㅅ',
  ㄾ: 'ㄹㅌ',
  ㄿ: 'ㄹㅍ',
  ㅀ: 'ㄹㅎ',
  ㅄ: 'ㅂㅅ',
}

const split = (jamo: string) => SPLIT[jamo] ?? jamo

// syllableIndex는 완성형 음절이면 0부터의 번호, 아니면 -1이다.
function syllableIndex(ch: string): number {
  const code = ch.codePointAt(0) ?? 0
  return code >= SYLLABLE_FIRST && code <= SYLLABLE_LAST ? code - SYLLABLE_FIRST : -1
}

// toJamo는 완성형 음절을 초성·중성·종성 호환 자모로 풀고 겹자모를 나눈다. 다른 글자는 그대로 둔다.
export function toJamo(s: string): string {
  let out = ''
  for (const ch of s) {
    const i = syllableIndex(ch)
    if (i < 0) {
      out += split(ch)
      continue
    }
    out += INITIALS[Math.floor(i / 588)] + split(MEDIALS[Math.floor((i % 588) / 28)]) + split(FINALS[i % 28])
  }
  return out
}

// initials는 완성형 음절을 초성으로 바꾼다. 다른 글자는 그대로 둔다.
export function initials(s: string): string {
  let out = ''
  for (const ch of s) {
    const i = syllableIndex(ch)
    out += i < 0 ? ch : INITIALS[Math.floor(i / 588)]
  }
  return out
}

// isConsonants는 문자열이 모두 한글 자음(호환 자모)인지다. 초성 검색어인지 판단한다.
export function isConsonants(s: string): boolean {
  return s.length > 0 && [...s].every((ch) => INITIALS.includes(ch) || ch in SPLIT)
}
