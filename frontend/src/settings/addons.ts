// 설정의 애드온 페이지(관리자)가 쓰는 계산: 목록 정렬, 상태·이유·위험·경고 문구 키, 업데이트로 바뀌는 권한 목록,
// 오류 문구, 확인 단계 판단. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 백엔드는 이유·위험·경고를 코드로만 준다(internal/addonpkg, internal/addons/extend.go). 모르는 코드는 일반 문구로
// 보인다. detail은 영어 설명이라 문구를 대신하지 않고 아래에 작게 보인다.
import type { ParseKeys } from 'i18next'
import { localizedText } from '../addonpkg/catalog'
import { ApiError } from '../api/client'
import type { AddonPackageStatus } from '../api/types.gen'

type Level = 'password' | 'twoFactor'

// packageTitle은 패키지 이름이다. manifest를 읽지 못한 패키지는 id다.
export function packageTitle(p: Pick<AddonPackageStatus, 'id' | 'name'>, lang: string): string {
  return localizedText(p.name, lang) || p.id
}

// needsAttention은 켜 두었지만 쓰지 못하는 패키지인지다(목록 앞에 둔다).
export function needsAttention(p: Pick<AddonPackageStatus, 'state'>): boolean {
  return p.state === 'rejected' || p.state === 'invalid'
}

// sortPackages는 쓰지 못하는 패키지를 앞에, 나머지는 이름순으로 둔다.
export function sortPackages(list: readonly AddonPackageStatus[], lang: string): AddonPackageStatus[] {
  const collator = new Intl.Collator(lang)
  return [...list].sort((a, b) => {
    const pa = needsAttention(a) ? 0 : 1
    const pb = needsAttention(b) ? 0 : 1
    if (pa !== pb) return pa - pb
    return collator.compare(packageTitle(a, lang), packageTitle(b, lang)) || a.id.localeCompare(b.id)
  })
}

// stateBadgeKey는 쓰지 못하는 패키지의 상태 배지다. 켜짐·꺼짐은 스위치가 보이므로 배지가 없다.
export function stateBadgeKey(p: Pick<AddonPackageStatus, 'state' | 'reason'>): ParseKeys | null {
  if (p.state === 'invalid') return p.reason === 'incompatible' ? 'settings.addons.state.incompatible' : 'settings.addons.state.invalid'
  if (p.state === 'rejected') {
    return p.reason === 'conflict_builtin' || p.reason === 'conflict_package'
      ? 'settings.addons.state.conflict'
      : 'settings.addons.state.notAllowed'
  }
  return null
}

const REASON_KEYS: Record<string, ParseKeys> = {
  archive_invalid: 'settings.addons.reason.archive_invalid',
  too_large: 'settings.addons.reason.too_large',
  manifest_invalid: 'settings.addons.reason.manifest_invalid',
  incompatible: 'settings.addons.reason.incompatible',
  arch_missing: 'settings.addons.reason.arch_missing',
  backend_invalid: 'settings.addons.reason.backend_invalid',
  conflict_builtin: 'settings.addons.reason.conflict_builtin',
  conflict_package: 'settings.addons.reason.conflict_package',
  rpc_denied: 'settings.addons.reason.rpc_denied',
  rpc_kind: 'settings.addons.reason.rpc_kind',
  reserved_id: 'settings.addons.reason.reserved_id',
  invalid: 'settings.addons.reason.invalid',
}

// reasonKey는 패키지를 받지 않거나 쓰지 못하는 이유 문구다.
export function reasonKey(reason: string | undefined): ParseKeys {
  return (reason && REASON_KEYS[reason]) || 'settings.addons.reason.unknown'
}

const RISK_KEYS: Record<string, ParseKeys> = {
  omv_write: 'settings.addons.risk.omv_write',
  backend: 'settings.addons.risk.backend',
  backend_shared_data: 'settings.addons.risk.backend_shared_data',
  net_lan: 'settings.addons.risk.net_lan',
  net_loopback: 'settings.addons.risk.net_loopback',
}

// riskKeys는 위험 코드의 문구다. 모르는 코드는 일반 문구 하나로 모은다.
export function riskKeys(risks: readonly string[]): ParseKeys[] {
  const out: ParseKeys[] = []
  for (const r of risks) {
    const k = RISK_KEYS[r] ?? 'settings.addons.risk.unknown'
    if (!out.includes(k)) out.push(k)
  }
  return out
}

