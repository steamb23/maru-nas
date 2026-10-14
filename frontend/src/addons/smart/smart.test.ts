import { describe, expect, it } from 'vitest'
import type { SmartDisk } from '../../api/types.gen'
import { countDisks, isSmartResponse, sortDisks, statusLevel } from './smart'

const disk = (device: string, status: SmartDisk['status'], celsius?: number): SmartDisk => ({
  device,
  model: 'Fake',
  sizeBytes: 1,
  celsius,
  status,
  overallStatus: 'X',
  monitored: true,
})

describe('isSmartResponse', () => {
  it('정상 응답을 받는다', () => {
    expect(
      isSmartResponse({
        sources: ['omv-smart'],
        checkedAt: '2026-01-01T00:00:00Z',
        refreshInSeconds: 0,
        disks: [disk('sda', 'good', 30)],
      }),
    ).toBe(true)
  })
  it('모르는 상태는 거부한다', () => {
    expect(isSmartResponse({ sources: [], checkedAt: '', refreshInSeconds: 0, disks: [{ ...disk('sda', 'good'), status: 'bad' }] })).toBe(false)
  })
})

it('statusLevel', () => {
  expect(statusLevel('good')).toBe('normal')
  expect(statusLevel('warning')).toBe('warn')
  expect(statusLevel('unknown')).toBe('warn')
  expect(statusLevel('failing')).toBe('crit')
})

it('sortDisks는 문제 있는 디스크를 먼저 둔다', () => {
  const list = [disk('sdb', 'good'), disk('sda', 'good'), disk('sdc', 'warning'), disk('sdd', 'failing')]
  expect(sortDisks(list).map((d) => d.device)).toEqual(['sdd', 'sdc', 'sda', 'sdb'])
})

it('countDisks는 상태를 모르는 디스크를 주의로 센다', () => {
  expect(countDisks([disk('a', 'good'), disk('b', 'unknown'), disk('c', 'failing')])).toEqual({
    good: 1,
    warning: 1,
    failing: 1,
  })
})
