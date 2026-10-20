// 파일 탐색기의 자세히 목록. 보이는 줄만 그리고(virtual.ts), 정렬·찾기를 적용한 보기에서 보이는 범위를 PAGE_SIZE줄씩
// 받아 둔다(pageCache.ts). 아직 받지 않은 줄은 자리 표시로 그린다. 스크롤하는 동안 위치 말풍선을 보인다.
import {
  type KeyboardEvent,
  type UIEvent,
  useCallback,
  useEffect,
  useId,
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
import { KIND_KEYS, kindIcon, kindOf } from './kinds'
import { PAGE_SIZE, PageCache } from './pageCache'
import { ROW_HEIGHT, indexAt, layout, scrollToShow } from './virtual'

export type SortKey = 'name' | 'mtime' | 'size'

export interface View {
  sort: SortKey
  desc: boolean
  query: string
}

interface Props {
  listing: FilesListing
  view: View
  active: boolean
  selected: number | null
  onSelect: (index: number | null, item: FileItem | null) => void
  // onOpen은 두 번 누르기·Enter다(폴더면 들어가고 파일이면 정보 패널).
  onOpen: (item: FileItem) => void
  onUp: () => void
  onSort: (key: SortKey) => void
  onTotal: (total: number) => void
  // onExpired는 목록이 서버에서 사라졌을 때(410), onSortPending은 날짜·크기순 정보가 모자랄 때(409)다.
  onExpired: () => void
  onSortPending: () => void
  onError: (err: unknown) => void
  menuItems: (item: FileItem) => readonly ContextMenuItem[]
  // focusOnMount면 처음 그릴 때 목록에 포커스한다(키보드로 폴더에 들어간 뒤). 한 뒤 onFocused를 부른다.
  focusOnMount?: boolean
  onFocused?: () => void
}

// MAX_PARALLEL은 동시에 받는 페이지 수다(도우미의 사용자당 동시 연산 4개 안).
const MAX_PARALLEL = 2
// FETCH_DELAY_MS는 스크롤이 멈춘 뒤 범위를 받기까지 기다리는 시간이다(빠르게 지나가는 범위는 받지 않는다).
const FETCH_DELAY_MS = 90
// BUBBLE_MIN은 위치 말풍선을 보이는 최소 항목 수, BUBBLE_HIDE_MS는 스크롤이 멈춘 뒤 말풍선을 숨기기까지의 시간이다.
const BUBBLE_MIN = 300
const BUBBLE_HIDE_MS = 900

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
  const { listing, view, active, selected } = props
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

  // select는 index 줄을 고르고 보이게 스크롤한다.
  const select = (index: number) => {
    const i = Math.min(Math.max(0, index), total - 1)
    if (i < 0) return
    props.onSelect(i, cache.pages.get(i) ?? null)
    const el = scrollRef.current
    if (!el) return
    const next = scrollToShow(i, total, el.scrollTop, el.clientHeight)
    if (next !== el.scrollTop) el.scrollTop = next
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const page = Math.max(1, Math.floor(viewHeight / ROW_HEIGHT) - 1)
    const cur = selected ?? -1
    switch (e.key) {
      case 'ArrowDown':
        select(cur + 1)
        break
      case 'ArrowUp':
        select(cur <= 0 ? 0 : cur - 1)
        break
      case 'PageDown':
        select(cur + page)
        break
      case 'PageUp':
        select(cur - page)
        break
      case 'Home':
        select(0)
        break
      case 'End':
        select(total - 1)
        break
      case 'Enter': {
        const item = selected !== null ? cache.pages.get(selected) : undefined
        if (item) props.onOpen(item)
        break
      }
      case 'Backspace':
        props.onUp()
        break
      default:
        return
    }
    e.preventDefault()
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
      const item = i === null ? undefined : cache.pages.get(i)
      if (i === null || !item) return []
      props.onSelect(i, item)
      return props.menuItems(item)
    },
  })
  const rows = []
  for (let i = lay.first; i < lay.last; i++) {
    const item = cache.pages.get(i)
    const top = lay.top + (i - lay.first) * ROW_HEIGHT
    const isSel = i === selected
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
    rows.push(
      <div
        key={i}
        id={`${baseId}-r${i}`}
        data-index={i}
        className={['files-row', isSel && 'selected', (item.locked || gone || item.badName) && 'dim'].filter(Boolean).join(' ')}
        role="row"
        aria-rowindex={i + 2}
        aria-selected={isSel}
        style={{ top }}
        onClick={(e) => {
          if (menu.openedByPress()) return
          props.onSelect(i, item)
          if (e.detail === 2) props.onOpen(item)
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
    <div className="files-list" role="grid" aria-label={t('files.list.label')} aria-rowcount={total + 1}>
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
        onContextMenu={(e) => {
          menuIndex.current = rowIndexOf(e.target)
          menu.handlers.onContextMenu(e)
        }}
        onPointerDown={(e) => {
          menuIndex.current = rowIndexOf(e.target)
          menu.handlers.onPointerDown(e)
        }}
        onPointerMove={(e) => menu.handlers.onPointerMove(e)}
        onPointerUp={(e) => menu.handlers.onPointerUp(e)}
        onPointerCancel={(e) => menu.handlers.onPointerCancel(e)}
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
