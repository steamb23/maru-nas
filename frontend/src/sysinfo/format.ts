// 시스템 정보 표시용 포매터.

const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB']

// formatBytes는 1024 단위로 바꿔 소수점 한 자리까지 표시한다. 예: 5.3 GiB
export function formatBytes(bytes: number): string {
  let v = Math.max(0, bytes)
  let i = 0
  while (v >= 1024 && i < BYTE_UNITS.length - 1) {
    v /= 1024
    i++
  }
  return i === 0 ? `${Math.round(v)} B` : `${v.toFixed(1)} ${BYTE_UNITS[i]}`
}

// formatUptime은 초를 "3일 4시간 12분" 형태로 바꾼다. 앞쪽의 0인 단위는 생략한다.
export function formatUptime(seconds: number): string {
  const totalMin = Math.floor(Math.max(0, seconds) / 60)
  const days = Math.floor(totalMin / 1440)
  const hours = Math.floor((totalMin % 1440) / 60)
  const minutes = totalMin % 60
  const parts: string[] = []
  if (days > 0) parts.push(`${days}일`)
  if (days > 0 || hours > 0) parts.push(`${hours}시간`)
  parts.push(`${minutes}분`)
  return parts.join(' ')
}

// formatPercent는 0~100 값을 소수점 한 자리 백분율로 표시한다.
export function formatPercent(percent: number): string {
  return `${percent.toFixed(1)}%`
}

// percentOf는 part/total을 0~100으로 반환한다. total이 0이면 0.
export function percentOf(part: number, total: number): number {
  return total > 0 ? Math.min(100, Math.max(0, (part / total) * 100)) : 0
}
