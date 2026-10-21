// 파일 탐색기의 휴지통 보기(시안 AC-Trash). 사용자의 모든 휴지통(홈 휴지통, 디스크의 .Trash/<uid>·.Trash-<uid>) 항목을
// 지운 시각이 늦은 것부터 보이고 되돌리기·영구 삭제·휴지통 비우기를 둔다. 휴지통은 서버의 trash-list·trash-restore와
// 같다. 휴지통이 없는 디스크는 위에 알린다. 항목을 하나 고르면 오른쪽에 정보 패널을 연다.
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
import { ApiError, emptyTrash, fetchTrash, fetchTrashVolumes, purgeTrash, restoreTrash } from '../api/client'
import type { TrashItem, TrashVolume } from '../api/types.gen'
import { formatBytes, formatDateTime } from '../addons/common/format'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import type { ContextMenuItem } from '../shell/contextMenu'
import type { Dialogs } from '../shell/dialogPresets'
import { useContextMenuTrigger } from '../shell/useContextMenuTrigger'
import { DeleteBody, DeleteTitle } from './DeleteBody'
import { KIND_KEYS, kindIcon, kindOf } from './kinds'
import { errorKey } from './messages'
import { askName } from './nameDialog'
import { type RunOp, onFilesChanged, opErrorKey } from './ops'
import { type Root, TRASH, baseName, dirName, joinPath, locationLabel, volumeOf, within } from './paths'
import {
  NO_SELECTION,
  type Selection,
  extendTo,
  isSelected,
  selectAll,
  selectOne,
  selectedCount,
  selectedIndices,
  soleIndex,
  toggleAt,
} from './selection'
import { withStepUp } from './stepUp'
import { ROW_HEIGHT, layout, scrollToShow } from './virtual'

interface Props {
  // query는 도구 막대의 찾기어다(이름에 든 항목만 보인다).
  query: string
  roots: readonly Root[]
  dialogs: Dialogs
  // canWrite는 다른 작업이 진행 중이 아닌지다.
  canWrite: boolean
  runOp: RunOp
  // changed는 이 창이 바꾼 폴더를 알린다(FilesWindow).
  changed: (dirs: readonly string[], gone?: readonly string[]) => void
  // reload가 바뀌면 다시 받는다(도구 막대의 새로 고침).
  reload: number
}

type ListState = { status: 'loading' } | { status: 'ready'; items: TrashItem[]; truncated: boolean } | { status: 'error'; err: unknown }

