// 창 제목. 창 상태에는 제목을 복사하지 않고 그릴 때 번역한다(언어를 바꾸면 바로 바뀌게).
// 창을 여러 개 여는 앱의 둘째 창부터는 앱 이름 뒤에 창 번호(WindowState.instance)를 붙인다.
import type { ParseKeys } from 'i18next'
import { getWindowApp } from './apps'
import type { WindowState } from './store'

export type Translate = (key: ParseKeys, params?: Record<string, string | number>) => string

export function windowTitle(t: Translate, win: Pick<WindowState, 'appId' | 'instance'>): string {
  const title = t(getWindowApp(win.appId).titleKey)
  return win.instance > 1 ? t('window.numberedTitle', { title, number: win.instance }) : title
}

// windowTitleLabel은 메뉴 항목(contextMenu.ts)으로 쓸 창 제목의 문구 키와 치환값이다.
export function windowTitleLabel(
  t: Translate,
  win: Pick<WindowState, 'appId' | 'instance'>,
): { labelKey: ParseKeys; labelParams?: Record<string, string | number> } {
  const { titleKey } = getWindowApp(win.appId)
  if (win.instance <= 1) return { labelKey: titleKey }
  return { labelKey: 'window.numberedTitle', labelParams: { title: t(titleKey), number: win.instance } }
}
