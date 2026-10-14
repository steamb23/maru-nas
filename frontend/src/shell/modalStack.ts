// 한 범위(창·데스크톱) 안에서 열린 모달의 쌓임 순서. DOM에 의존하지 않아 node 환경에서 테스트한다.
// 스택은 모달 key를 연 순서대로 담고, 맨 뒤가 맨 위다.

// pushLayer는 key를 맨 위에 올린다. 이미 있으면 그대로 둔다.
export function pushLayer(stack: readonly number[], key: number): readonly number[] {
  return stack.includes(key) ? stack : [...stack, key]
}

// popLayer는 key를 뺀다. 없으면 그대로 둔다.
export function popLayer(stack: readonly number[], key: number): readonly number[] {
  return stack.includes(key) ? stack.filter((k) => k !== key) : stack
}

// isUnder는 key가 다른 모달 아래에 깔려 있는지다. 스택에 아직 없는 key(막 열려 올리기 전)는 깔리지 않은 것으로 본다.
export function isUnder(stack: readonly number[], key: number): boolean {
  const i = stack.indexOf(key)
  return i >= 0 && i < stack.length - 1
}
