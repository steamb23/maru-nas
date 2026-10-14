import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { formatCelsius, formatTime } from '../common/format'
import { fitList, widgetCapacity } from '../common/widgetRows'
import type { WidgetViewProps } from '../types'
import { countDisks, sortDisks, statusLevel } from './smart'
import { useSmart } from './useSmart'

// 한 줄(.widget-line 20px)과 줄 사이 간격(.smart-widget의 gap)이다. 첫 줄은 요약, 마지막 줄은 확인 시각이다.
const ITEM_HEIGHT = 20
const ITEM_GAP = 6

// SmartWidget은 디스크 건강 상태 요약이다. 문제 있는 디스크를 먼저 보여 주고, 3칸 이상이면 온도도 보여 준다.
export function SmartWidget({ active, size }: WidgetViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data: sample, status } = useSmart(active)
  const data = sample?.response
  const disks = data ? sortDisks(data.disks) : []
  const counts = countDisks(disks)
  const capacity = widgetCapacity(size.rows, ITEM_HEIGHT, ITEM_GAP)
  // 요약 줄과 확인 시각 줄을 뺀 나머지에 디스크를 둔다. 한 줄이면 요약만 보여 준다.
  const { shown, hidden } = capacity >= 3 ? fitList(disks, capacity - 2) : { shown: [], hidden: 0 }
  const wide = size.cols >= 3
  const problems = counts.warning + counts.failing

  return (
    <div className={status === 'disconnected' ? 'addon-widget smart-widget stale' : 'addon-widget smart-widget'}>
      {status === 'disconnected' && (
        <p className="widget-status" role="status">
          {t('widgets.disconnected')}
        </p>
      )}
      {status === 'loading' && <p className="widget-empty">{t('smart.checking')}</p>}
      {data && (
        <div className="widget-line smart-summary">
          {problems === 0 ? (
            <span className="level-ok">{disks.length === 0 ? t('smart.noDisks') : t('smart.allGood')}</span>
          ) : (
            <span className={counts.failing > 0 ? 'level-crit' : 'level-warn'}>
              {t('smart.problems', { count: problems })}
            </span>
          )}
        </div>
      )}
      {shown.map((d) => (
        <div key={d.device} className="widget-line">
          <span className={`status-dot ${statusLevel(d.status)}`} aria-hidden="true" />
          <span className="widget-name">{d.device}</span>
          {wide && <span className="widget-detail">{d.model}</span>}
          {d.celsius !== undefined && <span className="widget-value">{formatCelsius(d.celsius, lang)}</span>}
        </div>
      ))}
      {hidden > 0 && <p className="widget-more">{t('widgets.more', { count: hidden })}</p>}
      {data && capacity >= 2 && (
        <p className="widget-more smart-checked">{t('smart.checkedShort', { time: formatTime(data.checkedAt, lang) })}</p>
      )}
    </div>
  )
}
