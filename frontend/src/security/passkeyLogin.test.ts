import { describe, expect, expectTypeOf, it } from 'vitest'
import { ApiError } from '../api/client'
import type { SecurityPolicy } from '../api/types.gen'
import {
  MAX_ENTRIES,
  MAX_PASSWORD_BYTES,
  PASSKEY_LOGIN_DAYS,
  PRF_SALT_BYTES,
  type PasskeyLoginEntry,
  activeEntries,
  addDismissed,
  deriveKey,
  emptyStore,
  entryAad,
  isPasskeyLoginGone,
  newPrfSalt,
  openPassword,
  parseStore,
  passwordBytes,
  removeEntry,
  sealPassword,
  serializeStore,
  upsertEntry,
} from './passkeyLogin'
import { b64urlEncode } from './webauthn'

// 모두 합성한 값이다.
const random = (n: number) => crypto.getRandomValues(new Uint8Array(n))
const ENROLLMENT = b64urlEncode(random(16))
const OTHER_ENROLLMENT = b64urlEncode(random(16))
const CREDENTIAL = b64urlEncode(random(16))
const RP = 'desk.example.lan'

function keyMaterial() {
  return { prf: random(32).buffer, share: random(32) }
}

describe('PASSKEY_LOGIN_DAYS', () => {
  it('정책의 만료 기간 타입과 같다', () => {
    expectTypeOf<(typeof PASSKEY_LOGIN_DAYS)[number]>().toEqualTypeOf<NonNullable<SecurityPolicy['passkeyLoginDays']>>()
    expect([...PASSKEY_LOGIN_DAYS]).toEqual([7, 30, 90, 180, 365])
  })
})

