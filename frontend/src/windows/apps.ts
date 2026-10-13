// 데스크톱에서 열 수 있는 앱 목록. 바탕화면 아이콘은 이 순서대로 놓인다.
import type { IconName } from '../icons/resolver'
import type { Size } from './geometry'

// system-info: 시스템 정보 창(실제 데이터). placeholder: "준비 중" 빈 창.
export type AppKind = 'system-info' | 'placeholder'

export interface AppDef {
  title: string
  icon: IconName
  kind: AppKind
  defaultSize: Size
}

export const APPS = {
  'system-info': {
    title: '시스템 정보',
    icon: 'utilities-system-monitor',
    kind: 'system-info',
    defaultSize: { width: 420, height: 320 },
  },
  files: {
    title: '파일',
    icon: 'folder',
    kind: 'placeholder',
    defaultSize: { width: 640, height: 420 },
  },
  storage: {
    title: '저장소',
    icon: 'drive-harddisk',
    kind: 'placeholder',
    defaultSize: { width: 560, height: 400 },
  },
} as const satisfies Record<string, AppDef>

export type AppId = keyof typeof APPS

export const APP_IDS = Object.keys(APPS) as AppId[]

export function getApp(id: AppId): AppDef {
  return APPS[id]
}
