// 컬러 아이콘(<img>)에 줄 style 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { CSSProperties } from 'react'
import type { Icon } from './types'

// colorIconStyle은 아이콘 표의 inset·appRadius를 style로 바꾼다. 줄 것이 없으면 undefined.
// 여백은 padding으로 둔다(box-sizing: border-box라 바깥 크기는 size 그대로다).
// 모서리는 app(앱 아이콘 자리)일 때만 깎으며, 여백 안쪽 그림을 clip-path로 자른다.
export function colorIconStyle(icon: Icon, size: number, app: boolean): CSSProperties | undefined {
  const pad = size * (icon.inset ?? 0)
  const style: CSSProperties = {}
  if (pad > 0) style.padding = pad
  if (app && icon.appRadius) style.clipPath = `inset(${pad}px round ${(size - 2 * pad) * icon.appRadius}px)`
  return Object.keys(style).length > 0 ? style : undefined
}
