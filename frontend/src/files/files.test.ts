import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
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
  active,
  canceled,
  clearFinished,
  emptyUploads,
  enqueue,
  failed,
  finished,
  nextQueued,
  progressed,
  remainingSeconds,
  retried,
  started,
} from './uploadQueue'
import { layout, MAX_SCROLL_HEIGHT, ROW_HEIGHT, indexAt, scrollHeightOf, scrollToShow, scrollTopFor, virtualOffset } from './virtual'

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

describe('올리기 목록', () => {
  it('하나씩 차례로 올리고 끝난 항목을 비운다', () => {
    let s = enqueue(emptyUploads, [
      { id: 1, dir: '/d', name: 'a', size: 100, overwrite: false },
      { id: 2, dir: '/d', name: 'b', size: 50, overwrite: false },
    ])
    const a = nextQueued(s)
    expect(a?.name).toBe('a')
    s = started(s, a!.id, 1000)
    expect(nextQueued(s)).toBeUndefined()
    s = progressed(s, a!.id, 150)
    expect(s.items[0].sent).toBe(100)
    s = finished(s, a!.id)
    const b = nextQueued(s)!
    s = failed(started(s, b.id, 2000), b.id, 'file_exists')
    expect(active(s)).toBe(0)
    s = retried(s, b.id, true)
    expect(s.items[1]).toMatchObject({ status: 'queued', overwrite: true, error: undefined })
    s = canceled(s, b.id)
    expect(clearFinished(s).items).toEqual([])
  })

  it('남은 시간은 3초가 지난 뒤 보낸 속도로 셈한다', () => {
    let s = enqueue(emptyUploads, [{ id: 1, dir: '/d', name: 'a', size: 1000, overwrite: false }])
    s = progressed(started(s, 1, 0), 1, 300)
    expect(remainingSeconds(s.items[0], 2000)).toBeNull()
    expect(remainingSeconds(s.items[0], 3000)).toBe(7)
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
