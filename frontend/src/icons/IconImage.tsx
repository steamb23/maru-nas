import type { CSSProperties } from 'react'
import { type IconName, getIcon } from './resolver'

interface Props {
  name: IconName
  size: number
  className?: string
}

// IconImage는 아이콘 종류에 맞게 그린다.
// 컬러 아이콘은 <img>로 그대로, symbolic 아이콘은 mask-image로 잘라 currentColor를 칠한다.
// 아이콘은 모두 장식용이며, 의미는 감싸는 요소의 라벨로 전달한다.
export function IconImage({ name, size, className }: Props) {
  const icon = getIcon(name)
  if (icon.kind === 'color') {
    // 여백은 padding으로 둔다(box-sizing: border-box라 바깥 크기는 size 그대로다).
    const style = icon.inset ? { padding: size * icon.inset } : undefined
    return <img className={className} src={icon.url} width={size} height={size} style={style} alt="" draggable={false} />
  }
  const mask = `url("${icon.url}")`
  const style: CSSProperties = { width: size, height: size, maskImage: mask, WebkitMaskImage: mask }
  return <span className={`symbolic-icon ${className ?? ''}`.trim()} style={style} aria-hidden="true" />
}
