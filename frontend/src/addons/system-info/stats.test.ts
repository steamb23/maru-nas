import { describe, expect, it } from 'vitest'
import type { SystemStatsResponse } from '../../api/types.gen'
import { HISTORY_MS, appendSample, busyPercent, isSystemStatsResponse, kernelPercent, latest } from './stats'

// sample은 합성 응답이다. 필요한 값만 덮어쓴다.
function sample(over: Partial<SystemStatsResponse> = {}): SystemStatsResponse {
  return {
    sources: ['proc'],
    sampledAtMs: 0,
    cpu: {
      totalTicks: 1000,
      idleTicks: 800,
      kernelTicks: 50,
      cores: [
        { id: 0, totalTicks: 500, idleTicks: 400, kernelTicks: 25 },
        { id: 1, totalTicks: 500, idleTicks: 400, kernelTicks: 25 },
      ],
    },
    memory: { totalBytes: 1000, usedBytes: 250, availableBytes: 750 },
    memoryDetail: {
      appsBytes: 200,
      sharedBytes: 50,
      compressedBytes: 0,
      buffersBytes: 50,
      cacheBytes: 400,
      freeBytes: 300,
      swapTotalBytes: 0,
      swapUsedBytes: 0,
    },
    uptimeSeconds: 10,
    load: { one: 0.5, five: 0.25, fifteen: 0.1, running: 1 },
    network: { rxBytes: 1000, txBytes: 500 },
    disk: { readBytes: 4096, writeBytes: 0 },
    ...over,
  }
}

describe('isSystemStatsResponse', () => {
  it('백엔드 응답 모양을 받는다', () => {
    expect(isSystemStatsResponse(sample())).toBe(true)
    // 네트워크·디스크·부하·프로세스는 없을 수 있다.
    expect(isSystemStatsResponse(sample({ network: undefined, disk: undefined, load: undefined }))).toBe(true)
    expect(
      isSystemStatsResponse(
        sample({
          processes: [
            {
              pid: 1,
              startTicks: 2,
              name: 'init',
              user: 'root',
              state: 'S',
              cpuTicks: 5,
              rssBytes: 100,
              threads: 1,
            },
          ],
        }),
      ),
    ).toBe(true)
  })

  it.each([
    ['null', null],
    ['cores 없음', { ...sample(), cpu: { totalTicks: 1, idleTicks: 1, kernelTicks: 0 } }],
    [
      '코어 id 없음',
      { ...sample(), cpu: { totalTicks: 1, idleTicks: 1, kernelTicks: 0, cores: [{ totalTicks: 1, idleTicks: 1 }] } },
    ],
    ['커널 시간 없음', { ...sample(), cpu: { totalTicks: 1, idleTicks: 1, cores: [] } }],
    ['메모리 구성 없음', { ...sample(), memoryDetail: undefined }],
    ['메모리 구성 값이 문자열', { ...sample(), memoryDetail: { ...sample().memoryDetail, cacheBytes: '1' } }],
    ['부하 값 없음', { ...sample(), load: { one: 1, five: 1, running: 1 } }],
    ['sources 없음', { ...sample(), sources: undefined }],
    ['프로세스 이름이 숫자', { ...sample(), processes: [{ pid: 1, name: 1 }] }],
    ['네트워크 값이 문자열', { ...sample(), network: { rxBytes: '1', txBytes: 0 } }],
  ])('%s는 거부한다', (_name, v) => {
    expect(isSystemStatsResponse(v)).toBe(false)
  })
})

describe('busyPercent', () => {
  it('두 누적 값 사이의 사용 비율이다', () => {
    expect(busyPercent({ totalTicks: 100, idleTicks: 50 }, { totalTicks: 200, idleTicks: 125 })).toBe(25)
    expect(busyPercent({ totalTicks: 100, idleTicks: 50 }, { totalTicks: 100, idleTicks: 50 })).toBeNull()
    expect(busyPercent({ totalTicks: 100, idleTicks: 50 }, { totalTicks: 90, idleTicks: 40 })).toBeNull()
  })
})

