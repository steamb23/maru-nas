// 창 상태 단일 저장소. 모든 창은 windows[] 하나로 관리한다.
//
// 데스크톱 영역 크기(bounds)는 호출하는 쪽이 넘긴다. DOM에 의존하지 않으므로
// node 환경 테스트에서 그대로 쓸 수 있다.
import { create } from 'zustand'
import { type WindowAppId, getWindowApp, isMultiInstance } from './apps'
import { type Rect, type Size, cascadePosition, clampPosition } from './geometry'
import { DEFAULT_TILE_SPLIT, type TileSide, type TileZone, splitForTiling, tilePartner } from './tiling'

export interface WindowState {
  id: string
  appId: WindowAppId
  // 같은 앱의 창 번호(1부터). 열 때 그 앱의 열린 창이 쓰지 않는 가장 작은 수를 받고 바뀌지 않는다.
  // 창을 여러 개 여는 앱(multiInstance)의 둘째 창부터 제목에 붙는다(windowTitle).
  instance: number
  // x, y, width, height는 최대화·타일링·최소화 중에도 보존되어 복원 시 그대로 쓰인다.
  x: number
  y: number
  width: number
  height: number
  // 클수록 앞. 포커스할 때마다 nextZ를 받아 맨 앞으로 간다.
  z: number
  minimized: boolean
  // maximized와 tile은 동시에 켜지지 않는다. 둘 다 꺼져 있으면 x, y, width, height에 놓인다.
  maximized: boolean
  tile: TileSide | null
}

interface WindowData {
  windows: WindowState[]
  nextZ: number
  nextId: number
  // 좌우 타일 그룹의 왼쪽 비율. 좌우 타일 창이 모두 이 값 하나를 공유하므로 바꾸면 함께 다시 배치된다.
  tileSplit: number
}

interface WindowActions {
  // open은 앱 창을 연다. 이미 열려 있으면 복원하고 맨 앞으로 가져온다(여러 개면 가장 앞에 있던 창). 창 id를 반환한다.
  open: (appId: WindowAppId, bounds: Size) => string
  // openNew는 창을 여러 개 여는 앱(multiInstance)이면 늘 새 창을 연다. 그 밖의 앱은 open과 같다.
  openNew: (appId: WindowAppId, bounds: Size) => string
  close: (id: string) => void
  // closeMany는 여러 창을 한 번에 닫는다. 없는 id는 무시한다.
  closeMany: (ids: readonly string[]) => void
  focus: (id: string) => void
  minimize: (id: string) => void
  // restore는 최소화를 풀고 맨 앞으로 가져온다. 최대화 여부는 바꾸지 않는다.
  restore: (id: string) => void
  // toggleMaximize는 타일링된 창을 최대화하면 타일링을 푼다. 다시 복원하면 타일링 전 위치·크기로 돌아간다.
  toggleMaximize: (id: string) => void
  // tile은 창을 타일링 영역 zone에 놓고 맨 앞으로 가져온다. max는 최대화다.
  // 좌우에 놓을 때 반대쪽에 타일 창이 없으면 새 그룹이므로 비율을 반반으로 되돌린다.
  tile: (id: string, zone: TileZone) => void
  // setTileSplit은 좌우 타일 그룹의 비율을 바꾼다. 제한은 tiling의 clampSplit이 계산하고 여기서는 반영만 한다.
  setTileSplit: (split: number) => void
  // raiseGroup은 창을 복원해 맨 앞으로 가져오고, 타일 그룹의 짝 창이 있으면 그 창도 복원해 바로 뒤에 둔다.
  // 짝 창이 없으면 restore와 같다.
  raiseGroup: (id: string) => void
  // detachAt은 최대화·타일링을 풀고 창을 (x, y)로 옮긴다. 크기는 최대화·타일링 전 크기로 돌아간다.
  // 최대화·타일링된 창을 끌어 떼어 낼 때 쓴다. 최대화도 타일링도 아닌 창에는 아무것도 하지 않는다.
  detachAt: (id: string, x: number, y: number, bounds: Size) => void
  // moveTo는 최대화·타일링된 창에는 아무것도 하지 않는다(보존한 위치를 지킨다).
  moveTo: (id: string, x: number, y: number, bounds: Size) => void
  // resize는 창 위치·크기를 rect로 바꾼다. 제한은 geometry의 resizeRect가 계산하고 여기서는 반영만 한다.
  // 최대화·타일링된 창에는 아무것도 하지 않는다.
  resize: (id: string, rect: Rect) => void
  // clampAll은 데스크톱 영역이 줄었을 때 모든 창을 다시 제한한다.
  clampAll: (bounds: Size) => void
  // reorder는 창을 windows[]의 toIndex 자리로 옮긴다. windows[] 순서가 작업표시줄 순서다.
  reorder: (id: string, toIndex: number) => void
}

export type WindowStore = WindowData & WindowActions

export const initialWindowData: WindowData = { windows: [], nextZ: 1, nextId: 1, tileSplit: DEFAULT_TILE_SPLIT }

