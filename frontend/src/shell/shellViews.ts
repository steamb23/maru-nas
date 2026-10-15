// 셸이 직접 그리는 창 앱(windows/apps.ts의 kind: 'shell')의 보기 컴포넌트.
// apps.ts는 node 환경에서 테스트하므로 컴포넌트를 import하지 않고 여기에 따로 둔다(addons/registry.ts와 같은 이유).
import type { ComponentType } from 'react'
import type { WindowViewProps } from '../addons/types'
import { SettingsWindow } from '../settings/SettingsWindow'
import type { ShellAppId } from '../windows/apps'

export const SHELL_VIEWS: Record<ShellAppId, ComponentType<WindowViewProps>> = {
  settings: SettingsWindow,
}
