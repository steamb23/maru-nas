// 작업표시줄 시계의 서식과 갱신 시점 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.

const MINUTE_MS = 60_000

// msUntilNextMinute는 다음 분 경계(초 0)까지 남은 ms다. 분 경계 바로 그 시각이면 1분 뒤를 가리킨다.
// 시간대 오프셋은 분 단위이므로 epoch 기준으로 계산해도 현지 시각의 분 경계와 맞는다.
export function msUntilNextMinute(nowMs: number): number {
  return MINUTE_MS - (((nowMs % MINUTE_MS) + MINUTE_MS) % MINUTE_MS)
}

// formatClockTime은 작업표시줄에 보여 줄 시:분이다. 12/24시간 표기는 로케일을 따른다.
export function formatClockTime(d: Date, locale?: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(locale, { hour: 'numeric', minute: '2-digit', timeZone }).format(d)
}

// formatClockDate는 시계 툴팁에 보여 줄 날짜다.
export function formatClockDate(d: Date, locale?: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'full', timeZone }).format(d)
}
