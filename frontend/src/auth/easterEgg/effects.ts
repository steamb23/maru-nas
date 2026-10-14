// 로그인 대기 이스터에그 효과의 계약과 목록(DOM 없음).
//
// 효과는 동적 import()로 효과별 청크에 나뉘어, 연출이 시작될 때 고른 효과만 내려받는다.
// 효과 모듈은 createEffect를 내보낸다. 크기·시간은 CSS px·ms 단위이며, devicePixelRatio 배율은 그리는 쪽이 맞춘다.
// 캔버스는 로그인 카드 뒤에 깔리므로, 글자처럼 가려지면 안 되는 내용은 카드 영역(avoid)을 피해 그린다.
//
// 효과 내용의 제약(로그인 전 화면이라 오해를 사지 않게):
// - 실제 시스템 정보(디스크, 서비스, 호스트 이름 등)를 반영하지 않는다. 전부 가짜 연출이다
// - 공격·침입처럼 보이는 연출(비밀번호 크래킹, ACCESS GRANTED), 진짜 장애로 보이는 실패 로그,
//   실제 시스템 정보처럼 보이는 화면(가짜 htop 등)은 쓰지 않는다
// - 로그인·셸 프롬프트처럼 보이는 입력 화면을 만들지 않는다(진짜로 오해해 비밀번호를 칠 수 있음)
// - IP가 필요하면 문서용 대역(192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24)만 쓴다
// - 소리는 쓰지 않는다. 문구는 영어로 고정하고 현지화하지 않는다(장식이라 aria-hidden)

import type { Rect } from '../../windows/geometry'

export type Rng = () => number

export interface EggEffect {
  // resize는 처음과 화면 크기·로그인 카드 크기가 바뀔 때 불린다. avoid는 캔버스 좌표의 로그인 카드 영역이다(없으면 null).
  resize: (width: number, height: number, avoid: Rect | null) => void
  // draw는 프레임마다 화면 전체를 다시 그린다. timeMs는 연출이 시작된 뒤 실제로 흐른 시간이다.
  // 탭이 숨겨진 동안에도 흐르므로, 돌아오면 그동안 재생된 것처럼 이어서 그린다.
  // 배경을 어둡게 하는 것은 오버레이(index.css의 .easter-egg)가 하므로, 캔버스는 지운 뒤 투명하게 두고 효과만 그린다.
  draw: (ctx: CanvasRenderingContext2D, timeMs: number) => void
}

export interface EggEffectModule {
  createEffect: (rng: Rng) => EggEffect
}

export type EffectLoader = () => Promise<EggEffectModule>

// 무작위로 고를 효과 목록
export const EFFECTS: readonly EffectLoader[] = [() => import('./decode'), () => import('./bootLog')]

// pickEffect는 목록에서 하나를 고른다. 비어 있으면 undefined.
export function pickEffect<T>(list: readonly T[], rng: Rng): T | undefined {
  if (list.length === 0) return undefined
  return list[Math.min(list.length - 1, Math.floor(rng() * list.length))]
}
