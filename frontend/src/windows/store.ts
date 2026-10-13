// 창 상태 단일 저장소. 모든 창은 windows[] 하나로 관리한다.
//
// 데스크톱 영역 크기(bounds)는 호출하는 쪽이 넘긴다. DOM에 의존하지 않으므로
// node 환경 테스트에서 그대로 쓸 수 있다.
import { create } from 'zustand'
import { type AppId, getApp } from './apps'
import { type Rect, type Size, cascadePosition, clampPosition } from './geometry'

export interface WindowState {
  id: string
  appId: AppId
  title: string
  // x, y, width, height는 최대화·최소화 중에도 보존되어 복원 시 그대로 쓰인다.
  x: number
  y: number
  width: number
  height: number
  // 클수록 앞. 포커스할 때마다 nextZ를 받아 맨 앞으로 간다.
  z: number
  minimized: boolean
  maximized: boolean
}

interface WindowData {
  windows: WindowState[]
  nextZ: number
  nextId: number
}

interface WindowActions {
  // open은 앱 창을 연다. 이미 열려 있으면 복원하고 맨 앞으로 가져온다. 창 id를 반환한다.
  open: (appId: AppId, bounds: Size) => string
  close: (id: string) => void
  focus: (id: string) => void
  minimize: (id: string) => void
  // restore는 최소화를 풀고 맨 앞으로 가져온다. 최대화 여부는 바꾸지 않는다.
  restore: (id: string) => void
  toggleMaximize: (id: string) => void
  // moveTo는 최대화된 창에는 아무것도 하지 않는다.
  moveTo: (id: string, x: number, y: number, bounds: Size) => void
  // resize는 창 위치·크기를 rect로 바꾼다. 제한은 geometry의 resizeRect가 계산하고 여기서는 반영만 한다.
  // 최대화된 창에는 아무것도 하지 않는다.
  resize: (id: string, rect: Rect) => void
  // clampAll은 데스크톱 영역이 줄었을 때 모든 창을 다시 제한한다.
  clampAll: (bounds: Size) => void
}

export type WindowStore = WindowData & WindowActions

export const initialWindowData: WindowData = { windows: [], nextZ: 1, nextId: 1 }

export const useWindowStore = create<WindowStore>()((set, get) => {
  // Change는 한 창의 새 상태와, 필요하면 갱신할 nextZ다. null이면 바뀐 것이 없다.
  type Change = { win: WindowState; nextZ?: number } | null

  const update = (id: string, fn: (w: WindowState, s: WindowData) => Change) => {
    set((s) => {
      const idx = s.windows.findIndex((w) => w.id === id)
      if (idx < 0) return s
      const change = fn(s.windows[idx], s)
      if (!change) return s
      const windows = s.windows.slice()
      windows[idx] = change.win
      return change.nextZ === undefined ? { windows } : { windows, nextZ: change.nextZ }
    })
  }

  // raise는 patch를 적용하고 창을 맨 앞으로 보낸다. 이미 맨 앞이면 z를 바꾸지 않는다.
  const raise = (w: WindowState, s: WindowData, patch: Partial<WindowState> = {}): Change => {
    const isTop = s.windows.every((o) => o === w || o.z < w.z)
    if (isTop) {
      const same = (Object.keys(patch) as (keyof WindowState)[]).every((k) => patch[k] === w[k])
      return same ? null : { win: { ...w, ...patch } }
    }
    return { win: { ...w, ...patch, z: s.nextZ }, nextZ: s.nextZ + 1 }
  }

  return {
    ...initialWindowData,

    open: (appId, bounds) => {
      const existing = get().windows.find((w) => w.appId === appId)
      if (existing) {
        get().restore(existing.id)
        return existing.id
      }
      const app = getApp(appId)
      const s = get()
      const id = `win-${s.nextId}`
      const pos = cascadePosition(s.windows.length, app.defaultSize, bounds)
      const win: WindowState = {
        id,
        appId,
        title: app.title,
        ...pos,
        ...app.defaultSize,
        z: s.nextZ,
        minimized: false,
        maximized: false,
      }
      set({ windows: [...s.windows, win], nextZ: s.nextZ + 1, nextId: s.nextId + 1 })
      return id
    },

    close: (id) => set((s) => ({ windows: s.windows.filter((w) => w.id !== id) })),

    focus: (id) => update(id, (w, s) => raise(w, s)),

    minimize: (id) => update(id, (w) => (w.minimized ? null : { win: { ...w, minimized: true } })),

    restore: (id) => update(id, (w, s) => raise(w, s, { minimized: false })),

    toggleMaximize: (id) => update(id, (w, s) => raise(w, s, { maximized: !w.maximized })),

    moveTo: (id, x, y, bounds) =>
      update(id, (w) => {
        if (w.maximized) return null
        const pos = clampPosition({ ...w, x, y }, bounds)
        return pos.x === w.x && pos.y === w.y ? null : { win: { ...w, ...pos } }
      }),

    resize: (id, { x, y, width, height }) =>
      update(id, (w) => {
        if (w.maximized) return null
        if (x === w.x && y === w.y && width === w.width && height === w.height) return null
        return { win: { ...w, x, y, width, height } }
      }),

    clampAll: (bounds) =>
      set((s) => {
        let changed = false
        const windows = s.windows.map((w) => {
          const pos = clampPosition(w, bounds)
          if (pos.x === w.x && pos.y === w.y) return w
          changed = true
          return { ...w, ...pos }
        })
        return changed ? { windows } : s
      }),
  }
})

// selectActiveId는 최소화되지 않은 창 중 맨 앞 창의 id를 반환한다. 없으면 null.
export function selectActiveId(s: WindowData): string | null {
  let top: WindowState | null = null
  for (const w of s.windows) {
    if (!w.minimized && (top === null || w.z > top.z)) top = w
  }
  return top?.id ?? null
}
