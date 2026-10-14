// 메뉴(프로필 메뉴, 컨텍스트 메뉴)의 키보드 이동 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.

// nextIndex는 화살표 키로 옮길 항목 위치다. 끝에서는 반대쪽 끝으로 넘어간다.
export function nextIndex(current: number, count: number, key: string): number {
  if (count === 0) return -1
  switch (key) {
    case 'ArrowDown':
      return (current + 1 + count) % count
    case 'ArrowUp':
      return current < 0 ? count - 1 : (current - 1 + count) % count
    case 'Home':
      return 0
    case 'End':
      return count - 1
  }
  return current
}
