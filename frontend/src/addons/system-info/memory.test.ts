import { describe, expect, it } from 'vitest'
import type { MemoryDetail } from '../../api/types.gen'
import { memorySegments } from './memory'

function detail(over: Partial<MemoryDetail> = {}): MemoryDetail {
  return {
    appsBytes: 300,
    sharedBytes: 50,
    compressedBytes: 0,
    buffersBytes: 50,
    cacheBytes: 400,
    freeBytes: 200,
    swapTotalBytes: 0,
    swapUsedBytes: 0,
    ...over,
  }
}

describe('memorySegments', () => {
  it('정해진 순서로 0인 항목을 빼고 이어 붙인다', () => {
    const segs = memorySegments(detail(), 1000)
    expect(segs.map((s) => [s.part, s.slot, s.bytes, s.percent, s.start])).toEqual([
      ['apps', 1, 300, 30, 0],
      ['shared', 2, 50, 5, 30],
      ['buffers', 4, 50, 5, 35],
      ['cache', 5, 400, 40, 40],
      ['free', null, 200, 20, 80],
    ])
  })

  it('압축이 있으면 공유와 버퍼 사이에 3번 색으로 둔다', () => {
    const segs = memorySegments(detail({ appsBytes: 250, compressedBytes: 50 }), 1000)
    expect(segs.map((s) => s.part)).toEqual(['apps', 'shared', 'compressed', 'buffers', 'cache', 'free'])
    expect(segs[2]).toMatchObject({ slot: 3, percent: 5, start: 30 })
  })

  it('합이 전체를 넘으면 100%에서 자른다', () => {
    const segs = memorySegments(detail({ freeBytes: 500 }), 1000)
    const last = segs[segs.length - 1]
    expect(last).toMatchObject({ part: 'free', percent: 20, start: 80 })
    expect(memorySegments(detail({ appsBytes: 2000 }), 1000).map((s) => s.part)).toEqual(['apps'])
  })

  it('전체가 0이면 조각이 없다', () => {
    expect(memorySegments(detail(), 0)).toEqual([])
  })
})
