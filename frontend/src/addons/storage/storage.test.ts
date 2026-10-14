import { describe, expect, it } from 'vitest'
import type { Filesystem } from '../../api/types.gen'
import { fsName, isStorageResponse, usedPercent, widgetFilesystems } from './storage'

const fs = (over: Partial<Filesystem>): Filesystem => ({
  device: '/dev/sdx1',
  mountPoint: '/mnt/x',
  type: 'ext4',
  totalBytes: 100,
  usedBytes: 40,
  availableBytes: 50,
  ...over,
})

describe('isStorageResponse', () => {
  it('정상 응답을 받는다', () => {
    expect(isStorageResponse({ sources: ['proc'], filesystems: [fs({ label: 'data', readOnly: true })] })).toBe(true)
  })
  it('형식이 다르면 거부한다', () => {
    expect(isStorageResponse({ sources: ['proc'], filesystems: [{ ...fs({}), totalBytes: '100' }] })).toBe(false)
    expect(isStorageResponse({ filesystems: [] })).toBe(false)
  })
})

describe('fsName', () => {
  it('라벨을 먼저 쓴다', () => {
    expect(fsName(fs({ label: 'data', mountPoint: '/' }))).toEqual({ kind: 'text', text: 'data' })
  })
  it('루트는 따로 표시한다', () => {
    expect(fsName(fs({ mountPoint: '/' }))).toEqual({ kind: 'root' })
  })
  it('OMV 마운트 경로면 장치 이름을 쓴다', () => {
    expect(fsName(fs({ device: '/dev/sdb1', mountPoint: '/srv/dev-disk-by-uuid-0000' }))).toEqual({
      kind: 'text',
      text: 'sdb1',
    })
  })
  it('그 밖에는 마운트 경로의 마지막 부분을 쓴다', () => {
    expect(fsName(fs({ mountPoint: '/srv/mergerfs/pool' }))).toEqual({ kind: 'text', text: 'pool' })
  })
})

describe('usedPercent', () => {
  it('root 예약분을 빼고 df처럼 계산한다', () => {
    expect(usedPercent(fs({ usedBytes: 40, availableBytes: 40 }))).toBe(50)
  })
  it('용량이 0이면 0', () => {
    expect(usedPercent(fs({ usedBytes: 0, availableBytes: 0 }))).toBe(0)
  })
})

it('widgetFilesystems는 /boot 아래를 뺀다', () => {
  const list = [fs({ mountPoint: '/' }), fs({ mountPoint: '/boot/efi' }), fs({ mountPoint: '/boot' }), fs({ mountPoint: '/bootleg' })]
  expect(widgetFilesystems(list).map((f) => f.mountPoint)).toEqual(['/', '/bootleg'])
})