const WARNING_KEYS: Record<string, ParseKeys> = {
  author_changed: 'settings.addons.warning.author_changed',
  downgrade: 'settings.addons.warning.downgrade',
  reinstall: 'settings.addons.warning.reinstall',
  same_digest: 'settings.addons.warning.same_digest',
  backends_disabled: 'settings.addons.warning.backends_disabled',
}

// warningKeys는 검토 화면의 경고 문구다. 모르는 코드는 뺀다(경고는 설치를 막지 않는다).
export function warningKeys(warnings: readonly string[]): ParseKeys[] {
  return warnings.flatMap((w) => (WARNING_KEYS[w] ? [WARNING_KEYS[w]] : []))
}

// PermissionItem은 권한 목록의 항목 하나다. change는 업데이트로 더해지거나 빠지는지다.
export interface PermissionItem {
  value: string
  change?: 'added' | 'removed'
}

// withChanges는 지금 목록에 업데이트로 더해진 표시를 붙이고, 빠진 항목을 뒤에 덧붙인다.
export function withChanges(
  current: readonly string[],
  added: readonly string[] = [],
  removed: readonly string[] = [],
): PermissionItem[] {
  const add = new Set(added)
  const items: PermissionItem[] = current.map((value) => (add.has(value) ? { value, change: 'added' } : { value }))
  for (const value of removed) if (!current.includes(value)) items.push({ value, change: 'removed' })
  return items
}

// AuthStep은 패키지를 바꾸는 요청이 거절된 뒤 거칠 확인이다.
export type AuthStep = { kind: 'manage' } | { kind: 'stepUp'; level?: Level }

// authStep은 오류가 확인을 거치면 다시 할 수 있는 거절인지다. 관리 창(manage_required)을 먼저 보고, 쓰기·백엔드가
// 있는 패키지는 그 뒤 단계 확인(security_required)을 본다. 그 밖의 오류는 null이다.
export function authStep(err: unknown): AuthStep | null {
  if (!(err instanceof ApiError) || err.status !== 403) return null
  if (err.code === 'manage_required') return { kind: 'manage' }
  if (err.code === 'security_required') return { kind: 'stepUp', level: err.required }
  return null
}

// MAX_AUTH_ROUNDS는 한 요청에서 확인을 거치는 횟수 상한이다(관리 창, 단계 확인, 그사이 창이 닫힌 경우 한 번).
export const MAX_AUTH_ROUNDS = 3

// AddonError는 화면에 보일 오류다. detail은 백엔드의 영어 설명이다.
export interface AddonError {
  key: ParseKeys
  params?: Record<string, string | number>
  detail?: string
}

// addonError는 패키지 관리 요청의 오류 문구다. maxMiB는 올릴 수 있는 크기다(413 문구).
export function addonError(err: unknown, maxMiB: number): AddonError {
  if (!(err instanceof ApiError)) return { key: 'settings.addons.errors.failed' }
  switch (err.code) {
    case 'addon_invalid':
      return { key: reasonKey(err.reason), detail: err.detail }
    case 'addon_too_large':
      return { key: 'settings.addons.errors.tooLarge', params: { size: maxMiB } }
    case 'addons_unavailable':
      return { key: 'settings.addons.errors.unavailable' }
    case 'upload_expired':
      return { key: 'settings.addons.errors.expired' }
    case 'addon_readonly':
      return { key: 'settings.addons.errors.readonly' }
    case 'action_busy':
      return { key: 'settings.addons.errors.busy' }
    case 'not_found':
      return { key: 'settings.addons.errors.notFound' }
    case 'manage_required':
    case 'security_required':
      return { key: 'settings.addons.errors.notConfirmed' }
  }
  if (err.status === 413) return { key: 'settings.addons.errors.tooLarge', params: { size: maxMiB } }
  return { key: 'settings.addons.errors.failed' }
}

// mib는 바이트를 MiB 정수로 바꾼다(올리기 한도 문구).
export function mib(bytes: number): number {
  return Math.floor(bytes / (1024 * 1024))
}
