// 작업표시줄 정렬 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
// 화면 반영은 shell/Taskbar.tsx가 한다(가운데면 .taskbar.centered).
import { isOneOf } from '../api/guards'

export type TaskbarAlign = 'center' | 'left'

export const TASKBAR_ALIGNS: readonly TaskbarAlign[] = ['center', 'left']

export const DEFAULT_TASKBAR_ALIGN: TaskbarAlign = 'center'

// taskbarAlignOf는 지금 쓸 정렬이다. 정하지 않았거나 모르는 값이면 가운데다(관리자 기본값은 없다).
export function taskbarAlignOf(v: unknown): TaskbarAlign {
  return isOneOf(v, TASKBAR_ALIGNS) ? v : DEFAULT_TASKBAR_ALIGN
}
