import { describe, expect, it } from 'vitest'
import type { HelperStatus } from '../api/types.gen'
import { elevationEndsAt, groupNames, secondsLeft } from './helper'

const active = (elevatedSeconds: number): HelperStatus => ({ state: 'active', canElevate: true, elevatedSeconds })

describe('elevationEndsAt', () => {
  it('상승 중이면 받은 시각 기준 끝나는 시각', () => {
    expect(elevationEndsAt(active(300), 1_000)).toBe(301_000)
  })

  it('상승하지 않았거나 쓸 수 없는 상태면 null', () => {
    expect(elevationEndsAt(active(0), 1_000)).toBeNull()
    expect(elevationEndsAt({ state: 'reauth', canElevate: false, elevatedSeconds: 10 }, 1_000)).toBeNull()
  })
})

describe('secondsLeft', () => {
  it('남은 초를 올림하고 끝나면 0', () => {
    expect(secondsLeft(10_000, 0)).toBe(10)
    expect(secondsLeft(10_000, 9_001)).toBe(1)
    expect(secondsLeft(10_000, 10_000)).toBe(0)
    expect(secondsLeft(10_000, 20_000)).toBe(0)
    expect(secondsLeft(null, 0)).toBe(0)
  })
})

describe('groupNames', () => {
  it('그룹 이름을 순서대로 잇는다', () => {
    const id = { uid: 1, gid: 100, user: 'u', groups: [{ id: 100, name: 'users' }, { id: 27, name: 'sudo' }] }
    expect(groupNames(id)).toBe('users, sudo')
  })
})
