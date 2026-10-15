import { describe, expect, it } from 'vitest'
import { normalize, searchApps } from './appSearch'

// 화면 언어와 무관하게 지원 언어 모두의 이름과 앱 id를 후보로 준다(AppDrawer와 같은 방식).
const TERMS: Record<string, string[]> = {
  'system-info': ['System Information', '시스템 정보', 'system-info'],
  temperature: ['Temperature', '온도', 'temperature'],
  network: ['Network', '네트워크', 'network'],
  smart: ['Disk Health', '디스크 건강', 'smart'],
  storage: ['Storage', '저장소', 'storage'],
  services: ['Services', '서비스', 'services'],
}
const IDS = Object.keys(TERMS)
const search = (q: string) => searchApps(IDS, (id) => TERMS[id], q)

describe('normalize', () => {
  it('소문자, 공백 제거, 완성형', () => {
    expect(normalize(' Disk  Health ')).toBe('diskhealth')
    // 조합형(NFD) 한글도 완성형으로 비교한다.
    expect(normalize('네트워크'.normalize('NFD'))).toBe('네트워크')
  })
})

describe('searchApps', () => {
  it('빈 검색어는 전부', () => {
    expect(search('')).toEqual(IDS)
    expect(search('   ')).toEqual(IDS)
  })

  it('어느 언어 이름으로도 찾는다', () => {
    expect(search('net')).toEqual(['network'])
    expect(search('네트')).toEqual(['network'])
    expect(search('디스크')).toEqual(['smart'])
  })

  it('대소문자·공백을 가리지 않는다', () => {
    expect(search('DISK health')).toEqual(['smart'])
    expect(search('시스템정보')).toEqual(['system-info'])
  })

  it('앱 id로도 찾는다', () => {
    expect(search('smart')).toEqual(['smart'])
  })

  it('앞부분이 같은 것이 먼저, 같은 순위는 원래 순서', () => {
    // "s": 모두 앞이 같다(smart는 앱 id). Network에는 s가 없다.
    expect(search('s')).toEqual(['system-info', 'smart', 'storage', 'services'])
    // "e": 앞이 같은 것이 없어 모두 중간 일치이므로 원래 순서
    expect(search('e')).toEqual(['system-info', 'temperature', 'network', 'smart', 'storage', 'services'])
    // "st": Storage(앞)가 System Information(중간 "st")보다 먼저
    expect(search('st')).toEqual(['storage', 'system-info'])
    expect(search('ser')).toEqual(['services'])
    expect(search('ork')).toEqual(['network'])
  })

  it('초성으로 찾는다', () => {
    expect(search('ㄴㅌ')).toEqual(['network'])
    expect(search('ㄷㅅㅋ')).toEqual(['smart'])
    // 입력기가 자음을 겹받침으로 묶어도 찾는다(ㅅ+ㅂ+ㅅ → ㅅㅄ)
    expect(search('ㅅㅄ')).toEqual(['services'])
    expect(search('ㅄ')).toEqual(['services'])
    // 앞 초성이 같은 것이 먼저: 시스템 정보·서비스(ㅅ로 시작), 디스크 건강(중간에 ㅅ)
    expect(search('ㅅ').slice(0, 2)).toEqual(['system-info', 'services'])
  })

  it('입력 중인 글자로도 결과가 끊기지 않는다', () => {
    // "네트워크"를 치는 동안: ㄴ → 네 → 넽(트의 ㅌ가 받침으로 먼저 붙음) → 네트 → 네트우 → 네트워
    for (const q of ['네', '넽', '네트', '네트우', '네트워']) expect(search(q)).toEqual(['network'])
    // "온도": 오 → 온 → 온ㄷ → 온도
    for (const q of ['오', '온', '온ㄷ', '온도']) expect(search(q)).toContain('temperature')
  })

  it('맞는 것이 없으면 빈 목록', () => {
    expect(search('zzz')).toEqual([])
  })
})
