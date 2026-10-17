// 패스키(WebAuthn) 브라우저 쪽. 백엔드가 준 옵션(base64url)을 navigator.credentials에 넘기고, 결과를 백엔드가
// 받는 JSON(base64url)으로 바꾼다. 확인은 백엔드가 한다(internal/security). 변환 함수는 DOM 없이 테스트한다.
import { passkeyChallenge } from '../api/client'
import type { PasskeyAssertion, PasskeyOptions, PasskeyRegistration } from '../api/types.gen'

// passkeysSupported는 이 브라우저·접속에서 패스키를 쓸 수 있는지다. 평문 HTTP(보안 컨텍스트가 아님)에서는 없다.
export function passkeysSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    window.isSecureContext &&
    typeof PublicKeyCredential !== 'undefined' &&
    typeof navigator.credentials?.get === 'function'
  )
}

// b64urlEncode는 바이트를 base64url(패딩 없음)로 쓴다.
export function b64urlEncode(data: ArrayBuffer | Uint8Array): string {
  const bytes = data instanceof Uint8Array ? data : new Uint8Array(data)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

// b64urlDecode는 base64url(패딩 있어도 됨)을 바이트로 푼다. 형식이 틀리면 예외를 던진다.
export function b64urlDecode(s: string): Uint8Array<ArrayBuffer> {
  const b64 = s.replace(/-/g, '+').replace(/_/g, '/').replace(/=+$/, '')
  const bin = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4))
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

const TRANSPORTS: readonly AuthenticatorTransport[] = ['usb', 'nfc', 'ble', 'hybrid', 'internal']

function descriptors(opts: PasskeyOptions): PublicKeyCredentialDescriptor[] {
  return opts.credentials.map((c) => ({
    type: 'public-key',
    id: b64urlDecode(c.id),
    transports: c.transports?.filter((t): t is AuthenticatorTransport => TRANSPORTS.includes(t as AuthenticatorTransport)),
  }))
}

// requestOptions는 확인 옵션이다. 사용자 확인(UV)은 필수다(백엔드도 UV 플래그를 확인한다).
export function requestOptions(opts: PasskeyOptions): PublicKeyCredentialRequestOptions {
  return {
    challenge: b64urlDecode(opts.challenge),
    rpId: opts.rpId,
    timeout: opts.timeoutMs,
    allowCredentials: descriptors(opts),
    userVerification: 'required',
  }
}

// creationOptions는 등록 옵션이다. 증명은 받지 않고(none), 사용자 확인은 필수다. 같은 기기에 다시 등록하지 않게
// 이미 등록한 자격 증명을 알린다.
export function creationOptions(opts: PasskeyOptions): PublicKeyCredentialCreationOptions {
  if (!opts.user || !opts.rpName || !opts.algorithms) throw new Error('passkey: not registration options')
  return {
    challenge: b64urlDecode(opts.challenge),
    rp: { id: opts.rpId, name: opts.rpName },
    user: { id: b64urlDecode(opts.user.id), name: opts.user.name, displayName: opts.user.displayName },
    pubKeyCredParams: opts.algorithms.map((alg) => ({ type: 'public-key', alg })),
    timeout: opts.timeoutMs,
    excludeCredentials: descriptors(opts),
    authenticatorSelection: { residentKey: 'preferred', userVerification: 'required' },
    attestation: 'none',
  }
}

// AssertionLike·AttestationLike는 쓰는 필드만 담은 형태다(테스트에서 가짜를 넣는다).
export interface AssertionLike {
  rawId: ArrayBuffer
  response: {
    clientDataJSON: ArrayBuffer
    authenticatorData: ArrayBuffer
    signature: ArrayBuffer
    userHandle: ArrayBuffer | null
  }
}

export interface AttestationLike {
  rawId: ArrayBuffer
  response: {
    clientDataJSON: ArrayBuffer
    getAuthenticatorData: () => ArrayBuffer
    getPublicKey: () => ArrayBuffer | null
    getPublicKeyAlgorithm: () => number
    getTransports?: () => string[]
  }
}

export function assertionJSON(cred: AssertionLike): PasskeyAssertion {
  const r = cred.response
  return {
    id: b64urlEncode(cred.rawId),
    clientDataJSON: b64urlEncode(r.clientDataJSON),
    authenticatorData: b64urlEncode(r.authenticatorData),
    signature: b64urlEncode(r.signature),
    userHandle: r.userHandle ? b64urlEncode(r.userHandle) : undefined,
  }
}

// registrationJSON은 등록 결과다. 브라우저가 알고리즘을 몰라 공개 키를 주지 못하면 빈 값이고 백엔드가 거절한다.
export function registrationJSON(cred: AttestationLike): PasskeyRegistration {
  const r = cred.response
  const key = r.getPublicKey()
  return {
    id: b64urlEncode(cred.rawId),
    clientDataJSON: b64urlEncode(r.clientDataJSON),
    authenticatorData: b64urlEncode(r.getAuthenticatorData()),
    publicKey: key ? b64urlEncode(key) : '',
    publicKeyAlgorithm: r.getPublicKeyAlgorithm(),
    transports: r.getTransports?.(),
  }
}

// getPasskey는 확인 challenge를 받아 패스키로 서명한다. 사용자가 취소하면 DOMException(NotAllowedError)을 던진다.
export async function getPasskey(): Promise<PasskeyAssertion> {
  const opts = await passkeyChallenge('assert')
  const cred = await navigator.credentials.get({ publicKey: requestOptions(opts) })
  if (!(cred instanceof PublicKeyCredential)) throw new Error('passkey: no credential')
  return assertionJSON(cred as unknown as AssertionLike)
}

// createPasskey는 등록 challenge를 받아 새 패스키를 만든다(관리 창 필요).
export async function createPasskey(): Promise<PasskeyRegistration> {
  const opts = await passkeyChallenge('register')
  const cred = await navigator.credentials.create({ publicKey: creationOptions(opts) })
  if (!(cred instanceof PublicKeyCredential)) throw new Error('passkey: no credential')
  return registrationJSON(cred as unknown as AttestationLike)
}
