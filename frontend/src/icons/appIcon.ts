// 앱 아이콘: 아이콘 표의 이름이거나, 애드온 패키지처럼 서버가 주는 그림의 주소다(대시보드 문서 기준 상대 주소).
import type { IconName } from './resolver'

export interface IconURL {
  url: string
}

export type AppIcon = IconName | IconURL

export function isIconURL(icon: AppIcon): icon is IconURL {
  return typeof icon === 'object'
}
