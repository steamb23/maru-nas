// 파일 탐색기의 자세히 목록. 보이는 줄만 그리고(virtual.ts), 정렬·찾기를 적용한 보기에서 보이는 범위를 PAGE_SIZE줄씩
// 받아 둔다(pageCache.ts). 아직 받지 않은 줄은 자리 표시로 그린다. 스크롤하는 동안 위치 말풍선을 보인다.
// 여러 줄을 고를 수 있고(selection.ts), 고른 줄을 마우스·펜으로 끌어 다른 폴더(data-drop-path가 있는 줄·경로·사이드바,
// 다른 탐색기 창의 목록 포함)에 놓으면 옮긴다(다른 디스크면 복사한다, 창이 dropMode로 정한다).
import {
  type KeyboardEvent,
  type PointerEvent,
  type Ref,
  type UIEvent,
  useCallback,
  useEffect,
  useId,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, fetchListingRange } from '../api/client'
import type { FileItem, FilesListing } from '../api/types.gen'
import { formatBytes, formatDateTime } from '../addons/common/format'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import type { ContextMenuItem } from '../shell/contextMenu'
import { useContextMenuTrigger } from '../shell/useContextMenuTrigger'
import type { DropMode } from './copyJobs'
import { KIND_KEYS, isFolderLike, kindIcon, kindOf } from './kinds'
import { PAGE_SIZE, PageCache } from './pageCache'
import { joinPath, within } from './paths'
import {
  NO_SELECTION,
  type Selection,
  extendTo,
  isSelected,
  selectAll,
  selectOne,
  selectedCount,
  toggleAt,
} from './selection'
import { ROW_HEIGHT, indexAt, layout, scrollToShow } from './virtual'

export type SortKey = 'name' | 'mtime' | 'size'

export interface View {
  sort: SortKey
  desc: boolean
  query: string
}

// FileListHandle은 창이 목록에 시키는 일이다.
export interface FileListHandle {
  // items는 고른 자리의 항목이다(앞에서부터). 받지 않은 줄은 이름만 받아 채운다(정보 없이 state가 pending).
  items: (sel: Selection) => Promise<FileItem[]>
  // reveal은 index 줄이 보이게 스크롤한다.
  reveal: (index: number) => void
}

interface Props {
  listing: FilesListing
  view: View
  active: boolean
  selection: Selection
  // onSelection은 고르기가 바뀔 때다. item은 위치(focus) 줄의 항목이다(받지 않았으면 null).
  onSelection: (sel: Selection, item: FileItem | null) => void
  // onOpen은 두 번 누르기·Enter다(폴더면 들어가고 파일이면 정보 패널).
  onOpen: (item: FileItem) => void
  onUp: () => void
  onSort: (key: SortKey) => void
  onTotal: (total: number) => void
  // onExpired는 목록이 서버에서 사라졌을 때(410), onSortPending은 날짜·크기순 정보가 모자랄 때(409)다.
  onExpired: () => void
  onSortPending: () => void
  onError: (err: unknown) => void
  // menuItems는 줄(item)이나 빈 곳(null)의 메뉴다. sel은 메뉴를 연 뒤의 고르기다(고르지 않은 줄을 누르면 그 줄 하나).
  menuItems: (item: FileItem | null, sel: Selection) => readonly ContextMenuItem[]
  // onRename·onDelete는 F2·Delete 키다(지금 고르기에 한다). Shift+Delete는 permanent(휴지통을 거치지 않음)다.
  onRename: () => void
  onDelete: (permanent: boolean) => void
  // canDrag면 고른 줄을 끌 수 있다. dropMode는 놓을 곳(data-drop-path)에서 할 일이고 null이면 놓을 수 없다(이 폴더에 쓸
  // 수 없으면 같은 디스크로는 옮기지 못한다). onMove는 놓은 곳의 경로다.
  canDrag: boolean
  dropMode: (target: string) => DropMode | null
  onMove: (sel: Selection, target: string) => void
  // focusOnMount면 처음 그릴 때 목록에 포커스한다(키보드로 폴더에 들어간 뒤). 한 뒤 onFocused를 부른다.
  focusOnMount?: boolean
  onFocused?: () => void
  ref?: Ref<FileListHandle>
}

