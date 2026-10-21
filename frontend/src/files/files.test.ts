import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import {
  MAX_NAMES,
  MAX_NAMES_JSON,
  NO_AMOUNT,
  addAmount,
  copyChunks,
  dropMode,
  durationOf,
  goJSONLength,
  newJob,
  nextRate,
  progressOf,
  remainingSeconds as copyRemaining,
} from './copyJobs'
import { back, canBack, canForward, current, forward, MAX_HISTORY, replaceCurrent, startHistory, visit } from './history'
import { extension, isFolderLike, kindOf, modeString } from './kinds'
import { PageCache, pagesFor } from './pageCache'
import { affects, groupFailures, notifyFilesChanged, onFilesChanged, opErrorKey, runEach } from './ops'
import {
  baseName,
  crumbs,
  dirName,
  findRoot,
  joinPath,
  locationLabel,
  moveAllowed,
  nameProblem,
  numberedName,
  parentOf,
  rootsOf,
  stemLength,
  uploadNameOk,
  volumeOf,
} from './paths'
import {
  NO_SELECTION,
  extendTo,
  isSelected,
  selectAll,
  selectOne,
  selectedCount,
  selectedIndices,
  soleIndex,
  toggleAt,
} from './selection'
import {
  completed,
  fileFailed,
  fileFinished,
  fileProgressed,
  fileStarted,
  newUploadJob,
  nextFile,
  retried,
  retryable,
  stopped,
  unfinished,
  uploadAmount,
  uploadProgress,
  uploadRemaining,
} from './uploadJobs'
import {
  GRID_PAD,
  TILE_HEIGHT,
  TILE_MIN_WIDTH,
  layout,
  MAX_SCROLL_HEIGHT,
  ROW_HEIGHT,
  gridOf,
  indexAt,
  itemRange,
  moveKey,
  rowsOf,
  scrollHeightOf,
  scrollToShow,
  scrollTopFor,
  virtualOffset,
} from './virtual'
import { createFilesPrefsStore, toLayout } from './viewPrefs'

const roots = rootsOf({
  home: { name: 'alice', path: '/home/alice' },
  shares: [
    { uuid: 'u1', name: '미디어', path: '/srv/d1/media', reachable: true },
    { uuid: 'u2', name: 'kvm', path: '/kvm', reachable: false },
    { uuid: 'u3', name: '영화', path: '/srv/d1/media/movies', reachable: true },
  ],
  disks: [{ name: 'data1', path: '/srv/d1' }],
  fallback: false,
})

describe('경로', () => {
  it('가장 긴 뿌리를 고르고 닿지 않는 뿌리는 건너뛴다', () => {
    expect(findRoot(roots, '/srv/d1/media/a')?.name).toBe('미디어')
    expect(findRoot(roots, '/srv/d1/media/movies/2024')?.name).toBe('영화')
    expect(findRoot(roots, '/srv/d1/other')?.name).toBe('data1')
    expect(findRoot(roots, '/kvm/x')).toBeNull()
    expect(findRoot(roots, '/srv/d10')).toBeNull()
  })

  it('경로 막대는 뿌리 이름부터 그린다', () => {
    const media = roots[1]
    expect(crumbs(media, '/srv/d1/media')).toEqual([{ name: '미디어', path: '/srv/d1/media' }])
    expect(crumbs(media, '/srv/d1/media/a/b')).toEqual([
      { name: '미디어', path: '/srv/d1/media' },
      { name: 'a', path: '/srv/d1/media/a' },
      { name: 'b', path: '/srv/d1/media/a/b' },
    ])
  })

  it('뿌리 밖으로는 올라가지 않는다', () => {
    const media = roots[1]
    expect(parentOf(media, '/srv/d1/media')).toBeNull()
    expect(parentOf(media, '/srv/d1/media/a/b')).toBe('/srv/d1/media/a')
  })

  it('이름을 잇고 나눈다', () => {
    expect(joinPath('/srv/d1', 'a b')).toBe('/srv/d1/a b')
    expect(joinPath('/', 'srv')).toBe('/srv')
    expect(baseName('/srv/d1/a.txt')).toBe('a.txt')
    expect(dirName('/srv/d1/a.txt')).toBe('/srv/d1')
    expect(dirName('/srv')).toBe('/')
    expect(locationLabel(roots, '/srv/d1/media/movies/x.mkv', '홈 폴더')).toBe('영화')
    expect(locationLabel(roots, '/srv/d1/media/a/x.mkv', '홈 폴더')).toBe('미디어 / a')
    expect(locationLabel(roots, '/home/alice/x.txt', '홈 폴더')).toBe('홈 폴더')
    expect(locationLabel(roots, '/home/alice/docs/x.txt', '홈 폴더')).toBe('홈 폴더 / docs')
  })

  it('올릴 수 없는 이름을 거른다', () => {
    expect(uploadNameOk('사진.jpg')).toBe(true)
    expect(uploadNameOk('.hidden')).toBe(true)
    for (const bad of ['', '.', '..', 'a/b', 'a\0b', '.onmaru-upload-x.part', 'ㄱ'.repeat(86)]) {
      expect(uploadNameOk(bad)).toBe(false)
    }
  })
})

