// 사용률·온도 같은 값을 정상·주의·위험으로 나눈다. 막대·숫자 색에 쓴다.
export type Level = 'normal' | 'warn' | 'crit'

// usageLevel은 사용률(0~100)이 80% 이상이면 주의, 90% 이상이면 위험이다.
export function usageLevel(percent: number): Level {
  if (percent >= 90) return 'crit'
  if (percent >= 80) return 'warn'
  return 'normal'
}
