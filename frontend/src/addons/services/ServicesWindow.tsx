import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import type { WindowViewProps } from '../types'
import { countServices, serviceState, serviceTone, sortByAttention } from './services'
import { useServices } from './useServices'

// ServicesWindow는 OMV 서비스 전체 목록(사용 설정, 실행 상태)이다.
export function ServicesWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const language = useLanguage()
  const { data, status } = useServices(active)
  const list = data ? sortByAttention(data.services, language) : []
  const counts = countServices(list)

  return (
    <div className="addon-window">
      {status === 'disconnected' && (
        <p className="addon-banner" role="status">
          {t('addonWindow.disconnected')}
        </p>
      )}
      {data && (
        <div className="ctr-summary">
          <span>{t('services.running', { count: counts.running })}</span>
          {counts.attention > 0 && <span>{t('services.attention', { count: counts.attention })}</span>}
        </div>
      )}
      {data && list.length === 0 && <p className="addon-empty">{t('services.empty')}</p>}
      <ul className={status === 'disconnected' ? 'addon-list ctr-list stale' : 'addon-list ctr-list'}>
        {list.map((s) => (
          <li key={s.name} className="ctr-row">
            <span className={`status-dot ${serviceTone(s)}`} aria-hidden="true" />
            <div className="ctr-main">
              <div className="ctr-name">{s.title}</div>
            </div>
            <div className="ctr-state">{t(`services.state.${serviceState(s)}`)}</div>
          </li>
        ))}
      </ul>
    </div>
  )
}
