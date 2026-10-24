import { describe, expect, it } from 'vitest'
import { NOTIFICATIONS_POLL_INTERVAL_MS, NOTIFICATIONS_TASK_INTERVAL_MS, pollInterval } from './useNotifications'

describe('알림 받기 주기', () => {
  it('진행 중인 작업이 있으면 자주 받는다', () => {
    expect(pollInterval(0)).toBe(NOTIFICATIONS_POLL_INTERVAL_MS)
    expect(pollInterval(1)).toBe(NOTIFICATIONS_TASK_INTERVAL_MS)
    expect(NOTIFICATIONS_TASK_INTERVAL_MS).toBeLessThan(NOTIFICATIONS_POLL_INTERVAL_MS)
  })
})
