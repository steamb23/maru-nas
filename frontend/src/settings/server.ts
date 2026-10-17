// 설정의 서버 페이지 계산(DOM 없음): 가동 설정 이름·출처 문구 키, 덮어쓰기 요청 만들기.
import type { ParseKeys } from 'i18next'
import type { RuntimeSetting, ServerOverrides, ServerSettings } from '../api/types.gen'

// 가동 설정(플래그)의 이름 문구 키. 모르는 플래그는 플래그 이름을 그대로 보여 준다.
const RUNTIME_LABELS: Record<string, ParseKeys> = {
  listen: 'settings.server.flags.listen',
  'allow-non-loopback': 'settings.server.flags.allowNonLoopback',
  'omv-url': 'settings.server.flags.omvUrl',
  'cookie-path': 'settings.server.flags.cookiePath',
  'state-dir': 'settings.server.flags.stateDir',
  'workbench-url': 'settings.server.flags.workbenchUrl',
  'smart-refresh-at': 'settings.server.flags.smartRefreshAt',
  'helper-socket': 'settings.server.flags.helperSocket',
  terminal: 'settings.server.flags.terminal',
  'ssh-addr': 'settings.server.flags.sshAddr',
  'ssh-host-keys': 'settings.server.flags.sshHostKeys',
  'public-origin': 'settings.server.flags.publicOrigin',
}

export function runtimeLabelKey(flag: string): ParseKeys | null {
  return Object.hasOwn(RUNTIME_LABELS, flag) ? RUNTIME_LABELS[flag] : null
}

export const SOURCE_LABELS: Record<RuntimeSetting['source'], ParseKeys> = {
  default: 'settings.server.sources.default',
  env: 'settings.server.sources.env',
  flag: 'settings.server.sources.flag',
  systemd: 'settings.server.sources.systemd',
}

// 웹에서 덮어쓰는 설정. 키는 ServerOverrides의 필드, flag는 가동 설정 현황의 플래그 이름이다.
export type OverrideKey = keyof ServerOverrides

export const OVERRIDE_FLAGS: Record<OverrideKey, string> = {
  workbenchUrl: 'workbench-url',
  smartRefreshAt: 'smart-refresh-at',
  terminal: 'terminal',
}

// overridesWith는 지금 덮어쓴 값에서 key 하나만 value로 바꾼(null이면 덮어쓰기를 지운) 요청 본문이다.
// PUT은 덮어쓰기 전체를 바꾸므로 다른 값도 그대로 담아 보낸다.
export function overridesWith(settings: ServerSettings, key: OverrideKey, value: string | null): ServerOverrides {
  const out: ServerOverrides = {}
  for (const k of Object.keys(OVERRIDE_FLAGS) as OverrideKey[]) {
    const s = settings[k]
    if (k === key) {
      if (value !== null) out[k] = value
    } else if (s.overridden) {
      out[k] = s.value
    }
  }
  return out
}
