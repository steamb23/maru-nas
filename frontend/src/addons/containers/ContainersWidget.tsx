import { useTranslation } from 'react-i18next'
import { fitList, widgetCapacity } from '../common/widgetRows'
import type { WidgetViewProps } from '../types'
import { containerTone, countContainers, sortByAttention } from './containers'
import { stateLabel } from './labels'
import { CONTAINERS_WIDGET_POLL_INTERVAL_MS, useContainers } from './useContainers'

// 한 줄(.widget-line 20px)과 줄 사이 간격(.ctr-widget의 gap)이다. 첫 줄은 개수 요약이다.
const ITEM_HEIGHT = 20
const ITEM_GAP = 6

// ContainersWidget은 실행 중·정지 개수와, 문제 있는 것부터 정렬한 컨테이너 목록이다.
// 3칸 이상이면 상태도 글자로 보여 준다.
export function ContainersWidget({ active, size }: WidgetViewProps) {
  const { t } = useTranslation()
  const { data, status } = useContainers(active, CONTAINERS_WIDGET_POLL_INTERVAL_MS)
  const list = data ? sortByAttention(data.containers) : []
  const counts = countContainers(list)
  // 개수 요약 줄을 뺀 나머지 줄에 목록을 둔다.
  const capacity = widgetCapacity(size.rows, ITEM_HEIGHT, ITEM_GAP) - 1
  const { shown, hidden } = capacity > 0 ? fitList(list, capacity) : { shown: [], hidden: 0 }
  const wide = size.cols >= 3

  return (
    <div className={status === 'disconnected' ? 'addon-widget ctr-widget stale' : 'addon-widget ctr-widget'}>
      {status === 'disconnected' && (
        <p className="widget-status" role="status">
          {t('widgets.disconnected')}
        </p>
      )}
      {data && (
        <div className="widget-line ctr-counts">
          <span>{t('containers.running', { count: counts.running })}</span>
          <span className="widget-detail-line">{t('containers.stopped', { count: counts.stopped })}</span>
        </div>
      )}
      {shown.map((c) => (
        <div key={c.name} className="widget-line">
          <span className={`status-dot ${containerTone(c)}`} aria-hidden="true" />
          <span className="widget-name">{c.name}</span>
          {wide && <span className="widget-detail">{stateLabel(t, c)}</span>}
        </div>
      ))}
      {hidden > 0 && <p className="widget-more">{t('widgets.more', { count: hidden })}</p>}
    </div>
  )
}
