import { useTranslation } from 'react-i18next'
import type { SmartDisk } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { formatBytes, formatCelsius, formatDateTime } from '../common/format'
import type { WindowViewProps } from '../types'
import { sortDisks, statusLevel } from './smart'
import { useRefreshReady, useSmart } from './useSmart'

function DiskCard({ disk }: { disk: SmartDisk }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const level = statusLevel(disk.status)
  return (
    <li className="addon-card">
      <div className="addon-card-head">
        <span className={`status-dot ${level}`} aria-hidden="true" />
        <span className="addon-card-title">{disk.device}</span>
        <span className={`badge level-${level}`}>{t(`smart.status.${disk.status}`)}</span>
        {!disk.monitored && <span className="badge">{t('smart.notMonitored')}</span>}
        {disk.celsius !== undefined && <span className="addon-card-value">{formatCelsius(disk.celsius, lang)}</span>}
      </div>
      <div className="addon-card-sub">
        {disk.model}
        {disk.sizeBytes > 0 && ` · ${formatBytes(disk.sizeBytes, lang)}`}
      </div>
      {disk.status !== 'good' && <div className="addon-card-sub selectable">{disk.overallStatus}</div>}
    </li>
  )
}

// SmartWindow는 디스크별 SMART 상태다. 결과는 백엔드가 보관한 것이며 새로고침 버튼으로 다시 가져온다
// (디스크를 깨우므로 자동으로는 하루 한 번만 다시 가져온다).
export function SmartWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data: sample, status, refresh, refreshing } = useSmart(active)
  const data = sample?.response
  const disks = data ? sortDisks(data.disks) : []
  // 마지막 확인 뒤 1분 동안은 백엔드가 다시 가져오지 않으므로 버튼을 막아 둔다.
  const ready = useRefreshReady(sample?.refreshAfterMs)

  return (
    <div className="addon-window">
      {status === 'disconnected' && (
        <p className="addon-banner" role="status">
          {t('addonWindow.disconnected')}
        </p>
      )}
      <div className="smart-toolbar">
        <span className="addon-card-sub">
          {data ? t('smart.checkedAt', { time: formatDateTime(data.checkedAt, lang) }) : t('smart.checking')}
        </span>
        <button type="button" className="addon-button" onClick={refresh} disabled={refreshing || !ready || status === 'loading'}>
          {refreshing ? t('smart.refreshing') : t('smart.refresh')}
        </button>
      </div>
      <p className="addon-hint">{t('smart.wakeHint')}</p>
      {data && disks.length === 0 && <p className="addon-empty">{t('smart.noDisks')}</p>}
      <ul className={status === 'disconnected' ? 'addon-list stale' : 'addon-list'}>
        {disks.map((d) => (
          <DiskCard key={d.device} disk={d} />
        ))}
      </ul>
    </div>
  )
}
