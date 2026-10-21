// 보안 강화 모드의 화면 계산(DOM 없음): 실패 문구 키, 보증 수준 이름, 확인 수단 고르기, QR 경로.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../api/client'
import type { SecurityPolicy, SecurityStatus } from '../api/types.gen'
import { PasskeyLoginStorageError, PasswordTooLongError, PrfUnsupportedError } from './passkeyLoginFlow'

export type Factor = SecurityStatus['methods'][number]
export type Assurance = SecurityStatus['assurance']
export type Level = NonNullable<SecurityPolicy['terminal']>
// 보안 강화 정책의 대상(단계를 고르는 기능). 패스키로 로그인의 만료 기간은 대상이 아니다.
export type SecurityTarget = Exclude<keyof SecurityPolicy, 'passkeyLoginDays' | 'filesScope'>

export interface SecurityError {
  key: ParseKeys
  // rate_limited일 때 다시 시도할 수 있기까지 남은 초
  retryAfter?: number
}

// securityError는 확인·관리 요청의 실패를 문구 키로 바꾼다. method는 시도한 수단이다(틀렸을 때 문구가 다르다).
// both는 비밀번호와 2차 수단을 함께 보낸 관리 확인이다(백엔드는 어느 쪽이 틀렸는지 알리지 않는다).
export function securityError(err: unknown, method?: Choice | 'password' | 'both'): SecurityError {
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
    case 'passkey_uv_unsupported':
      return { key: 'security.errors.passkeyNoUV' }
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

// passkeyLoginEnableError는 패스키로 로그인 켜기(passkeyLoginFlow.ts의 enablePasskeyLogin)의 실패를 문구 키로 바꾼다.
// 비밀번호와 패스키를 함께 보내므로 틀린 값은 관리 확인과 같은 문구다(method 'both'). 로그인 직후처럼 방금 맞힌
// 비밀번호를 쓰면 'passkey'로 패스키 실패 문구를 고른다.
export function passkeyLoginEnableError(err: unknown, method: 'both' | 'passkey' = 'both'): SecurityError {
  if (err instanceof PrfUnsupportedError) return { key: 'security.passkeyLogin.prfUnsupported' }
  if (err instanceof PasswordTooLongError) return { key: 'security.passkeyLogin.passwordTooLong' }
  if (err instanceof PasskeyLoginStorageError) return { key: 'security.passkeyLogin.storageFailed' }
  return securityError(err, method)
}

// passkeyNotSaved는 새 패스키의 등록·시험 확인 요청이 거절돼 백엔드가 그 패스키를 저장하지 않았다고 확실히 알 수
// 있는지다(400, 403, 429). 그때만 저장소에서 지우게 한다. 409(이미 등록됨일 수 있다)·연결 실패는 모른다.
export function passkeyNotSaved(err: unknown): boolean {
  return err instanceof ApiError && (err.status === 400 || err.status === 403 || err.status === 429)
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

// Choice는 확인 폼에서 고를 수 있는 확인 방법이다. 등록한 2차 수단과, 관리 확인에서만 받는 기기 추가 코드다.
export type Choice = Factor | 'deviceCode'

// ManagePurpose는 관리 확인을 여는 목적이다. 패스키 등록은 고를 수 있는 방법과 순서가 다르다.
export type ManagePurpose = 'addPasskey'

const DEFAULT_ORDER: readonly Choice[] = ['passkey', 'totp', 'recovery']
// 패스키 등록은 새 기기에서 하는 경우가 많고, 그 기기에는 등록한 패스키가 없기 쉽다.
const ADD_PASSKEY_ORDER: readonly Choice[] = ['totp', 'deviceCode', 'passkey', 'recovery']

// factorChoices는 확인 폼에서 고를 수 있는 방법을 보여 줄 순서로 돌려준다(첫째를 처음 보인다). 패스키는 이
// 브라우저가 쓸 수 있을 때만 넣는다. 보통은 패스키, 인증 앱, 복구 코드 순이고, 패스키 등록의 관리 확인은 인증 앱,
// 기기 추가 코드, 패스키, 복구 코드 순이다. 등록한 수단이 없으면 빈 목록이다.
export function factorChoices(methods: readonly Factor[], passkeys: boolean, purpose?: ManagePurpose): Choice[] {
  if (methods.length === 0) return []
  const order = purpose === 'addPasskey' ? ADD_PASSKEY_ORDER : DEFAULT_ORDER
  return order.filter((c) => c === 'deviceCode' || (methods.includes(c) && (c !== 'passkey' || passkeys)))
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
