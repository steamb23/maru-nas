// openmediavault 공식 로고(vendor/icons/openmediavault, GPL-3.0). OMV 자신을 가리킬 때만 쓴다.
// 브랜드 로고는 원본 그대로 쓴다(색·모양을 바꾸지 않는다).
import favicon from '@vendor-icons/openmediavault/favicon.svg?url'
import type { Icon } from '../types'

// 로고는 캔버스를 꽉 채운 사각형이다. Colloid 앱 아이콘은 64칸 중 가운데 56칸을 쓰므로
// 같은 크기로 보이게 그릴 때 4/64씩 여백을 둔다.
export const OPENMEDIAVAULT_ICONS = {
  openmediavault: { url: favicon, kind: 'color', inset: 4 / 64 },
} as const satisfies Record<string, Icon>
