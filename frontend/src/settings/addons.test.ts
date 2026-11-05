import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import type { AddonPackageStatus } from '../api/types.gen'
import en from '../i18n/locales/en.json'
import {
  addonError,
  authStep,
  mib,
  packageTitle,
  reasonKey,
  riskKeys,
  sortPackages,
  stateBadgeKey,
  warningKeys,
  withChanges,
} from './addons'

function pkg(over: Partial<AddonPackageStatus> & { id: string }): AddonPackageStatus {
  return { dev: false, enabled: true, state: 'active', ...over }
}

// keyExists는 en.json에 그 키가 있는지다(문구 키를 빠뜨리지 않았는지).
function keyExists(key: string): boolean {
  let v: unknown = en
  for (const part of key.split('.')) {
    if (typeof v !== 'object' || v === null) return false
    v = (v as Record<string, unknown>)[part]
  }
  return typeof v === 'string'
}

describe('packageTitle', () => {
  it('언어의 이름, 없으면 영어, 그것도 없으면 id다', () => {
    expect(packageTitle(pkg({ id: 'hello', name: { en: 'Hello', ko: '안녕' } }), 'ko')).toBe('안녕')
    expect(packageTitle(pkg({ id: 'hello', name: { en: 'Hello' } }), 'de')).toBe('Hello')
    expect(packageTitle(pkg({ id: 'hello' }), 'ko')).toBe('hello')
  })
})

describe('sortPackages', () => {
  it('쓰지 못하는 패키지를 앞에, 나머지는 이름순으로 둔다', () => {
    const list = [
      pkg({ id: 'zeta', name: { en: 'Zeta' } }),
      pkg({ id: 'alpha', name: { en: 'Alpha' }, enabled: false, state: 'disabled' }),
      pkg({ id: 'broken', state: 'invalid', reason: 'manifest_invalid' }),
      pkg({ id: 'clash', name: { en: 'Clash' }, state: 'rejected', reason: 'conflict_builtin' }),
    ]
    expect(sortPackages(list, 'en').map((p) => p.id)).toEqual(['broken', 'clash', 'alpha', 'zeta'])
  })

  it('원래 배열을 바꾸지 않는다', () => {
    const list = [pkg({ id: 'b' }), pkg({ id: 'a' })]
    sortPackages(list, 'en')
    expect(list.map((p) => p.id)).toEqual(['b', 'a'])
  })
})

describe('stateBadgeKey', () => {
  it('켜짐·꺼짐에는 배지가 없다', () => {
    expect(stateBadgeKey({ state: 'active' })).toBeNull()
    expect(stateBadgeKey({ state: 'disabled' })).toBeNull()
  })

  it('이유에 따라 배지가 다르다', () => {
    expect(stateBadgeKey({ state: 'invalid', reason: 'incompatible' })).toBe('settings.addons.state.incompatible')
    expect(stateBadgeKey({ state: 'invalid', reason: 'archive_invalid' })).toBe('settings.addons.state.invalid')
    expect(stateBadgeKey({ state: 'rejected', reason: 'conflict_package' })).toBe('settings.addons.state.conflict')
    expect(stateBadgeKey({ state: 'rejected', reason: 'rpc_denied' })).toBe('settings.addons.state.notAllowed')
  })
})

describe('문구 키', () => {
  // 백엔드의 이유 코드(internal/addonpkg/archive.go, internal/addons/extend.go)
  const reasons = [
    'archive_invalid',
    'too_large',
    'manifest_invalid',
    'incompatible',
    'arch_missing',
    'backend_invalid',
    'conflict_builtin',
    'conflict_package',
    'rpc_denied',
    'rpc_kind',
    'reserved_id',
    'invalid',
  ]

  it('백엔드의 이유 코드마다 문구가 있다', () => {
    for (const r of reasons) {
      const key = reasonKey(r)
      expect(key).not.toBe('settings.addons.reason.unknown')
      expect(keyExists(key), key).toBe(true)
    }
    expect(reasonKey('something_new')).toBe('settings.addons.reason.unknown')
    expect(reasonKey(undefined)).toBe('settings.addons.reason.unknown')
  })

  it('위험 코드마다 문구가 있고, 모르는 코드는 하나로 모은다', () => {
    const keys = riskKeys(['omv_write', 'backend', 'backend_shared_data', 'net_lan', 'net_loopback'])
    expect(keys).toHaveLength(5)
    for (const k of keys) expect(keyExists(k), k).toBe(true)
    expect(riskKeys(['x', 'y'])).toEqual(['settings.addons.risk.unknown'])
  })

  it('경고 코드마다 문구가 있고, 모르는 코드는 뺀다', () => {
    const keys = warningKeys(['author_changed', 'downgrade', 'reinstall', 'same_digest', 'backends_disabled', 'other'])
    expect(keys).toHaveLength(5)
    for (const k of keys) expect(keyExists(k), k).toBe(true)
  })
})

describe('withChanges', () => {
  it('더해진 항목에 표시를 붙이고 빠진 항목을 뒤에 둔다', () => {
    expect(withChanges(['A.get', 'B.get'], ['B.get'], ['C.get'])).toEqual([
      { value: 'A.get' },
      { value: 'B.get', change: 'added' },
      { value: 'C.get', change: 'removed' },
    ])
  })

  it('차이가 없으면 지금 목록 그대로다', () => {
    expect(withChanges(['A.get'])).toEqual([{ value: 'A.get' }])
  })
})

describe('authStep', () => {
  it('관리 창과 단계 확인 거절만 다시 할 수 있다', () => {
    expect(authStep(new ApiError(403, 'x', 'manage_required'))).toEqual({ kind: 'manage' })
    expect(authStep(new ApiError(403, 'x', 'security_required', undefined, 'twoFactor'))).toEqual({
      kind: 'stepUp',
      level: 'twoFactor',
    })
    expect(authStep(new ApiError(403, 'x', 'admin_required'))).toBeNull()
    expect(authStep(new ApiError(409, 'x', 'action_busy'))).toBeNull()
    expect(authStep(new Error('network'))).toBeNull()
  })
})

describe('addonError', () => {
  it('받지 않은 이유와 영어 설명을 보인다', () => {
    const err = new ApiError(409, 'x', 'addon_invalid', undefined, undefined, {
      reason: 'conflict_builtin',
      detail: 'id "files" is a built-in addon',
    })
    expect(addonError(err, 64)).toEqual({ key: 'settings.addons.reason.conflict_builtin', detail: 'id "files" is a built-in addon' })
  })

  it('크기 초과는 한도를 함께 보인다', () => {
    expect(addonError(new ApiError(413, 'x', 'addon_too_large'), 64)).toEqual({
      key: 'settings.addons.errors.tooLarge',
      params: { size: 64 },
    })
    // 프록시가 본문 크기로 끊으면 코드가 없다.
    expect(addonError(new ApiError(413, 'x'), 64).key).toBe('settings.addons.errors.tooLarge')
  })

  it('그 밖의 오류는 일반 문구다', () => {
    expect(addonError(new ApiError(500, 'x'), 64).key).toBe('settings.addons.errors.failed')
    expect(addonError(new TypeError('network'), 64).key).toBe('settings.addons.errors.failed')
    expect(addonError(new ApiError(404, 'x', 'upload_expired'), 64).key).toBe('settings.addons.errors.expired')
  })
})

describe('mib', () => {
  it('MiB 정수로 내린다', () => {
    expect(mib(64 * 1024 * 1024)).toBe(64)
    expect(mib(64 * 1024 * 1024 - 1)).toBe(63)
  })
})