describe('기록', () => {
  it('뒤로·앞으로 가고 새 폴더로 가면 앞 기록을 버린다', () => {
    let h = startHistory('/a')
    h = visit(h, '/a/b')
    h = visit(h, '/a/b/c')
    h = back(back(h))
    expect(current(h)).toBe('/a')
    expect(canBack(h)).toBe(false)
    h = forward(h)
    expect(current(h)).toBe('/a/b')
    h = visit(h, '/x')
    expect(canForward(h)).toBe(false)
    expect(h.entries).toEqual(['/a', '/a/b', '/x'])
    expect(visit(h, '/x')).toBe(h)
    expect(replaceCurrent(h, '/y').entries).toEqual(['/a', '/a/b', '/y'])
  })

  it('오래된 기록부터 버린다', () => {
    let h = startHistory('/0')
    for (let i = 1; i <= MAX_HISTORY + 5; i++) h = visit(h, `/${i}`)
    expect(h.entries.length).toBe(MAX_HISTORY)
    expect(current(h)).toBe(`/${MAX_HISTORY + 5}`)
  })
})

describe('가상 스크롤', () => {
  it('보이는 줄과 여유 줄만 그린다', () => {
    const l = layout(1000, 340, 340, ROW_HEIGHT, 2)
    expect(l).toEqual({ scrollHeight: 34000, first: 8, last: 22, top: 8 * ROW_HEIGHT })
    expect(layout(0, 0, 300)).toEqual({ scrollHeight: 0, first: 0, last: 0, top: 0 })
    expect(layout(5, 0, 340, ROW_HEIGHT, 2)).toMatchObject({ first: 0, last: 5 })
  })

  it('높이가 한계를 넘으면 비율로 줄이고 끝까지 닿는다', () => {
    const total = 500_000
    const view = 600
    expect(scrollHeightOf(total)).toBe(MAX_SCROLL_HEIGHT)
    const end = MAX_SCROLL_HEIGHT - view
    expect(virtualOffset(end, view, total)).toBeCloseTo(total * ROW_HEIGHT - view)
    const l = layout(total, end, view, ROW_HEIGHT, 0)
    expect(l.last).toBe(total)
    // 마지막 줄의 아래끝이 보이는 영역 아래끝과 맞는다.
    expect(l.top + (l.last - l.first) * ROW_HEIGHT).toBeCloseTo(end + view)
    expect(scrollTopFor(virtualOffset(12345, view, total), view, total)).toBeCloseTo(12345)
    expect(indexAt(total, end, view)).toBe(total - Math.ceil(view / ROW_HEIGHT))
  })

  it('선택한 줄이 보이도록 스크롤한다', () => {
    expect(scrollToShow(5, 100, 0, 340)).toBe(0)
    expect(scrollToShow(20, 100, 0, 340)).toBe(21 * ROW_HEIGHT - 340)
    expect(scrollToShow(2, 100, 500, 340)).toBe(2 * ROW_HEIGHT)
  })
})

