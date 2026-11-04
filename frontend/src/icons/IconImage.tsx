import type { CSSProperties } from 'react'
import { type AppIcon, isIconURL } from './appIcon'
import { colorIconStyle } from './iconStyle'
import { getIcon } from './resolver'

interface Props {
  // 아이콘 표의 이름 또는 주소(애드온 패키지 아이콘). 주소는 컬러 그림으로 <img>에만 넣는다(SVG 안의 스크립트는
  // <img>에서 돌지 않는다).
  name: AppIcon
  size: number
  className?: string
  // app이면 앱을 나타내는 아이콘(getApp(id).icon)으로 그린다: "모든 앱" 타일, 바탕화면 바로가기,
  // 작업표시줄, 제목 표시줄. 아이콘 표에 appRadius가 있으면 이때만 모서리를 둥글게 깎는다.
  app?: boolean
}

// IconImage는 아이콘 종류에 맞게 그린다.
// 컬러 아이콘은 <img>로 그대로, symbolic 아이콘은 mask-image로 잘라 currentColor를 칠한다.
// 아이콘은 모두 장식용이며, 의미는 감싸는 요소의 라벨로 전달한다.
export function IconImage({ name, size, className, app = false }: Props) {
  if (isIconURL(name)) {
    return (
      <img
        className={`package-icon ${className ?? ''}`.trim()}
        src={name.url}
        width={size}
        height={size}
        alt=""
        draggable={false}
        referrerPolicy="no-referrer"
      />
    )
  }
  const icon = getIcon(name)
  if (icon.kind === 'color') {
    const style = colorIconStyle(icon, size, app)
    return <img className={className} src={icon.url} width={size} height={size} style={style} alt="" draggable={false} />
  }
  const mask = `url("${icon.url}")`
  const style: CSSProperties = { width: size, height: size, maskImage: mask, WebkitMaskImage: mask }
  return <span className={`symbolic-icon ${className ?? ''}`.trim()} style={style} aria-hidden="true" />
}
