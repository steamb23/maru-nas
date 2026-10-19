// 패스키로 로그인의 순수 부분(DOM·localStorage 없음): 브라우저 항목 형식, 저장 형식 검사, 분할 키 암호.
// 설계는 .claude/rules/security.md의 "패스키로 로그인". 키 = HKDF-SHA256(PRF 결과 ‖ share, salt = 등록 ID)이고,
// 암호문은 브라우저 항목에만, share는 서버 기록에만 있다. 저장은 passkeyLoginStorage.ts, 흐름은 passkeyLoginFlow.ts.
import { ApiError, type ErrorCode } from '../api/client'
import type { SecurityPolicy } from '../api/types.gen'
import { b64urlDecode, b64urlEncode } from './webauthn'

// PASSKEY_LOGIN_DAYS는 관리자가 고를 수 있는 등록 만료 기간(일)이다(SecurityPolicy.passkeyLoginDays).
export const PASSKEY_LOGIN_DAYS = [7, 30, 90, 180, 365] as const satisfies readonly NonNullable<SecurityPolicy['passkeyLoginDays']>[]

// PRF_SALT_BYTES는 등록마다 만드는 PRF 입력의 길이다.
export const PRF_SALT_BYTES = 32
// MAX_PASSWORD_BYTES는 암호화할 비밀번호(UTF-8)의 상한이다.
export const MAX_PASSWORD_BYTES = 1024
// MAX_ENTRIES는 브라우저 하나에 두는 항목(사용자) 수다.
export const MAX_ENTRIES = 5
// MAX_DISMISSED는 "다시 묻지 않기"를 기억할 사용자 수다.
const MAX_DISMISSED = 20

// SHARE_BYTES는 서버 키 조각의 길이다(백엔드 security.ShareBytes).
const SHARE_BYTES = 32
// PRF 결과는 HMAC-SHA-256 출력이라 32바이트다. 더 짧으면 받지 않는다.
const MIN_PRF_BYTES = 32
const IV_BYTES = 12
const HKDF_INFO = new TextEncoder().encode('onmaru passkey login v1')
const AAD_TAG = 'onmaru-passkey-login-v1'

// PasskeyLoginEntry는 이 브라우저의 등록 하나다. 이진 값은 base64url 문자열이고 expiresAt은 ms다.
export interface PasskeyLoginEntry {
  v: 1
  username: string
  // enrollment는 서버의 등록 ID다(HKDF salt, 로그인 단계를 시작하는 값).
  enrollment: string
  // credential은 등록에 쓴 패스키의 자격 증명 ID다.
  credential: string
  rpId: string
  // salt는 PRF 입력이다.
  salt: string
  iv: string
  data: string
  expiresAt: number
}

// PasskeyLoginStore는 localStorage에 두는 형식이다. dismissed는 로그인 화면의 권유에 "다시 묻지 않기"를 고른
// 사용자 이름이다.
export interface PasskeyLoginStore {
  v: 1
  entries: PasskeyLoginEntry[]
  dismissed: string[]
}

export function emptyStore(): PasskeyLoginStore {
  return { v: 1, entries: [], dismissed: [] }
}

const B64URL = /^[A-Za-z0-9_-]+$/

// b64urlBytes는 base64url 문자열의 바이트 수다. 형식이 틀리면 -1이다.
function b64urlBytes(s: unknown): number {
  if (typeof s !== 'string' || !B64URL.test(s)) return -1
  try {
    return b64urlDecode(s).byteLength
  } catch {
    return -1
  }
}

const nonEmpty = (s: unknown): s is string => typeof s === 'string' && s !== ''

function isEntry(v: unknown): v is PasskeyLoginEntry {
  if (typeof v !== 'object' || v === null) return false
  const e = v as Record<string, unknown>
  return (
    e.v === 1 &&
    nonEmpty(e.username) &&
    nonEmpty(e.rpId) &&
    b64urlBytes(e.enrollment) > 0 &&
    b64urlBytes(e.credential) > 0 &&
    b64urlBytes(e.salt) === PRF_SALT_BYTES &&
    b64urlBytes(e.iv) === IV_BYTES &&
    b64urlBytes(e.data) > 0 &&
    typeof e.expiresAt === 'number' &&
    Number.isFinite(e.expiresAt)
  )
}

// entryOf는 쓰는 필드만 새 객체로 옮긴다(손으로 고친 저장소의 다른 필드를 남기지 않는다).
function entryOf(e: PasskeyLoginEntry): PasskeyLoginEntry {
  const { username, enrollment, credential, rpId, salt, iv, data, expiresAt } = e
  return { v: 1, username, enrollment, credential, rpId, salt, iv, data, expiresAt }
}

// parseStore는 저장한 문자열을 읽는다. 형식이 틀린 항목은 버리고, 전체가 틀리면 빈 값이다. 예외를 던지지 않는다.
export function parseStore(raw: string | null): PasskeyLoginStore {
  if (raw === null) return emptyStore()
  let v: unknown
  try {
    v = JSON.parse(raw)
  } catch {
    return emptyStore()
  }
  if (typeof v !== 'object' || v === null) return emptyStore()
  const o = v as Record<string, unknown>
  if (o.v !== 1) return emptyStore()
  let entries: PasskeyLoginEntry[] = []
  if (Array.isArray(o.entries)) {
    // 같은 사용자 이름이 여럿이면 앞의 것(최근)만 남긴다.
    for (const e of o.entries.filter(isEntry).map(entryOf).reverse()) entries = upsertEntry(entries, e)
  }
  const dismissed = Array.isArray(o.dismissed) ? [...new Set(o.dismissed.filter(nonEmpty))].slice(0, MAX_DISMISSED) : []
  return { v: 1, entries, dismissed }
}

