// 바탕화면 바로가기 목록 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 저장 목록에는 지금 사용자에게 보이지 않는 앱(일반 사용자의 관리자 전용 앱)과 지금 없는 앱(꺼지거나 지운 애드온
// 패키지, 아직 목록을 받지 못한 패키지)도 남긴다. 역할이 바뀌거나 패키지를 다시 켜도 바로가기를 잃지 않게 하기
// 위해서다(위젯 배치와 같은 규칙). 화면 순서와 끌어 놓을 자리는 보이는 목록 기준이며, 무엇이 보이는지는 부르는 쪽이
// include로 정한다(visibleShortcuts는 지금 없는 앱을 늘 뺀다).
import type { Shortcut } from '../api/types.gen'
import { type AppId, isAppId } from '../windows/apps'

// 처음 쓰는 사용자(바로가기를 한 번도 저장하지 않음)의 바로가기
const DEFAULT_SHORTCUTS: readonly string[] = ['system-info', 'files', 'storage']

export function defaultShortcuts(): AppId[] {
  return DEFAULT_SHORTCUTS.filter(isAppId)
}

// MAX_SHORTCUTS는 저장할 수 있는 바로가기 수다(백엔드 addons.MaxShortcuts와 같다).
export const MAX_SHORTCUTS = 64

// SHORTCUT_ID는 백엔드가 받는 앱 id 형식이다(addons.ValidateShortcuts).
const SHORTCUT_ID = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/

// storedShortcuts는 저장된 바로가기에서 형식이 틀린 id와 중복을 뺀다(중복이면 앞의 것을 남긴다). 지금 없는 앱은
// 지우지 않는다. 다음 저장 때 사라지면 다시 켠 패키지의 바로가기가 돌아오지 않는다.
export function storedShortcuts(stored: readonly Shortcut[]): AppId[] {
  const apps = new Set<string>()
  for (const s of stored) if (s.app.length <= 64 && SHORTCUT_ID.test(s.app)) apps.add(s.app)
  return [...apps] as AppId[]
}

export function visibleShortcuts(saved: readonly AppId[], include: (id: AppId) => boolean): AppId[] {
  return saved.filter((id) => isAppId(id) && include(id))
}

// capShortcuts는 목록이 MAX_SHORTCUTS를 넘으면 지금 없는 앱부터(앞쪽부터) 뺀다. 그래도 넘으면 뒤를 자른다.
export function capShortcuts(list: readonly AppId[]): AppId[] {
  let out = [...list]
  while (out.length > MAX_SHORTCUTS) {
    const i = out.findIndex((id) => !isAppId(id))
    if (i < 0) break
    out.splice(i, 1)
  }
  if (out.length > MAX_SHORTCUTS) out = out.slice(0, MAX_SHORTCUTS)
  return out
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
  const visible = visibleShortcuts(rest, include)
  if (to >= visible.length) return capShortcuts([...rest, id])
  const at = rest.indexOf(visible[Math.max(0, to)])
  return capShortcuts([...rest.slice(0, at), id, ...rest.slice(at)])
}

export function removeShortcut(saved: readonly AppId[], id: AppId): AppId[] {
  return saved.filter((s) => s !== id)
}