export function TrashView({ query, roots, dialogs, canWrite, runOp, changed, reload }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const baseId = useId()
  const [list, setList] = useState<ListState>({ status: 'loading' })
  const [volumes, setVolumes] = useState<TrashVolume[]>([])
  const [sel, setSel] = useState<Selection>(NO_SELECTION)
  const [version, setVersion] = useState(0)
  const scrollRef = useRef<HTMLDivElement>(null)
  const [viewHeight, setViewHeight] = useState(0)
  const [scrollTop, setScrollTop] = useState(0)

  // 목록을 받는다. 이 창이나 다른 창이 휴지통을 바꾸면 다시 받는다.
  useEffect(() => {
    const ctl = new AbortController()
    fetchTrash(ctl.signal).then(
      (r) => {
        setList({ status: 'ready', items: r.items, truncated: r.truncated })
        setSel(NO_SELECTION)
      },
      (err: unknown) => {
        if (!ctl.signal.aborted) setList({ status: 'error', err })
      },
    )
    fetchTrashVolumes(ctl.signal).then(
      (r) => setVolumes(r.volumes),
      () => {},
    )
    return () => ctl.abort()
  }, [version, reload])
  useEffect(() => onFilesChanged((c) => c.dirs.includes(TRASH) && setVersion((v) => v + 1)), [])

  const all = useMemo(() => (list.status === 'ready' ? list.items : []), [list])
  const items = useMemo(() => {
    const q = query.toLowerCase()
    return q ? all.filter((i) => i.name.toLowerCase().includes(q)) : all
  }, [all, query])
  // 찾기어가 바뀌면 고르기를 지운다(자리가 바뀐다).
  const [prevQuery, setPrevQuery] = useState(query)
  if (prevQuery !== query) {
    setPrevQuery(query)
    setSel(NO_SELECTION)
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

  const total = items.length
  const lay = layout(total, scrollTop, viewHeight)
  const one = soleIndex(sel)
  const current = one === null ? null : (items[one] ?? null)
  const chosen = useCallback((s: Selection) => selectedIndices(s).map((i) => items[i]).filter(Boolean), [items])

  // 휴지통이 없는 곳: 보이는 폴더 가운데 그 디스크에 휴지통이 없는 것(시안의 안내)
  const noTrash = roots.filter((r) => r.reachable && r.kind !== 'home' && volumeOf(volumes, r.path)?.kind === 'none').map((r) => r.name)

  const fmt = (n: number) => new Intl.NumberFormat(lang).format(n)
  const totalSize = all.reduce((n, i) => n + (i.type === 'file' ? i.size : 0), 0)

  // restoreItems는 원래 자리로 되돌린다. 하나일 때 같은 이름이 있으면 새 이름을 묻는다.
  const restoreItems = async (picked: readonly TrashItem[]) => {
    const usable = picked.filter((i) => !i.badName)
    if (usable.length === 0 || !canWrite) return
    if (usable.length === 1) {
      const it = usable[0]
      try {
        await withStepUp(dialogs.scope, () => restoreTrash(it.trash, it.name, it.path))
        changed([TRASH, dirName(it.path)])
      } catch (err) {
        if (err instanceof ApiError && err.code === 'file_exists') {
          const dir = dirName(it.path)
          const done = await askName(dialogs.scope, {
            mode: 'restore',
            message: t('files.trash.conflict', { name: baseName(it.path) }),
            initial: baseName(it.path),
            folder: it.type === 'dir',
            submit: (name) => withStepUp(dialogs.scope, () => restoreTrash(it.trash, it.name, joinPath(dir, name))),
          })
          if (done) changed([TRASH, dir])
        } else if (!(err instanceof ApiError && (err.status === 401 || err.code === 'security_required'))) {
          await dialogs.alert({ title: it.name, message: t(opErrorKey(err)) })
        }
      }
      return
    }
    await runOp('restore', usable, (it) => restoreTrash(it.trash, it.name, it.path), {
      nameOf: (i) => i.name,
      change: (done) => ({ dirs: [TRASH, ...new Set(done.map((i) => dirName(i.path)))], gone: [] }),
    })
  }

  const purgeItems = async (picked: readonly TrashItem[]) => {
    const usable = picked.filter((i) => !i.badName)
    if (usable.length === 0 || !canWrite) return
    const ok = await dialogs.confirm({
      title: <DeleteTitle items={usable} mode="purge" />,
      message: <DeleteBody items={usable} mode="purge" />,
      confirmLabel: t('files.delete.confirm'),
      danger: true,
    })
    if (!ok) return
    await runOp('purge', usable, (it) => purgeTrash(it.trash, it.name), {
      nameOf: (i) => i.name,
      change: () => ({ dirs: [TRASH], gone: [] }),
    })
  }

  const empty = async () => {
    if (all.length === 0 || !canWrite) return
    const ok = await dialogs.confirm({
      title: t('files.trash.emptyTitle'),
      message: t('files.trash.emptyMessage', { count: all.length, total: fmt(all.length) }),
      confirmLabel: t('files.trash.empty'),
      danger: true,
    })
    if (!ok) return
    await runOp('empty', [TRASH], () => emptyTrash(), { nameOf: () => t('files.nav.trash'), change: () => ({ dirs: [TRASH], gone: [] }) })
  }

  const reveal = (i: number) => {
    const el = scrollRef.current
    if (!el) return
    const next = scrollToShow(i, total, el.scrollTop, el.clientHeight)
    if (next !== el.scrollTop) el.scrollTop = next
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const cur = sel.focus ?? -1
    const page = Math.max(1, Math.floor(viewHeight / ROW_HEIGHT) - 1)
    const go = (to: number) => {
      const i = Math.min(Math.max(0, to), total - 1)
      if (i < 0) return
      setSel(e.shiftKey ? extendTo(sel, i) : selectOne(i))
      reveal(i)
    }
    switch (e.key) {
      case 'ArrowDown':
        go(cur + 1)
        break
      case 'ArrowUp':
        go(cur <= 0 ? 0 : cur - 1)
        break
      case 'PageDown':
        go(cur + page)
        break
      case 'PageUp':
        go(cur - page)
        break
      case 'Home':
        go(0)
        break
      case 'End':
        go(total - 1)
        break
      case 'a':
      case 'A':
        if (!(e.ctrlKey || e.metaKey)) return
        setSel(selectAll(sel, total))
        break
      case 'Delete':
        void purgeItems(chosen(sel))
        break
      case 'Escape':
        if (selectedCount(sel) === 0) return
        setSel(NO_SELECTION)
        break
      default:
        return
    }
    e.preventDefault()
  }

  const menuIndex = useRef<number | null>(null)
  const rowIndexOf = (target: EventTarget | null): number | null => {
    const row = target instanceof Element ? target.closest<HTMLElement>('[data-index]') : null
    return row ? Number(row.dataset.index) : null
  }
  const menu = useContextMenuTrigger({
    items: (): ContextMenuItem[] => {
      const i = menuIndex.current
      if (i === null || !items[i]) {
        return [
          {
            kind: 'action',
            id: 'empty',
            labelKey: 'files.trash.emptyMenu',
            icon: 'edit-delete-symbolic',
            disabled: all.length === 0 || !canWrite,
            onSelect: () => void empty(),
          },
        ]
      }
      const s = isSelected(sel, i) ? sel : selectOne(i)
      setSel(s)
      const picked = chosen(s)
      const usable = canWrite && picked.some((p) => !p.badName)
      return [
        { kind: 'action', id: 'restore', labelKey: 'files.trash.restore', disabled: !usable, onSelect: () => void restoreItems(picked) },
        {
          kind: 'action',
          id: 'purge',
          labelKey: 'files.trash.purgeMenu',
          icon: 'edit-delete-symbolic',
          disabled: !usable,
          detailKey: 'files.menu.deleteKey',
          onSelect: () => void purgeItems(picked),
        },
      ]
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
  useEffect(() => () => {
    if (raf.current !== undefined) cancelAnimationFrame(raf.current)
  }, [])

  const homeName = t('files.nav.home')
  const where = (it: TrashItem) => locationLabel(roots, it.path, homeName)
  const storedLabel = (st: Stored) =>
    st.kind === 'home' ? t('files.trash.storedHome', { home: homeName }) : t('files.trash.storedDisk', { disk: st.disk })
  const rows = []
  for (let i = lay.first; i < lay.last; i++) {
    const it = items[i]
    if (!it) continue
    const isSel = isSelected(sel, i)
    const kind = kindOf({ name: it.name, type: it.type })
    rows.push(
      <div
        key={`${it.trash}/${it.name}`}
        id={`${baseId}-t${i}`}
        data-index={i}
        className={['files-row', 'trash', isSel && 'selected', i === sel.focus && 'current', it.badName && 'dim'].filter(Boolean).join(' ')}
        role="row"
        aria-rowindex={i + 2}
        aria-selected={isSel}
        style={{ top: lay.top + (i - lay.first) * ROW_HEIGHT }}
        onClick={(e) => {
          if (menu.openedByPress()) return
          const mod = e.ctrlKey || e.metaKey
          if (e.shiftKey) setSel(extendTo(sel, i, mod))
          else if (mod) setSel(toggleAt(sel, i))
          else setSel(selectOne(i))
        }}
      >
        <span className="files-cell name" role="gridcell">
          <span className="files-icon">
            <IconImage name={kindIcon(kind)} size={20} />
          </span>
          <span className="files-name">{it.name}</span>
        </span>
        <span className="files-cell from" role="gridcell" title={it.path}>
          {where(it)}
        </span>
        <span className="files-cell date" role="gridcell">
          {it.deletedAt ? formatDateTime(new Date(it.deletedAt).toISOString(), lang) : ''}
        </span>
        <span className="files-cell size" role="gridcell">
          {it.type === 'file' ? formatBytes(it.size, lang) : ''}
        </span>
      </div>,
    )
  }

  const picked = chosen(sel)
  const usable = canWrite && picked.some((p) => !p.badName)
  const activeId = sel.focus !== null && sel.focus >= lay.first && sel.focus < lay.last ? `${baseId}-t${sel.focus}` : undefined

  return (
    <>
      <div className="files-actions">
        <button type="button" className="addon-button primary files-action-button" disabled={!usable} onClick={() => void restoreItems(picked)}>
          {t('files.trash.restore')}
        </button>
        <button type="button" className="addon-button files-action-button" disabled={!usable} onClick={() => void purgeItems(picked)}>
          {t('files.trash.purge')}
        </button>
        <button type="button" className="addon-button files-action-button" disabled={all.length === 0 || !canWrite} onClick={() => void empty()}>
          {t('files.trash.emptyMenu')}
        </button>
      </div>
      {noTrash.length > 0 && (
        <p className="addon-banner notice files-banner" role="status">
          <IconImage name="dialog-warning-symbolic" size={16} />
          {t('files.trash.noTrash', { names: noTrash.join(', ') })}
        </p>
      )}
      {list.status === 'ready' && list.truncated && (
        <p className="addon-banner notice files-banner" role="status">
          <IconImage name="dialog-information-symbolic" size={16} />
          {t('files.trash.truncated', { total: fmt(all.length) })}
        </p>
      )}
      <div className={`files-content${current ? ' with-pane' : ''}`}>
        {list.status === 'error' ? (
          <div className="files-center" role="alert">
            <p>{t(errorKey(list.err))}</p>
            <button type="button" className="addon-button" onClick={() => setVersion((v) => v + 1)}>
              {t('files.errors.retry')}
            </button>
          </div>
        ) : (
          <div className="files-list" role="grid" aria-label={t('files.trash.label')} aria-rowcount={total + 1} aria-multiselectable="true">
            <div className="files-row header trash" role="row" aria-rowindex={1}>
              <span className="files-cell name" role="columnheader">
                {t('files.columns.name')}
              </span>
              <span className="files-cell from" role="columnheader">
                {t('files.trash.from')}
              </span>
              <span className="files-cell date" role="columnheader">
                {t('files.trash.deleted')}
              </span>
              <span className="files-cell size" role="columnheader">
                {t('files.columns.size')}
              </span>
            </div>
            <div
              ref={scrollRef}
              className="files-scroll"
              tabIndex={0}
              aria-activedescendant={activeId}
              aria-busy={list.status === 'loading' || undefined}
              onScroll={onScroll}
              onClick={(e) => {
                if (menu.openedByPress() || rowIndexOf(e.target) !== null) return
                if (selectedCount(sel) > 0) setSel(NO_SELECTION)
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
              {list.status === 'ready' && total === 0 && (
                <p className="files-empty">{query ? t('files.list.noMatch', { query }) : t('files.trash.emptyList')}</p>
              )}
            </div>
          </div>
        )}
        {current && (
          <TrashPane
            item={current}
            location={where(current)}
            stored={storedLabel(storedIn(current, roots))}
            canWrite={canWrite}
            onRestore={() => void restoreItems([current])}
            onClose={() => setSel(NO_SELECTION)}
          />
        )}
      </div>
      <footer className="files-status">
        {list.status === 'ready' && (
          <span>{t('files.trash.status', { count: all.length, total: fmt(all.length), size: formatBytes(totalSize, lang) })}</span>
        )}
        {selectedCount(sel) > 0 && <span>{t('files.status.selected', { count: selectedCount(sel), total: fmt(selectedCount(sel)) })}</span>}
      </footer>
    </>
  )
}

type Stored = { kind: 'home' } | { kind: 'disk'; disk: string }

// storedIn은 항목이 든 휴지통이다: 홈 폴더의 휴지통, 또는 그 디스크(보이는 디스크 이름, 없으면 마운트 지점 이름).
function storedIn(it: TrashItem, roots: readonly Root[]): Stored {
  const home = roots.find((r) => r.kind === 'home')
  if (home && within(it.trash, home.path)) return { kind: 'home' }
  const top = it.trash.replace(/\/\.Trash(-\d+|\/\d+)$/, '')
  const disk = roots.find((r) => r.kind === 'disk' && r.path === top)
  return { kind: 'disk', disk: disk?.name ?? baseName(top) }
}

function TrashPane({
  item,
  location,
  stored,
  canWrite,
  onRestore,
  onClose,
}: {
  item: TrashItem
  location: string
  stored: string
  canWrite: boolean
  onRestore: () => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const kind = kindOf({ name: item.name, type: item.type })
  return (
    <aside className="files-info" aria-label={t('files.info.label')}>
      <button type="button" className="files-icon-button files-info-close" aria-label={t('files.info.close')} title={t('files.info.close')} onClick={onClose}>
        <IconImage name="window-close-symbolic" size={16} />
      </button>
      <div className="files-info-head">
        <IconImage name={kindIcon(kind)} size={72} />
        <h2 className="selectable">{item.name}</h2>
        <p>{item.type === 'file' ? t('files.info.kindSize', { kind: t(KIND_KEYS[kind]), size: formatBytes(item.size, lang) }) : t(KIND_KEYS[kind])}</p>
      </div>
      <div className="files-info-actions">
        <button type="button" className="addon-button primary" disabled={!canWrite || item.badName} onClick={onRestore}>
          {t('files.trash.restore')}
        </button>
      </div>
      <dl className="files-info-list">
        <dt>{t('files.trash.from')}</dt>
        <dd className="selectable">{location}</dd>
        <dt>{t('files.trash.deleted')}</dt>
        <dd>{item.deletedAt ? formatDateTime(new Date(item.deletedAt).toISOString(), lang) : t('files.info.unknown')}</dd>
        <dt>{t('files.info.modified')}</dt>
        <dd>{formatDateTime(new Date(item.modTime).toISOString(), lang)}</dd>
        <dt>{t('files.trash.stored')}</dt>
        <dd>{stored}</dd>
      </dl>
      <div className="files-info-note">
        <p>{t('files.trash.note')}</p>
      </div>
    </aside>
  )
}
