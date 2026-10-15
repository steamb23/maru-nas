// openmediavault 공식 로고(vendor/icons/openmediavault, GPL-3.0). OMV 자신을 가리킬 때만 쓴다.
// 브랜드 로고는 원본 그대로 쓴다(색·모양을 바꾸지 않는다).
import favicon from '@vendor-icons/openmediavault/favicon.svg?url'
import type { Icon } from '../types'

export const OPENMEDIAVAULT_ICONS = {
  openmediavault: { url: favicon, kind: 'color' },
} as const satisfies Record<string, Icon>