describe('아이콘 보기 배치', () => {
  it('너비에 들어가는 만큼 칸을 두고 남는 폭을 나눈다', () => {
    expect(gridOf('list', 600)).toEqual({ cols: 1, rowHeight: ROW_HEIGHT, tileWidth: 600 - 2 * GRID_PAD })
    const g = gridOf('grid', 5 * TILE_MIN_WIDTH + 2 * GRID_PAD + 50)
    expect(g).toEqual({ cols: 5, rowHeight: TILE_HEIGHT, tileWidth: TILE_MIN_WIDTH + 10 })
    // 너비를 아직 모르거나 아주 좁아도 한 칸은 둔다.
    expect(gridOf('grid', 0).cols).toBe(1)
  })

  it('그릴 행의 칸 범위를 구한다', () => {
    expect(rowsOf(0, 4)).toBe(0)
    expect(rowsOf(9, 4)).toBe(3)
    const total = 1000
    const cols = 4
    const lay = layout(rowsOf(total, cols), 10 * TILE_HEIGHT, 3 * TILE_HEIGHT, TILE_HEIGHT, 1)
    expect(lay).toMatchObject({ first: 9, last: 14 })
    expect(itemRange(lay, cols, total)).toEqual({ first: 36, last: 56 })
    // 마지막 행은 칸 수까지만이다.
    expect(itemRange({ ...lay, first: 249, last: 250 }, cols, 998)).toEqual({ first: 996, last: 998 })
  })

  it('방향 키로 행·칸을 옮긴다', () => {
    expect(moveKey('ArrowRight', 3, 4, 10, 2)).toBe(4)
    expect(moveKey('ArrowLeft', 0, 4, 10, 2)).toBe(0)
    expect(moveKey('ArrowDown', 1, 4, 10, 2)).toBe(5)
    // 마지막 행이 짧으면 마지막 칸으로 간다.
    expect(moveKey('ArrowDown', 7, 4, 10, 2)).toBe(9)
    expect(moveKey('ArrowUp', 2, 4, 10, 2)).toBe(0)
    expect(moveKey('ArrowDown', -1, 4, 10, 2)).toBe(0)
    expect(moveKey('PageDown', 0, 4, 100, 3)).toBe(12)
    expect(moveKey('PageUp', 5, 4, 100, 3)).toBe(0)
    expect(moveKey('End', 0, 4, 10, 2)).toBe(9)
    // 목록 보기는 왼쪽·오른쪽 키를 쓰지 않는다.
    expect(moveKey('ArrowRight', 3, 1, 10, 2)).toBeNull()
    expect(moveKey('ArrowDown', 3, 1, 10, 2)).toBe(4)
    expect(moveKey('ArrowDown', 0, 4, 0, 2)).toBeNull()
  })
})

describe('보기 설정', () => {
  it('저장된 보기를 불러오고 바꾸면 저장한다', async () => {
    const saved: string[] = []
    const store = createFilesPrefsStore({
      fetchPreferences: async () => ({ files: { view: 'grid' } }),
      saveFilesPrefs: async (p) => {
        saved.push(p.view)
      },
    })
    // 불러오기 전에는 저장하지 않는다(저장된 값을 덮어쓰지 않게).
    store.getState().set('grid')
    expect(saved).toEqual([])
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'ready', layout: 'grid' })
    store.getState().set('grid')
    store.getState().set('list')
    await Promise.resolve()
    expect(saved).toEqual(['list'])
    expect(toLayout('columns')).toBe('list')
    expect(toLayout(undefined)).toBe('list')
  })

  it('불러오지 못하면 목록 보기로 둔다', async () => {
    const store = createFilesPrefsStore({
      fetchPreferences: async () => {
        throw new Error('down')
      },
      saveFilesPrefs: async () => {},
    })
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'error', layout: 'list' })
  })
})

describe('페이지 캐시', () => {
  it('보이는 범위의 빠진 페이지만 요청하고 오래된 페이지를 버린다', () => {
    expect(pagesFor(0, 0)).toEqual([])
    expect(pagesFor(150, 450)).toEqual([0, 1, 2])
    const c = new PageCache<number>(10, 3)
    expect(c.missing(0, 25)).toEqual([0, 1, 2])
    c.start(0)
    expect(c.missing(0, 25)).toEqual([1, 2])
    c.put(0, [0, 1, 2, 3, 4, 5, 6, 7, 8, 9])
    c.put(1, [10, 11])
    c.put(2, [20])
    expect(c.get(11)).toBe(11)
    expect(c.get(15)).toBeUndefined()
    c.touch(0, 5)
    c.put(3, [30])
    expect(c.size).toBe(3)
    expect(c.get(0)).toBe(0)
    expect(c.get(10)).toBeUndefined()
    c.set(30, 99)
    expect(c.get(30)).toBe(99)
    c.start(5)
    c.fail(5)
    expect(c.missing(50, 51)).toEqual([5])
  })
})

