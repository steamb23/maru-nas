// 패스키로 로그인의 흐름(브라우저 API를 쓰므로 테스트하지 않는다. 계산은 passkeyLogin.ts에 둔다).
// 켜기는 보안 페이지·로그인 직후 권유가, 풀기는 로그인 화면(인증 저장소)이 부른다. 비밀번호·share·PRF 결과는
// 메모리에서만 다루고 기록하지 않는다.
import {
  deletePasskeyLogin,
  discardPasskeyLogin,
  enrollPasskeyLogin,
  passkeyChallenge,
  passkeyLoginOptions,
  passkeyLoginUnlock,
} from '../api/client'
import type { PasskeyAssertion, SecurityStatus } from '../api/types.gen'
import {
  MAX_PASSWORD_BYTES,
  type PasskeyLoginEntry,
  deriveKey,
  entryAad,
  isPasskeyLoginGone,
  newPrfSalt,
  openPassword,
  passwordBytes,
  sealPassword,
} from './passkeyLogin'
import { forgetPasskeyLogin, loadPasskeyLogins, rememberPasskeyLogin } from './passkeyLoginStorage'
import { assertWithPrf, b64urlDecode, b64urlEncode } from './webauthn'

// PrfUnsupportedError는 패스키(인증기·브라우저)가 PRF 결과를 주지 않았다는 뜻이다. 패스키로 로그인을 쓸 수 없다.
// 켜기(enablePasskeyLogin)에서는 받은 확인 값(assertion)을 담는다. 보내지 않은 세션 challenge의 확인이라, 로그인
// 직후 단계가 같은 확인으로 2차 인증만 마칠 수 있다(verifySecurity).
export class PrfUnsupportedError extends Error {
  readonly assertion?: PasskeyAssertion

  constructor(assertion?: PasskeyAssertion) {
    super('passkey: PRF is not supported')
    this.name = 'PrfUnsupportedError'
    this.assertion = assertion
  }
}

// PasskeyLoginBrokenError는 서버 확인은 통과했지만 브라우저 항목을 풀지 못했다는 뜻이다. 서버 등록과 항목을 지웠다.
export class PasskeyLoginBrokenError extends Error {
  constructor() {
    super('passkey login: cannot decrypt the saved password')
    this.name = 'PasskeyLoginBrokenError'
  }
}

// PasswordTooLongError는 비밀번호가 MAX_PASSWORD_BYTES(UTF-8)를 넘어 암호화할 수 없다는 뜻이다. 요청을 보내기 전에 던진다.
export class PasswordTooLongError extends Error {
  constructor() {
    super('passkey login: password too long')
    this.name = 'PasswordTooLongError'
  }
}

// PasskeyLoginStorageError는 브라우저 저장소에 항목을 쓰지 못했다는 뜻이다(사생활 보호 모드 등). 서버 등록은 지웠다.
export class PasskeyLoginStorageError extends Error {
  constructor() {
    super('passkey login: cannot write browser storage')
    this.name = 'PasskeyLoginStorageError'
  }
}

export interface EnablePasskeyLoginOptions {
  // username은 로그인한 세션의 사용자 이름이다(비밀번호는 서버가 이 사용자로 확인한다).
  username: string
  password: string
  // replace는 이 브라우저가 전에 쓰던 등록 ID다. 없으면 그 사용자의 저장된 항목에서 찾는다.
  replace?: string
}

