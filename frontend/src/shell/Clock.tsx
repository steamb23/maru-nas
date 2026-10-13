import { useEffect, useState } from 'react'
import { formatClockDate, formatClockTime, msUntilNextMinute } from './clock'

// Clock은 작업표시줄의 현재 시각(시:분)이다. 분이 바뀌는 순간에 맞춰 갱신하고, 툴팁에 날짜를 보여 준다.
// 탭이 백그라운드에 있다가 돌아오면 타이머가 늦을 수 있어 화면에 보일 때 바로 다시 맞춘다.
export function Clock() {
  const [now, setNow] = useState(() => new Date())

  useEffect(() => {
    const id = window.setTimeout(() => setNow(new Date()), msUntilNextMinute(now.getTime()))
    return () => window.clearTimeout(id)
  }, [now])

  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === 'visible') setNow(new Date())
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [])

  return (
    <time className="taskbar-clock" dateTime={now.toISOString()} title={formatClockDate(now)}>
      {formatClockTime(now)}
    </time>
  )
}