describe('암호', () => {
  const aad = entryAad(RP, 'alice', CREDENTIAL, ENROLLMENT)

  it('같은 PRF·share·등록·AAD로 풀린다', async () => {
    const { prf, share } = keyMaterial()
    const key = await deriveKey(prf, share, ENROLLMENT)
    expect(key.extractable).toBe(false)
    expect(key.usages.toSorted()).toEqual(['decrypt', 'encrypt'])
    const sealed = await sealPassword(key, aad, 'pässwörd 비밀번호')
    const again = await deriveKey(prf, share, ENROLLMENT)
    await expect(openPassword(again, aad, sealed.iv, sealed.data)).resolves.toBe('pässwörd 비밀번호')
  })

  it('IV는 암호화마다 다르다', async () => {
    const { prf, share } = keyMaterial()
    const key = await deriveKey(prf, share, ENROLLMENT)
    const a = await sealPassword(key, aad, 'pw')
    const b = await sealPassword(key, aad, 'pw')
    expect(a.iv).not.toBe(b.iv)
    expect(a.data).not.toBe(b.data)
  })

  it('share·PRF·등록 ID 중 하나라도 다르면 풀리지 않는다', async () => {
    const { prf, share } = keyMaterial()
    const sealed = await sealPassword(await deriveKey(prf, share, ENROLLMENT), aad, 'pw')
    const otherShare = share.slice()
    otherShare[0] ^= 1
    const otherPrf = new Uint8Array(prf.slice(0))
    otherPrf[31] ^= 1
    for (const key of [
      await deriveKey(prf, otherShare, ENROLLMENT),
      await deriveKey(otherPrf.buffer, share, ENROLLMENT),
      await deriveKey(prf, share, OTHER_ENROLLMENT),
    ]) {
      await expect(openPassword(key, aad, sealed.iv, sealed.data)).rejects.toThrow()
    }
  })

  it('AAD의 RP·사용자·자격 증명·등록 중 하나라도 다르면 풀리지 않는다', async () => {
    const { prf, share } = keyMaterial()
    const key = await deriveKey(prf, share, ENROLLMENT)
    const sealed = await sealPassword(key, aad, 'pw')
    const others = [
      entryAad('other.example.lan', 'alice', CREDENTIAL, ENROLLMENT),
      entryAad(RP, 'bob', CREDENTIAL, ENROLLMENT),
      entryAad(RP, 'alice', b64urlEncode(random(16)), ENROLLMENT),
      entryAad(RP, 'alice', CREDENTIAL, OTHER_ENROLLMENT),
    ]
    for (const other of others) {
      await expect(openPassword(key, other, sealed.iv, sealed.data)).rejects.toThrow()
    }
  })

  it('암호문이나 IV가 바뀌면 풀리지 않는다', async () => {
    const { prf, share } = keyMaterial()
    const key = await deriveKey(prf, share, ENROLLMENT)
    const sealed = await sealPassword(key, aad, 'pw')
    const flip = (s: string) => (s[0] === 'A' ? 'B' : 'A') + s.slice(1)
    await expect(openPassword(key, aad, sealed.iv, flip(sealed.data))).rejects.toThrow()
    await expect(openPassword(key, aad, flip(sealed.iv), sealed.data)).rejects.toThrow()
  })

  it('평문은 UTF-8 1024바이트까지이고 비어 있으면 안 된다', async () => {
    const { prf, share } = keyMaterial()
    const key = await deriveKey(prf, share, ENROLLMENT)
    const max = 'a'.repeat(MAX_PASSWORD_BYTES)
    const sealed = await sealPassword(key, aad, max)
    await expect(openPassword(key, aad, sealed.iv, sealed.data)).resolves.toBe(max)
    await expect(sealPassword(key, aad, max + 'a')).rejects.toThrow(/too long/)
    // 한글 한 자는 3바이트다: 342자 = 1026바이트
    expect(passwordBytes('가'.repeat(342))).toBe(1026)
    await expect(sealPassword(key, aad, '가'.repeat(342))).rejects.toThrow(/too long/)
    await expect(sealPassword(key, aad, '')).rejects.toThrow(/empty/)
  })

  it('짧은 PRF 결과, 길이가 틀린 share, 빈 등록 ID는 받지 않는다', async () => {
    const { prf, share } = keyMaterial()
    await expect(deriveKey(random(16).buffer, share, ENROLLMENT)).rejects.toThrow()
    await expect(deriveKey(prf, random(31), ENROLLMENT)).rejects.toThrow()
    await expect(deriveKey(prf, share, '')).rejects.toThrow()
  })

  it('PRF 입력은 32바이트 무작위다', () => {
    const a = newPrfSalt()
    expect(a.byteLength).toBe(PRF_SALT_BYTES)
    expect(b64urlEncode(a)).not.toBe(b64urlEncode(newPrfSalt()))
  })
})

function entry(username: string, over: Partial<PasskeyLoginEntry> = {}): PasskeyLoginEntry {
  return {
    v: 1,
    username,
    enrollment: b64urlEncode(random(16)),
    credential: CREDENTIAL,
    rpId: RP,
    salt: b64urlEncode(random(PRF_SALT_BYTES)),
    iv: b64urlEncode(random(12)),
    data: b64urlEncode(random(24)),
    expiresAt: 2_000_000_000_000,
    ...over,
  }
}

