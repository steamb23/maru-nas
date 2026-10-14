import { type Level, usageLevel } from './levels'

interface Props {
  // 0~100. null이면 빈 막대.
  percent: number | null
  // 없으면 사용률로 정한다(usageLevel).
  level?: Level
}

// UsageBar는 사용률 막대다. 값은 옆의 글자로 따로 보여 주므로 화면 낭독기에는 숨긴다.
export function UsageBar({ percent, level }: Props) {
  const lv = level ?? (percent === null ? 'normal' : usageLevel(percent))
  return (
    <div className="meter" aria-hidden="true">
      <div className={`meter-fill ${lv}`} style={{ width: `${percent ?? 0}%` }} />
    </div>
  )
}
