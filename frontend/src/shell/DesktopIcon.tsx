import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'
import { type AppId, getApp } from '../windows/apps'
import type { Size } from '../windows/geometry'
import { useWindowStore } from '../windows/store'

interface Props {
  appId: AppId
  bounds: Size
}

// DesktopIcon은 한 번 클릭(또는 Enter)으로 앱 창을 연다. 이미 열려 있으면 앞으로 가져온다.
export function DesktopIcon({ appId, bounds }: Props) {
  const { t } = useTranslation()
  const app = getApp(appId)
  return (
    <button type="button" className="desktop-icon" onClick={() => useWindowStore.getState().open(appId, bounds)}>
      <IconImage name={app.icon} size={48} />
      <span className="desktop-icon-label">{t(app.titleKey)}</span>
    </button>
  )
}
