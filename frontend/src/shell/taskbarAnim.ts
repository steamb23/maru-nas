// 작업표시줄 버튼 등장·퇴장 애니메이션의 설정값과 순서 계산(DOM 없음). 재생은 Taskbar.tsx가 한다.
// 창 애니메이션(windowAnim.ts)과 같은 화면 설정의 애니메이션·동작 줄이기 설정(animationsEnabled)을 따른다.

// 창을 열면 버튼이 너비 0에서 펼쳐지며 나타나고, 닫으면 접히며 사라진다(뒤쪽 버튼이 당겨진다).
export const TASK_ENTER_MS = 200
export const TASK_LEAVE_MS = 180

// withLeaving은 작업표시줄에 그릴 버튼 id 순서를 반환한다. next(windows[] 순서)를 따르되,
// 퇴장 애니메이션 중인 id(leaving)는 prev(직전에 그린 순서)에서 바로 앞에 있던 버튼 뒤에 끼워 제자리에 남긴다.
// 앞에 남은 버튼이 없으면 맨 앞에 둔다.
export function withLeaving(
  prev: readonly string[],
  next: readonly string[],
  leaving: { has: (id: string) => boolean },
): string[] {
  const alive = new Set(next)
  // 앞에 있던 버튼 id(없으면 null) → 그 뒤에 끼울 퇴장 중인 id들
  const after = new Map<string | null, string[]>()
  let anchor: string | null = null
  for (const id of prev) {
    if (alive.has(id)) anchor = id
    else if (leaving.has(id)) after.set(anchor, [...(after.get(anchor) ?? []), id])
  }
  const out = [...(after.get(null) ?? [])]
  for (const id of next) out.push(id, ...(after.get(id) ?? []))
  return out
}