// MAX_PARALLEL은 동시에 받는 페이지 수다(도우미의 사용자당 동시 연산 4개 안).
const MAX_PARALLEL = 2
// FETCH_DELAY_MS는 스크롤이 멈춘 뒤 범위를 받기까지 기다리는 시간이다(빠르게 지나가는 범위는 받지 않는다).
const FETCH_DELAY_MS = 90
// BUBBLE_MIN은 위치 말풍선을 보이는 최소 항목 수, BUBBLE_HIDE_MS는 스크롤이 멈춘 뒤 말풍선을 숨기기까지의 시간이다.
const BUBBLE_MIN = 300
const BUBBLE_HIDE_MS = 900
// NAMES_LIMIT는 고른 줄의 이름만 한 번에 받는 수다(백엔드 filesNamesLimit).
const NAMES_LIMIT = 2000
// DRAG_THRESHOLD는 누른 줄에서 이만큼(px) 움직여야 끌기로 보는 거리다. DRAG_EDGE는 끄는 동안 목록 위·아래 가장자리에서
// 스크롤하는 띠의 높이, DRAG_FOLDERS는 놓을 수 없는 곳(자기 안)을 가리려고 보는 고른 폴더의 최대 수다.
const DRAG_THRESHOLD = 6
const DRAG_EDGE = 32
const DRAG_FOLDERS = 20_000
// DROP_KEYS는 끄는 이름표에 보이는, 놓으면 할 일이다.
const DROP_KEYS = { move: 'files.drag.move', copy: 'files.drag.copy', trash: 'files.drag.trash' } as const

// DROP_TARGET은 끄는 동안 놓을 곳에 붙이는 클래스다. 다른 창의 요소에도 붙으므로 렌더 없이 요소에 직접 둔다.
export const DROP_TARGET = 'files-drop-target'

// Drag는 누른 뒤 끌기가 시작되기 전후의 상태다.
interface Drag {
  pointerId: number
  x: number
  y: number
  index: number
  started: boolean
  sel: Selection
  // folders는 끄는 줄 가운데 받은 폴더의 경로다(그 안에는 놓을 수 없다).
  folders: string[]
  ghost: HTMLElement | null
  // mode는 지금 놓을 곳에서 할 일이다(이름표에 보인다).
  mode: DropMode | null
  target: HTMLElement | null
  targetPath: string | null
  lastX: number
  lastY: number
  raf: number | undefined
  cleanup: () => void
}

const COLUMNS: readonly { key: SortKey | 'kind'; labelKey: 'files.columns.name' | 'files.columns.modified' | 'files.columns.size' | 'files.columns.kind' }[] = [
  { key: 'name', labelKey: 'files.columns.name' },
  { key: 'mtime', labelKey: 'files.columns.modified' },
  { key: 'size', labelKey: 'files.columns.size' },
  { key: 'kind', labelKey: 'files.columns.kind' },
]

interface CacheState {
  key: string
  pages: PageCache<FileItem>
  total: number | null
  // names는 위치 말풍선용 이름만 받은 페이지다(정보 없이 가볍다).
  names: PageCache<string>
}