export function serializeStore(store: PasskeyLoginStore): string {
  return JSON.stringify({ v: 1, entries: store.entries.map(entryOf), dismissed: store.dismissed })
}

// upsertEntry는 항목을 맨 앞에 넣는다. 같은 사용자 이름(또는 같은 등록)의 항목은 바꾸고, MAX_ENTRIES개를 넘으면
// 오래된 것을 버린다.
export function upsertEntry(entries: readonly PasskeyLoginEntry[], entry: PasskeyLoginEntry): PasskeyLoginEntry[] {
  const rest = entries.filter((e) => e.username !== entry.username && e.enrollment !== entry.enrollment)
  return [entry, ...rest].slice(0, MAX_ENTRIES)
}

// removeEntry는 사용자 이름 또는 등록 ID가 같은 항목을 뺀다.
export function removeEntry(entries: readonly PasskeyLoginEntry[], usernameOrEnrollment: string): PasskeyLoginEntry[] {
  return entries.filter((e) => e.username !== usernameOrEnrollment && e.enrollment !== usernameOrEnrollment)
}

// activeEntries는 만료되지 않은 항목이다(now는 ms). 서버가 기간을 줄였으면 서버가 passkey_login_expired로 알린다.
export function activeEntries(entries: readonly PasskeyLoginEntry[], now: number): PasskeyLoginEntry[] {
  return entries.filter((e) => e.expiresAt > now)
}

// addDismissed는 "다시 묻지 않기"에 사용자 이름을 맨 앞에 넣는다(중복 없음, MAX_DISMISSED개까지).
export function addDismissed(dismissed: readonly string[], username: string): string[] {
  return [username, ...dismissed.filter((u) => u !== username)].slice(0, MAX_DISMISSED)
}

const GONE_CODES: readonly ErrorCode[] = ['passkey_login_unknown', 'passkey_login_expired', 'passkey_login_stale']

// isPasskeyLoginGone은 서버에 그 등록이 없어졌다는 오류인지다. 그러면 브라우저 항목도 지운다.
export function isPasskeyLoginGone(err: unknown): boolean {
  return err instanceof ApiError && err.code !== undefined && GONE_CODES.includes(err.code)
}

// passwordBytes는 암호화할 비밀번호의 UTF-8 길이다. 등록 요청을 보내기 전에 상한을 확인하는 데 쓴다.
export function passwordBytes(password: string): number {
  return new TextEncoder().encode(password).byteLength
}

export function newPrfSalt(): Uint8Array<ArrayBuffer> {
  return crypto.getRandomValues(new Uint8Array(PRF_SALT_BYTES))
}

// deriveKey는 PRF 결과와 share로 AES-GCM-256 키를 만든다(추출 불가, 저장하지 않는다). salt는 등록 ID다.
export async function deriveKey(prf: ArrayBuffer, share: Uint8Array, enrollment: string): Promise<CryptoKey> {
  if (prf.byteLength < MIN_PRF_BYTES) throw new Error('passkey login: short PRF result')
  if (share.byteLength !== SHARE_BYTES) throw new Error('passkey login: bad share')
  const salt = b64urlDecode(enrollment)
  if (salt.byteLength === 0) throw new Error('passkey login: bad enrollment')
  const ikm = new Uint8Array(prf.byteLength + share.byteLength)
  ikm.set(new Uint8Array(prf), 0)
  ikm.set(share, prf.byteLength)
  try {
    const base = await crypto.subtle.importKey('raw', ikm, 'HKDF', false, ['deriveKey'])
    return await crypto.subtle.deriveKey(
      { name: 'HKDF', hash: 'SHA-256', salt, info: HKDF_INFO },
      base,
      { name: 'AES-GCM', length: 256 },
      false,
      ['encrypt', 'decrypt'],
    )
  } finally {
    ikm.fill(0)
  }
}

// entryAad는 암호문을 묶는 추가 인증 데이터다. 다른 RP·계정·패스키·등록의 키로는 풀리지 않는다.
export function entryAad(rpId: string, username: string, credential: string, enrollment: string): Uint8Array<ArrayBuffer> {
  return new TextEncoder().encode(JSON.stringify([AAD_TAG, rpId, username, credential, enrollment]))
}

// sealPassword는 비밀번호를 암호화한다. iv(12바이트 무작위)와 data는 base64url이다. 비어 있거나 상한을 넘으면
// 예외를 던진다.
export async function sealPassword(
  key: CryptoKey,
  aad: Uint8Array<ArrayBuffer>,
  password: string,
): Promise<{ iv: string; data: string }> {
  const plain = new TextEncoder().encode(password)
  try {
    if (plain.byteLength === 0) throw new Error('passkey login: empty password')
    if (plain.byteLength > MAX_PASSWORD_BYTES) throw new Error('passkey login: password too long')
    const iv = crypto.getRandomValues(new Uint8Array(IV_BYTES))
    const data = await crypto.subtle.encrypt({ name: 'AES-GCM', iv, additionalData: aad }, key, plain)
    return { iv: b64urlEncode(iv), data: b64urlEncode(data) }
  } finally {
    plain.fill(0)
  }
}

// openPassword는 sealPassword의 결과를 푼다. 키·AAD가 다르거나 암호문이 바뀌었으면(인증 실패) 예외를 던진다.
export async function openPassword(key: CryptoKey, aad: Uint8Array<ArrayBuffer>, iv: string, data: string): Promise<string> {
  const plain = new Uint8Array(
    await crypto.subtle.decrypt({ name: 'AES-GCM', iv: b64urlDecode(iv), additionalData: aad }, key, b64urlDecode(data)),
  )
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(plain)
  } finally {
    plain.fill(0)
  }
}
