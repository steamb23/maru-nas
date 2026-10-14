// 입력 없이 delayMs가 지나면 그 시각(performance.now 기준)을 돌려주는 대기 타이머 훅. 대기 중이 아니면 null.
//
// - 페이지 안의 클릭·탭(pointerdown: 마우스·터치·펜)마다 null로 돌리고 처음부터 다시 잰다.
//   키 입력, 마우스 이동, 휠은 입력으로 보지 않는다. 연출 중에도 카드가 보이므로 키 입력은 평소처럼 입력란으로 간다.
// - 입력 이벤트를 막지 않는다(preventDefault 없음).
// - 탭이 숨겨져도 계속 잰다. OMV의 매트릭스 효과처럼, 다른 일을 하다 돌아오면 연출이 이미 재생 중이어야 하기 때문이다.
//   숨겨진 탭의 타이머는 늦게 불릴 수 있으므로 다시 보일 때 한 번 더 확인한다.
import { useEffect, useState } from 'react'

export function useIdle(enabled: boolean, delayMs: number): number | null {
  const [idleSince, setIdleSince] = useState<number | null>(null)

  useEffect(() => {
    if (!enabled) return
    let timer: number | undefined
    let lastInput = performance.now()

    // arm은 마지막 입력에서 delayMs가 되는 시점에 대기 시작 시각을 알린다. 이미 지났으면 바로 알린다.
    const arm = () => {
      window.clearTimeout(timer)
      const since = lastInput + delayMs
      const remaining = since - performance.now()
      if (remaining <= 0) setIdleSince(since)
      else timer = window.setTimeout(() => setIdleSince(since), remaining)
    }
    const onInput = () => {
      lastInput = performance.now()
      setIdleSince(null)
      arm()
    }
    const onVisibility = () => {
      if (!document.hidden) arm()
    }

    const opts = { capture: true, passive: true }
    window.addEventListener('pointerdown', onInput, opts)
    document.addEventListener('visibilitychange', onVisibility)
    arm()
    return () => {
      window.clearTimeout(timer)
      window.removeEventListener('pointerdown', onInput, opts)
      document.removeEventListener('visibilitychange', onVisibility)
      setIdleSince(null)
    }
  }, [enabled, delayMs])

  return enabled ? idleSince : null
}
