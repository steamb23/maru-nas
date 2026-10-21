// 파일 탐색기 창. 왼쪽 사이드바(홈 폴더, 공유 폴더, 디스크), 위 도구 막대(뒤로·앞으로·상위 폴더·새로 고침, 경로,
// 찾기), 자세히 목록(FileList), 파일을 눌렀을 때만 여는 정보 패널(InfoPane), 아래 상태 표시줄로 된 앱형 창이다.
// 파일은 권한 도우미가 로그인한 사용자의 권한으로 다룬다(백엔드 /api/files/*). 창마다 따로 둘러본다.
// 폴더 만들기·이름 바꾸기·삭제·옮기기(목록의 줄을 끌어 놓기)와, 바깥에서 끌어 온 파일 올리기를 한다. 폴더 안을 바꾸면
// 같은 폴더를 보는 다른 창에도 알린다(ops.ts의 notifyFilesChanged).
import { type ChangeEvent, type DragEvent, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { WindowViewProps } from '../addons/types'
import type { ParseKeys } from 'i18next'
import {
  ApiError,
  type RequiredLevel,
  deleteFile,
  fetchFilesRoots,
  fetchTrashVolumes,
  findInListing,
  makeFolder,
  openListing,
  prepareListing,
  renameFile,
  trashFile,
} from '../api/client'
import type { FileItem, FilesListing, FilesRoots, TrashVolume } from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import type { IconName } from '../icons/resolver'
import { requestStepUp } from '../security/stepUpDialog'
import { SidebarLayout } from '../shell/SidebarLayout'
import type { ContextMenuAction, ContextMenuItem } from '../shell/contextMenu'
import { useContextMenuStore } from '../shell/contextMenuStore'
import { useDialogs } from '../shell/dialogPresets'
import { DeleteBody, DeleteTitle, FailuresBody } from './DeleteBody'
import { dropMode } from './copyJobs'
import { DROP_TARGET, FileList, type FileListHandle, type SortKey, type View } from './FileList'
import { InfoPane } from './InfoPane'
import { type Search, SearchView } from './SearchView'
import { TrashView } from './TrashView'
import { startDownload } from './download'
import { type History, back, canBack, canForward, current, forward, replaceCurrent, startHistory, visit } from './history'
import { type PreviewKind, isFolderLike, previewKind } from './kinds'
import { openViewer } from '../viewer/viewerStore'
import { errorKey } from './messages'
import { askName } from './nameDialog'
import {
  type FilesChange,
  type OpKind,
  type RunOp,
  affects,
  groupFailures,
  notifyFilesChanged,
  onFilesChanged,
  runEach,
} from './ops'
import {
  type Root,
  TRASH,
  crumbs,
  dirName,
  findRoot,
  joinPath,
  moveAllowed,
  numberedName,
  parentOf,
  rootsOf,
  volumeOf,
  within,
} from './paths'
import { NO_SELECTION, type Selection, selectAll, selectOne, selectedCount, soleIndex } from './selection'
import { withStepUp } from './stepUp'
import { TaskBanner } from './TaskProgress'
import { startCopy, startUpload } from './transferStore'
import { checkUploads } from './uploads'
import { type FilesLayout, useFilesPrefsStore } from './viewPrefs'

// 창이 이보다 좁으면 사이드바를 숨기고 메뉴 단추로 덮어 연다(SidebarLayout).
const FILES_NARROW_WIDTH = 720
// 찾기 입력을 멈춘 뒤 보기를 바꾸기까지 기다리는 시간
const QUERY_DELAY_MS = 250
// 날짜·크기순 준비 배너는 이 시간이 지나도 끝나지 않을 때만 보인다(작은 폴더는 깜박이지 않게).
const PREPARE_BANNER_MS = 300
// 찾기 상자에 항목 수를 함께 보이는 기준
const MANY_ITEMS = 10_000
// 삭제·옮기기 진행 배너도 이 시간이 지나도 끝나지 않을 때만 보인다.
const OP_BANNER_MS = 300
// 새 폴더 이름에 번호를 붙여 볼 최대 수
const NEW_FOLDER_TRIES = 50
// 다른 창이 같은 폴더를 바꾸면 그 창이 새로 읽을 때까지 이만큼 기다렸다가 그 목록을 함께 쓴다.
const FOLLOW_MS = 300

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

// Op은 진행 중인 여러 항목 작업이다. 하나씩 하므로 창마다 하나만 한다.
interface Op {
  kind: OpKind
  done: number
  total: number
  // current는 지금 하는 항목의 이름이다(휴지통 비우기는 없다).
  current: string | null
  shown: boolean
}

const OP_KEYS: Record<OpKind, ParseKeys> = {
  delete: 'files.ops.deleting',
  move: 'files.ops.moving',
  trash: 'files.ops.trashing',
  restore: 'files.ops.restoring',
  purge: 'files.ops.purging',
  empty: 'files.ops.emptying',
}

const OP_FAILED_KEYS: Record<OpKind, ParseKeys> = {
  delete: 'files.ops.deleteFailed',
  move: 'files.ops.moveFailed',
  trash: 'files.ops.trashFailed',
  restore: 'files.ops.restoreFailed',
  purge: 'files.ops.purgeFailed',
  empty: 'files.ops.emptyFailed',
}

// trashLeft는 휴지통에 넣지 못해 영구 삭제를 따로 물을 실패다(휴지통이 없는 디스크, 다른 파일시스템).
const trashLeft = (err: unknown) => err instanceof ApiError && (err.code === 'no_trash' || err.code === 'cross_device')

// hasFiles는 바깥(운영체제)에서 파일을 끌어 왔는지다.
const hasFiles = (dt: DataTransfer) => Array.from(dt.types).includes('Files')

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
  const [sel, setSel] = useState<Selection>(NO_SELECTION)
  const [pane, setPane] = useState<string | null>(null)
  // deep이면 찾기 칸이 지금 폴더를 거르지 않고, Enter로 하위 폴더까지 찾는다. search가 있으면 목록 대신 검색 결과를 보인다.
  const [deep, setDeep] = useState(false)
  const [search, setSearch] = useState<Search | null>(null)
  // trashOpen이면 목록 대신 휴지통 보기를 보인다(둘러보던 폴더와 기록은 그대로 둔다).
  const [trashOpen, setTrashOpen] = useState(false)
  const [trashCount, setTrashCount] = useState<number | null>(null)
  // volumes는 홈 폴더와 디스크(도우미의 기준 디렉터리)다. 끌어 놓을 곳이 다른 디스크면 옮기지 않고 복사한다.
  const [volumes, setVolumes] = useState<readonly TrashVolume[] | null>(null)
  const [trashVersion, setTrashVersion] = useState(0)
  const [trashReload, setTrashReload] = useState(0)
  const [op, setOp] = useState<Op | null>(null)
  // pendingSelect는 만들거나 이름을 바꾼 항목이다. 목록을 다시 받으면(listingId와 다른 목록) 그 줄을 고른다.
  const [pendingSelect, setPendingSelect] = useState<{ name: string; listingId: string } | null>(null)
  // dropList는 바깥에서 끌어 온 파일을 지금 폴더에 놓으려는 중인지다(목록 테두리).
  const [dropList, setDropList] = useState(false)
  const [viewTotal, setViewTotal] = useState<number | null>(null)
  const [prep, setPrep] = useState<Prepare | null>(null)
  const [listError, setListError] = useState<unknown>(null)
  // refocus는 목록에 포커스가 있던 채로 다른 폴더로 갔는지다. 새 목록이 그려지면 포커스를 돌려준다(키보드로 둘러보기).
  const [refocus, setRefocus] = useState(false)
  const prepCtl = useRef<AbortController | null>(null)
  const opCtl = useRef<AbortController | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const listRef = useRef<FileListHandle>(null)
  // dropRow는 바깥 파일을 끄는 동안 놓을 곳으로 표시한 폴더 줄이다.
  const dropRow = useRef<HTMLElement | null>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  // layoutChoice는 이 창에서 고른 보기다. 고르기 전에는 저장된 보기(새 창의 기본값)를 따른다.
  const [layoutChoice, setLayoutChoice] = useState<FilesLayout | null>(null)
  const savedLayout = useFilesPrefsStore((s) => (s.status === 'ready' ? s.layout : 'list'))
  const layout = layoutChoice ?? savedLayout
  const sortButton = useRef<HTMLButtonElement>(null)
  // self는 변경 알림에서 이 창을 가리킨다.
  const [self] = useState(() => Symbol('files-window'))

  const reopen = useCallback((refresh: boolean) => {
    setListError(null)
    setReload((r) => ({ n: r.n + 1, refresh }))
  }, [])
  // changed는 이 창이 바꾼 폴더를 알린다. 이 창의 폴더가 바뀌었으면 이 창이 바로 새로 읽는다.
  const changed = (dirs: readonly string[], gone: readonly string[] = []) => {
    const c: FilesChange = { dirs, gone, origin: self }
    if (path && affects(c, path)) c.refreshed = path
    notifyFilesChanged(c)
  }

  useEffect(() => {
    void useFilesPrefsStore.getState().load()
  }, [])

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
        setSel(NO_SELECTION)
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

  // 찾기 입력을 잠시 기다렸다가 보기에 반영한다. 하위 폴더 검색 범위면 지금 폴더를 거르지 않는다.
  useEffect(() => {
    const next = deep ? '' : queryInput
    const timer = window.setTimeout(() => {
      setView((v) => (v.query === next ? v : { ...v, query: next }))
      setSel(NO_SELECTION)
    }, QUERY_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [queryInput, deep])

  useEffect(
    () => () => {
      prepCtl.current?.abort()
      opCtl.current?.abort()
    },
    [],
  )

  const listing = listingState.status === 'ready' ? listingState.listing : null
  const roots = rootsState.status === 'ready' ? rootsState.roots : []
  const root = path ? findRoot(roots, path) : null

  // 이 창이나 다른 창이 폴더 안을 바꾸면 그 폴더를 보는 창이 목록을 다시 연다. 없어진 파일의 정보 패널은 닫는다.
  useEffect(() => {
    let follow: number | undefined
    const off = onFilesChanged((c) => {
      if (c.dirs.includes(TRASH)) setTrashVersion((v) => v + 1)
      if (pane && c.gone.some((g) => within(pane, g))) setPane(null)
      if (!path || !affects(c, path)) return
      if (c.origin !== self && c.refreshed === path) follow = window.setTimeout(() => reopen(false), FOLLOW_MS)
      else reopen(true)
    })
    return () => {
      off()
      window.clearTimeout(follow)
    }
  }, [path, pane, reopen, self])

  // 만들거나 이름을 바꾼 항목을 다시 받은 목록에서 찾아 고른다.
  useEffect(() => {
    if (!pendingSelect || !listing || listing.id === pendingSelect.listingId) return
    const ctl = new AbortController()
    findInListing(listing.id, pendingSelect.name, view, ctl.signal).then(
      (i) => {
        setPendingSelect(null)
        if (i < 0) return
        setSel(selectOne(i))
        listRef.current?.reveal(i)
      },
      (err: unknown) => {
        // 다른 창이 같은 폴더를 새로 읽어 목록이 바뀌었으면 새 목록에서 다시 찾는다.
        if (ctl.signal.aborted || (err instanceof ApiError && err.code === 'listing_expired')) return
        setPendingSelect(null)
      },
    )
    return () => ctl.abort()
  }, [pendingSelect, listing, view])

  // 사이드바 휴지통 항목의 개수와 볼륨. 휴지통이 바뀌면 다시 센다(항목 정보는 읽지 않는 가벼운 요청).
  useEffect(() => {
    if (!rootsReady) return
    const ctl = new AbortController()
    fetchTrashVolumes(ctl.signal).then(
      (r) => {
        setTrashCount(r.items)
        setVolumes(r.volumes)
      },
      () => {},
    )
    return () => ctl.abort()
  }, [rootsReady, trashVersion])

  // moveTo는 다른 폴더로 간다. 고른 항목·정보 패널·찾기·정렬 준비를 지우고, 날짜·크기순은 새 폴더의 정보를 다시
  // 읽어야 하므로 이름순으로 돌아간다.
  const moveTo = (next: (h: History | null) => History | null) => {
    setRefocus(document.activeElement instanceof HTMLElement && document.activeElement.classList.contains('files-scroll'))
    setSel(NO_SELECTION)
    setPane(null)
    setPendingSelect(null)
    setQueryInput('')
    setView((v) => (v.sort === 'name' ? { ...v, query: '' } : DEFAULT_VIEW))
    setViewTotal(null)
    prepCtl.current?.abort()
    setPrep(null)
    setListError(null)
    setTrashOpen(false)
    setSearch(null)
    setHistory(next)
  }
  const navigate = (to: string) => {
    if (!findRoot(roots, to)) return
    if (to === path) {
      if (trashOpen) closeTrash()
      if (search) closeSearch()
      return
    }
    moveTo((h) => (h ? visit(h, to) : startHistory(to)))
  }
  const closeSearch = () => {
    setSearch(null)
    setQueryInput('')
  }
  // startSearch는 지금 폴더와 그 아래에서 찾는다. 같은 찾기어로 다시 부르면 다시 찾는다.
  const startSearch = (query: string) => {
    if (!path || !query.trim()) return
    setSel(NO_SELECTION)
    setPane(null)
    setSearch((s) => ({ path: s?.path ?? path, query, seq: (s?.seq ?? 0) + 1 }))
  }
  const retrySearch = () => setSearch((s) => (s ? { ...s, seq: s.seq + 1 } : s))
  const searchSecurity = async (level?: RequiredLevel) => {
    if (await requestStepUp(dialogs.scope, { level, target: 'files' })) retrySearch()
    else closeSearch()
  }
  // locate는 찾은 항목이 든 폴더를 열고 그 항목을 고른다.
  const locate = (p: string) => {
    const dir = dirName(p)
    const pick = { name: p.slice(dir.length + 1), listingId: listing?.id ?? '' }
    if (dir === path) {
      closeSearch()
      setPendingSelect(pick)
      reopen(true)
      return
    }
    // moveTo가 고르기 대상을 지우므로 그 뒤에 둔다.
    moveTo((h) => (h ? visit(h, dir) : startHistory(dir)))
    setPendingSelect(pick)
  }

  const openTrash = () => {
    if (trashOpen) return
    setTrashOpen(true)
    setSearch(null)
    setSel(NO_SELECTION)
    setPane(null)
    setQueryInput('')
  }
  const closeTrash = () => {
    setTrashOpen(false)
    setQueryInput('')
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
      setSel(NO_SELECTION)
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
      setSel(NO_SELECTION)
      return
    }
    if (key === 'name') {
      cancelPrepare()
      setView((v) => ({ ...v, sort: 'name', desc: false }))
      setSel(NO_SELECTION)
      return
    }
    startPrepare(key, true)
  }

  const chooseLayout = (next: FilesLayout) => {
    setLayoutChoice(next)
    useFilesPrefsStore.getState().set(next)
  }

  // openSortMenu는 아이콘 보기의 정렬 메뉴다(목록 보기는 열 머리로 정렬한다).
  const openSortMenu = (viaKeyboard: boolean) => {
    const el = sortButton.current
    if (!el) return
    const r = el.getBoundingClientRect()
    const keys: readonly [SortKey, ParseKeys][] = [
      ['name', 'files.columns.name'],
      ['mtime', 'files.columns.modified'],
      ['size', 'files.columns.size'],
    ]
    const setDesc = (d: boolean) => {
      if (d !== view.desc) onSort(view.sort)
    }
    const sortKey = prep?.sort ?? view.sort
    const desc = prep?.desc ?? view.desc
    const items: ContextMenuItem[] = [
      ...keys.map(
        ([key, labelKey]): ContextMenuAction => ({
          kind: 'action',
          id: key,
          labelKey,
          checked: key === sortKey,
          onSelect: () => {
            if (key !== sortKey) onSort(key)
          },
        }),
      ),
      { kind: 'separator', id: 'sep' },
      { kind: 'action', id: 'asc', labelKey: 'files.view.ascending', checked: !desc, disabled: prep !== null, onSelect: () => setDesc(false) },
      { kind: 'action', id: 'desc', labelKey: 'files.view.descending', checked: desc, disabled: prep !== null, onSelect: () => setDesc(true) },
    ]
    useContextMenuStore.getState().openAt({ x: r.left, y: r.bottom + 4 }, items, viaKeyboard)
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
    if (isFolderLike(item)) {
      navigate(joinPath(path, item.name))
      return
    }
    // 미리볼 수 있는 파일은 미리보기 창에 연다. 그 밖의 파일은 정보 패널이다.
    const kind = previewKind(item)
    if (kind) preview(joinPath(path, item.name), item.name, kind)
    else if (item.state !== 'gone') setPane(joinPath(path, item.name))
  }

  const preview = (p: string, name: string, kind: PreviewKind) => openViewer({ path: p, name, kind }, desktopBounds())

  const onSelection = (s: Selection, item: FileItem | null) => {
    setSel(s)
    if (!path) return
    // 여러 개를 고르거나 고르기를 지우면 정보 패널을 닫는다.
    if (selectedCount(s) !== 1) {
      setPane(null)
      return
    }
    if (!item || soleIndex(s) !== s.focus) return
    // 파일을 누르면 정보 패널을 연다. 폴더는 고르기만 한다(시안 AC-Main).
    if (isFolderLike(item) || item.type === 'dir') setPane(null)
    else if (item.state !== 'gone' && !item.badName) setPane(joinPath(path, item.name))
  }

  const writable = listing?.writable ?? false
  // canWrite는 지금 폴더 안을 바꿀 수 있는지다(쓰기 권한, 다른 작업이 진행 중이 아님).
  const canWrite = writable && op === null

  // onListFailure는 목록에서 고른 항목을 받지 못했을 때다.
  const onListFailure = (err: unknown) => {
    if (err instanceof ApiError && err.status === 401) return
    if (err instanceof ApiError && err.code === 'listing_expired') reopen(false)
    else setListError(err)
  }

  // itemsOf는 고른 항목이다. 이름이 UTF-8이 아닌 항목은 경로로 가리킬 수 없으므로 뺀다. 받지 못하면 null이다.
  const itemsOf = async (s: Selection): Promise<FileItem[] | null> => {
    try {
      return ((await listRef.current?.items(s)) ?? []).filter((i) => !i.badName)
    } catch (err) {
      onListFailure(err)
      return null
    }
  }

  // runOp은 여러 항목 작업을 하나씩 하며 진행 배너와 중지를 보인다. 쓰기에 보안 확인이 필요하면 확인한 뒤 다시 하고,
  // 끝나면 바뀐 폴더를 알리고 하지 못한 항목을 보인다. 세션이 끝났거나(401) 확인을 취소했거나 stopOn이면 거기서 멈춘다.
  const runOp: RunOp = async (kind, items, run, opts) => {
    const ctl = new AbortController()
    opCtl.current = ctl
    setOp({ kind, done: 0, total: items.length, current: null, shown: false })
    const timer = window.setTimeout(() => setOp((o) => (o ? { ...o, shown: true } : o)), OP_BANNER_MS)
    const done: (typeof items)[number][] = []
    const res = await runEach(
      items,
      async (it) => {
        const current = kind === 'empty' ? null : opts.nameOf(it)
        setOp((o) => (o ? { ...o, current } : o))
        try {
          await withStepUp(dialogs.scope, () => run(it))
          done.push(it)
        } catch (err) {
          if ((err instanceof ApiError && (err.status === 401 || err.code === 'security_required')) || opts.stopOn?.(err)) ctl.abort()
          throw err
        }
      },
      (p) => setOp((o) => (o ? { ...o, done: p.done } : o)),
      ctl.signal,
    )
    window.clearTimeout(timer)
    if (opCtl.current === ctl) opCtl.current = null
    setOp(null)
    if (done.length > 0) {
      const c = opts.change(done)
      changed(c.dirs, c.gone)
    }
    const kept = res.failures.filter((f) => opts.keep?.(f.err))
    const shown = res.failures.filter(
      (f) => !opts.keep?.(f.err) && !(f.err instanceof ApiError && (f.err.status === 401 || f.err.code === 'security_required')),
    )
    if (shown.length > 0) {
      await dialogs.alert({
        title: t(OP_FAILED_KEYS[kind]),
        message: <FailuresBody groups={groupFailures(shown, opts.nameOf)} />,
      })
    }
    return kept.map((f) => f.item)
  }

  const newFolder = async () => {
    if (!listing || !path || !canWrite) return
    const dir = path
    const base = t('files.name.newFolder')
    let initial = base
    // 같은 이름이 없는 번호를 목록에서 찾는다(찾지 못하면 기본 이름으로 두고 서버가 알린다).
    for (let n = 1; n <= NEW_FOLDER_TRIES; n++) {
      const candidate = numberedName(base, n)
      try {
        if ((await findInListing(listing.id, candidate, DEFAULT_VIEW)) < 0) {
          initial = candidate
          break
        }
      } catch {
        break
      }
    }
    const made = await askName(dialogs.scope, { mode: 'mkdir', initial, folder: true, submit: (name) => withStepUp(dialogs.scope, () => makeFolder(joinPath(dir, name))) })
    if (!made) return
    setPendingSelect({ name: made.name, listingId: listing.id })
    changed([dir])
  }

  const renameSelection = async (s: Selection = sel) => {
    if (!listing || !path || !canWrite || selectedCount(s) !== 1) return
    const [item] = (await itemsOf(s)) ?? []
    if (item) await renameItem(item)
  }

  const renameItem = async (item: FileItem) => {
    if (!listing || !path || !canWrite || item.state === 'gone' || item.badName) return
    const dir = path
    const from = joinPath(dir, item.name)
    const renamed = await askName(dialogs.scope, {
      mode: 'rename',
      initial: item.name,
      folder: isFolderLike(item) || item.type === 'dir',
      submit: (name) =>
        name === item.name ? Promise.resolve(item) : withStepUp(dialogs.scope, () => renameFile(from, joinPath(dir, name))),
    })
    if (!renamed || renamed.name === item.name) return
    const showing = pane === from
    setPendingSelect({ name: renamed.name, listingId: listing.id })
    changed([dir], [from])
    if (showing) setPane(joinPath(dir, renamed.name))
  }

  const deleteSelection = async (s: Selection = sel, permanent = false) => {
    if (!listing || !path || !canWrite || selectedCount(s) === 0) return
    const items = await itemsOf(s)
    if (items) await deleteItems(items, permanent)
  }

  // trashKind는 dir이 든 디스크(또는 홈)의 휴지통 종류다. 알 수 없으면 null이고 휴지통에 넣어 본다.
  const trashKind = async (dir: string): Promise<TrashVolume['kind'] | null> => {
    try {
      return volumeOf((await fetchTrashVolumes()).volumes, dir)?.kind ?? null
    } catch {
      return null
    }
  }

  // deleteItems는 항목을 휴지통으로 옮긴다(permanent면 영구 삭제). 확인 대화상자에서 영구 삭제를 고를 수 있고, 지금 폴더의
  // 디스크에 휴지통이 없으면 영구 삭제를 묻는다(시안 S-Delete).
  const deleteItems = async (all: readonly FileItem[], permanent = false) => {
    if (!listing || !path || !canWrite) return
    const dir = path
    const items = all.filter((i) => i.state !== 'gone' && !i.badName)
    if (items.length === 0) return
    if (permanent) return deleteForever(dir, items, 'permanent')
    if ((await trashKind(dir)) === 'none') return deleteForever(dir, items, 'noTrash')
    const choice = await dialogs.message({
      title: <DeleteTitle items={items} mode="trash" />,
      message: <DeleteBody items={items} mode="trash" />,
      buttons: [
        { id: 'permanent', labelKey: 'files.delete.permanentButton' },
        { id: 'cancel', labelKey: 'dialog.cancel' },
        { id: 'trash', labelKey: 'files.delete.trashButton', tone: 'primary' },
      ],
      focusId: 'trash',
    })
    if (choice === 'permanent') return deleteForever(dir, items, 'permanent')
    if (choice === 'trash') await trashItems(dir, items)
  }

  // trashItems는 확인 없이 휴지통으로 옮긴다(삭제 대화상자, 휴지통으로 끌어 놓기). 넣지 못한 항목은 영구 삭제를 묻는다.
  const trashItems = async (dir: string, items: readonly FileItem[]) => {
    const left = await runOp('trash', items, (it) => trashFile(joinPath(dir, it.name)), {
      nameOf: (i) => i.name,
      change: (done) => ({ dirs: [dir, TRASH], gone: done.map((i) => joinPath(dir, i.name)) }),
      keep: trashLeft,
    })
    if (left.length > 0) await deleteForever(dir, left, 'noTrash')
  }

  // deleteForever는 확인한 뒤 영구 삭제한다. 폴더(심볼릭 링크 아님)만 안까지 지우고, 종류를 모르는 항목은 비어 있지 않으면
  // 지우지 않는다(not_empty).
  const deleteForever = async (dir: string, items: readonly FileItem[], mode: 'permanent' | 'noTrash') => {
    const ok = await dialogs.confirm({
      title: <DeleteTitle items={items} mode={mode} />,
      message: <DeleteBody items={items} mode={mode} />,
      confirmLabel: t('files.delete.confirm'),
      danger: true,
    })
    if (!ok) return
    await runOp('delete', items, (it) => deleteFile(joinPath(dir, it.name), it.type === 'dir'), {
      nameOf: (i) => i.name,
      change: (done) => ({ dirs: [dir], gone: done.map((i) => joinPath(dir, i.name)) }),
    })
  }

  // dropModeFor는 고른 항목을 target에 놓을 때 할 일이다(끄는 이름표, null이면 놓을 수 없다).
  const dropModeFor = (target: string) => (path ? dropMode(volumes, path, target, TRASH, writable) : null)

  // desktopBounds는 이 창이 놓인 데스크톱 영역의 크기다(파일 작업 창을 열 자리).
  const desktopBounds = () => {
    const area = contentRef.current?.closest('.desktop-area')
    return area ? { width: area.clientWidth, height: area.clientHeight } : { width: window.innerWidth, height: window.innerHeight }
  }

  // copyItems는 항목을 target 폴더에 복사한다. 복사는 파일 작업 목록(transferStore)이 이어서 하므로 이 창은 기다리지 않는다.
  const copyItems = (dir: string, items: readonly FileItem[], target: string) => {
    startCopy({ dir, names: items.map((i) => i.name), target, roots, scope: dialogs.scope, bounds: desktopBounds() })
  }

  // uploadFiles는 파일을 dir에 올린다. 같은 이름을 먼저 확인하고, 올리기는 파일 작업 목록이 이어서 한다(이 창을 닫아도
  // 이어진다).
  const uploadFiles = async (dir: string, picked: readonly File[]) => {
    const uploads = await checkUploads(dialogs, t, dir, picked)
    startUpload({ target: dir, uploads, roots, scope: dialogs.scope, bounds: desktopBounds() })
  }

  // moveSelection은 고른 항목을 target 폴더로 옮긴다(끌어 놓기). 다른 디스크면 옮기지 않고 복사한다. 같은 이름이 있으면 그
  // 항목은 옮기거나 복사하지 않는다. 휴지통에 놓으면 휴지통으로 옮긴다.
  const moveSelection = async (s: Selection, target: string) => {
    if (!listing || !path || op !== null) return
    const dir = path
    const items = (await itemsOf(s))?.filter((i) => i.state !== 'gone')
    if (!items || items.length === 0) return
    if (target === TRASH) {
      if (!writable) return
      if ((await trashKind(dir)) === 'none') await deleteForever(dir, items, 'noTrash')
      else await trashItems(dir, items)
      return
    }
    if (!moveAllowed(items.map((i) => joinPath(dir, i.name)), target)) return
    let vols = volumes
    if (!vols) {
      try {
        vols = (await fetchTrashVolumes()).volumes
        setVolumes(vols)
      } catch {
        // 모르면 옮겨 보고, 다른 디스크면 그때 복사로 바꾼다.
      }
    }
    const mode = dropMode(vols, dir, target, TRASH, writable)
    if (mode === 'copy') return copyItems(dir, items, target)
    if (mode !== 'move') return
    const left = await runOp('move', items, (it) => renameFile(joinPath(dir, it.name), joinPath(target, it.name)), {
      nameOf: (i) => i.name,
      change: (done) => ({ dirs: [dir, target], gone: done.map((i) => joinPath(dir, i.name)) }),
      // 다른 디스크(같은 디스크 안의 다른 마운트 포함)면 남은 항목도 옮길 수 없으므로 거기서부터 복사한다.
      stopOn: (err) => err instanceof ApiError && err.code === 'cross_device',
      keep: (err) => err instanceof ApiError && err.code === 'cross_device',
    })
    if (left.length > 0) copyItems(dir, items.slice(items.indexOf(left[0])), target)
  }

  // 바깥(운영체제)에서 끌어 온 파일을 놓으면 그 폴더 줄이나 지금 폴더에 올린다. 폴더는 아직 올리지 못한다.
  // 목록 자신(빈 곳, 파일 줄)은 지금 폴더이므로 폴더 줄처럼 표시하지 않고 창 전체에 표시한다.
  const dropTargetOf = (e: DragEvent<HTMLElement>): { el: HTMLElement | null; path: string | null } => {
    const el = e.target instanceof Element ? e.target.closest<HTMLElement>('[data-drop-path]') : null
    if (el?.dataset.dropPath && !el.classList.contains('files-list')) return { el, path: el.dataset.dropPath }
    return { el: null, path: listing && writable ? listing.path : null }
  }
  const markDrop = (el: HTMLElement | null, list: boolean) => {
    if (dropRow.current !== el) {
      dropRow.current?.classList.remove(DROP_TARGET)
      el?.classList.add(DROP_TARGET)
      dropRow.current = el
    }
    if (list !== dropList) setDropList(list)
  }
  const onDragOver = (e: DragEvent<HTMLDivElement>) => {
    if (!hasFiles(e.dataTransfer)) return
    e.preventDefault()
    const target = dropTargetOf(e)
    e.dataTransfer.dropEffect = target.path ? 'copy' : 'none'
    markDrop(target.el, target.path !== null && target.el === null)
  }
  const onDragLeave = (e: DragEvent<HTMLDivElement>) => {
    if (e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget)) return
    markDrop(null, false)
  }
  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    if (!hasFiles(e.dataTransfer)) return
    e.preventDefault()
    const target = dropTargetOf(e)
    markDrop(null, false)
    if (!target.path) return
    const files: File[] = []
    const folders: string[] = []
    for (const it of Array.from(e.dataTransfer.items)) {
      if (it.kind !== 'file') continue
      const entry = it.webkitGetAsEntry?.()
      const f = it.getAsFile()
      if (entry?.isDirectory) folders.push(entry.name)
      else if (f) files.push(f)
    }
    if (files.length > 0) void uploadFiles(target.path, files)
    if (folders.length > 0) {
      void dialogs.alert({ title: t('files.drop.foldersTitle'), message: t('files.drop.folders', { names: folders.join(', ') }) })
    }
  }

  const downloadPath = (p: string, name: string) => {
    startDownload(p, name).catch((err: unknown) => {
      if (err instanceof ApiError && err.status === 401) return
      dialogs.alert({ title: name, message: t(errorKey(err)) })
    })
  }
  const download = (item: FileItem) => {
    if (path) downloadPath(joinPath(path, item.name), item.name)
  }

  const menuItems = (item: FileItem | null, s: Selection): ContextMenuItem[] => {
    const readOnly = listing !== null && !writable ? 'files.menu.readOnly' : undefined
    if (!item) {
      const total = viewTotal ?? listing?.total ?? 0
      return [
        {
          kind: 'action',
          id: 'mkdir',
          labelKey: 'files.menu.newFolder',
          icon: 'folder-new-symbolic',
          disabled: !canWrite,
          detailKey: readOnly,
          onSelect: () => void newFolder(),
        },
        {
          kind: 'action',
          id: 'upload',
          labelKey: 'files.menu.upload',
          icon: 'upload-symbolic',
          disabled: !listing || !writable,
          detailKey: readOnly,
          onSelect: () => fileInput.current?.click(),
        },
        { kind: 'separator', id: 'sep' },
        { kind: 'action', id: 'selectAll', labelKey: 'files.menu.selectAll', disabled: total === 0, onSelect: () => setSel(selectAll(sel, total)) },
        { kind: 'action', id: 'refresh', labelKey: 'files.toolbar.refresh', icon: 'view-refresh-symbolic', onSelect: () => reopen(true) },
      ]
    }
    const count = selectedCount(s)
    const usable = item.state !== 'gone' && !item.badName
    const out: ContextMenuItem[] = []
    if (count === 1) {
      if (isFolderLike(item) || item.type === 'dir') {
        out.push({ kind: 'action', id: 'open', labelKey: 'files.menu.open', disabled: item.locked, onSelect: () => openItem(item) })
      } else {
        const kind = previewKind(item)
        if (kind && path) {
          out.push({
            kind: 'action',
            id: 'preview',
            labelKey: 'files.menu.preview',
            icon: 'view-reveal-symbolic',
            onSelect: () => preview(joinPath(path, item.name), item.name, kind),
          })
        }
        out.push({
          kind: 'action',
          id: 'download',
          labelKey: 'files.menu.download',
          icon: 'download-symbolic',
          disabled: !usable || item.type !== 'file',
          onSelect: () => download(item),
        })
      }
    }
    out.push(
      {
        kind: 'action',
        id: 'rename',
        labelKey: 'files.menu.rename',
        icon: 'document-edit-symbolic',
        disabled: !canWrite || count !== 1 || !usable,
        detailKey: readOnly ?? 'files.menu.renameKey',
        onSelect: () => void renameSelection(s),
      },
      {
        kind: 'action',
        id: 'delete',
        labelKey: count > 1 ? 'files.menu.deleteMany' : 'files.menu.delete',
        labelParams: { count, total: fmt(count) },
        icon: 'edit-delete-symbolic',
        disabled: !canWrite || (count === 1 && !usable),
        detailKey: readOnly ?? 'files.menu.deleteKey',
        onSelect: () => void deleteSelection(s),
      },
    )
    if (count === 1 && !(isFolderLike(item) || item.type === 'dir')) {
      out.push(
        { kind: 'separator', id: 'sep' },
        { kind: 'action', id: 'info', labelKey: 'files.menu.info', disabled: !usable, onSelect: () => path && setPane(joinPath(path, item.name)) },
      )
    }
    return out
  }

  const onPick = (e: ChangeEvent<HTMLInputElement>) => {
    const picked = Array.from(e.currentTarget.files ?? [])
    e.currentTarget.value = ''
    if (path && picked.length > 0) void uploadFiles(path, picked)
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
      data-drop-path={r.reachable ? r.path : undefined}
      aria-current={!trashOpen && root === r ? 'page' : undefined}
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
          data-drop-path={r.path}
          aria-current={!trashOpen && root === r ? 'page' : undefined}
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
      {rootsState.status === 'ready' && roots.length > 0 && (
        <button
          type="button"
          className="sidebar-nav-item files-nav-trash"
          data-drop-path={TRASH}
          aria-current={trashOpen ? 'page' : undefined}
          onClick={openTrash}
        >
          <IconImage name="user-trash-symbolic" size={16} />
          <span>{t('files.nav.trash')}</span>
          {trashCount ? (
            <span className="files-nav-count" aria-label={t('files.nav.trashCount', { count: trashCount, total: fmt(trashCount) })}>
              {fmt(trashCount)}
            </span>
          ) : null}
        </button>
      )}
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
  const selCount = selectedCount(sel)
  const header = (
    <div className="files-toolbar">
      <div className="files-toolbar-buttons">
        <button
          type="button"
          className="files-icon-button"
          aria-label={t('files.toolbar.back')}
          title={t('files.toolbar.back')}
          disabled={!trashOpen && !search && (!history || !canBack(history))}
          onClick={() => (trashOpen ? closeTrash() : search ? closeSearch() : moveTo((h) => (h ? back(h) : h)))}
        >
          <IconImage name="go-previous-symbolic" size={16} />
        </button>
        <button
          type="button"
          className="files-icon-button"
          aria-label={t('files.toolbar.forward')}
          title={t('files.toolbar.forward')}
          disabled={trashOpen || !history || !canForward(history)}
          onClick={() => moveTo((h) => (h ? forward(h) : h))}
        >
          <IconImage name="go-next-symbolic" size={16} />
        </button>
        <button
          type="button"
          className="files-icon-button"
          aria-label={t('files.toolbar.up')}
          title={t('files.toolbar.up')}
          disabled={trashOpen || !root || !path || parentOf(root, path) === null}
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
          onClick={() => (trashOpen ? setTrashReload((n) => n + 1) : search ? retrySearch() : reopen(true))}
        >
          <IconImage name="view-refresh-symbolic" size={16} />
        </button>
      </div>
      <nav className="files-path" aria-label={t('files.toolbar.path')}>
        {trashOpen && (
          <span className="files-crumb">
            <button type="button" aria-current="page" onClick={() => setTrashReload((n) => n + 1)}>
              <IconImage name="user-trash-symbolic" size={16} />
              <span>{t('files.nav.trash')}</span>
            </button>
          </span>
        )}
        {!trashOpen &&
          root &&
          path &&
          crumbs(root, path).map((c, i, all) => (
            <span key={c.path} className="files-crumb">
              {i > 0 && <span className="files-crumb-sep" aria-hidden="true" />}
              <button
                type="button"
                aria-current={i === all.length - 1 ? 'page' : undefined}
                data-drop-path={c.path}
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
            trashOpen
              ? t('files.trash.search')
              : deep
                ? t('files.search.placeholder')
                : listing && listing.total > MANY_ITEMS
                ? t('files.toolbar.searchMany', { total: fmt(listing.total) })
                : t('files.toolbar.search')
          }
          aria-label={t('files.toolbar.search')}
          maxLength={255}
          onChange={(e) => setQueryInput(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape' && (queryInput || search)) {
              e.preventDefault()
              if (search) closeSearch()
              else setQueryInput('')
            } else if (e.key === 'Enter' && deep && !trashOpen) {
              e.preventDefault()
              startSearch(queryInput)
            }
          }}
        />
      </label>
      {!trashOpen && (
        <button
          type="button"
          className="files-icon-button files-search-scope"
          aria-label={t('files.search.deep')}
          title={t('files.search.deep')}
          aria-pressed={deep}
          disabled={!path}
          onClick={() => {
            if (deep) setSearch(null)
            setDeep(!deep)
          }}
        >
          <IconImage name="folder-saved-search-symbolic" size={16} />
        </button>
      )}
    </div>
  )

  const sortName: Record<SortKey, string> = {
    name: t('files.status.sortName'),
    mtime: t('files.status.sortMtime'),
    size: t('files.status.sortSize'),
  }

  const opBanner = op?.shown && (
    <TaskBanner
      label={t(OP_KEYS[op.kind])}
      count={t('files.banner.progress', { done: fmt(op.done), total: fmt(op.total) })}
      fraction={op.done / Math.max(1, op.total)}
      current={op.current}
      action={{ label: t('files.ops.stop'), title: t('files.ops.stopHint'), onClick: () => opCtl.current?.abort() }}
    />
  )

  if (trashOpen) {
    return (
      <SidebarLayout className="files" narrowWidth={FILES_NARROW_WIDTH} navLabel={t('files.nav.label')} nav={nav} header={header}>
        {opBanner}
        <TrashView
          query={view.query}
          roots={roots}
          dialogs={dialogs}
          canWrite={op === null}
          runOp={runOp}
          changed={changed}
          reload={trashReload}
        />
      </SidebarLayout>
    )
  }

  if (search) {
    return (
      <SidebarLayout className="files" narrowWidth={FILES_NARROW_WIDTH} navLabel={t('files.nav.label')} nav={nav} header={header}>
        <SearchView
          search={search}
          roots={roots}
          admin={admin}
          onFolder={navigate}
          onLocate={locate}
          onPreview={preview}
          onDownload={downloadPath}
          onSecurity={(level) => void searchSecurity(level)}
          onRetry={retrySearch}
        />
      </SidebarLayout>
    )
  }

  return (
    <SidebarLayout className="files" narrowWidth={FILES_NARROW_WIDTH} navLabel={t('files.nav.label')} nav={nav} header={header}>
      <div className="files-actions">
        <button
          type="button"
          className="addon-button primary files-action-button"
          disabled={!listing || !writable}
          title={listing && !writable ? t('files.toolbar.uploadDenied') : undefined}
          onClick={() => fileInput.current?.click()}
        >
          <IconImage name="upload-symbolic" size={16} />
          {t('files.toolbar.upload')}
        </button>
        <button
          type="button"
          className="addon-button files-action-button"
          disabled={!listing || !canWrite}
          title={listing && !writable ? t('files.toolbar.uploadDenied') : undefined}
          onClick={() => void newFolder()}
        >
          <IconImage name="folder-new-symbolic" size={16} />
          {t('files.toolbar.newFolder')}
        </button>
        <input ref={fileInput} type="file" multiple hidden onChange={onPick} />
        <span className="files-actions-fill" />
        {layout === 'grid' && (
          <button
            ref={sortButton}
            type="button"
            className="files-icon-button"
            aria-label={t('files.view.sort')}
            title={t('files.view.sort')}
            aria-haspopup="menu"
            disabled={!listing}
            onClick={(e) => openSortMenu(e.detail === 0)}
          >
            <IconImage name="view-sort-ascending-symbolic" size={16} />
          </button>
        )}
        <div className="files-view-toggle" role="group" aria-label={t('files.view.label')}>
          <button
            type="button"
            className="files-icon-button"
            aria-label={t('files.view.list')}
            title={t('files.view.list')}
            aria-pressed={layout === 'list'}
            onClick={() => chooseLayout('list')}
          >
            <IconImage name="view-list-symbolic" size={16} />
          </button>
          <button
            type="button"
            className="files-icon-button"
            aria-label={t('files.view.grid')}
            title={t('files.view.grid')}
            aria-pressed={layout === 'grid'}
            onClick={() => chooseLayout('grid')}
          >
            <IconImage name="view-grid-symbolic" size={16} />
          </button>
        </div>
      </div>
      {listing?.truncated && (
        <p className="addon-banner notice files-banner" role="status">
          <IconImage name="dialog-warning-symbolic" size={16} />
          {t('files.banner.truncated')}
        </p>
      )}
      {prep?.shown && (
        <TaskBanner
          label={prep.sort === 'mtime' ? t('files.banner.prepareMtime') : t('files.banner.prepareSize')}
          count={t('files.banner.progress', { done: fmt(prep.filled), total: fmt(prep.total) })}
          fraction={prep.filled / Math.max(1, prep.total)}
          action={{ label: t('files.banner.cancel'), onClick: cancelPrepare }}
        />
      )}
      {opBanner}
      {listError !== null && (
        <p className="addon-banner files-banner" role="alert">
          {t(errorKey(listError))}
        </p>
      )}
      <div
        ref={contentRef}
        className={['files-content', pane && 'with-pane', dropList && 'drop-here'].filter(Boolean).join(' ')}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
      >
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
            ref={listRef}
            listing={listing}
            view={view}
            mode={layout}
            active={active}
            selection={sel}
            onSelection={onSelection}
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
            onRename={() => void renameSelection()}
            onDelete={(permanent) => void deleteSelection(sel, permanent)}
            canDrag={op === null}
            dropMode={dropModeFor}
            onMove={(s, target) => void moveSelection(s, target)}
            focusOnMount={refocus}
            onFocused={() => setRefocus(false)}
          />
        ) : (
          <div className="files-list" aria-busy="true" />
        )}
        {pane && (
          <InfoPane
            path={pane}
            roots={roots}
            admin={admin}
            canWrite={canWrite && dirName(pane) === path}
            onRename={(it) => void renameItem(it)}
            onDelete={(it) => void deleteItems([it])}
            onPreview={(kind) => preview(pane, pane.slice(pane.lastIndexOf('/') + 1), kind)}
            onClose={() => setPane(null)}
          />
        )}
      </div>
      <footer className="files-status">
        {listing && (
          <span>
            {listing.truncated
              ? t('files.status.shown', { total: fmt(listing.total) })
              : t('files.status.items', { count: total, total: fmt(total) })}
          </span>
        )}
        {selCount > 0 && <span>{t('files.status.selected', { count: selCount, total: fmt(selCount) })}</span>}
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
