// 현황 표시용 포매터. 숫자와 시간 단위는 Intl로 locale에 맞춰 표시하며, locale은 호출하는 쪽이 넘긴다.

// 단위 기호는 언어와 상관없이 IEC 이진 단위를 쓴다.
const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB']

// formatBytes는 1024 단위로 바꿔 소수점 한 자리까지 표시한다. 예: 5.3 GiB
export function formatBytes(bytes: number, locale: string): string {
  let v = Math.max(0, bytes)
  let i = 0
  while (v >= 1024 && i < BYTE_UNITS.length - 1) {
    v /= 1024
    i++
  }
  const digits = i === 0 ? 0 : 1
  const n = new Intl.NumberFormat(locale, { minimumFractionDigits: digits, maximumFractionDigits: digits })
  return `${n.format(v)} ${BYTE_UNITS[i]}`
}

// UnitDisplay는 단위 표기 길이다. narrow는 위젯처럼 좁은 곳에 쓴다(en "3d", ko "3일").
export type UnitDisplay = 'long' | 'narrow'

// formatUnit은 수와 단위를 locale에 맞게 표시한다. 예: ko "30초", en "30 seconds"
export function formatUnit(
  value: number,
  unit: 'day' | 'hour' | 'minute' | 'second',
  locale: string,
  unitDisplay: UnitDisplay = 'long',
): string {
  return new Intl.NumberFormat(locale, { style: 'unit', unit, unitDisplay }).format(value)
}

// formatUptime은 초를 ko "3일 4시간 12분", en "3 days 4 hours 12 minutes" 형태로 바꾼다.
// 앞쪽의 0인 단위는 생략한다. unitDisplay가 narrow면 en "3d 4h 12m"처럼 짧게 쓴다.
export function formatUptime(seconds: number, locale: string, unitDisplay: UnitDisplay = 'long'): string {
  const totalMin = Math.floor(Math.max(0, seconds) / 60)
  const days = Math.floor(totalMin / 1440)
  const hours = Math.floor((totalMin % 1440) / 60)
  const minutes = totalMin % 60
  const parts: string[] = []
  if (days > 0) parts.push(formatUnit(days, 'day', locale, unitDisplay))
  if (days > 0 || hours > 0) parts.push(formatUnit(hours, 'hour', locale, unitDisplay))
  parts.push(formatUnit(minutes, 'minute', locale, unitDisplay))
  return new Intl.ListFormat(locale, { type: 'unit', style: 'narrow' }).format(parts)
}

// formatPercent는 0~100 값을 소수점 한 자리 백분율로 표시한다.
export function formatPercent(percent: number, locale: string): string {
  return new Intl.NumberFormat(locale, { style: 'percent', minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(
    percent / 100,
  )
}

// percentOf는 part/total을 0~100으로 반환한다. total이 0이면 0.
export function percentOf(part: number, total: number): number {
  return total > 0 ? Math.min(100, Math.max(0, (part / total) * 100)) : 0
}

// formatRate는 초당 바이트를 1024 단위로 표시한다. 예: 1.2 MiB/s
export function formatRate(bytesPerSecond: number, locale: string): string {
  return `${formatBytes(bytesPerSecond, locale)}/s`
}

// formatLinkSpeed는 링크 속도(Mbps)를 표시한다. 1000 이상은 Gbps로 바꾼다. 예: 2.5 Gbps, 100 Mbps
export function formatLinkSpeed(mbps: number, locale: string): string {
  const n = new Intl.NumberFormat(locale, { maximumFractionDigits: 1 })
  return mbps >= 1000 ? `${n.format(mbps / 1000)} Gbps` : `${n.format(mbps)} Mbps`
}

// formatCelsius는 섭씨 온도를 정수로 표시한다. 예: 45°C
export function formatCelsius(celsius: number, locale: string): string {
  return new Intl.NumberFormat(locale, { style: 'unit', unit: 'celsius', maximumFractionDigits: 0 }).format(celsius)
}

// formatDateTime은 RFC 3339 시각을 짧은 날짜·시각으로 표시한다. 읽을 수 없으면 원문을 그대로 준다.
export function formatDateTime(iso: string, locale: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'short' }).format(d)
}

// formatTime은 RFC 3339 시각을 짧은 시각으로 표시한다. 오늘이 아니면 날짜도 붙인다.
export function formatTime(iso: string, locale: string, now = new Date()): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const sameDay = d.toDateString() === now.toDateString()
  return new Intl.DateTimeFormat(
    locale,
    sameDay ? { timeStyle: 'short' } : { month: 'numeric', day: 'numeric', hour: 'numeric', minute: '2-digit' },
  ).format(d)
}