// isFree는 창이 최대화·타일링되지 않아 x, y, width, height에 놓여 있는지 확인한다.
const isFree = (w: WindowState) => !w.maximized && w.tile === null

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

  // openWindow는 앱의 새 창을 맨 앞에 열고 id를 반환한다.
  const openWindow = (appId: WindowAppId, bounds: Size): string => {
    const app = getWindowApp(appId)
    const s = get()
    const id = `win-${s.nextId}`
    const pos = cascadePosition(s.windows.length, app.defaultSize, bounds)
    const win: WindowState = {
      id,
      appId,
      instance: nextInstance(s.windows, appId),
      ...pos,
      ...app.defaultSize,
      z: s.nextZ,
      minimized: false,
      maximized: false,
      tile: null,
    }
    set({ windows: [...s.windows, win], nextZ: s.nextZ + 1, nextId: s.nextId + 1 })
    return id
  }

  return {
    ...initialWindowData,

    open: (appId, bounds) => {
      const existing = topWindowOf(get().windows, appId)
      if (existing) {
        get().restore(existing.id)
        return existing.id
      }
      return openWindow(appId, bounds)
    },

    openNew: (appId, bounds) => (isMultiInstance(appId) ? openWindow(appId, bounds) : get().open(appId, bounds)),
    close: (id) => set((s) => ({ windows: s.windows.filter((w) => w.id !== id) })),

    closeMany: (ids) =>
      set((s) => {
        const drop = new Set(ids)
        const windows = s.windows.filter((w) => !drop.has(w.id))
        return windows.length === s.windows.length ? s : { windows }
      }),

    focus: (id) => update(id, (w, s) => raise(w, s)),

    minimize: (id) => update(id, (w) => (w.minimized ? null : { win: { ...w, minimized: true } })),

    restore: (id) => update(id, (w, s) => raise(w, s, { minimized: false })),

    toggleMaximize: (id) => update(id, (w, s) => raise(w, s, { maximized: !w.maximized, tile: null })),

    tile: (id, zone) => {
      const s = get()
      if (zone !== 'max') {
        const split = splitForTiling(s.windows, id, zone, s.tileSplit)
        if (split !== s.tileSplit) set({ tileSplit: split })
      }
      update(id, (w, s) =>
        raise(w, s, zone === 'max' ? { maximized: true, tile: null } : { maximized: false, tile: zone }),
      )
    },

    setTileSplit: (split) => set((s) => (s.tileSplit === split ? s : { tileSplit: split })),

    raiseGroup: (id) => {
      const partner = tilePartner(get().windows, id)
      if (partner) get().restore(partner.id)
      get().restore(id)
    },

    detachAt: (id, x, y, bounds) =>
      update(id, (w) => {
        if (isFree(w)) return null
        return { win: { ...w, maximized: false, tile: null, ...clampPosition({ ...w, x, y }, bounds) } }
      }),

    moveTo: (id, x, y, bounds) =>
      update(id, (w) => {
        if (!isFree(w)) return null
        const pos = clampPosition({ ...w, x, y }, bounds)
        return pos.x === w.x && pos.y === w.y ? null : { win: { ...w, ...pos } }
      }),

    resize: (id, { x, y, width, height }) =>
      update(id, (w) => {
        if (!isFree(w)) return null
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

    reorder: (id, toIndex) =>
      set((s) => {
        const from = s.windows.findIndex((w) => w.id === id)
        if (from < 0) return s
        const to = Math.min(Math.max(Math.trunc(toIndex), 0), s.windows.length - 1)
        return to === from ? s : { windows: moveItem(s.windows, from, to) }
      }),
  }
})

// topWindowOf는 앱의 창 중 맨 앞(z가 가장 큰) 창이다. 최소화된 창도 포함한다. 없으면 undefined.
export function topWindowOf(windows: readonly WindowState[], appId: WindowAppId): WindowState | undefined {
  let top: WindowState | undefined
  for (const w of windows) {
    if (w.appId === appId && (top === undefined || w.z > top.z)) top = w
  }
  return top
}

// nextInstance는 앱의 새 창이 받을 번호다. 그 앱의 열린 창이 쓰지 않는 가장 작은 수(1부터)라
// 창을 닫으면 그 번호를 다시 쓴다.
export function nextInstance(windows: readonly WindowState[], appId: WindowAppId): number {
  const used = new Set(windows.filter((w) => w.appId === appId).map((w) => w.instance))
  let n = 1
  while (used.has(n)) n++
  return n
}

// moveItem은 arr[from]을 to 자리로 옮긴 새 배열을 반환한다.
export function moveItem<T>(arr: readonly T[], from: number, to: number): T[] {
  const out = arr.slice()
  const [item] = out.splice(from, 1)
  out.splice(to, 0, item)
  return out
}

// stableOrder는 화면에 그릴 창 id 순서를 반환한다. prev의 순서를 유지한 채 닫힌 id는 빼고
// 새로 열린 id는 next의 순서대로 뒤에 붙인다. 작업표시줄 순서(windows[])를 바꿔도 창 DOM이
// 다시 붙지 않게 해서, 창 안의 스크롤 위치나 iframe이 초기화되지 않게 한다.
// keep에 든 id는 닫혔어도 제자리에 남긴다(닫기 애니메이션 중인 창).
export function stableOrder(
  prev: readonly string[],
  next: readonly string[],
  keep: { has: (id: string) => boolean } = new Set(),
): string[] {
  const alive = new Set(next)
  const kept = prev.filter((id) => alive.has(id) || keep.has(id))
  const seen = new Set(kept)
  return [...kept, ...next.filter((id) => !seen.has(id))]
}

// selectActiveId는 최소화되지 않은 창 중 맨 앞 창의 id를 반환한다. 없으면 null.
export function selectActiveId(s: WindowData): string | null {
  let top: WindowState | null = null
  for (const w of s.windows) {
    if (!w.minimized && (top === null || w.z > top.z)) top = w
  }
  return top?.id ?? null
}
