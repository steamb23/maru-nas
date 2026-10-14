import { useTranslation } from 'react-i18next'

// PlaceholderWindow는 아직 구현하지 않은 앱 창의 내용이다.
export function PlaceholderWindow() {
  const { t } = useTranslation()
  return (
    <div className="placeholder">
      <p>{t('window.placeholder')}</p>
    </div>
  )
}