describe('kernelPercent', () => {
  it('두 누적 값 사이의 커널 시간 비율이다', () => {
    expect(kernelPercent({ totalTicks: 100, kernelTicks: 10 }, { totalTicks: 200, kernelTicks: 30 })).toBe(20)
    expect(kernelPercent({ totalTicks: 100, kernelTicks: 10 }, { totalTicks: 100, kernelTicks: 10 })).toBeNull()
    expect(kernelPercent({ totalTicks: 100, kernelTicks: 10 }, { totalTicks: 200, kernelTicks: 5 })).toBeNull()
  })
})

describe('appendSample', () => {
  it('첫 응답은 점 없이 시작하고, 두 번째부터 차이로 값을 구한다', () => {
    const first = appendSample(null, sample())
    expect(first.points).toEqual([])
    expect(latest(first)).toBeNull()

    const h = appendSample(
      first,
      sample({
        sampledAtMs: 2000,
        cpu: {
          totalTicks: 1400,
          idleTicks: 1100, // 사용 100 / 400 = 25%
          kernelTicks: 90,
          cores: [
            { id: 0, totalTicks: 700, idleTicks: 500, kernelTicks: 65 }, // 50%
            { id: 1, totalTicks: 700, idleTicks: 600, kernelTicks: 25 }, // 0%
          ],
        },
        memory: { totalBytes: 1000, usedBytes: 500, availableBytes: 500 },
        network: { rxBytes: 3000, txBytes: 500 },
        disk: { readBytes: 4096, writeBytes: 8192 },
      }),
    )
    const p = latest(h)
    expect(p).toMatchObject({
      t: 2000,
      cpu: 25,
      cpuKernel: 10, // 커널 40 / 400
      memory: 50,
      memoryUsed: 500,
      rx: 1000,
      tx: 0,
      read: 0,
      write: 4096,
    })
    expect(p?.cores).toEqual(
      new Map([
        [0, 50],
        [1, 0],
      ]),
    )
    expect(p?.coresKernel).toEqual(
      new Map([
        [0, 20],
        [1, 0],
      ]),
    )
  })

  it('코어는 id로 맞추고, 새로 켜진 코어는 다음 샘플부터 나온다', () => {
    const first = appendSample(null, sample())
    const cur = sample({
      sampledAtMs: 2000,
      cpu: {
        totalTicks: 1400,
        idleTicks: 1100,
        kernelTicks: 50,
        cores: [
          { id: 1, totalTicks: 700, idleTicks: 500, kernelTicks: 25 },
          { id: 2, totalTicks: 100, idleTicks: 100, kernelTicks: 0 },
        ],
      },
    })
    expect(latest(appendSample(first, cur))?.cores).toEqual(new Map([[1, 50]]))
  })

  it('카운터가 줄었거나 값이 없으면 그 값만 비운다', () => {
    const first = appendSample(null, sample())
    const p = latest(appendSample(first, sample({ sampledAtMs: 2000, network: { rxBytes: 10, txBytes: 600 }, disk: undefined })))
    expect(p).toMatchObject({ rx: null, tx: 50, read: null, write: null })
  })

  it('간격이 너무 길거나 백엔드가 다시 시작하면 기록을 새로 시작한다', () => {
    let h = appendSample(null, sample())
    h = appendSample(h, sample({ sampledAtMs: 2000 }))
    expect(h.points).toHaveLength(1)
    expect(appendSample(h, sample({ sampledAtMs: 2000 + 31_000 })).points).toEqual([])
    expect(appendSample(h, sample({ sampledAtMs: 500 })).points).toEqual([])
  })

  it('60초보다 오래된 점은 창 왼쪽 끝에 닿는 하나만 남긴다', () => {
    let h = appendSample(null, sample())
    for (let t = 2000; t <= 70_000; t += 2000) h = appendSample(h, sample({ sampledAtMs: t }))
    const ts = h.points.map((p) => p.t)
    expect(ts[0]).toBeLessThanOrEqual(70_000 - HISTORY_MS)
    expect(ts[1]).toBeGreaterThan(70_000 - HISTORY_MS)
    expect(ts[ts.length - 1]).toBe(70_000)
  })
})
