// 저장소에서 지워진(닫힌) 창의 마지막 상태를 닫기 애니메이션이 끝날 때까지 보관한다.
// 창(Desktop)과 작업표시줄 버튼(Taskbar)이 함께 쓴다.
import { useCallback, useEffect, useState } from 'react'
import { type WindowState, type WindowStore, useWindowStore } from '../windows/store'
import { animationsEnabled } from './useWindowAnimation'

// useClosingWindows는 닫힌 창 id → snapshot(창, 지우기 직전 저장소) 값을 돌려준다. done(id)을 부르면 뺀다.
// 이 갱신은 창을 지운 갱신과 같은 렌더에 반영되어, 그리는 쪽에서 창이 한 번도 빠지지 않는다.
// 애니메이션이 꺼져 있으면 보관하지 않아 바로 사라진다. snapshot은 바뀌지 않는 함수여야 한다.
export function useClosingWindows<T>(
  snapshot: (win: WindowState, prev: WindowStore) => T,
): [ReadonlyMap<string, T>, (id: string) => void] {
  const [closing, setClosing] = useState<ReadonlyMap<string, T>>(() => new Map())

  useEffect(
    () =>
      useWindowStore.subscribe((s, prev) => {
        if (s.windows === prev.windows || !animationsEnabled()) return
        const alive = new Set(s.windows.map((w) => w.id))
        const removed = prev.windows.filter((w) => !alive.has(w.id))
        if (removed.length === 0) return
        setClosing((m) => new Map([...m, ...removed.map((w) => [w.id, snapshot(w, prev)] as const)]))
      }),
    [snapshot],
  )

  const done = useCallback(
    (id: string) =>
      setClosing((m) => {
        if (!m.has(id)) return m
        const next = new Map(m)
        next.delete(id)
        return next
      }),
    [],
  )

  return [closing, done]
}
