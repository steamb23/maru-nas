// "모든 앱" 검색. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 앱마다 검색어 후보(지원하는 모든 언어의 앱 이름과 앱 id)를 받아, 화면 언어와 무관하게 어느 언어로 입력해도
// 찾는다. 후보 목록은 부르는 쪽(AppDrawer)이 i18next로 만든다. 이 모듈은 i18n을 import하지 않는다.
// 한글은 자모로 풀어 비교해 초성 검색과 입력 중인 글자를 함께 다룬다(hangul.ts).
import { initials, isConsonants, toJamo } from './hangul'

// normalize는 비교할 문자열을 맞춘다: 한글 조합형을 완성형으로(NFC), 소문자, 공백과 소프트 하이픈 제거.
// 소프트 하이픈(U+00AD)은 긴 앱 이름이 줄을 바꿀 자리로 카탈로그에 넣는다(es의 "Almacena­miento").
export function normalize(s: string): string {
  return s.normalize('NFC').toLowerCase().replace(/[\s­]+/g, '')
}

// matchRank는 후보 하나가 검색어와 얼마나 맞는지다. 0: 앞부분이 같음, 1: 중간에 들어 있음, null: 안 맞음.
// 둘 다 normalize한 값이다. 자모로 풀어 비교하고(입력 중인 "넽"도 "네트워크"에 맞음), 검색어가 자음뿐이면
// 초성끼리도 비교한다("ㄴㅌ" → "네트워크").
export function matchRank(term: string, query: string): 0 | 1 | null {
  const t = toJamo(term)
  const q = toJamo(query)
  const ini = isConsonants(query) ? initials(term) : null
  // 초성도 나눈 자모로 비교한다. 입력기가 자음을 겹받침으로 묶어도(ㅅ+ㅂ+ㅅ → ㅅㅄ) 맞게 한다.
  if (t.startsWith(q) || ini?.startsWith(q)) return 0
  if (t.includes(q) || ini?.includes(q)) return 1
  return null
}

// searchApps는 검색어에 맞는 앱을 앞부분이 같은 것부터, 같은 순위 안에서는 ids 순서로 돌려준다.
// 검색어가 비어 있으면 ids 그대로다.
export function searchApps<T extends string>(
  ids: readonly T[],
  terms: (id: T) => readonly string[],
  query: string,
): T[] {
  const q = normalize(query)
  if (q === '') return [...ids]
  const ranked: { id: T; rank: number; index: number }[] = []
  ids.forEach((id, index) => {
    let best: number | null = null
    for (const term of terms(id)) {
      const r = matchRank(normalize(term), q)
      if (r !== null && (best === null || r < best)) best = r
    }
    if (best !== null) ranked.push({ id, rank: best, index })
  })
  ranked.sort((a, b) => a.rank - b.rank || a.index - b.index)
  return ranked.map((r) => r.id)
}
