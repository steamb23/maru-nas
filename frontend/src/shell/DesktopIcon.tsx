import type { ButtonHTMLAttributes, Ref } from 'react'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'
import { type AppId, getApp } from '../windows/apps'

// DesktopIconFace는 바탕화면 아이콘의 모양(아이콘과 이름)이다. 끌어 옮기는 고스트도 같은 모양을 쓴다.
export function DesktopIconFace({ appId }: { appId: AppId }) {
  const { t } = useTranslation()
  const app = getApp(appId)
  return (
    <>
      <IconImage name={app.icon} size={48} app />
      <span className="desktop-icon-label">{t(app.titleKey)}</span>
    </>
  )
}

interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  appId: AppId
  ref?: Ref<HTMLButtonElement>
}

// DesktopIcon은 바탕화면 바로가기 버튼이다. 누르는 동작(창 열기)과 메뉴·끌기 처리기는 DesktopShortcuts가 준다.
// data-app으로 메뉴·끌기 처리기가 어느 앱인지 찾는다.
export function DesktopIcon({ appId, ref, ...rest }: Props) {
  return (
    <button ref={ref} type="button" className="desktop-icon" data-app={appId} {...rest}>
      <DesktopIconFace appId={appId} />
    </button>
  )
}
