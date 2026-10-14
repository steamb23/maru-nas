import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { formatBytes, formatPercent } from '../common/format'
import { UsageBar } from '../common/UsageBar'
import { widgetCapacity } from '../common/widgetRows'
import type { WidgetViewProps } from '../types'
import { fsName, usedPercent, widgetFilesystems } from './storage'
import { STORAGE_WIDGET_POLL_INTERVAL_MS, useStorage } from './useStorage'

// 파일시스템 한 줄(이름·값 줄 + 막대)의 높이와 줄 사이 간격(px). CSS(.storage-widget)와 맞춘다.
const ITEM_HEIGHT = 30
const ITEM_GAP = 8

// StorageWidget은 파일시스템별 사용률 막대다. 3칸 이상이면 사용량/전체 용량도 보여 준다.
// 다 들어가지 않으면 마지막 줄에 나머지 개수를 적는다.
export function StorageWidget({ active, size }: WidgetViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data, status } = useStorage(active, STORAGE_WIDGET_POLL_INTERVAL_MS)
  const list = data ? widgetFilesystems(data.filesystems) : []
  const capacity = widgetCapacity(size.rows, ITEM_HEIGHT, ITEM_GAP)
  const shown = list.length > capacity ? list.slice(0, capacity - 1) : list
  const hidden = list.length - shown.length
  const wide = size.cols >= 3

  return (
    <div className={status === 'disconnected' ? 'addon-widget storage-widget stale' : 'addon-widget storage-widget'}>
      {status === 'disconnected' && (
        <p className="widget-status" role="status">
          {t('widgets.disconnected')}
        </p>
      )}
      {data && list.length === 0 && <p className="widget-empty">{t('storage.empty')}</p>}
      {shown.map((fs) => {
        const name = fsName(fs)
        const pct = usedPercent(fs)
        return (
          <div key={fs.mountPoint} className="widget-item">
            <div className="widget-line">
              <span className="widget-name">{name.kind === 'root' ? t('storage.root') : name.text}</span>
              {wide && (
                <span className="widget-detail">
                  {formatBytes(fs.usedBytes, lang)} / {formatBytes(fs.totalBytes, lang)}
                </span>
              )}
              <span className="widget-value">{formatPercent(pct, lang)}</span>
            </div>
            <UsageBar percent={pct} />
          </div>
        )
      })}
      {hidden > 0 && <p className="widget-more">{t('widgets.more', { count: hidden })}</p>}
    </div>
  )
}
