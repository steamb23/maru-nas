// 격자(행 우선으로 채운 cols열) 안의 키보드 이동 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
// "모든 앱" 화면의 앱 격자가 쓴다.

// gridNextIndex는 화살표·Home·End 키로 옮길 항목 위치다. 가장자리에서 넘어가지 않는다.
// 아래 줄이 덜 차 바로 아래에 항목이 없으면 마지막 항목으로 간다. 첫 줄에서 위 화살표는 -1(격자 위의 검색
// 상자로 가라는 뜻)이다. 다른 키면 current를 그대로 돌려준다.
export function gridNextIndex(current: number, count: number, cols: number, key: string): number {
  if (count === 0) return -1
  const c = Math.max(1, Math.floor(cols))
  const i = Math.min(Math.max(current, 0), count - 1)
  switch (key) {
    case 'ArrowLeft':
      return Math.max(0, i - 1)
    case 'ArrowRight':
      return Math.min(count - 1, i + 1)
    case 'ArrowUp':
      return i - c < 0 ? -1 : i - c
    case 'ArrowDown':
      if (i + c < count) return i + c
      return Math.floor(i / c) < Math.floor((count - 1) / c) ? count - 1 : i
    case 'Home':
      return 0
    case 'End':
      return count - 1
  }
  return current
}
