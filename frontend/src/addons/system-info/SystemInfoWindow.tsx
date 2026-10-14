import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import type { WindowViewProps } from '../types'
import { formatBytes, formatPercent, formatUptime, percentOf } from '../common/format'
import { useSystemInfo } from './useSystemInfo'


interface MeterProps {
  label: string
  percent: number | null
  detail?: string
}

function Meter({ label, percent, detail }: MeterProps) {
  const lang = useLanguage()
  return (
    <div className="sysinfo-row">
      <dt>{label}</dt>
      <dd>
        <div className="sysinfo-value">
          <span>{percent === null ? '—' : formatPercent(percent, lang)}</span>
          {detail && <span className="sysinfo-detail">{detail}</span>}
        </div>
        <div className="sysinfo-bar" aria-hidden="true">
          <div className="sysinfo-bar-fill" style={{ width: `${percent ?? 0}%` }} />
        </div>
      </dd>
    </div>
  )
}

export function SystemInfoWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data, status } = useSystemInfo(active)
  const mem = data?.memory

  return (
    <div className="sysinfo">
      {status === 'disconnected' && (
        <p className="sysinfo-banner" role="status">
          {t('sysinfo.disconnected')}
        </p>
      )}
      <dl className={status === 'disconnected' ? 'sysinfo-list stale' : 'sysinfo-list'}>
        <Meter label={t('sysinfo.cpu')} percent={data ? data.cpu.usagePercent : null} />
        <Meter
          label={t('sysinfo.memory')}
          percent={mem ? percentOf(mem.usedBytes, mem.totalBytes) : null}
          detail={mem ? `${formatBytes(mem.usedBytes, lang)} / ${formatBytes(mem.totalBytes, lang)}` : undefined}
        />
        <div className="sysinfo-row">
          <dt>{t('sysinfo.uptime')}</dt>
          <dd className="sysinfo-value">{data ? formatUptime(data.uptimeSeconds, lang) : '—'}</dd>
        </div>
      </dl>
    </div>
  )
}
