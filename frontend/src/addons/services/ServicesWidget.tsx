import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { fitList, widgetCapacity } from '../common/widgetRows'
import type { WidgetViewProps } from '../types'
import { countServices, serviceState, serviceTone, sortByAttention } from './services'
import { SERVICES_WIDGET_POLL_INTERVAL_MS, useServices } from './useServices'

// 한 줄(.widget-line 20px)과 줄 사이 간격(.svc-widget의 gap)이다. 첫 줄은 개수 요약이다.
const ITEM_HEIGHT = 20
const ITEM_GAP = 6

// ServicesWidget은 실행 중·주의 개수와, 문제 있는 것부터 정렬한 서비스 목록이다.
// 3칸 이상이면 상태도 글자로 보여 준다.
export function ServicesWidget({ active, size }: WidgetViewProps) {
  const { t } = useTranslation()
  const language = useLanguage()
  const { data, status } = useServices(active, SERVICES_WIDGET_POLL_INTERVAL_MS)
  const list = data ? sortByAttention(data.services, language) : []
  const counts = countServices(list)
  const capacity = widgetCapacity(size.rows, ITEM_HEIGHT, ITEM_GAP) - 1
  const { shown, hidden } = capacity > 0 ? fitList(list, capacity) : { shown: [], hidden: 0 }
  const wide = size.cols >= 3

  return (
    <div className={status === 'disconnected' ? 'addon-widget svc-widget stale' : 'addon-widget svc-widget'}>
      {status === 'disconnected' && (
        <p className="widget-status" role="status">
          {t('widgets.disconnected')}
        </p>
      )}
      {data && (
        <div className="widget-line svc-counts">
          <span>{t('services.running', { count: counts.running })}</span>
          {counts.attention > 0 && (
            <span className="widget-detail-line">{t('services.attention', { count: counts.attention })}</span>
          )}
        </div>
      )}
      {shown.map((s) => (
        <div key={s.name} className="widget-line">
          <span className={`status-dot ${serviceTone(s)}`} aria-hidden="true" />
          <span className="widget-name">{s.title}</span>
          {wide && <span className="widget-detail">{t(`services.state.${serviceState(s)}`)}</span>}
        </div>
      ))}
      {data && list.length === 0 && <p className="widget-more">{t('services.empty')}</p>}
      {hidden > 0 && <p className="widget-more">{t('widgets.more', { count: hidden })}</p>}
    </div>
  )
}
