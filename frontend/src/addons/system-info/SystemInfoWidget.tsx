import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { formatBytes, formatPercent, formatUptime, percentOf } from '../common/format'
import type { WidgetViewProps } from '../types'
import { SYSTEM_WIDGET_POLL_INTERVAL_MS, useSystemInfo } from './useSystemInfo'

interface MeterProps {
  label: string
  percent: number | null
  detail?: string
}

function Meter({ label, percent, detail }: MeterProps) {
  const lang = useLanguage()
  return (
    <div className="sysinfo-widget-meter">
      <div className="sysinfo-widget-line">
        <span className="sysinfo-widget-label">{label}</span>
        {detail && <span className="sysinfo-detail">{detail}</span>}
        <span className="sysinfo-widget-value">{percent === null ? '—' : formatPercent(percent, lang)}</span>
      </div>
      <div className="sysinfo-bar" aria-hidden="true">
        <div className="sysinfo-bar-fill" style={{ width: `${percent ?? 0}%` }} />
      </div>
    </div>
  )
}

// SystemInfoWidget은 CPU·메모리 사용률과 가동 시간 요약이다. 자세한 내용은 창 보기에 있다.
// 2칸 너비에서는 메모리 용량(사용/전체)을 빼고, 가동 시간은 라벨과 값을 두 줄로 나눈다.
export function SystemInfoWidget({ active, size }: WidgetViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data, status } = useSystemInfo(active, SYSTEM_WIDGET_POLL_INTERVAL_MS)
  const mem = data?.memory
  const disconnected = status === 'disconnected'
  const narrow = size.cols < 3

  return (
    <div className={disconnected ? 'sysinfo-widget stale' : 'sysinfo-widget'}>
      <Meter label={t('sysinfo.cpu')} percent={data ? data.cpu.usagePercent : null} />
      <Meter
        label={t('sysinfo.memory')}
        percent={mem ? percentOf(mem.usedBytes, mem.totalBytes) : null}
        detail={mem && !narrow ? `${formatBytes(mem.usedBytes, lang)} / ${formatBytes(mem.totalBytes, lang)}` : undefined}
      />
      {disconnected ? (
        <p className="widget-status" role="status">
          {t('widgets.disconnected')}
        </p>
      ) : (
        <div className={narrow ? 'sysinfo-widget-line stacked' : 'sysinfo-widget-line'}>
          <span className="sysinfo-widget-label">{t('sysinfo.uptime')}</span>
          <span className="sysinfo-widget-uptime">{data ? formatUptime(data.uptimeSeconds, lang, 'narrow') : '—'}</span>
        </div>
      )}
    </div>
  )
}
