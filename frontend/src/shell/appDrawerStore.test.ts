import { beforeEach, describe, expect, it } from 'vitest'
import { useAppDrawerStore } from './appDrawerStore'

beforeEach(() => useAppDrawerStore.getState().close())

describe('appDrawerStore', () => {
  it('열고 닫는다', () => {
    const s = useAppDrawerStore.getState()
    s.show()
    expect(useAppDrawerStore.getState()).toMatchObject({ open: true, viaTouch: false })
    s.close()
    expect(useAppDrawerStore.getState().open).toBe(false)
  })

  it('toggle은 열려 있으면 닫고 닫혀 있으면 연다', () => {
    const s = useAppDrawerStore.getState()
    s.toggle(true)
    expect(useAppDrawerStore.getState()).toMatchObject({ open: true, viaTouch: true })
    s.toggle()
    expect(useAppDrawerStore.getState().open).toBe(false)
  })

  it('애니메이션 없이 닫기를 기억하고 다시 열면 지운다', () => {
    const s = useAppDrawerStore.getState()
    s.show()
    s.close(true)
    expect(useAppDrawerStore.getState()).toMatchObject({ open: false, instant: true })
    s.show()
    expect(useAppDrawerStore.getState()).toMatchObject({ open: true, instant: false })
  })

  it('닫혀 있을 때 close는 상태를 바꾸지 않는다', () => {
    const before = useAppDrawerStore.getState()
    before.close()
    expect(useAppDrawerStore.getState()).toBe(before)
  })
})
