// 패스키로 로그인의 브라우저 항목을 localStorage(onmaru.passkeyLogin)에 읽고 쓴다. 형식 검사는 passkeyLogin.ts가 한다.
// 컴포넌트, 흐름 모듈(passkeyLoginFlow.ts), 실제 인증 저장소의 권유 판단(auth/store.ts의 useAuthStore)에서만 import한다.
//
// 항목은 로그인 상태가 아니다. 서버 share와 패스키 PRF가 함께 있어야 풀리는 비밀번호 암호문이다. 저장소에 접근할 수
// 없으면(사생활 보호 모드 등) 빈 값으로 동작하고, 저장하지 못했으면 false를 돌려준다.
import type { PasskeyLoginInfo } from '../api/types.gen'
import {
  type PasskeyLoginEntry,
  type PasskeyLoginStore,
  activeEntries,
  addDismissed,
  emptyStore,
  parseStore,
  removeDismissed,
  removeEntry,
  serializeStore,
  syncEntries,
  upsertEntry,
} from './passkeyLogin'

const STORAGE_KEY = 'onmaru.passkeyLogin'

export function loadPasskeyLogins(): PasskeyLoginStore {
  try {
    return parseStore(localStorage.getItem(STORAGE_KEY))
  } catch {
    return emptyStore()
  }
}

// savePasskeyLogins는 저장소 전체를 쓴다. 비었으면 키를 지운다.
export function savePasskeyLogins(store: PasskeyLoginStore): boolean {
  try {
    if (store.entries.length === 0 && store.dismissed.length === 0) localStorage.removeItem(STORAGE_KEY)
    else localStorage.setItem(STORAGE_KEY, serializeStore(store))
    return true
  } catch {
    return false
  }
}

// activePasskeyLogins는 만료되지 않은 항목이다(최근에 켠 것부터).
export function activePasskeyLogins(now = Date.now()): PasskeyLoginEntry[] {
  return activeEntries(loadPasskeyLogins().entries, now)
}

// passkeyLoginFor는 그 사용자의 만료되지 않은 항목이다.
export function passkeyLoginFor(username: string, now = Date.now()): PasskeyLoginEntry | undefined {
  return activePasskeyLogins(now).find((e) => e.username === username)
}

// rememberPasskeyLogin은 항목을 넣는다(같은 사용자의 항목은 바꾼다). 켰으므로 그 사용자를 "다시 묻지 않기"에서 뺀다.
export function rememberPasskeyLogin(entry: PasskeyLoginEntry): boolean {
  const store = loadPasskeyLogins()
  return savePasskeyLogins({
    ...store,
    entries: upsertEntry(store.entries, entry),
    dismissed: removeDismissed(store.dismissed, entry.username),
  })
}

// syncPasskeyLogins는 보안 상태의 등록 목록(passkeyLogins)에 맞춰 그 사용자의 항목을 지우거나 만료 시각을 맞춘다
// (passkeyLogin.ts의 syncEntries). 바꿀 것이 없으면 쓰지 않는다.
export function syncPasskeyLogins(username: string, logins: readonly PasskeyLoginInfo[]): boolean {
  const store = loadPasskeyLogins()
  const entries = syncEntries(store.entries, username, logins)
  if (!entries) return true
  return savePasskeyLogins({ ...store, entries })
}

// forgetPasskeyLogin은 사용자 이름 또는 등록 ID가 같은 항목을 지운다.
export function forgetPasskeyLogin(usernameOrEnrollment: string): boolean {
  const store = loadPasskeyLogins()
  const entries = removeEntry(store.entries, usernameOrEnrollment)
  if (entries.length === store.entries.length) return true
  return savePasskeyLogins({ ...store, entries })
}

// dismissPasskeyLoginOffer는 그 사용자에게 로그인 화면이 패스키로 로그인을 다시 권하지 않게 한다.
export function dismissPasskeyLoginOffer(username: string): boolean {
  const store = loadPasskeyLogins()
  return savePasskeyLogins({ ...store, dismissed: addDismissed(store.dismissed, username) })
}

export function isPasskeyLoginOfferDismissed(username: string): boolean {
  return loadPasskeyLogins().dismissed.includes(username)
}
