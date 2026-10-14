import { describe, expect, it, vi } from 'vitest'
import { mergeHandlers } from './useContextMenuTrigger'

describe('mergeHandlers', () => {
  it('같은 이벤트의 처리기를 앞에서부터 차례로 부른다', () => {
    const calls: string[] = []
    const a = { onPointerDown: (e: number) => calls.push(`a${e}`), onKeyDown: vi.fn() }
    const b = { onPointerDown: (e: number) => calls.push(`b${e}`), onPointerUp: vi.fn() }
    const m = mergeHandlers(a, b)
    m.onPointerDown(1)
    expect(calls).toEqual(['a1', 'b1'])
    m.onKeyDown(2)
    m.onPointerUp(3)
    expect(a.onKeyDown).toHaveBeenCalledWith(2)
    expect(b.onPointerUp).toHaveBeenCalledWith(3)
  })
})
