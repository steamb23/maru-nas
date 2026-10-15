// 아이콘 출처 테이블(sources/)과 리졸버가 함께 쓰는 타입.

// color: 컬러 아이콘. <img>로 그대로 쓴다.
// symbolic: 단색 -symbolic 아이콘. IconImage가 mask-image로 currentColor를 입힌다.
export type IconKind = 'color' | 'symbolic'

export interface Icon {
  url: string
  kind: IconKind
}
