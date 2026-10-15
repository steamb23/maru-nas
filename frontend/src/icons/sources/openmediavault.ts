// openmediavault 공식 로고(vendor/icons/openmediavault, GPL-3.0). OMV 자신을 가리킬 때만 쓴다.
// 파일은 원본 그대로 두고 색을 바꾸지 않는다. 크기·모서리는 그릴 때만 테마에 맞춘다(inset, appRadius).
import favicon from '@vendor-icons/openmediavault/favicon.svg?url'
import type { Icon } from '../types'

// 로고는 캔버스를 꽉 채운 각진 사각형이다. Colloid 앱 아이콘은 64칸 중 가운데 56칸에 반지름 13칸의
// 둥근 사각형이므로, 같은 크기로 보이게 4/64씩 여백을 두고 앱 아이콘 자리에서는 같은 비율로 모서리를 깎는다.
export const OPENMEDIAVAULT_ICONS = {
  openmediavault: { url: favicon, kind: 'color', inset: 4 / 64, appRadius: 13 / 56 },
} as const satisfies Record<string, Icon>
