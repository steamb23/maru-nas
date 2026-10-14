// 데스크톱에서 열 수 있는 앱 목록. 바탕화면 아이콘은 이 순서대로 놓인다.
import type { ParseKeys } from 'i18next'
import type { IconName } from '../icons/resolver'
import type { Size } from './geometry'

// system-info: 시스템 정보 창(실제 데이터). placeholder: "준비 중" 빈 창.
export type AppKind = 'system-info' | 'placeholder'

export interface AppDef {
  // 창 제목의 번역 키. 창 상태에는 제목을 복사하지 않고 그릴 때 번역한다(언어를 바꾸면 바로 바뀌게).
  titleKey: ParseKeys
  icon: IconName
  kind: AppKind
  defaultSize: Size
  // 크기 조절로 줄일 수 있는 최소 크기. 생략하면 공통 하한(geometry의 MIN_WIDTH·MIN_HEIGHT)을 쓰며,
  // 공통 하한보다 작은 값은 공통 하한으로 올라간다.
  minSize?: Size
}

export const APPS = {
  'system-info': {
    titleKey: 'apps.systemInfo',
    icon: 'utilities-system-monitor',
    kind: 'system-info',
    defaultSize: { width: 420, height: 360 },
    // 평상시 내용이 스크롤 없이 들어가는 높이. Chrome 실측 265px을 10px 단위로 올렸다.
    // "연결 끊김" 배너(실측 311px 필요)는 예외 상황이므로 최소 크기에서는 스크롤을 허용한다.
    // 창 내용을 바꾸면 다시 잰다.
    minSize: { width: 320, height: 270 },
  },
  files: {
    titleKey: 'apps.files',
    icon: 'folder',
    kind: 'placeholder',
    defaultSize: { width: 640, height: 420 },
  },
  storage: {
    titleKey: 'apps.storage',
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