describe('종류', () => {
  it('확장자와 항목 종류로 정한다', () => {
    expect(extension('a.TAR.GZ')).toBe('gz')
    expect(extension('.bashrc')).toBe('')
    expect(extension('noext')).toBe('')
    expect(kindOf({ name: '영화.mkv', type: 'file' })).toBe('video')
    expect(kindOf({ name: 'x.unknownext', type: 'file' })).toBe('file')
    expect(kindOf({ name: 'd', type: 'dir', locked: true })).toBe('locked')
    expect(kindOf({ name: 'l', type: 'symlink', linkType: 'dir' })).toBe('folder')
    expect(kindOf({ name: 'l.mkv', type: 'symlink' })).toBe('link')
    expect(kindOf({ name: 'p', type: 'unknown' })).toBe('unknown')
    expect(isFolderLike({ type: 'symlink', linkType: 'dir' })).toBe(true)
    expect(isFolderLike({ type: 'file' })).toBe(false)
  })

  it('권한 비트를 ls 형식과 8진수로 보인다', () => {
    expect(modeString(0o644)).toBe('rw-r--r-- (644)')
    expect(modeString(0o2775)).toBe('rwxrwsr-x (2775)')
    expect(modeString(0o1777)).toBe('rwxrwxrwt (1777)')
    expect(modeString(0o4644)).toBe('rwSr--r-- (4644)')
    expect(modeString(0)).toBe('--------- (000)')
  })
})

describe('올리기 작업', () => {
  const files = [
    { name: 'a', size: 100, overwrite: false },
    { name: 'b', size: 50, overwrite: false },
    { name: 'c', size: 50, overwrite: false },
  ]

  it('파일을 하나씩 차례로 올리고 실패한 파일만 다시 올린다', () => {
    let j = newUploadJob(1, '/d', files)
    expect(nextFile(j)).toBe(0)
    j = fileProgressed(fileStarted(j, 0), 0, 150)
    expect(j.files[0].sent).toBe(100)
    expect(nextFile(j)).toBe(1)
    j = fileFinished(j, 0)
    j = fileFailed(fileStarted(j, 1), 1, 'file_exists')
    j = fileFailed(fileStarted(j, 2), 2, 'no_space')
    expect(nextFile(j)).toBe(-1)
    j = completed(j)
    expect(j.status).toBe('failed')
    expect(retryable(j, true)).toBe(1)
    expect(retryable(j, false)).toBe(1)
    // 덮어쓰기는 같은 이름 때문에 올리지 못한 파일만 덮어쓴다.
    const over = retried(j, true)
    expect(over.status).toBe('queued')
    expect(over.files.map((f) => [f.status, f.overwrite])).toEqual([
      ['done', false],
      ['queued', true],
      ['failed', false],
    ])
    expect(completed(fileFinished(over, 1)).status).toBe('failed')
  })

  it('멈추면 남은 파일을 취소하고 취소한 파일은 다시 올릴 수 있다', () => {
    let j = fileProgressed(fileStarted(newUploadJob(1, '/d', files), 0), 0, 40)
    j = stopped(j)
    expect(j.status).toBe('cancelled')
    expect(j.files.every((f) => f.status === 'cancelled')).toBe(true)
    expect(retryable(j, false)).toBe(3)
    expect(stopped(j, new ApiError(500, 'x')).status).toBe('failed')
  })

  it('올리지 못했거나 취소한 파일이 남았는지 본다', () => {
    let j = newUploadJob(1, '/d', files)
    for (const i of [0, 1, 2]) j = fileFinished(fileStarted(j, i), i)
    expect(unfinished(completed(j))).toBe(false)
    // 마지막 파일을 올리는 중에 취소하면 그 파일이 남는다.
    j = fileStarted(fileFinished(fileStarted(newUploadJob(1, '/d', files.slice(0, 2)), 0), 0), 1)
    expect(unfinished(stopped(j))).toBe(true)
    expect(unfinished(completed(fileFailed(j, 1, 'no_space')))).toBe(true)
    // 다 올린 뒤에 취소가 늦게 오면 남은 파일이 없다.
    expect(unfinished(stopped(fileFinished(j, 1)))).toBe(false)
  })

  it('올린 양은 취소한 파일을 빼고 바이트로 잰다', () => {
    let j = fileFinished(fileStarted(newUploadJob(1, '/d', files), 0), 0)
    j = { ...fileProgressed(fileStarted(j, 1), 1, 25), status: 'running' }
    expect(uploadAmount(j)).toEqual({ bytes: 125, totalBytes: 200, files: 1, totalFiles: 3 })
    expect(uploadProgress(j)).toBeCloseTo(0.625)
    expect(uploadRemaining(j)).toBeNull()
    expect(uploadRemaining({ ...j, rate: { at: 0, bytes: 0, rate: 25 } })).toBe(3)
    const empty = newUploadJob(2, '/d', [
      { name: 'x', size: 0, overwrite: false },
      { name: 'y', size: 0, overwrite: false },
    ])
    expect(uploadProgress(fileFinished(empty, 0))).toBe(0.5)
  })
})

