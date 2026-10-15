import { describe, expect, it } from 'vitest'
import lock from '../../../package-lock.json'
import { NOTICES, missingNotices } from './notices'

describe('NOTICES', () => {
  it('프론트 런타임 의존성이 모두 고지에 있다', () => {
    expect(missingNotices(lock.packages as Record<string, { dev?: boolean }>)).toEqual([])
  })

  it('고지에 없는 런타임 패키지를 찾는다', () => {
    expect(
      missingNotices({
        '': {},
        'node_modules/react': {},
        'node_modules/left-pad': {},
        'node_modules/vite': { dev: true },
        'node_modules/csstype': {},
      }),
    ).toEqual(['left-pad'])
  })

  it('이름이 겹치지 않는다', () => {
    expect(new Set(NOTICES.map((n) => n.name)).size).toBe(NOTICES.length)
  })
})
