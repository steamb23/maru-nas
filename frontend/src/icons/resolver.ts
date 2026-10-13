// 아이콘 리졸버. 컴포넌트는 아이콘 경로 대신 freedesktop 아이콘 이름으로 getIcon()을 부른다.
// 테마를 바꿀 때는 이 파일의 매핑 테이블만 수정한다.
//
// 테이블에 import한 파일만 번들에 포함된다. 디렉터리 단위 glob을 쓰면 테마 전체가
// 실행 파일에 들어가므로, 아이콘을 쓸 때마다 여기에 한 줄씩 추가한다.

import folder from '@colloid/places/scalable/folder.svg?url'

// color: 컬러 아이콘. <img>로 그대로 쓴다.
// symbolic: 단색 -symbolic 아이콘. mask-image 등으로 currentColor를 입힌다(1단계에서 구현).
export type IconKind = 'color' | 'symbolic'

export interface Icon {
  url: string
  kind: IconKind
}

const ICONS = {
  folder: { url: folder, kind: 'color' },
} as const satisfies Record<string, Icon>

export type IconName = keyof typeof ICONS

export function getIcon(name: IconName): Icon {
  return ICONS[name]
}
