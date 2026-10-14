// 메뉴 항목 사이의 키보드 포커스 이동(DOM). 위치 계산은 menuNav.ts가 한다.
import type { KeyboardEvent } from 'react'
import { nextIndex } from './menuNav'

// moveFocus는 화살표·Home·End 키로 root 안의 selector 항목 사이에서 포커스를 옮긴다. 옮겼으면 true.
export function moveFocus(root: HTMLElement | null, selector: string, e: KeyboardEvent<HTMLElement>): boolean {
  const items = Array.from(root?.querySelectorAll<HTMLButtonElement>(selector) ?? [])
  const current = items.indexOf(document.activeElement as HTMLButtonElement)
  const next = nextIndex(current, items.length, e.key)
  if (next === current) return false
  e.preventDefault()
  items[next]?.focus()
  return true
}
