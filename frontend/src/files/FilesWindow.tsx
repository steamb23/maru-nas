// 파일 탐색기 창. 왼쪽 사이드바(홈 폴더, 공유 폴더, 디스크), 위 도구 막대(뒤로·앞으로·상위 폴더·새로 고침, 경로,
// 찾기), 자세히 목록(FileList), 파일을 눌렀을 때만 여는 정보 패널(InfoPane), 아래 상태 표시줄로 된 앱형 창이다.
// 파일은 권한 도우미가 로그인한 사용자의 권한으로 다룬다(백엔드 /api/files/*). 창마다 따로 둘러본다.
import { type ChangeEvent, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { WindowViewProps } from '../addons/types'
import { ApiError, type RequiredLevel, fetchFilesRoots, openListing, prepareListing } from '../api/client'
import type { FileItem, FilesListing, FilesRoots } from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import type { IconName } from '../icons/resolver'
import { requestStepUp } from '../security/stepUpDialog'
import { SidebarLayout } from '../shell/SidebarLayout'
import type { ContextMenuItem } from '../shell/contextMenu'
import { useDialogs } from '../shell/dialogPresets'
import { FileList, type SortKey, type View } from './FileList'
import { InfoPane } from './InfoPane'
import { UploadPanel } from './UploadPanel'
import { startDownload } from './download'
import { type History, back, canBack, canForward, current, forward, replaceCurrent, startHistory, visit } from './history'
import { isFolderLike } from './kinds'
import { errorKey } from './messages'
import { type Root, crumbs, findRoot, joinPath, parentOf, rootsOf } from './paths'
import { useUploads } from './useUploads'

// 창이 이보다 좁으면 사이드바를 숨기고 메뉴 단추로 덮어 연다(SidebarLayout).
const FILES_NARROW_WIDTH = 720
// 찾기 입력을 멈춘 뒤 보기를 바꾸기까지 기다리는 시간
const QUERY_DELAY_MS = 250
// 날짜·크기순 준비 배너는 이 시간이 지나도 끝나지 않을 때만 보인다(작은 폴더는 깜박이지 않게).
const PREPARE_BANNER_MS = 300
// 찾기 상자에 항목 수를 함께 보이는 기준
const MANY_ITEMS = 10_000

// 상태에는 그 결과를 받은 요청(try, path)을 함께 둔다. 지금 요청과 다르면 받는 중으로 본다(effect 안에서 상태를 지우지 않게).
type RootsState = { try: number } & (
  | { status: 'loading' }
  | { status: 'ready'; info: FilesRoots; roots: Root[] }
  | { status: 'error'; err: unknown }
  | { status: 'security'; level?: RequiredLevel }
)

type ListingState = { path: string | null } & (
  | { status: 'loading' }
  | { status: 'ready'; listing: FilesListing }
  | { status: 'error'; err: unknown }
)

interface Prepare {
  sort: SortKey
  desc: boolean
  filled: number
  total: number
  shown: boolean
}

const DEFAULT_VIEW: View = { sort: 'name', desc: false, query: '' }

export function FilesWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const dialogs = useDialogs()
  const admin = useAuthStore((s) => s.user?.admin === true)
  const [rootsTry, setRootsTry] = useState(0)
  const [rootsResult, setRootsState] = useState<RootsState>({ try: -1, status: 'loading' })
  const [history, setHistory] = useState<History | null>(null)
  const path = history ? current(history) : null
  const [listingResult, setListingState] = useState<ListingState>({ path: null, status: 'loading' })
  const [reload, setReload] = useState<{ n: number; refresh: boolean }>({ n: 0, refresh: false })
  const [view, setView] = useState<View>(DEFAULT_VIEW)
  const [queryInput, setQueryInput] = useState('')
  const [selected, setSelected] = useState<{ index: number; item: FileItem | null } | null>(null)
  const [pane, setPane] = useState<string | null>(null)
  const [viewTotal, setViewTotal] = useState<number | null>(null)
  const [prep, setPrep] = useState<Prepare | null>(null)
  const [listError, setListError] = useState<unknown>(null)
  // refocus는 목록에 포커스가 있던 채로 다른 폴더로 갔는지다. 새 목록이 그려지면 포커스를 돌려준다(키보드로 둘러보기).
  const [refocus, setRefocus] = useState(false)
  const prepCtl = useRef<AbortController | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const reopen = useCallback((refresh: boolean) => {
    setListError(null)
    setReload((r) => ({ n: r.n + 1, refresh }))
  }, [])
  const uploads = useUploads(dialogs, (dir) => {
    if (dir === path) reopen(false)
  })

  // 처음 보이는 폴더. 보안 확인이 필요하면 확인 단추를 보인다.
  useEffect(() => {
    const ctl = new AbortController()
    fetchFilesRoots(ctl.signal).then(
      (info) => {
        const roots = rootsOf(info)
        setRootsState({ try: rootsTry, status: 'ready', info, roots })
        setHistory((h) => {
          if (h && findRoot(roots, current(h))) return h
          const first = roots.find((r) => r.reachable)
          return first ? startHistory(first.path) : null
        })
      },
      (err: unknown) => {
        if (ctl.signal.aborted) return
        if (err instanceof ApiError && err.code === 'security_required') {
          setRootsState({ try: rootsTry, status: 'security', level: err.required })
        } else setRootsState({ try: rootsTry, status: 'error', err })
      },
    )
    return () => ctl.abort()
  }, [rootsTry])
  const rootsState: RootsState = rootsResult.try === rootsTry ? rootsResult : { try: rootsTry, status: 'loading' }

  // 폴더 목록을 연다. 같은 폴더를 다시 여는 동안(새로 고침, 만료)은 이전 목록을 그대로 보인다.
  const rootsReady = rootsState.status === 'ready'
  useEffect(() => {
    if (!path || !rootsReady) return
    const ctl = new AbortController()
    openListing(path, reload.refresh, ctl.signal).then(
      (listing) => {
        // 다시 받은 목록은 순서가 바뀌었을 수 있으므로 고른 줄을 지운다(정보 패널은 경로로 그대로 둔다).
        setSelected(null)
        setListingState({ path, status: 'ready', listing })
      },
      (err: unknown) => {
        if (ctl.signal.aborted) return
        if (err instanceof ApiError && err.code === 'security_required') {
          setRootsState({ try: rootsTry, status: 'security', level: err.required })
          return
        }
        setListingState({ path, status: 'error', err })
      },
    )
    return () => ctl.abort()
  }, [path, reload, rootsReady, rootsTry])
  const listingState: ListingState = listingResult.path === path ? listingResult : { path, status: 'loading' }

  // 찾기 입력을 잠시 기다렸다가 보기에 반영한다.
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setView((v) => (v.query === queryInput ? v : { ...v, query: queryInput }))
      setSelected(null)
    }, QUERY_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [queryInput])

  useEffect(() => () => prepCtl.current?.abort(), [])

  const listing = listingState.status === 'ready' ? listingState.listing : null
  const roots = rootsState.status === 'ready' ? rootsState.roots : []
  const root = path ? findRoot(roots, path) : null

  // moveTo는 다른 폴더로 간다. 고른 항목·정보 패널·찾기·정렬 준비를 지우고, 날짜·크기순은 새 폴더의 정보를 다시
  // 읽어야 하므로 이름순으로 돌아간다.
  const moveTo = (next: (h: History | null) => History | null) => {
    setRefocus(document.activeElement instanceof HTMLElement && document.activeElement.classList.contains('files-scroll'))
    setSelected(null)
    setPane(null)
    setQueryInput('')
    setView((v) => (v.sort === 'name' ? { ...v, query: '' } : DEFAULT_VIEW))
    setViewTotal(null)
    prepCtl.current?.abort()
    setPrep(null)
    setListError(null)
    setHistory(next)
  }
  const navigate = (to: string) => {
    if (!findRoot(roots, to) || to === path) return
    moveTo((h) => (h ? visit(h, to) : startHistory(to)))
  }
  const goUp = () => {
    if (!root || !path) return
    const parent = parentOf(root, path)
    if (parent) navigate(parent)
  }

  // startPrepare는 날짜·크기순에 필요한 정보를 다 읽을 때까지 prepare를 부른다. 다 읽으면 그 순서로 바꾼다.
  const startPrepare = (sort: SortKey, desc: boolean) => {
    if (!listing) return
    prepCtl.current?.abort()
    const ctl = new AbortController()
    prepCtl.current = ctl
    setPrep({ sort, desc, filled: 0, total: listing.total, shown: false })
    const timer = window.setTimeout(() => setPrep((p) => (p ? { ...p, shown: true } : p)), PREPARE_BANNER_MS)
    const run = async () => {
      for (;;) {
        const r = await prepareListing(listing.id, ctl.signal)
        if (ctl.signal.aborted) return
        setPrep((p) => (p ? { ...p, filled: r.filled, total: r.total } : p))
        if (r.filled >= r.total) break
      }
      setView((v) => ({ ...v, sort, desc }))
      setSelected(null)
      setPrep(null)
    }
    run()
      .catch((err: unknown) => {
        if (ctl.signal.aborted) return
        setPrep(null)
        if (err instanceof ApiError && err.code === 'listing_expired') reopen(false)
        else setListError(err)
      })
      .finally(() => window.clearTimeout(timer))
  }
  const cancelPrepare = () => {
    prepCtl.current?.abort()
    setPrep(null)
  }

  const onSort = (key: SortKey) => {
    if (key === view.sort) {
      setView((v) => ({ ...v, desc: !v.desc }))
      setSelected(null)
      return
    }
    if (key === 'name') {
      cancelPrepare()
      setView((v) => ({ ...v, sort: 'name', desc: false }))
      setSelected(null)
      return
    }
    startPrepare(key, true)
  }

  const openItem = (item: FileItem) => {
    if (!path) return
    if (item.badName) {
      dialogs.alert({ title: item.name, message: t('files.list.badName') })
      return
    }
    if (item.locked) {
      dialogs.alert({ title: item.name, message: t('files.list.lockedFolder') })
      return
    }
    if (isFolderLike(item)) navigate(joinPath(path, item.name))
    else if (item.state !== 'gone') setPane(joinPath(path, item.name))
  }

  const onSelect = (index: number | null, item: FileItem | null) => {
    setSelected(index === null ? null : { index, item })
    if (!path || !item) return
    // 파일을 누르면 정보 패널을 연다. 폴더는 고르기만 한다(시안 AC-Main).
    if (isFolderLike(item) || item.type === 'dir') setPane(null)
    else if (item.state !== 'gone' && !item.badName) setPane(joinPath(path, item.name))
  }

  const download = (item: FileItem) => {
    if (!path) return
    startDownload(joinPath(path, item.name), item.name).catch((err: unknown) => {
      if (err instanceof ApiError && err.status === 401) return
      dialogs.alert({ title: item.name, message: t(errorKey(err)) })
    })
  }

  const menuItems = (item: FileItem): ContextMenuItem[] => {
    if (isFolderLike(item) || item.type === 'dir') {
      return [{ kind: 'action', id: 'open', labelKey: 'files.menu.open', disabled: item.locked, onSelect: () => openItem(item) }]
    }
    const usable = item.state !== 'gone' && !item.badName
    return [
      {
        kind: 'action',
        id: 'download',
        labelKey: 'files.menu.download',
        icon: 'download-symbolic',
        disabled: !usable || item.type !== 'file',
        onSelect: () => download(item),
      },
      { kind: 'action', id: 'info', labelKey: 'files.menu.info', disabled: !usable, onSelect: () => openItem(item) },
    ]
  }

  const onPick = (e: ChangeEvent<HTMLInputElement>) => {
    const picked = Array.from(e.currentTarget.files ?? [])
    e.currentTarget.value = ''
    if (path && picked.length > 0) uploads.add(path, picked)
  }

  const verify = async () => {
    const level = rootsState.status === 'security' ? rootsState.level : undefined
    if (await requestStepUp(dialogs.scope, { level, target: 'files' })) setRootsTry((n) => n + 1)
  }

  const fmt = (n: number) => new Intl.NumberFormat(lang).format(n)

  // 보안 확인이 필요하면 안내와 확인 단추만 보인다(웹 터미널과 같은 공통 확인 대화상자).
  if (rootsState.status === 'security') {
    return (
      <div className="files-center">
        <IconImage name="security-high-symbolic" size={40} />
        <p>{t('files.security.message')}</p>
        <button type="button" className="addon-button primary" onClick={verify}>
          {t('files.security.verify')}
        </button>
      </div>
    )
  }
  if (rootsState.status === 'error') {
    return (
      <div className="files-center" role="alert">
        <p>{t(errorKey(rootsState.err))}</p>
        <button type="button" className="addon-button" onClick={() => setRootsTry((n) => n + 1)}>
          {t('files.errors.retry')}
        </button>
      </div>
    )
  }

  const navItem = (r: Root, icon: IconName) => (
    <button
      key={`${r.kind}:${r.path}`}
      type="button"
      className="sidebar-nav-item"
      aria-current={root === r ? 'page' : undefined}
      aria-disabled={!r.reachable || undefined}
      title={r.reachable ? r.path : t('files.nav.unreachable')}
      onClick={() => r.reachable && navigate(r.path)}
    >
      <IconImage name={icon} size={16} />
      <span>{r.name}</span>
    </button>
  )
  const homes = roots.filter((r) => r.kind === 'home')
  const shares = roots.filter((r) => r.kind === 'share')
  const disks = roots.filter((r) => r.kind === 'disk')
  const fallback = rootsState.status === 'ready' && rootsState.info.fallback

  const nav = (
    <>
      {homes.map((r) => (
        <button
          key="home"
          type="button"
          className="sidebar-nav-item"
          aria-current={root === r ? 'page' : undefined}
          title={r.path}
          onClick={() => navigate(r.path)}
        >
          <IconImage name="user-home-symbolic" size={16} />
          <span>{t('files.nav.home')}</span>
        </button>
      ))}
      {shares.length > 0 && <h3 className="files-nav-group">{t('files.nav.shares')}</h3>}
      {shares.map((r) => navItem(r, 'folder-remote-symbolic'))}
      {disks.length > 0 && <h3 className="files-nav-group">{t('files.nav.disks')}</h3>}
      {disks.map((r) => navItem(r, 'drive-harddisk-symbolic'))}
      {fallback && (
        <p className="files-nav-note">
          <IconImage name="dialog-information-symbolic" size={14} />
          <span>{t('files.nav.fallback')}</span>
        </p>
      )}
      {rootsState.status === 'ready' && !fallback && shares.length === 0 && roots.length > 0 && (
        <p className="files-nav-note">
          <IconImage name="dialog-information-symbolic" size={14} />
          <span>{t('files.nav.noShares')}</span>
        </p>
      )}
      {rootsState.status === 'ready' && roots.length === 0 && <p className="files-nav-note">{t('files.nav.none')}</p>}
    </>
  )

  const total = viewTotal ?? listing?.total ?? 0
  const writable = listing?.writable ?? false
  const header = (
    <div className="files-toolbar">
      <div className="files-toolbar-buttons">
        <button
          type="button"
          className="files-icon-button"
          aria-label={t('files.toolbar.back')}
          title={t('files.toolbar.back')}
          disabled={!history || !canBack(history)}
          onClick={() => moveTo((h) => (h ? back(h) : h))}
        >
          <IconImage name="go-previous-symbolic" size={16} />
        </button>
        <button
          type="button"
          className="files-icon-button"
          aria-label={t('files.toolbar.forward')}
          title={t('files.toolbar.forward')}
          disabled={!history || !canForward(history)}
          onClick={() => moveTo((h) => (h ? forward(h) : h))}
        >
          <IconImage name="go-next-symbolic" size={16} />
        </button>
        <button
          type="button"
          className="files-icon-button"
          aria-label={t('files.toolbar.up')}
          title={t('files.toolbar.up')}
          disabled={!root || !path || parentOf(root, path) === null}
          onClick={goUp}
        >
          <IconImage name="go-up-symbolic" size={16} />
        </button>
        <button
          type="button"
          className="files-icon-button"
          aria-label={t('files.toolbar.refresh')}
          title={t('files.toolbar.refresh')}
          disabled={!path}
          onClick={() => reopen(true)}
        >
          <IconImage name="view-refresh-symbolic" size={16} />
        </button>
      </div>
      <nav className="files-path" aria-label={t('files.toolbar.path')}>
        {root &&
          path &&
          crumbs(root, path).map((c, i, all) => (
            <span key={c.path} className="files-crumb">
              {i > 0 && <span className="files-crumb-sep" aria-hidden="true" />}
              <button
                type="button"
                aria-current={i === all.length - 1 ? 'page' : undefined}
                onClick={() => navigate(c.path)}
                title={c.path}
              >
                {i === 0 && <IconImage name={root.kind === 'home' ? 'user-home-symbolic' : root.kind === 'disk' ? 'drive-harddisk-symbolic' : 'folder-remote-symbolic'} size={16} />}
                <span>{i === 0 && root.kind === 'home' ? t('files.nav.home') : c.name}</span>
              </button>
            </span>
          ))}
      </nav>
      <label className="files-search">
        <IconImage name="edit-find-symbolic" size={16} />
        <input
          type="search"
          value={queryInput}
          placeholder={
            listing && listing.total > MANY_ITEMS ? t('files.toolbar.searchMany', { total: fmt(listing.total) }) : t('files.toolbar.search')
          }
          aria-label={t('files.toolbar.search')}
          maxLength={255}
          onChange={(e) => setQueryInput(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape' && queryInput) {
              e.preventDefault()
              setQueryInput('')
            }
          }}
        />
      </label>
    </div>
  )

  const sortName: Record<SortKey, string> = {
    name: t('files.status.sortName'),
    mtime: t('files.status.sortMtime'),
    size: t('files.status.sortSize'),
  }

  return (
    <SidebarLayout className="files" narrowWidth={FILES_NARROW_WIDTH} navLabel={t('files.nav.label')} nav={nav} header={header}>
      <div className="files-actions">
        <button
          type="button"
          className="addon-button primary files-upload-button"
          disabled={!listing || !writable}
          title={listing && !writable ? t('files.toolbar.uploadDenied') : undefined}
          onClick={() => fileInput.current?.click()}
        >
          <IconImage name="upload-symbolic" size={16} />
          {t('files.toolbar.upload')}
        </button>
        <input ref={fileInput} type="file" multiple hidden onChange={onPick} />
      </div>
      {listing?.truncated && (
        <p className="addon-banner notice files-banner" role="status">
          <IconImage name="dialog-warning-symbolic" size={16} />
          {t('files.banner.truncated')}
        </p>
      )}
      {prep?.shown && (
        <div className="files-prepare" role="status">
          <div className="files-prepare-line">
            <IconImage name="dialog-information-symbolic" size={16} />
            <span>{prep.sort === 'mtime' ? t('files.banner.prepareMtime') : t('files.banner.prepareSize')}</span>
            <span className="files-prepare-count">{t('files.banner.progress', { done: fmt(prep.filled), total: fmt(prep.total) })}</span>
            <button type="button" className="addon-button" onClick={cancelPrepare}>
              {t('files.banner.cancel')}
            </button>
          </div>
          <div className="meter" aria-hidden="true">
            <div className="meter-fill" style={{ width: `${(prep.filled / Math.max(1, prep.total)) * 100}%` }} />
          </div>
        </div>
      )}
      {listError !== null && (
        <p className="addon-banner files-banner" role="alert">
          {t(errorKey(listError))}
        </p>
      )}
      <div className={`files-content${pane ? ' with-pane' : ''}`}>
        {listingState.status === 'error' ? (
          <div className="files-center" role="alert">
            <p>{t(errorKey(listingState.err))}</p>
            <div className="files-center-buttons">
              {root && path && parentOf(root, path) && (
                <button
                  type="button"
                  className="addon-button"
                  onClick={() => {
                    const parent = parentOf(root, path)
                    if (parent) moveTo((h) => (h ? replaceCurrent(h, parent) : h))
                  }}
                >
                  {t('files.toolbar.up')}
                </button>
              )}
              <button type="button" className="addon-button" onClick={() => reopen(true)}>
                {t('files.errors.retry')}
              </button>
            </div>
          </div>
        ) : listing ? (
          <FileList
            listing={listing}
            view={view}
            active={active}
            selected={selected?.index ?? null}
            onSelect={onSelect}
            onOpen={openItem}
            onUp={goUp}
            onSort={onSort}
            onTotal={setViewTotal}
            onExpired={() => reopen(false)}
            onSortPending={() => {
              const { sort, desc } = view
              setView((v) => ({ ...v, sort: 'name', desc: false }))
              if (sort !== 'name') startPrepare(sort, desc)
            }}
            onError={setListError}
            menuItems={menuItems}
            focusOnMount={refocus}
            onFocused={() => setRefocus(false)}
          />
        ) : (
          <div className="files-list" aria-busy="true" />
        )}
        {pane && <InfoPane path={pane} roots={roots} admin={admin} onClose={() => setPane(null)} />}
        <UploadPanel state={uploads.state} onCancel={uploads.cancel} onRetry={uploads.retry} onClear={uploads.clear} />
      </div>
      <footer className="files-status">
        {listing && (
          <span>
            {listing.truncated
              ? t('files.status.shown', { total: fmt(listing.total) })
              : t('files.status.items', { count: total, total: fmt(total) })}
          </span>
        )}
        {selected && <span>{t('files.status.selected')}</span>}
        {prep ? (
          <span>{t('files.status.preparing', { sort: sortName[prep.sort] })}</span>
        ) : (
          listing && listing.total > MANY_ITEMS && <span>{sortName[view.sort]}</span>
        )}
        <span className="files-status-fill" />
        {listing && <span>{writable ? t('files.status.writable') : t('files.status.readOnly')}</span>}
      </footer>
    </SidebarLayout>
  )
}