describe('고르기', () => {
  it('누르기, Ctrl, Shift로 구간을 고른다', () => {
    let s = selectOne(3)
    expect(soleIndex(s)).toBe(3)
    s = toggleAt(s, 5)
    expect(selectedIndices(s)).toEqual([3, 5])
    expect(soleIndex(s)).toBeNull()
    s = toggleAt(s, 4)
    expect(s.spans).toEqual([{ start: 3, end: 6 }])
    s = toggleAt(s, 4)
    expect(selectedIndices(s)).toEqual([3, 5])
    expect(s.focus).toBe(4)
    // Shift는 기준(마지막으로 Ctrl로 누른 자리)에서 넓히고 나머지는 지운다.
    s = extendTo(s, 8)
    expect(s.spans).toEqual([{ start: 4, end: 9 }])
    s = extendTo(s, 1)
    expect(s.spans).toEqual([{ start: 1, end: 5 }])
    expect(s.anchor).toBe(4)
    // Ctrl+Shift는 이미 고른 것을 남긴다.
    s = extendTo({ ...selectOne(10), anchor: 7 }, 9, true)
    expect(s.spans).toEqual([{ start: 7, end: 11 }])
    expect(selectedCount(s)).toBe(4)
    expect(isSelected(s, 6)).toBe(false)
    expect(isSelected(s, 7)).toBe(true)
  })

  it('모두 고르기는 받지 않은 줄까지 구간 하나로 둔다', () => {
    const s = selectAll(selectOne(2), 300_000)
    expect(s.spans).toEqual([{ start: 0, end: 300_000 }])
    expect(selectedCount(s)).toBe(300_000)
    expect(s.focus).toBe(2)
    expect(selectAll(NO_SELECTION, 0)).toBe(NO_SELECTION)
    expect(extendTo(NO_SELECTION, 4).spans).toEqual([{ start: 4, end: 5 }])
  })
})

describe('새 이름', () => {
  it('올리기·만들기·바꾸기에 쓸 수 없는 이름을 가린다', () => {
    expect(nameProblem('')).toBe('empty')
    expect(nameProblem('..')).toBe('dots')
    expect(nameProblem('a/b')).toBe('chars')
    expect(nameProblem('a\0b')).toBe('chars')
    expect(nameProblem('가'.repeat(85))).toBeNull()
    expect(nameProblem('가'.repeat(86))).toBe('tooLong')
    expect(nameProblem('.onmaru-upload-x')).toBe('reserved')
    expect(nameProblem('.bashrc')).toBeNull()
    expect(uploadNameOk(' 공백 ')).toBe(true)
  })

  it('이름 바꾸기는 확장자를 빼고 고른다', () => {
    expect(stemLength('여름 휴가.mkv', false)).toBe(5)
    expect(stemLength('archive.tar.gz', false)).toBe(11)
    expect(stemLength('.bashrc', false)).toBe(7)
    expect(stemLength('noext', false)).toBe(5)
    expect(stemLength('폴더.d', true)).toBe(4)
    expect(numberedName('새 폴더', 1)).toBe('새 폴더')
    expect(numberedName('새 폴더', 3)).toBe('새 폴더 (3)')
  })

  it('옮길 수 있는 폴더를 경로 모양으로 가린다', () => {
    const src = ['/srv/d1/media/a.mkv', '/srv/d1/media/sub']
    expect(moveAllowed(src, '/srv/d1/media/other')).toBe(true)
    expect(moveAllowed(src, '/srv/d1')).toBe(true)
    expect(moveAllowed(src, '/srv/d1/media')).toBe(false)
    expect(moveAllowed(src, '/srv/d1/media/sub')).toBe(false)
    expect(moveAllowed(src, '/srv/d1/media/sub/inner')).toBe(false)
    expect(moveAllowed(['/srv/d1/media/sub'], '/srv/d1/media/subway')).toBe(true)
    expect(moveAllowed([], '/srv/d1')).toBe(false)
  })
})