export function FileList(props: Props) {
  const { listing, view, active, selection, ref } = props
  const selected = selection.focus
  const { t } = useTranslation()
  const lang = useLanguage()
  const baseId = useId()
  const scrollRef = useRef<HTMLDivElement>(null)
  const [viewHeight, setViewHeight] = useState(0)
  const [scrollTop, setScrollTop] = useState(0)
  const [, setVersion] = useState(0)
  const bump = useCallback(() => setVersion((v) => v + 1), [])
  const [bubble, setBubble] = useState(false)
  const bubbleTimer = useRef<number | undefined>(undefined)
  const propsRef = useRef(props)
  useLayoutEffect(() => {
    propsRef.current = props
  })

  const key = `${listing.id}|${view.sort}|${view.desc ? 'd' : 'a'}|${view.query}`
  // 보기마다 캐시 하나다. 받은 페이지는 이 객체에 쌓고 bump로 다시 그린다. 보기가 바뀌면 이전 보기의 요청을 끊는다.
  const cache = useMemo<CacheState>(
    () => ({
      key,
      pages: new PageCache<FileItem>(),
      // 찾기가 없으면 목록 전체 수를 바로 안다.
      total: view.query ? null : listing.total,
      names: new PageCache<string>(PAGE_SIZE, 20),
    }),
    // key가 목록·정렬·방향·찾기어를 모두 담는다.
    [key, view.query, listing.total],
  )
  // 요청을 끊는 컨트롤러는 마운트마다 새로 둔다(개발 모드 StrictMode가 정리를 한 번 더 불러도 끊긴 채 남지 않게).
  const abortRef = useRef<AbortController | null>(null)
  useEffect(() => {
    const ctl = new AbortController()
    abortRef.current = ctl
    return () => ctl.abort()
  }, [cache])

  // 다른 폴더·정렬·찾기로 바뀌면 맨 위로 올린다. 같은 폴더를 다시 받은 경우(만료 뒤 다시 열기, 올린 뒤 새로 고침)는
  // 위치를 둔다.
  const scrollKey = `${listing.path}|${view.sort}|${view.desc ? 'd' : 'a'}|${view.query}`
  const prevScrollKey = useRef(scrollKey)
  useLayoutEffect(() => {
    if (prevScrollKey.current === scrollKey) return
    prevScrollKey.current = scrollKey
    scrollRef.current?.scrollTo({ top: 0 })
    setScrollTop(0)
  }, [scrollKey])

  const { focusOnMount, onFocused } = props
  useEffect(() => {
    if (!focusOnMount) return
    scrollRef.current?.focus()
    onFocused?.()
  }, [focusOnMount, onFocused])

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const measure = () => setViewHeight(el.clientHeight)
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const total = cache.total ?? 0
  const { onTotal } = props
  useEffect(() => {
    if (cache.total !== null) onTotal(cache.total)
  }, [cache.total, onTotal])

  const lay = layout(total, scrollTop, viewHeight)
  // 찾기 결과 수를 모르면 첫 페이지부터 받는다.
  const first = cache.total === null ? 0 : lay.first
  const last = cache.total === null ? PAGE_SIZE : lay.last

  // 보이는 범위에서 빠진 페이지를 받는다. 스크롤이 멈출 때까지 조금 기다린다.
  useEffect(() => {
    if (!active) return
    const c = cache
    c.pages.touch(first, last)
    const timer = window.setTimeout(() => {
      const signal = abortRef.current?.signal
      if (!signal || signal.aborted) return
      const missing = c.pages.missing(first, last).slice(0, MAX_PARALLEL)
      for (const page of missing) {
        c.pages.start(page)
        fetchListingRange(
          listing.id,
          { offset: page * PAGE_SIZE, limit: PAGE_SIZE, sort: view.sort, desc: view.desc, query: view.query },
          signal,
        ).then(
          (res) => {
            if (signal.aborted) return
            c.pages.put(page, res.items)
            c.total = res.total
            bump()
          },
          (err: unknown) => {
            c.pages.fail(page)
            if (signal.aborted) return
            const p = propsRef.current
            if (err instanceof ApiError && err.code === 'listing_expired') p.onExpired()
            else if (err instanceof ApiError && err.code === 'sort_pending') p.onSortPending()
            else p.onError(err)
          },
        )
      }
    }, FETCH_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [active, first, last, listing.id, view.sort, view.desc, view.query, bump, cache])

  // 위치 말풍선의 이름: 받은 줄이 없으면 이름만 가볍게 받는다.
  const bubbleIndex = indexAt(total, scrollTop, viewHeight)
  const bubbleName = cache.pages.get(bubbleIndex)?.name ?? cache.names.get(bubbleIndex)
  useEffect(() => {
    if (!bubble || bubbleName !== undefined || total < BUBBLE_MIN) return
    const c = cache
    const page = Math.floor(bubbleIndex / PAGE_SIZE)
    if (c.names.missing(page * PAGE_SIZE, page * PAGE_SIZE + 1).length === 0) return
    const timer = window.setTimeout(() => {
      const signal = abortRef.current?.signal
      if (!signal || signal.aborted) return
      c.names.start(page)
      fetchListingRange(
        listing.id,
        { offset: page * PAGE_SIZE, limit: PAGE_SIZE, sort: view.sort, desc: view.desc, query: view.query, namesOnly: true },
        signal,
      ).then(
        (res) => {
          if (signal.aborted) return
          c.names.put(
            page,
            res.items.map((i) => i.name),
          )
          bump()
        },
        () => c.names.fail(page),
      )
    }, 50)
    return () => window.clearTimeout(timer)
  }, [bubble, bubbleIndex, bubbleName, total, listing.id, view.sort, view.desc, view.query, bump, cache])

  const raf = useRef<number | undefined>(undefined)
  const onScroll = (e: UIEvent<HTMLDivElement>) => {
    const top = e.currentTarget.scrollTop
    if (raf.current === undefined) {
      raf.current = requestAnimationFrame(() => {
        raf.current = undefined
        setScrollTop(scrollRef.current?.scrollTop ?? top)
      })
    }
    if (total >= BUBBLE_MIN) {
      setBubble(true)
      window.clearTimeout(bubbleTimer.current)
      bubbleTimer.current = window.setTimeout(() => setBubble(false), BUBBLE_HIDE_MS)
    }
  }
  useEffect(
    () => () => {
      window.clearTimeout(bubbleTimer.current)
      if (raf.current !== undefined) cancelAnimationFrame(raf.current)
    },
    [],
  )

  // choose는 고르기를 바꾼다. 위치 줄의 항목을 함께 알린다.
  const choose = (sel: Selection) => {
    props.onSelection(sel, sel.focus === null ? null : (cache.pages.get(sel.focus) ?? null))
  }

  const reveal = (i: number) => {
    const el = scrollRef.current
    if (!el) return
    const next = scrollToShow(i, total, el.scrollTop, el.clientHeight)
    if (next !== el.scrollTop) el.scrollTop = next
  }

  // select는 index 줄로 옮겨 고르고(extend면 기준부터 넓히고) 보이게 스크롤한다.
  const select = (index: number, extend: boolean) => {
    const i = Math.min(Math.max(0, index), total - 1)
    if (i < 0) return
    choose(extend ? extendTo(selection, i) : selectOne(i))
    reveal(i)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const page = Math.max(1, Math.floor(viewHeight / ROW_HEIGHT) - 1)
    const cur = selected ?? -1
    const mod = e.ctrlKey || e.metaKey
    switch (e.key) {
      case 'ArrowDown':
        select(cur + 1, e.shiftKey)
        break
      case 'ArrowUp':
        select(cur <= 0 ? 0 : cur - 1, e.shiftKey)
        break
      case 'PageDown':
        select(cur + page, e.shiftKey)
        break
      case 'PageUp':
        select(cur - page, e.shiftKey)
        break
      case 'Home':
        select(0, e.shiftKey)
        break
      case 'End':
        select(total - 1, e.shiftKey)
        break
      case 'a':
      case 'A':
        if (!mod) return
        choose(selectAll(selection, total))
        break
      case 'Enter': {
        const item = selected !== null ? cache.pages.get(selected) : undefined
        if (item) props.onOpen(item)
        break
      }
      case 'Backspace':
        props.onUp()
        break
      case 'F2':
        props.onRename()
        break
      case 'Delete':
        props.onDelete(e.shiftKey)
        break
      case 'Escape':
        if (selectedCount(selection) === 0) return
        choose(NO_SELECTION)
        break
      default:
        return
    }
    e.preventDefault()
  }

  // resolveItems는 고른 자리의 항목이다. 받은 줄은 그대로 쓰고, 빠진 구간은 이름만 NAMES_LIMIT개씩 받는다.
  const resolveItems = async (sel: Selection): Promise<FileItem[]> => {
    const c = cache
    const out: FileItem[] = []
    for (const span of sel.spans) {
      for (let start = span.start; start < span.end; start += NAMES_LIMIT) {
        const end = Math.min(span.end, start + NAMES_LIMIT)
        const chunk: FileItem[] = []
        for (let i = start; i < end; i++) {
          const it = c.pages.get(i)
          if (!it) break
          chunk.push(it)
        }
        if (chunk.length === end - start) {
          out.push(...chunk)
          continue
        }
        const res = await fetchListingRange(listing.id, {
          offset: start,
          limit: end - start,
          sort: view.sort,
          desc: view.desc,
          query: view.query,
          namesOnly: true,
        })
        res.items.forEach((it, k) => out.push(c.pages.get(start + k) ?? it))
      }
    }
    return out
  }
  useImperativeHandle(ref, () => ({ items: resolveItems, reveal }))

  // 끌어서 옮기기. 누른 줄에서 임계 거리를 넘게 움직이면 시작하고(그때 pointer capture를 잡아 그냥 누르기의 click은
  // 줄에 그대로 간다), 놓은 곳의 data-drop-path 폴더로 옮긴다. 터치는 스크롤·길게 누르기 메뉴와 겹쳐 끌지 않는다.
  const dragRef = useRef<Drag | null>(null)
  // suppressClick은 끌기를 끝낸 뒤 따라오는 click 하나를 무시하게 한다(놓은 자리의 줄을 고르거나 고르기를 지우지 않게).
  // Esc로 취소하면 놓기 전에 끝나므로 다음 click까지 남긴다. 다음 누름에서 지운다(click이 오지 않은 경우).
  const suppressClick = useRef(false)
  const consumeClick = () => {
    if (!suppressClick.current) return false
    suppressClick.current = false
    return true
  }

  const hitTest = (d: Drag) => {
    const el = document.elementFromPoint(d.lastX, d.lastY)?.closest<HTMLElement>('[data-drop-path]') ?? null
    const p = el?.dataset.dropPath ?? null
    const mode = p !== null && p !== listing.path && !d.folders.some((f) => within(p, f)) ? propsRef.current.dropMode(p) : null
    const next = mode ? el : null
    if (next !== d.target) {
      d.target?.classList.remove(DROP_TARGET)
      next?.classList.add(DROP_TARGET)
      d.target = next
      d.targetPath = mode ? p : null
    }
    if (mode !== d.mode) {
      d.mode = mode
      const label = d.ghost?.querySelector<HTMLElement>('.files-drag-mode')
      if (label) {
        label.textContent = mode ? t(DROP_KEYS[mode]) : ''
        label.hidden = mode === null
      }
      if (d.ghost) d.ghost.dataset.mode = mode ?? ''
    }
  }

  const startDrag = (d: Drag, e: PointerEvent<HTMLDivElement>) => {
    const item = cache.pages.get(d.index)
    if (!item) {
      dragRef.current = null
      return
    }
    d.started = true
    d.sel = isSelected(selection, d.index) ? selection : selectOne(d.index)
    // 항목 없이 알려 정보 패널을 열지 않는다(끄는 도중 목록 폭이 바뀌면 놓을 곳이 움직인다).
    if (d.sel !== selection) props.onSelection(d.sel, null)
    const count = selectedCount(d.sel)
    let seen = 0
    for (const span of d.sel.spans) {
      for (let i = span.start; i < span.end && seen < DRAG_FOLDERS; i++, seen++) {
        const it = cache.pages.get(i)
        if (it && isFolderLike(it)) d.folders.push(joinPath(listing.path, it.name))
      }
    }
    e.currentTarget.setPointerCapture(e.pointerId)
    menu.cancelLongPress()
    const ghost = document.createElement('div')
    ghost.className = 'files-drag-ghost'
    const name = document.createElement('span')
    name.className = 'files-drag-name'
    name.textContent = count === 1 ? item.name : t('files.drag.items', { count, total: new Intl.NumberFormat(lang).format(count) })
    const mode = document.createElement('span')
    mode.className = 'files-drag-mode'
    mode.hidden = true
    ghost.append(name, mode)
    document.body.append(ghost)
    d.ghost = ghost
    const onKey = (ev: globalThis.KeyboardEvent) => {
      if (ev.key !== 'Escape') return
      ev.preventDefault()
      endDrag(false)
    }
    const onBlur = () => endDrag(false)
    window.addEventListener('keydown', onKey, true)
    window.addEventListener('blur', onBlur)
    // 목록 위·아래 가장자리에 있으면 스크롤한다(큰 폴더에서 아래쪽 폴더에 놓을 수 있게).
    const tick = () => {
      const el = scrollRef.current
      if (el && dragRef.current === d) {
        const r = el.getBoundingClientRect()
        let dy = 0
        if (d.lastX >= r.left && d.lastX <= r.right) {
          if (d.lastY < r.top + DRAG_EDGE && d.lastY > r.top - DRAG_EDGE) dy = -Math.ceil((r.top + DRAG_EDGE - d.lastY) / 3)
          else if (d.lastY > r.bottom - DRAG_EDGE && d.lastY < r.bottom + DRAG_EDGE) dy = Math.ceil((d.lastY - r.bottom + DRAG_EDGE) / 3)
        }
        if (dy !== 0) {
          el.scrollTop += dy
          hitTest(d)
        }
        d.raf = requestAnimationFrame(tick)
      }
    }
    d.raf = requestAnimationFrame(tick)
    d.cleanup = () => {
      window.removeEventListener('keydown', onKey, true)
      window.removeEventListener('blur', onBlur)
      if (d.raf !== undefined) cancelAnimationFrame(d.raf)
      d.target?.classList.remove(DROP_TARGET)
      d.ghost?.remove()
    }
    moveDrag(d, e.clientX, e.clientY)
  }

  const moveDrag = (d: Drag, x: number, y: number) => {
    d.lastX = x
    d.lastY = y
    if (d.ghost) d.ghost.style.transform = `translate(${x + 14}px, ${y + 12}px)`
    hitTest(d)
  }

  // endDrag는 끌기를 끝낸다. drop이면 놓은 곳으로 옮긴다.
  const endDrag = (drop: boolean) => {
    const d = dragRef.current
    dragRef.current = null
    if (!d?.started) return
    d.cleanup()
    suppressClick.current = true
    if (drop && d.targetPath) propsRef.current.onMove(d.sel, d.targetPath)
  }
  const endDragRef = useRef(endDrag)
  useLayoutEffect(() => {
    endDragRef.current = endDrag
  })
  useEffect(() => () => endDragRef.current(false), [])

  const pressStart = (e: PointerEvent<HTMLDivElement>) => {
    if (!props.canDrag || e.button !== 0 || e.pointerType === 'touch' || e.shiftKey || e.ctrlKey || e.metaKey) return
    const i = rowIndexOf(e.target)
    const item = i === null ? undefined : cache.pages.get(i)
    if (i === null || !item || item.badName || item.state === 'gone') return
    dragRef.current = {
      pointerId: e.pointerId,
      x: e.clientX,
      y: e.clientY,
      index: i,
      started: false,
      sel: NO_SELECTION,
      folders: [],
      ghost: null,
      mode: null,
      target: null,
      targetPath: null,
      lastX: e.clientX,
      lastY: e.clientY,
      raf: undefined,
      cleanup: () => {},
    }
  }
  const pressMove = (e: PointerEvent<HTMLDivElement>) => {
    const d = dragRef.current
    if (!d || d.pointerId !== e.pointerId) return
    if (d.started) moveDrag(d, e.clientX, e.clientY)
    else if (Math.hypot(e.clientX - d.x, e.clientY - d.y) >= DRAG_THRESHOLD) startDrag(d, e)
  }
  const pressEnd = (e: PointerEvent<HTMLDivElement>, drop: boolean) => {
    if (dragRef.current?.pointerId !== e.pointerId) return
    if (dragRef.current.started) endDrag(drop)
    else dragRef.current = null
  }

  // 메뉴를 연 줄. 우클릭·길게 누르기는 누른 줄, 키보드(메뉴 키)는 고른 줄이다.
  const menuIndex = useRef<number | null>(null)
  const rowIndexOf = (target: EventTarget | null): number | null => {
    const row = target instanceof Element ? target.closest<HTMLElement>('[data-index]') : null
    return row ? Number(row.dataset.index) : null
  }
  const menu = useContextMenuTrigger({
    items: () => {
      const i = menuIndex.current
      if (i === null) return props.menuItems(null, selection)
      const item = cache.pages.get(i)
      if (!item) return []
      const sel = isSelected(selection, i) ? { ...selection, focus: i } : selectOne(i)
      choose(sel)
      return props.menuItems(item, sel)
    },
  })
  const rows = []
  for (let i = lay.first; i < lay.last; i++) {
    const item = cache.pages.get(i)
    const top = lay.top + (i - lay.first) * ROW_HEIGHT
    const isSel = isSelected(selection, i)
    if (!item) {
      rows.push(
        <div key={i} className="files-row skeleton" role="row" aria-rowindex={i + 2} aria-busy="true" style={{ top }}>
          <span className="files-skel icon" />
          <span className="files-skel name" style={{ width: `${40 + ((i * 37) % 30)}%` }} />
        </div>,
      )
      continue
    }
    const kind = kindOf(item)
    const pending = item.state === 'pending'
    const gone = item.state === 'gone'
    const droppable = isFolderLike(item) && !item.locked && !gone && !item.badName
    rows.push(
      <div
        key={i}
        id={`${baseId}-r${i}`}
        data-index={i}
        data-drop-path={droppable ? joinPath(listing.path, item.name) : undefined}
        className={['files-row', isSel && 'selected', i === selected && 'current', (item.locked || gone || item.badName) && 'dim']
          .filter(Boolean)
          .join(' ')}
        role="row"
        aria-rowindex={i + 2}
        aria-selected={isSel}
        style={{ top }}
        onClick={(e) => {
          if (menu.openedByPress() || consumeClick()) return
          const mod = e.ctrlKey || e.metaKey
          if (e.shiftKey) choose(extendTo(selection, i, mod))
          else if (mod) choose(toggleAt(selection, i))
          else choose(selectOne(i))
          if (e.detail === 2 && !e.shiftKey && !mod) props.onOpen(item)
        }}
      >
        <span className="files-cell name" role="gridcell">
          <span className="files-icon">
            <IconImage name={kindIcon(kind)} size={20} />
            {item.locked && (
              <span className="files-lock" title={t('files.kinds.locked')}>
                <IconImage name="system-lock-screen-symbolic" size={9} />
              </span>
            )}
          </span>
          <span className="files-name">{item.name}</span>
        </span>
        <span className="files-cell date" role="gridcell">
          {item.modTime ? formatDateTime(new Date(item.modTime).toISOString(), lang) : ''}
        </span>
        <span className="files-cell size" role="gridcell">
          {item.type === 'file' && item.state === 'ok' ? formatBytes(item.size ?? 0, lang) : ''}
        </span>
        <span className="files-cell kind" role="gridcell">
          {pending && item.type === 'unknown' ? '' : t(KIND_KEYS[kind])}
        </span>
      </div>,
    )
  }

  const sortLabel = (k: SortKey | 'kind'): 'ascending' | 'descending' | undefined =>
    k === view.sort ? (view.desc ? 'descending' : 'ascending') : undefined
  const maxTop = Math.max(1, lay.scrollHeight - viewHeight)
  const bubbleTop = Math.min(Math.max(0, (scrollTop / maxTop) * (viewHeight - 48)), Math.max(0, viewHeight - 48))
  const activeId = selected !== null && selected >= lay.first && selected < lay.last ? `${baseId}-r${selected}` : undefined

  return (
    <div
      className="files-list"
      role="grid"
      aria-label={t('files.list.label')}
      aria-rowcount={total + 1}
      aria-multiselectable="true"
      // 다른 창에서 끌어 온 항목을 폴더 줄이 아닌 곳(빈 곳, 파일 줄)에 놓으면 이 폴더로 옮긴다.
      data-drop-path={listing.writable ? listing.path : undefined}
    >
      <div className="files-row header" role="row" aria-rowindex={1}>
        {COLUMNS.map((c) => (
          <span key={c.key} className={`files-cell ${c.key === 'mtime' ? 'date' : c.key}`} role="columnheader" aria-sort={sortLabel(c.key)}>
            {c.key === 'kind' ? (
              t(c.labelKey)
            ) : (
              <button type="button" className={c.key === view.sort ? 'sorted' : ''} onClick={() => props.onSort(c.key as SortKey)}>
                {t(c.labelKey)}
                {c.key === view.sort && <span className={`files-sort ${view.desc ? 'desc' : 'asc'}`} aria-hidden="true" />}
              </button>
            )}
          </span>
        ))}
      </div>
      <div
        ref={scrollRef}
        className="files-scroll"
        tabIndex={0}
        aria-activedescendant={activeId}
        onScroll={onScroll}
        onClick={(e) => {
          // 빈 곳을 누르면 고르기를 지운다.
          if (consumeClick() || menu.openedByPress() || rowIndexOf(e.target) !== null) return
          if (selectedCount(selection) > 0) choose(NO_SELECTION)
        }}
        onContextMenu={(e) => {
          menuIndex.current = rowIndexOf(e.target)
          menu.handlers.onContextMenu(e)
        }}
        onPointerDown={(e) => {
          suppressClick.current = false
          menuIndex.current = rowIndexOf(e.target)
          menu.handlers.onPointerDown(e)
          pressStart(e)
        }}
        onPointerMove={(e) => {
          menu.handlers.onPointerMove(e)
          pressMove(e)
        }}
        onPointerUp={(e) => {
          menu.handlers.onPointerUp(e)
          pressEnd(e, true)
        }}
        onPointerCancel={(e) => {
          menu.handlers.onPointerCancel(e)
          pressEnd(e, false)
        }}
        onLostPointerCapture={(e) => pressEnd(e, false)}
        onKeyDown={(e) => {
          menuIndex.current = selected
          menu.handlers.onKeyDown(e)
          if (!e.defaultPrevented) onKeyDown(e)
        }}
      >
        <div className="files-spacer" style={{ height: lay.scrollHeight }}>
          {rows}
        </div>
        {cache.total === 0 && (
          <p className="files-empty">{view.query ? t('files.list.noMatch', { query: view.query }) : t('files.list.empty')}</p>
        )}
      </div>
      {bubble && total >= BUBBLE_MIN && (
        <div className="files-bubble" role="status" style={{ top: bubbleTop + 30 }}>
          <strong>{bubbleName ?? '…'}</strong>
          <span>
            {t('files.list.position', {
              index: new Intl.NumberFormat(lang).format(bubbleIndex + 1),
              total: new Intl.NumberFormat(lang).format(total),
            })}
          </span>
        </div>
      )}
    </div>
  )
}
