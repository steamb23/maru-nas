// 아이콘 출처 테이블(sources/)과 리졸버가 함께 쓰는 타입.

// color: 컬러 아이콘. <img>로 그대로 쓴다.
// symbolic: 단색 -symbolic 아이콘. IconImage가 mask-image로 currentColor를 입힌다.
export type IconKind = 'color' | 'symbolic'

export interface Icon {
  url: string
  kind: IconKind
  // inset은 그릴 때 네 변에 둘 투명 여백의 비율(아이콘 크기 기준)이다. 테마 밖 아이콘이 테마 아이콘과
  // 같은 크기로 보이게 맞출 때 쓴다. 파일은 바꾸지 않는다.
  inset?: number
}