describe('여러 항목 작업', () => {
  it('하나씩 차례로 하고 실패를 모으며 중지하면 남은 항목을 하지 않는다', async () => {
    const seen: string[] = []
    const ctl = new AbortController()
    const progress: number[] = []
    const res = await runEach(
      ['a', 'b', 'c', 'd'],
      async (n) => {
        seen.push(n)
        if (n === 'b') throw new ApiError(409, 'x', 'file_exists')
        if (n === 'c') ctl.abort()
      },
      (p) => progress.push(p.done),
      ctl.signal,
    )
    expect(seen).toEqual(['a', 'b', 'c'])
    expect(progress).toEqual([1, 2, 3])
    expect(res).toMatchObject({ done: 3, stopped: true })
    expect(res.failures.map((f) => f.item)).toEqual(['b'])
  })

  it('실패를 이유별로 묶는다', () => {
    const failures = [
      { item: 'a', err: new ApiError(409, 'x', 'cross_device') },
      { item: 'b', err: new ApiError(409, 'x', 'file_exists') },
      { item: 'c', err: new ApiError(409, 'x', 'cross_device') },
      { item: 'd', err: new ApiError(409, 'x', 'cross_device') },
    ]
    expect(groupFailures(failures, (n) => n, 2)).toEqual([
      { key: 'files.ops.errors.cross_device', names: ['a', 'c'], more: 1 },
      { key: 'files.ops.errors.file_exists', names: ['b'], more: 0 },
    ])
    expect(groupFailures([{ item: 'a', err: 'no_space' }], (n) => n, 5, () => 'files.upload.errors.no_space')).toEqual([
      { key: 'files.upload.errors.no_space', names: ['a'], more: 0 },
    ])
    expect(opErrorKey(new TypeError('network'))).toBe('files.ops.errors.network')
    expect(opErrorKey(new ApiError(500, 'x'))).toBe('files.ops.errors.generic')
  })

  it('바뀐 폴더와 없어진 경로 아래를 보는 창에 알린다', () => {
    const change = { dirs: ['/srv/d1/media'], gone: ['/srv/d1/media/sub'] }
    expect(affects(change, '/srv/d1/media')).toBe(true)
    expect(affects(change, '/srv/d1/media/sub/deep')).toBe(true)
    expect(affects(change, '/srv/d1/media/subway')).toBe(false)
    expect(affects(change, '/srv/d1')).toBe(false)
    const got: string[][] = []
    const off = onFilesChanged((c) => got.push([...c.dirs]))
    notifyFilesChanged(change)
    off()
    notifyFilesChanged(change)
    expect(got).toEqual([['/srv/d1/media']])
  })
})

describe('휴지통 볼륨', () => {
  it('경로가 든 가장 긴 볼륨을 고른다', () => {
    const volumes = [
      { top: '/home/alice', kind: 'home', shared: false },
      { top: '/srv/d1', kind: 'none', shared: false },
      { top: '/srv/d1/pool', kind: 'shared', shared: true },
    ] as const
    expect(volumeOf(volumes, '/home/alice/a.txt')?.kind).toBe('home')
    expect(volumeOf(volumes, '/srv/d1/media/x')?.kind).toBe('none')
    expect(volumeOf(volumes, '/srv/d1/pool/x')?.kind).toBe('shared')
    expect(volumeOf(volumes, '/srv/d10/x')).toBeNull()
  })
})

