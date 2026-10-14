// 입력 없이 delayMs가 지나면 true가 되는 대기 타이머 훅.
//
// - 키 입력과 클릭·탭(pointerdown: 마우스·터치·펜)마다 false로 돌리고 처음부터 다시 잰다.
//   마우스 이동과 휠은 입력으로 보지 않는다.
// - 입력 이벤트를 막지 않는다(preventDefault 없음). 연출 중에 친 키는 그대로 포커스된 입력란으로 간다.
// - 탭이 숨겨지면 false로 돌리고 타이머를 멈춘다. 다시 보이면 처음부터 잰다.
import { useEffect, useState } from 'react'

const INPUT_EVENTS = ['keydown', 'pointerdown'] as const

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
    for (const type of INPUT_EVENTS) window.addEventListener(type, onInput, opts)
    document.addEventListener('visibilitychange', onVisibility)
    arm()
    return () => {
      window.clearTimeout(timer)
      for (const type of INPUT_EVENTS) window.removeEventListener(type, onInput, opts)
      document.removeEventListener('visibilitychange', onVisibility)
      setIdle(false)
    }
  }, [enabled, delayMs])

  return enabled && idle
}
