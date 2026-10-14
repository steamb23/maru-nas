// 입력 없이 delayMs가 지나면 true가 되는 대기 타이머 훅.
//
// - 페이지 안의 클릭·탭(pointerdown: 마우스·터치·펜)마다 false로 돌리고 처음부터 다시 잰다.
//   키 입력, 마우스 이동, 휠은 입력으로 보지 않는다. 연출 중에도 카드가 보이므로 키 입력은 평소처럼 입력란으로 간다.
// - 입력 이벤트를 막지 않는다(preventDefault 없음).
// - 탭이 숨겨지면 false로 돌리고 타이머를 멈춘다. 다시 보이면 처음부터 잰다.
import { useEffect, useState } from 'react'

export function useIdle(enabled: boolean, delayMs: number): boolean {
  const [idle, setIdle] = useState(false)

  useEffect(() => {
    if (!enabled) return
    let timer: number | undefined

    const arm = () => {
      window.clearTimeout(timer)
      timer = document.hidden ? undefined : window.setTimeout(() => setIdle(true), delayMs)
    }
    const onInput = () => {
      setIdle(false)
      arm()
    }
    const onVisibility = () => {
      if (document.hidden) setIdle(false)
      arm()
    }

    const opts = { capture: true, passive: true }
    window.addEventListener('pointerdown', onInput, opts)
    document.addEventListener('visibilitychange', onVisibility)
    arm()
    return () => {
      window.clearTimeout(timer)
      window.removeEventListener('pointerdown', onInput, opts)
      document.removeEventListener('visibilitychange', onVisibility)
      setIdle(false)
    }
  }, [enabled, delayMs])

  return enabled && idle
}