describe('복사 작업', () => {
  const volumes = [
    { top: '/home/u', kind: 'home', shared: false },
    { top: '/srv/d1', kind: 'own', shared: false },
    { top: '/srv/d2', kind: 'none', shared: false },
  ] as const

  it('다른 디스크면 복사하고, 쓸 수 없는 폴더에서는 같은 디스크로 옮기지 않는다', () => {
    expect(dropMode(volumes, '/srv/d1/a', '/srv/d1/b', 'trash:', true)).toBe('move')
    expect(dropMode(volumes, '/srv/d1/a', '/srv/d2', 'trash:', true)).toBe('copy')
    expect(dropMode(volumes, '/home/u/x', '/srv/d1/a', 'trash:', false)).toBe('copy')
    expect(dropMode(volumes, '/srv/d1/a', '/srv/d1/b', 'trash:', false)).toBeNull()
    expect(dropMode(volumes, '/srv/d1/a', 'trash:', 'trash:', true)).toBe('trash')
    expect(dropMode(volumes, '/srv/d1/a', 'trash:', 'trash:', false)).toBeNull()
    // 볼륨을 모르면 옮겨 본다.
    expect(dropMode(null, '/srv/d1/a', '/srv/d2', 'trash:', true)).toBe('move')
  })

  it('Go JSON 길이로 이름을 요청마다 나눈다', () => {
    expect(goJSONLength('abc')).toBe(5)
    expect(goJSONLength('가')).toBe(5)
    expect(goJSONLength('a"<\n')).toBe(2 + 1 + 2 + 6 + 6)
    expect(goJSONLength('😀')).toBe(6)
    const many = Array.from({ length: MAX_NAMES * 2 + 1 }, (_, i) => `f${i}`)
    expect(copyChunks(many).map((c) => c.length)).toEqual([MAX_NAMES, MAX_NAMES, 1])
    const long = Array.from({ length: 300 }, (_, i) => `${'가'.repeat(80)}${i}`)
    const chunks = copyChunks(long)
    expect(chunks.length).toBeGreaterThan(1)
    expect(chunks.flat()).toEqual(long)
    for (const c of chunks) expect(c.reduce((n, x) => n + goJSONLength(x) + 1, 2)).toBeLessThanOrEqual(MAX_NAMES_JSON)
    expect(copyChunks([])).toEqual([])
  })

  it('요청 여러 개의 진행을 합치고 속도와 남은 시간을 잰다', () => {
    const p = { files: 1, bytes: 50, totalFiles: 2, totalBytes: 100 }
    expect(addAmount(NO_AMOUNT, p)).toEqual(p)
    const job = { ...newJob(1, '/a', ['x'], '/b', 1), status: 'running' as const, scanning: false, amount: p }
    expect(progressOf(job)).toBe(0.5)
    expect(progressOf({ ...job, scanning: true })).toBeNull()
    // 둘째 요청의 절반: (1 + 0.5) / 2
    const base = { files: 4, bytes: 400, totalFiles: 4, totalBytes: 400 }
    expect(progressOf({ ...job, chunks: 2, chunk: 1, base, amount: addAmount(base, p) })).toBe(0.75)
    // 내용이 없는 파일만 있으면 파일 수로 잰다.
    expect(progressOf({ ...job, amount: { files: 1, bytes: 0, totalFiles: 4, totalBytes: 0 } })).toBe(0.25)

    const r0 = { at: 1000, bytes: 0, rate: null }
    expect(nextRate(r0, 1200, 100)).toBe(r0)
    const r1 = nextRate(r0, 2000, 1000)
    expect(r1).toEqual({ at: 2000, bytes: 1000, rate: 1000 })
    expect(nextRate(r1, 3000, 3000).rate).toBeCloseTo(1000 * 0.7 + 2000 * 0.3)
    expect(copyRemaining({ ...job, rate: { at: 0, bytes: 0, rate: 10 } })).toBe(5)
    expect(copyRemaining({ ...job, chunks: 2, rate: { at: 0, bytes: 0, rate: 10 } })).toBeNull()
    expect(durationOf(5.2)).toEqual({ unit: 'second', value: 6 })
    expect(durationOf(61)).toEqual({ unit: 'minute', value: 2 })
    expect(durationOf(5400)).toEqual({ unit: 'hour', value: 1.5 })
  })
})
