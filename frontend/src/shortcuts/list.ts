// 바탕화면 바로가기 목록 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 저장 목록에는 지금 사용자에게 보이지 않는 앱(일반 사용자의 관리자 전용 앱)도 남긴다. 역할이 바뀌어도
// 바로가기를 잃지 않게 하기 위해서다(위젯 배치와 같은 규칙). 화면 순서와 끌어 놓을 자리는 보이는 목록
// 기준이며, 무엇이 보이는지는 부르는 쪽이 include로 정한다.
import type { Shortcut } from '../api/types.gen'
import { type AppId, isAppId } from '../windows/apps'

// 처음 쓰는 사용자(바로가기를 한 번도 저장하지 않음)의 바로가기
const DEFAULT_SHORTCUTS: readonly string[] = ['system-info', 'files', 'storage']

export function defaultShortcuts(): AppId[] {
  return DEFAULT_SHORTCUTS.filter(isAppId)
}

// knownShortcuts는 저장된 바로가기에서 지금 빌드에 없는 앱(지워진 애드온 등)과 중복을 뺀다.
// 중복이면 앞의 것을 남긴다.
export function knownShortcuts(stored: readonly Shortcut[]): AppId[] {
  const apps = new Set<AppId>()
  for (const s of stored) if (isAppId(s.app)) apps.add(s.app)
  return [...apps]
}

export function visibleShortcuts(saved: readonly AppId[], include: (id: AppId) => boolean): AppId[] {
  return saved.filter(include)
}

// placeShortcut은 id를 보이는 목록의 to번째 자리에 놓은 저장 목록을 반환한다. 이미 있으면 옮기고, 없으면
// 더한다. to가 보이는 목록 길이 이상이면 맨 끝이다. 보이지 않는 앱은 저장 목록에 남고 서로의 순서를 지킨다.
export function placeShortcut(
  saved: readonly AppId[],
  id: AppId,
  to: number,
  include: (id: AppId) => boolean,
): AppId[] {
  const rest = saved.filter((s) => s !== id)
  const visible = rest.filter(include)
  if (to >= visible.length) return [...rest, id]
  const at = rest.indexOf(visible[Math.max(0, to)])
  return [...rest.slice(0, at), id, ...rest.slice(at)]
}

export function removeShortcut(saved: readonly AppId[], id: AppId): AppId[] {
  return saved.filter((s) => s !== id)
}
