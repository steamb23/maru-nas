import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { formatLinkSpeed, formatRate } from '../common/format'
import { fitList, widgetCapacity } from '../common/widgetRows'
import type { WidgetViewProps } from '../types'
import { isUp, widgetInterfaces } from './network'
import { NETWORK_WIDGET_POLL_INTERVAL_MS, useNetwork } from './useNetwork'

// 인터페이스 하나의 높이와 간격(px). CSS(.widget-line 20px, .widget-item 간격 4px)와 맞춘다.
// 넓으면 이름 줄 + 속도 줄, 좁으면(2칸) 이름 줄 + 받기 줄 + 보내기 줄이다.
const ITEM_HEIGHT_WIDE = 44
const ITEM_HEIGHT_NARROW = 68
const ITEM_GAP = 8

// NetworkWidget은 물리 인터페이스별 수신·송신 속도다. 3칸 이상이면 링크 속도도 보여 주고,
// 2칸이면 받기·보내기를 두 줄로 나눈다.
export function NetworkWidget({ active, size }: WidgetViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data, status } = useNetwork(active, NETWORK_WIDGET_POLL_INTERVAL_MS)
  const list = data ? widgetInterfaces(data.response.interfaces) : []
  const wide = size.cols >= 3
  const capacity = widgetCapacity(size.rows, wide ? ITEM_HEIGHT_WIDE : ITEM_HEIGHT_NARROW, ITEM_GAP)
  const { shown, hidden } = fitList(list, capacity)

  return (
    <div className={status === 'disconnected' ? 'addon-widget stale' : 'addon-widget'}>
      {status === 'disconnected' && (
        <p className="widget-status" role="status">
          {t('widgets.disconnected')}
        </p>
      )}
      {data && list.length === 0 && <p className="widget-empty">{t('network.empty')}</p>}
      {shown.map((i) => {
        const rate = data?.rates.get(i.name)
        return (
          <div key={i.name} className="widget-item">
            <div className="widget-line">
              <span className="widget-name">{i.name}</span>
              {wide && i.speedMbps && <span className="widget-detail">{formatLinkSpeed(i.speedMbps, lang)}</span>}
            </div>
            {isUp(i) ? (
              <div className={wide ? 'widget-line net-rates' : 'net-rates stacked'}>
                <span aria-label={t('network.rx')} title={t('network.rx')}>
                  ↓ {rate ? formatRate(rate.rx, lang) : '—'}
                </span>
                <span aria-label={t('network.tx')} title={t('network.tx')}>
                  ↑ {rate ? formatRate(rate.tx, lang) : '—'}
                </span>
              </div>
            ) : (
              <div className="widget-line widget-detail-line">{t('network.down')}</div>
            )}
          </div>
        )
      })}
      {hidden > 0 && <p className="widget-more">{t('widgets.more', { count: hidden })}</p>}
    </div>
  )
}
