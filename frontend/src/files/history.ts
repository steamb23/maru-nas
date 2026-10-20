// 파일 탐색기의 뒤로·앞으로 기록. 창마다 하나다. DOM에 의존하지 않아 node 환경에서 테스트한다.

export interface History {
  entries: readonly string[]
  index: number
}

// MAX_HISTORY는 기억하는 폴더 수다. 넘으면 오래된 것부터 버린다.
export const MAX_HISTORY = 100

export function startHistory(path: string): History {
  return { entries: [path], index: 0 }
}

export function current(h: History): string {
  return h.entries[h.index]
}

// visit은 새 폴더로 간다. 앞으로 갈 기록은 버린다. 같은 폴더면 그대로다.
export function visit(h: History, path: string): History {
  if (current(h) === path) return h
  const entries = [...h.entries.slice(0, h.index + 1), path].slice(-MAX_HISTORY)
  return { entries, index: entries.length - 1 }
}

// replaceCurrent는 지금 기록을 바꾼다(없어진 폴더에서 상위 폴더로 물러날 때).
export function replaceCurrent(h: History, path: string): History {
  const entries = [...h.entries]
  entries[h.index] = path
  return { entries, index: h.index }
}

export const canBack = (h: History): boolean => h.index > 0
export const canForward = (h: History): boolean => h.index < h.entries.length - 1

export function back(h: History): History {
  return canBack(h) ? { entries: h.entries, index: h.index - 1 } : h
}

export function forward(h: History): History {
  return canForward(h) ? { entries: h.entries, index: h.index + 1 } : h
}
