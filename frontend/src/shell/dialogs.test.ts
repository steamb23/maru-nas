import { beforeEach, describe, expect, it, vi } from 'vitest'

type Dialogs = typeof import('./dialogs')
let d: Dialogs

const desktop = { kind: 'desktop' } as const
const win = (windowId: string) => ({ kind: 'window', windowId }) as const
const render = () => null

beforeEach(async () => {
  // 저장소와 대기 목록이 모듈 상태라 테스트마다 새로 불러온다.
  vi.resetModules()
  d = await import('./dialogs')
})

const entries = () => d.useDialogStore.getState().entries

describe('dialogs', () => {
  it('resolves null at once when the scope has no outlet', async () => {
    await expect(d.openDialog(desktop, { render })).resolves.toBeNull()
    expect(entries()).toEqual([])
  })

  it('resolves with the closed value and keeps the entry until removed', async () => {
    d.attachOutlet(desktop)
    const result = d.openDialog<{ name: string }>(desktop, { render })
    const [entry] = entries()
    expect(entry).toMatchObject({ scope: 'desktop', open: true })

    d.settleDialog(entry.key, { name: 'web' })
    await expect(result).resolves.toEqual({ name: 'web' })
    // 닫는 애니메이션 동안은 남아 있다. 두 번째 결과는 무시한다.
    expect(entries()[0].open).toBe(false)
    d.settleDialog(entry.key, { name: 'db' })

    d.removeDialog(entry.key)
    expect(entries()).toEqual([])
  })

  it('cancels with null when removed before settling', async () => {
    d.attachOutlet(desktop)
    const result = d.openDialog(desktop, { render })
    d.removeDialog(entries()[0].key)
    await expect(result).resolves.toBeNull()
  })

  it('dismisses only the given scope', async () => {
    d.attachOutlet(desktop)
    d.attachOutlet(win('w1'))
    d.attachOutlet(win('w2'))
    const a = d.openDialog(win('w1'), { render })
    const b = d.openDialog(win('w2'), { render })
    const c = d.openDialog(desktop, { render })

    d.dismissScope(win('w1'))
    await expect(a).resolves.toBeNull()
    expect(entries().map((e) => [e.scope, e.open])).toEqual([
      ['window:w1', false],
      ['window:w2', true],
      ['desktop', true],
    ])

    for (const e of entries()) d.settleDialog(e.key, 'ok')
    await expect(b).resolves.toBe('ok')
    await expect(c).resolves.toBe('ok')
  })

  it('drops the scope dialogs when the outlet detaches', async () => {
    d.attachOutlet(desktop)
    d.attachOutlet(win('w1'))
    const a = d.openDialog(win('w1'), { render })
    d.openDialog(desktop, { render })

    d.detachOutlet(win('w1'))
    await expect(a).resolves.toBeNull()
    expect(entries().map((e) => e.scope)).toEqual(['desktop'])
    expect(d.useDialogStore.getState().outlets).toEqual(['desktop'])
    await expect(d.openDialog(win('w1'), { render })).resolves.toBeNull()
  })
})
