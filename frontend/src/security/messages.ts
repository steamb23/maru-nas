// 보안 강화 모드의 화면 계산(DOM 없음): 실패 문구 키, 보증 수준 이름, 확인 수단 고르기, QR 경로.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../api/client'
import type { SecurityPolicy, SecurityStatus } from '../api/types.gen'

export type Factor = SecurityStatus['methods'][number]
export type Assurance = SecurityStatus['assurance']
export type Level = NonNullable<SecurityPolicy['terminal']>
// 보안 강화 정책의 대상
export type SecurityTarget = keyof SecurityPolicy

export interface SecurityError {
  key: ParseKeys
  // rate_limited일 때 다시 시도할 수 있기까지 남은 초
  retryAfter?: number
}

// securityError는 확인·관리 요청의 실패를 문구 키로 바꾼다. method는 시도한 수단이다(틀렸을 때 문구가 다르다).
// both는 비밀번호와 2차 수단을 함께 보낸 관리 확인이다(백엔드는 어느 쪽이 틀렸는지 알리지 않는다).
export function securityError(err: unknown, method?: Factor | 'password' | 'both'): SecurityError {
  // 패스키 창의 결과(DOMException). 취소·시간 초과는 NotAllowedError로 온다.
  if (typeof DOMException !== 'undefined' && err instanceof DOMException) {
    switch (err.name) {
      case 'NotAllowedError':
      case 'AbortError':
        return { key: 'security.errors.passkeyCanceled' }
      case 'InvalidStateError':
        return { key: 'security.errors.passkeyExists' }
      case 'SecurityError':
        return { key: 'security.errors.passkeyDomain' }
      default:
        return { key: 'security.errors.passkeyFailed' }
    }
  }
  if (!(err instanceof ApiError)) return { key: 'security.errors.failed' }
  switch (err.code) {
    case 'factor_failed':
      if (method === 'password') return { key: 'security.errors.wrongPassword' }
      if (method === 'both') return { key: 'security.errors.wrongEither' }
      if (method === 'passkey') return { key: 'security.errors.passkeyFailed' }
      return { key: 'security.errors.wrongCode' }
    case 'rate_limited':
      return { key: 'security.errors.rateLimited', retryAfter: err.retryAfterSeconds ?? 0 }
    case 'passkey_unavailable':
      return { key: 'security.errors.passkeyUnavailable' }
    case 'security_unavailable':
      return { key: 'security.errors.unavailable' }
    case 'manage_required':
      return { key: 'security.errors.manageExpired' }
    case 'omv_unavailable':
      return { key: 'security.errors.omvUnavailable' }
    default:
      return { key: 'security.errors.failed' }
  }
}

export const ASSURANCE_KEYS: Record<Assurance, ParseKeys> = {
  login: 'security.assurance.login',
  password: 'security.assurance.password',
  twoFactor: 'security.assurance.twoFactor',
}

export const LEVEL_KEYS: Record<Level, ParseKeys> = {
  off: 'security.levels.off',
  password: 'security.levels.password',
  twoFactor: 'security.levels.twoFactor',
}

const RANK: Record<Assurance | Level, number> = { off: 0, login: 0, password: 1, twoFactor: 2 }

// satisfies는 보증 수준 a가 정책 단계 level을 만족하는지다(백엔드 server.satisfies와 같다).
export function satisfies(a: Assurance, level: Level): boolean {
  return RANK[a] >= RANK[level]
}

// firstFactor는 처음 보여 줄 확인 수단이다. 패스키(이 브라우저가 쓸 수 있을 때), 인증 앱, 복구 코드 순이다.
export function firstFactor(methods: readonly Factor[], passkeys: boolean): Factor | null {
  if (passkeys && methods.includes('passkey')) return 'passkey'
  if (methods.includes('totp')) return 'totp'
  if (methods.includes('recovery')) return 'recovery'
  return null
}

// usableFactors는 이 브라우저에서 쓸 수 있는 수단이다(패스키를 쓸 수 없는 브라우저에서는 뺀다).
export function usableFactors(methods: readonly Factor[], passkeys: boolean): Factor[] {
  return methods.filter((m) => m !== 'passkey' || passkeys)
}

// qrPath는 QR 모듈(true가 검은 칸)을 SVG path 하나로 바꾼다. 한 칸이 1이다.
export function qrPath(data: readonly (readonly boolean[])[]): string {
  let d = ''
  data.forEach((row, y) => {
    let x = 0
    while (x < row.length) {
      if (!row[x]) {
        x++
        continue
      }
      // 같은 줄의 이어진 검은 칸은 사각형 하나로 그린다.
      let w = 1
      while (x + w < row.length && row[x + w]) w++
      d += `M${x} ${y}h${w}v1h-${w}z`
      x += w
    }
  })
  return d
}

// groupSecret은 TOTP 비밀 키를 4자씩 띄어 읽기 쉽게 한다(인증 앱은 공백을 무시한다).
export function groupSecret(secret: string): string {
  return secret.replace(/(.{4})(?=.)/g, '$1 ')
}