describe('저장 형식', () => {
  it('없거나 JSON이 아니거나 버전이 다르면 빈 값이다', () => {
    expect(parseStore(null)).toEqual(emptyStore())
    expect(parseStore('')).toEqual(emptyStore())
    expect(parseStore('{')).toEqual(emptyStore())
    expect(parseStore('null')).toEqual(emptyStore())
    expect(parseStore('[]')).toEqual(emptyStore())
    expect(parseStore(JSON.stringify({ v: 2, entries: [entry('alice')], dismissed: [] }))).toEqual(emptyStore())
  })

  it('쓴 값을 그대로 읽는다', () => {
    const store = { v: 1 as const, entries: [entry('alice'), entry('bob')], dismissed: ['carol'] }
    expect(parseStore(serializeStore(store))).toEqual(store)
  })

  it('형식이 틀린 항목은 버린다', () => {
    const good = entry('alice')
    const bad: unknown[] = [
      null,
      'alice',
      { ...entry('b1'), v: 2 },
      { ...entry('b2'), username: '' },
      { ...entry('b3'), rpId: 1 },
      { ...entry('b4'), enrollment: 'not base64url!' },
      { ...entry('b5'), credential: '' },
      { ...entry('b6'), salt: b64urlEncode(random(16)) },
      { ...entry('b7'), iv: b64urlEncode(random(16)) },
      { ...entry('b8'), data: '' },
      { ...entry('b9'), expiresAt: '2000000000000' },
      { ...entry('b10'), expiresAt: null },
      { ...entry('b11'), salt: 'a' },
    ]
    const raw = JSON.stringify({ v: 1, entries: [...bad, good], dismissed: ['x', 1, '', 'x', null] })
    expect(parseStore(raw)).toEqual({ v: 1, entries: [good], dismissed: ['x'] })
    expect(parseStore(JSON.stringify({ v: 1, entries: 'x', dismissed: 'y' }))).toEqual(emptyStore())
  })

  it('모르는 필드는 남기지 않고, 같은 사용자는 앞의 항목만 남긴다', () => {
    const first = entry('alice')
    const raw = JSON.stringify({ v: 1, entries: [{ ...first, extra: 'x' }, entry('alice')], dismissed: [] })
    expect(parseStore(raw).entries).toEqual([first])
  })
})

describe('항목 목록', () => {
  it('upsertEntry는 새 항목을 앞에 두고 같은 사용자를 바꾸며 최대 개수를 지킨다', () => {
    const a = entry('alice')
    const b = entry('bob')
    expect(upsertEntry([a], b)).toEqual([b, a])
    const a2 = entry('alice')
    expect(upsertEntry([b, a], a2)).toEqual([a2, b])
    const many = Array.from({ length: MAX_ENTRIES }, (_, i) => entry(`u${i}`))
    const next = upsertEntry(many, entry('new'))
    expect(next).toHaveLength(MAX_ENTRIES)
    expect(next[0].username).toBe('new')
    expect(next.map((e) => e.username)).not.toContain(`u${MAX_ENTRIES - 1}`)
  })

  it('removeEntry는 사용자 이름이나 등록 ID로 지운다', () => {
    const a = entry('alice')
    const b = entry('bob')
    expect(removeEntry([a, b], 'alice')).toEqual([b])
    expect(removeEntry([a, b], b.enrollment)).toEqual([a])
    expect(removeEntry([a, b], 'nobody')).toEqual([a, b])
  })

  it('activeEntries는 만료된 항목을 뺀다', () => {
    const now = 1_000_000
    const live = entry('alice', { expiresAt: now + 1 })
    const dead = entry('bob', { expiresAt: now })
    expect(activeEntries([live, dead], now)).toEqual([live])
  })

  it('addDismissed는 중복 없이 앞에 넣는다', () => {
    expect(addDismissed(['a', 'b'], 'b')).toEqual(['b', 'a'])
    expect(addDismissed([], 'a')).toEqual(['a'])
  })
})

describe('isPasskeyLoginGone', () => {
  it('등록이 없어졌다는 오류만 고른다', () => {
    expect(isPasskeyLoginGone(new ApiError(400, 'x', 'passkey_login_unknown'))).toBe(true)
    expect(isPasskeyLoginGone(new ApiError(400, 'x', 'passkey_login_expired'))).toBe(true)
    expect(isPasskeyLoginGone(new ApiError(400, 'x', 'passkey_login_stale'))).toBe(true)
    expect(isPasskeyLoginGone(new ApiError(400, 'x', 'factor_failed'))).toBe(false)
    expect(isPasskeyLoginGone(new ApiError(429, 'x', 'rate_limited'))).toBe(false)
    expect(isPasskeyLoginGone(new ApiError(502, 'x'))).toBe(false)
    expect(isPasskeyLoginGone(new Error('passkey_login_unknown'))).toBe(false)
  })
})