// enablePasskeyLogin은 이 브라우저의 패스키로 로그인을 켠다. PRF를 함께 요청한 확인 한 번으로 서버 확인과 키 유도를
// 함께 하고, 암호문을 브라우저에 둔다. 등록 뒤 단계가 실패하면 서버 등록을 지우고(실패해도 넘어간다) 오류를 다시 던진다.
export async function enablePasskeyLogin({
  username,
  password,
  replace,
}: EnablePasskeyLoginOptions): Promise<{ entry: PasskeyLoginEntry; status: SecurityStatus }> {
  const size = passwordBytes(password)
  if (size === 0) throw new Error('passkey login: empty password')
  if (size > MAX_PASSWORD_BYTES) throw new PasswordTooLongError()
  const salt = newPrfSalt()
  const opts = await passkeyChallenge('assert')
  const { assertion, prf } = await assertWithPrf(opts, salt)
  // PRF 결과가 없으면 등록 요청을 보내지 않는다(쓰지 않은 확인 값은 오류에 담아 넘긴다).
  if (!prf) throw new PrfUnsupportedError(assertion)
  const previous = replace ?? loadPasskeyLogins().entries.find((e) => e.username === username)?.enrollment
  const res = await enrollPasskeyLogin({ password, passkey: assertion, replace: previous })
  try {
    const key = await deriveKey(prf, b64urlDecode(res.share), res.id)
    const sealed = await sealPassword(key, entryAad(opts.rpId, username, assertion.id, res.id), password)
    const entry: PasskeyLoginEntry = {
      v: 1,
      username,
      enrollment: res.id,
      credential: assertion.id,
      rpId: opts.rpId,
      salt: b64urlEncode(salt),
      iv: sealed.iv,
      data: sealed.data,
      expiresAt: Date.parse(res.expiresAt),
    }
    if (!rememberPasskeyLogin(entry)) throw new PasskeyLoginStorageError()
    return { entry, status: res.status }
  } catch (err) {
    await deletePasskeyLogin(res.id).catch(() => undefined)
    throw err
  }
}

// forgetOnError는 서버에 등록이 없어졌다는 오류(passkey_login_unknown·expired·stale)면 브라우저 항목을 지운다.
// 지웠으면 true다. unlockPasskeyLogin은 스스로 부르고, finishPasskeyLogin의 오류에는 인증 저장소가 부른다.
export function forgetOnError(err: unknown, entry: PasskeyLoginEntry): boolean {
  if (!isPasskeyLoginGone(err)) return false
  forgetPasskeyLogin(entry.enrollment)
  return true
}

// unlockPasskeyLogin은 항목의 패스키로 확인해 share를 받고 비밀번호를 풀어 돌려준다. 로그인을 마치는
// finishPasskeyLogin은 부르지 않는다. 사용자가 취소하면 DOMException(NotAllowedError)을 그대로 던진다. 패스키가
// PRF 결과를 주지 않으면 항목을 지우고 PrfUnsupportedError를 던진다.
export async function unlockPasskeyLogin(entry: PasskeyLoginEntry): Promise<string> {
  let share: string
  let prf: ArrayBuffer | null
  try {
    const opts = await passkeyLoginOptions(entry.username, entry.enrollment)
    const res = await assertWithPrf(opts, b64urlDecode(entry.salt))
    prf = res.prf
    if (!prf) {
      // 항목은 이 자격 증명 하나만 허용하므로 다시 해도 풀 수 없다. 브라우저 항목만 지운다. 서버 등록은 share를 받기
      // 전이라 지울 수 없고(로그인 단계 쿠키가 없다), 만료나 보안 페이지의 해제로 정리된다. 서버의 확인 상태는 그대로 만료된다.
      forgetPasskeyLogin(entry.enrollment)
      throw new PrfUnsupportedError()
    }
    share = (await passkeyLoginUnlock(res.assertion)).share
  } catch (err) {
    forgetOnError(err, entry)
    throw err
  }
  try {
    const key = await deriveKey(prf, b64urlDecode(share), entry.enrollment)
    const aad = entryAad(entry.rpId, entry.username, entry.credential, entry.enrollment)
    return await openPassword(key, aad, entry.iv, entry.data)
  } catch {
    // share를 받은 뒤에만 서버 등록을 지울 수 있다(로그인 단계 쿠키).
    await discardPasskeyLogin().catch(() => undefined)
    forgetPasskeyLogin(entry.enrollment)
    throw new PasskeyLoginBrokenError()
  }
}
