// 파일 탐색기의 하위 폴더 검색 보기. 도구 막대 찾기 칸의 범위를 "하위 폴더 포함"으로 두고 Enter를 누르면 목록 대신
// 그린다. 권한 도우미가 폴더와 그 아래를 너비 우선으로 찾으며(searchFiles), 찾은 항목은 오는 대로 이름·위치·종류 열에
// 보인다(얕은 폴더의 항목이 먼저 온다). 폴더를 열면 그 폴더로 가고, 파일은 고르면 정보 패널, 두 번 누르면 미리보기다.
// 결과에서는 쓰기 작업(끌기, 삭제)을 하지 않는다. 위치를 열어 그 폴더에서 한다.
import { type KeyboardEvent, type UIEvent, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, type RequiredLevel, searchFiles } from '../api/client'
import type { FileItem, FilesSearchHit } from '../api/types.gen'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import type { ContextMenuItem } from '../shell/contextMenu'
import { useContextMenuTrigger } from '../shell/useContextMenuTrigger'
import { InfoPane } from './InfoPane'
import { KIND_KEYS, type PreviewKind, isFolderLike, kindIcon, kindOf, previewKind } from './kinds'
import { errorKey } from './messages'
import { type Root, baseName, dirName, folderLabel, locationLabel } from './paths'
import { NO_SELECTION, type Selection, extendTo, isSelected, selectOne, selectedCount, soleIndex, toggleAt } from './selection'
import { ROW_HEIGHT, layout, moveKey, scrollToShow } from './virtual'

// Search는 지금 검색이다. seq는 같은 찾기어로 다시 찾을 때 바뀐다.
export interface Search {
  path: string
  query: string
  seq: number
}

interface Props {
  search: Search
  roots: readonly Root[]
  admin: boolean
  // onFolder는 폴더를 연다. onLocate는 항목이 든 폴더를 열고 그 항목을 고른다.
  onFolder: (path: string) => void
  onLocate: (path: string) => void
  onPreview: (path: string, name: string, kind: PreviewKind) => void
  onDownload: (path: string, name: string) => void
  // onSecurity는 보안 확인이 필요할 때다(창이 확인한 뒤 다시 찾는다).
  onSecurity: (level?: RequiredLevel) => void
  // onRetry는 같은 찾기어로 다시 찾는다.
  onRetry: () => void
}

type State = { key: string } & (
  | { status: 'running'; hits: FilesSearchHit[]; dirs: number; skipped: number }
  | { status: 'done'; hits: FilesSearchHit[]; dirs: number; skipped: number; truncated: boolean; stopped: boolean }
  | { status: 'error'; err: unknown; hits: FilesSearchHit[] }
)

// itemOf는 찾은 항목을 목록 항목 모양으로 바꾼다(종류·아이콘 계산과 열기에 쓴다). 링크 대상의 종류는 모른다.
const itemOf = (h: FilesSearchHit): FileItem => ({ name: baseName(h.path), type: h.type, state: 'ok' })

