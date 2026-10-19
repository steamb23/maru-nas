import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import { factorChoices, groupSecret, passkeyLoginEnableError, passkeyNotSaved, qrPath, satisfies, securityError } from './messages'
import { PasskeyLoginStorageError, PasswordTooLongError, PrfUnsupportedError } from './passkeyLoginFlow'

describe('securityError', () => {
  it('틀린 값은 시도한 수단에 맞는 문구다', () => {
    const err = new ApiError(400, 'x', 'factor_failed')
    expect(securityError(err, 'password').key).toBe('security.errors.wrongPassword')
    expect(securityError(err, 'totp').key).toBe('security.errors.wrongCode')
    expect(securityError(err, 'recovery').key).toBe('security.errors.wrongCode')
    expect(securityError(err, 'passkey').key).toBe('security.errors.passkeyFailed')
  })

  it('잠금은 남은 시간을 함께 준다', () => {
    expect(securityError(new ApiError(429, 'x', 'rate_limited', 30))).toEqual({
      key: 'security.errors.rateLimited',
      retryAfter: 30,
    })
  })

  it('패스키 창의 결과를 구분한다', () => {
    expect(securityError(new DOMException('x', 'NotAllowedError')).key).toBe('security.errors.passkeyCanceled')
    expect(securityError(new DOMException('x', 'InvalidStateError')).key).toBe('security.errors.passkeyExists')
    expect(securityError(new DOMException('x', 'SecurityError')).key).toBe('security.errors.passkeyDomain')
  })

  it('사용자 확인을 알리지 않는 패스키 저장소를 안내한다', () => {
    expect(securityError(new ApiError(400, 'x', 'passkey_uv_unsupported'), 'passkey').key).toBe('security.errors.passkeyNoUV')
  })

  it('모르는 오류는 일반 실패다', () => {
    expect(securityError(new Error('boom')).key).toBe('security.errors.failed')
    expect(securityError(new ApiError(503, 'x', 'security_unavailable')).key).toBe('security.errors.unavailable')
  })
})

describe('passkeyNotSaved', () => {
  it('거절된 요청만 저장하지 않았다고 본다', () => {
    expect(passkeyNotSaved(new ApiError(400, 'x', 'passkey_uv_unsupported'))).toBe(true)
    expect(passkeyNotSaved(new ApiError(400, 'x', 'factor_failed'))).toBe(true)
    expect(passkeyNotSaved(new ApiError(403, 'x', 'manage_required'))).toBe(true)
    expect(passkeyNotSaved(new ApiError(429, 'x', 'rate_limited'))).toBe(true)
    // 이미 등록한 패스키일 수 있다.
    expect(passkeyNotSaved(new ApiError(409, 'x', 'bad_request'))).toBe(false)
    // 연결이 끊겼으면 저장됐는지 모른다.
    expect(passkeyNotSaved(new ApiError(502, 'x'))).toBe(false)
    expect(passkeyNotSaved(new TypeError('network'))).toBe(false)
    expect(passkeyNotSaved(new DOMException('x', 'NotAllowedError'))).toBe(false)
  })
})

describe('satisfies', () => {
  it('높은 보증 수준은 낮은 단계도 만족한다', () => {
    expect(satisfies('login', 'off')).toBe(true)
    expect(satisfies('login', 'password')).toBe(false)
    expect(satisfies('twoFactor', 'password')).toBe(true)
    expect(satisfies('password', 'twoFactor')).toBe(false)
  })
})

describe('factorChoices', () => {
  it('쓸 수 있는 패스키를 먼저 고른다', () => {
    expect(factorChoices(['passkey', 'totp', 'recovery'], true)).toEqual(['passkey', 'totp', 'recovery'])
    expect(factorChoices(['passkey', 'totp', 'recovery'], false)).toEqual(['totp', 'recovery'])
    expect(factorChoices(['passkey', 'recovery'], false)).toEqual(['recovery'])
    expect(factorChoices(['passkey'], false)).toEqual([])
    expect(factorChoices([], true)).toEqual([])
  })

  it('패스키 등록은 인증 앱, 기기 추가 코드, 패스키, 복구 코드 순이다', () => {
    expect(factorChoices(['passkey', 'totp', 'recovery'], true, 'addPasskey')).toEqual(['totp', 'deviceCode', 'passkey', 'recovery'])
    expect(factorChoices(['passkey', 'recovery'], true, 'addPasskey')).toEqual(['deviceCode', 'passkey', 'recovery'])
    expect(factorChoices(['totp', 'recovery'], true, 'addPasskey')).toEqual(['totp', 'deviceCode', 'recovery'])
    expect(factorChoices(['passkey', 'recovery'], false, 'addPasskey')).toEqual(['deviceCode', 'recovery'])
    // 수단이 없으면 비밀번호만 받으므로 코드도 묻지 않는다.
    expect(factorChoices([], true, 'addPasskey')).toEqual([])
  })
})

describe('qrPath', () => {
  it('이어진 검은 칸을 한 사각형으로 그린다', () => {
    expect(
      qrPath([
        [true, true, false],
        [false, true, true],
      ]),
    ).toBe('M0 0h2v1h-2zM1 1h2v1h-2z')
  })
})

describe('groupSecret', () => {
  it('4자씩 띄운다', () => {
    expect(groupSecret('ABCDEFGHIJ')).toBe('ABCD EFGH IJ')
    expect(groupSecret('ABCD')).toBe('ABCD')
  })
})

describe('passkeyLoginEnableError', () => {
  it('켜기 단계의 실패를 구분한다', () => {
    expect(passkeyLoginEnableError(new PrfUnsupportedError()).key).toBe('security.passkeyLogin.prfUnsupported')
    expect(passkeyLoginEnableError(new PasswordTooLongError()).key).toBe('security.passkeyLogin.passwordTooLong')
    expect(passkeyLoginEnableError(new PasskeyLoginStorageError()).key).toBe('security.passkeyLogin.storageFailed')
  })

  it('그 밖에는 확인 실패 문구다', () => {
    const wrong = new ApiError(400, 'x', 'factor_failed')
    expect(passkeyLoginEnableError(wrong).key).toBe('security.errors.wrongEither')
    expect(passkeyLoginEnableError(wrong, 'passkey').key).toBe('security.errors.passkeyFailed')
    expect(passkeyLoginEnableError(new DOMException('x', 'NotAllowedError')).key).toBe('security.errors.passkeyCanceled')
    expect(passkeyLoginEnableError(new ApiError(429, 'x', 'rate_limited', 9))).toEqual({ key: 'security.errors.rateLimited', retryAfter: 9 })
  })
})
