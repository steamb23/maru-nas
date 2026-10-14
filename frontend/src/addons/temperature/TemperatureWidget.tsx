import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { formatCelsius } from '../common/format'
import { fitList, widgetCapacity } from '../common/widgetRows'
import type { WidgetViewProps } from '../types'
import { summaryRows, tempLevel } from './temperature'
import { TEMPERATURE_WIDGET_POLL_INTERVAL_MS, useTemperature } from './useTemperature'

// 한 줄(.widget-line 20px)과 줄 사이 간격(.temp-widget의 gap)이다.
const ITEM_HEIGHT = 20
const ITEM_GAP = 6

// TemperatureWidget은 CPU와 디스크의 대표 온도다. 경고 온도에 가까우면 색을 바꾼다.
export function TemperatureWidget({ active, size }: WidgetViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data, status } = useTemperature(active, TEMPERATURE_WIDGET_POLL_INTERVAL_MS)
  const rows = data ? summaryRows(data.groups) : []
  const { shown, hidden } = fitList(rows, widgetCapacity(size.rows, ITEM_HEIGHT, ITEM_GAP))

  return (
    <div className={status === 'disconnected' ? 'addon-widget temp-widget stale' : 'addon-widget temp-widget'}>
      {status === 'disconnected' && (
        <p className="widget-status" role="status">
          {t('widgets.disconnected')}
        </p>
      )}
      {data && rows.length === 0 && <p className="widget-empty">{t('temperature.empty')}</p>}
      {shown.map((r) => (
        <div key={r.key} className="widget-line">
          <span className="widget-name">{r.kind === 'cpu' ? t('temperature.cpu') : r.name}</span>
          <span className={`widget-value level-${tempLevel(r.sensor, r.kind)}`}>{formatCelsius(r.sensor.celsius, lang)}</span>
        </div>
      ))}
      {hidden > 0 && <p className="widget-more">{t('widgets.more', { count: hidden })}</p>}
    </div>
  )
}