export function SearchView({ search, roots, admin, onFolder, onLocate, onPreview, onDownload, onSecurity, onRetry }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const baseId = useId()
  const key = `${search.seq}|${search.path}|${search.query}`
  const [result, setResult] = useState<State>({ key: '', status: 'running', hits: [], dirs: 0, skipped: 0 })
  const [sel, setSel] = useState<Selection>(NO_SELECTION)
  const [pane, setPane] = useState<string | null>(null)
  const ctlRef = useRef<AbortController | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const [viewHeight, setViewHeight] = useState(0)
  const [scrollTop, setScrollTop] = useState(0)
  // onSecurity는 창이 렌더마다 새로 만든다. 검색은 key가 바뀔 때만 다시 하므로 마지막 함수를 둔다.
  const securityRef = useRef(onSecurity)
  useLayoutEffect(() => {
    securityRef.current = onSecurity
  })

  useEffect(() => {
    const ctl = new AbortController()
    ctlRef.current = ctl
    let hits: FilesSearchHit[] = []
    let dirs = 0
    let skipped = 0
    // 찾은 항목은 한 번에 여러 사건으로 오므로 그리기를 한 프레임에 묶는다.
    let raf: number | undefined
    const flush = () => {
      raf = undefined
      if (!ctl.signal.aborted) setResult({ key, status: 'running', hits, dirs, skipped })
    }
    searchFiles(
      search.path,
      search.query,
      (p) => {
        if (p.hits.length > 0) hits = [...hits, ...p.hits]
        dirs = p.dirs
        skipped = p.skipped
        raf ??= requestAnimationFrame(flush)
      },
      ctl.signal,
    ).then(
      (r) => {
        if (raf !== undefined) cancelAnimationFrame(raf)
        setResult({ key, status: 'done', hits, dirs: r.dirs, skipped: r.skipped ?? 0, truncated: r.truncated ?? false, stopped: false })
      },
      (err: unknown) => {
        if (raf !== undefined) cancelAnimationFrame(raf)
        // 중지 단추로 끊었으면 받은 결과를 남긴다.
        if (ctl.signal.aborted) {
          if (ctlRef.current === ctl && ctl.signal.reason === 'stop') {
            setResult({ key, status: 'done', hits, dirs, skipped, truncated: false, stopped: true })
          }
          return
        }
        if (err instanceof ApiError && err.code === 'security_required') {
          securityRef.current(err.required)
          return
        }
        if (err instanceof ApiError && err.status === 401) return
        setResult({ key, status: 'error', err, hits })
      },
    )
    return () => {
      if (raf !== undefined) cancelAnimationFrame(raf)
      ctl.abort()
    }
  }, [key, search.path, search.query])
  const state: State = result.key === key ? result : { key, status: 'running', hits: [], dirs: 0, skipped: 0 }
  // 새 검색이면 고르기와 정보 패널을 지운다(자리가 바뀐다).
  const [prevKey, setPrevKey] = useState(key)
  if (prevKey !== key) {
    setPrevKey(key)
    setSel(NO_SELECTION)
    setPane(null)
  }

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const measure = () => setViewHeight(el.clientHeight)
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const hits = state.hits
  const total = hits.length
  const lay = layout(total, scrollTop, viewHeight)
  const fmt = (n: number) => new Intl.NumberFormat(lang).format(n)
  const homeName = t('files.nav.home')

  const stop = () => ctlRef.current?.abort('stop')

  const open = (h: FilesSearchHit) => {
    const it = itemOf(h)
    if (isFolderLike(it)) {
      onFolder(h.path)
      return
    }
    const kind = previewKind(it)
    if (kind) onPreview(h.path, it.name, kind)
    else setPane(h.path)
  }

  const choose = (s: Selection) => {
    setSel(s)
    const one = soleIndex(s)
    const h = one === null ? null : hits[one]
    // 파일 하나를 고르면 정보 패널을 연다. 폴더는 고르기만 한다(목록과 같다).
    if (!h || isFolderLike(itemOf(h))) setPane(null)
    else setPane(h.path)
  }

  const reveal = (i: number) => {
    const el = scrollRef.current
    if (!el) return
    const next = scrollToShow(i, total, el.scrollTop, el.clientHeight)
    if (next !== el.scrollTop) el.scrollTop = next
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const to = moveKey(e.key, sel.focus ?? -1, 1, total, Math.max(1, Math.floor(viewHeight / ROW_HEIGHT) - 1))
    if (to !== null) {
      choose(e.shiftKey ? extendTo(sel, to) : selectOne(to))
      reveal(to)
      e.preventDefault()
      return
    }
    if (e.key === 'Enter' && sel.focus !== null && hits[sel.focus]) {
      open(hits[sel.focus])
      e.preventDefault()
    } else if (e.key === 'Escape' && selectedCount(sel) > 0) {
      choose(NO_SELECTION)
      e.preventDefault()
    }
  }

  const menuIndex = useRef<number | null>(null)
  const rowIndexOf = (target: EventTarget | null): number | null => {
    const row = target instanceof Element ? target.closest<HTMLElement>('[data-index]') : null
    return row ? Number(row.dataset.index) : null
  }
  const menu = useContextMenuTrigger({
    items: (): ContextMenuItem[] => {
      const i = menuIndex.current
      const h = i === null ? undefined : hits[i]
      if (i === null || !h) return []
      if (!isSelected(sel, i) || selectedCount(sel) !== 1) choose(selectOne(i))
      const it = itemOf(h)
      const folder = isFolderLike(it)
      const kind = previewKind(it)
      const out: ContextMenuItem[] = []
      if (folder) out.push({ kind: 'action', id: 'open', labelKey: 'files.menu.open', onSelect: () => onFolder(h.path) })
      else {
        if (kind) {
          out.push({
            kind: 'action',
            id: 'preview',
            labelKey: 'files.menu.preview',
            icon: 'view-reveal-symbolic',
            onSelect: () => onPreview(h.path, it.name, kind),
          })
        }
        out.push({
          kind: 'action',
          id: 'download',
          labelKey: 'files.menu.download',
          icon: 'download-symbolic',
          disabled: h.type !== 'file',
          onSelect: () => onDownload(h.path, it.name),
        })
      }
      out.push({ kind: 'action', id: 'locate', labelKey: 'files.search.locate', icon: 'folder-open-symbolic', onSelect: () => onLocate(h.path) })
      return out
    },
  })

  const raf = useRef<number | undefined>(undefined)
  const onScroll = (e: UIEvent<HTMLDivElement>) => {
    const top = e.currentTarget.scrollTop
    if (raf.current !== undefined) return
    raf.current = requestAnimationFrame(() => {
      raf.current = undefined
      setScrollTop(scrollRef.current?.scrollTop ?? top)
    })
  }
  useEffect(
    () => () => {
      if (raf.current !== undefined) cancelAnimationFrame(raf.current)
    },
    [],
  )

  const rows = []
  for (let i = lay.first; i < lay.last; i++) {
    const h = hits[i]
    if (!h) continue
    const it = itemOf(h)
    const kind = kindOf(it)
    const isSel = isSelected(sel, i)
    rows.push(
      <div
        key={h.path}
        id={`${baseId}-s${i}`}
        data-index={i}
        className={['files-row', 'search', isSel && 'selected', i === sel.focus && 'current'].filter(Boolean).join(' ')}
        role="row"
        aria-rowindex={i + 2}
        aria-selected={isSel}
        style={{ top: lay.top + (i - lay.first) * ROW_HEIGHT }}
        onClick={(e) => {
          if (menu.openedByPress()) return
          const mod = e.ctrlKey || e.metaKey
          if (e.shiftKey) choose(extendTo(sel, i, mod))
          else if (mod) choose(toggleAt(sel, i))
          else choose(selectOne(i))
          if (e.detail === 2 && !e.shiftKey && !mod) open(h)
        }}
      >
        <span className="files-cell name" role="gridcell">
          <span className="files-icon">
            <IconImage name={kindIcon(kind)} size={20} />
          </span>
          <span className="files-name">{it.name}</span>
        </span>
        <span className="files-cell from" role="gridcell" title={dirName(h.path)}>
          {locationLabel(roots, h.path, homeName)}
        </span>
        <span className="files-cell kind" role="gridcell">
          {h.type === 'unknown' ? '' : t(KIND_KEYS[kind])}
        </span>
      </div>,
    )
  }
  const activeId = sel.focus !== null && sel.focus >= lay.first && sel.focus < lay.last ? `${baseId}-s${sel.focus}` : undefined

  const running = state.status === 'running'
  const status =
    state.status === 'error'
      ? null
      : t(running ? 'files.search.running' : 'files.search.done', { count: total, total: fmt(total), dirs: fmt(state.dirs) })

  return (
    <>
      <div className="files-actions files-search-bar" role="status">
        {running && <span className="files-search-spinner" aria-hidden="true" />}
        <span className="files-search-status">
          {t('files.search.title', { query: search.query, folder: folderLabel(roots, search.path, homeName) })}
          {status && <span>{status}</span>}
        </span>
        {running ? (
          <button type="button" className="addon-button files-action-button" onClick={stop}>
            {t('files.search.stop')}
          </button>
        ) : (
          <button type="button" className="addon-button files-action-button" onClick={onRetry}>
            {t('files.search.again')}
          </button>
        )}
      </div>
      {state.status === 'done' && state.truncated && (
        <p className="addon-banner notice files-banner" role="status">
          <IconImage name="dialog-information-symbolic" size={16} />
          {t('files.search.truncated', { total: fmt(total) })}
        </p>
      )}
      {state.status === 'done' && state.stopped && (
        <p className="addon-banner notice files-banner" role="status">
          <IconImage name="dialog-information-symbolic" size={16} />
          {t('files.search.stopped')}
        </p>
      )}
      {state.status !== 'error' && state.skipped > 0 && (
        <p className="addon-banner notice files-banner" role="status">
          <IconImage name="dialog-warning-symbolic" size={16} />
          {t('files.search.skipped', { count: state.skipped, total: fmt(state.skipped) })}
        </p>
      )}
      {state.status === 'error' && (
        <p className="addon-banner files-banner" role="alert">
          {t(errorKey(state.err))}
        </p>
      )}
      <div className={`files-content${pane ? ' with-pane' : ''}`}>
        <div className="files-list" role="grid" aria-label={t('files.search.label')} aria-rowcount={total + 1} aria-multiselectable="true">
          <div className="files-row header search" role="row" aria-rowindex={1}>
            <span className="files-cell name" role="columnheader">
              {t('files.columns.name')}
            </span>
            <span className="files-cell from" role="columnheader">
              {t('files.search.location')}
            </span>
            <span className="files-cell kind" role="columnheader">
              {t('files.columns.kind')}
            </span>
          </div>
          <div
            ref={scrollRef}
            className="files-scroll"
            tabIndex={0}
            aria-activedescendant={activeId}
            aria-busy={running || undefined}
            onScroll={onScroll}
            onClick={(e) => {
              if (menu.openedByPress() || rowIndexOf(e.target) !== null) return
              if (selectedCount(sel) > 0) choose(NO_SELECTION)
            }}
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
              menuIndex.current = sel.focus
              menu.handlers.onKeyDown(e)
              if (!e.defaultPrevented) onKeyDown(e)
            }}
          >
            <div className="files-spacer" style={{ height: lay.scrollHeight }}>
              {rows}
            </div>
            {!running && total === 0 && state.status !== 'error' && (
              <p className="files-empty">{t('files.list.noMatch', { query: search.query })}</p>
            )}
          </div>
        </div>
        {pane && (
          <InfoPane
            path={pane}
            roots={roots}
            admin={admin}
            canWrite={false}
            onRename={() => {}}
            onDelete={() => {}}
            onPreview={(kind) => onPreview(pane, baseName(pane), kind)}
            onClose={() => setPane(null)}
          />
        )}
      </div>
      <footer className="files-status">
        {status && <span>{status}</span>}
        {selectedCount(sel) > 0 && <span>{t('files.status.selected', { count: selectedCount(sel), total: fmt(selectedCount(sel)) })}</span>}
      </footer>
    </>
  )
}
