// 셸이 직접 그리는 창 앱(windows/apps.ts의 kind: 'shell')의 보기 컴포넌트.
// apps.ts는 node 환경에서 테스트하므로 컴포넌트를 import하지 않고 여기에 따로 둔다(addons/registry.ts와 같은 이유).
// 애드온 보기처럼 처음 열 때 내려받는다(lazyView).
import type { ComponentType } from 'react'
import { lazyView } from '../addons/registry'
import type { WindowViewProps } from '../addons/types'
import type { ShellAppId } from '../windows/apps'

export const SHELL_VIEWS: Record<ShellAppId, ComponentType<WindowViewProps>> = {
  settings: lazyView(() => import('../settings/SettingsWindow'), (m) => m.SettingsWindow),
  terminal: lazyView(() => import('../terminal/TerminalWindow'), (m) => m.TerminalWindow),
}
