import type { TFunction } from 'i18next'
import type { Container } from '../../api/types.gen'

// stateLabel은 상태(와 헬스체크 결과)를 짧은 문구로 바꾼다. 예: 실행 중 · 비정상
export function stateLabel(t: TFunction, c: Container): string {
  const state = t(`containers.state.${c.state}`)
  return c.health ? `${state} · ${t(`containers.health.${c.health}`)}` : state
}
