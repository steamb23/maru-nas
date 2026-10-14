import { describe, expect, it, vi } from 'vitest'
import { INITIAL_ACTION_STATE, start } from './actionState'
import { type ActionSpec, actionMenuItems } from './actionSpec'

const specs: ActionSpec[] = [
  {
    action: 'stop',
    icon: 'media-playback-stop-symbolic',
    labelKey: 'actions.working',
    runningKey: 'actions.working',
    confirmKey: 'actions.working',
  },
  {
    action: 'restart',
    icon: 'view-refresh-symbolic',
    labelKey: 'actions.dismiss',
    runningKey: 'actions.dismiss',
    confirmKey: 'actions.dismiss',
  },
]

describe('actionMenuItems', () => {
  it('builds one action item per spec', () => {
    const select = vi.fn()
    const items = actionMenuItems(INITIAL_ACTION_STATE, 'web', specs, { select })
    expect(items.map((i) => i.id)).toEqual(['stop', 'restart'])
    const first = items[0]
    if (first.kind !== 'action') throw new Error('not an action')
    expect(first.disabled).toBe(false)
    first.onSelect()
    expect(select).toHaveBeenCalledWith(specs[0])
  })

  it('disables items while the target is busy', () => {
    const items = actionMenuItems(start(INITIAL_ACTION_STATE, 'web', 'stop'), 'web', specs, { select: vi.fn() })
    expect(items.every((i) => i.kind === 'action' && i.disabled)).toBe(true)
  })
})
