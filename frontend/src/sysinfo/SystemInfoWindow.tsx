import { formatBytes, formatPercent, formatUptime, percentOf } from './format'
import { useSystemInfo } from './useSystemInfo'

interface Props {
  // 창이 최소화되면 false. 이때는 갱신 요청을 멈춘다.
  active: boolean
}

interface MeterProps {
  label: string
  percent: number | null
  detail?: string
}

function Meter({ label, percent, detail }: MeterProps) {
  return (
    <div className="sysinfo-row">
      <dt>{label}</dt>
      <dd>
        <div className="sysinfo-value">
          <span>{percent === null ? '—' : formatPercent(percent)}</span>
          {detail && <span className="sysinfo-detail">{detail}</span>}
        </div>
        <div className="sysinfo-bar" aria-hidden="true">
          <div className="sysinfo-bar-fill" style={{ width: `${percent ?? 0}%` }} />
        </div>
      </dd>
    </div>
  )
}

export function SystemInfoWindow({ active }: Props) {
  const { data, status } = useSystemInfo(active)
  const mem = data?.memory

  return (
    <div className="sysinfo">
      {status === 'disconnected' && (
        <p className="sysinfo-banner" role="status">
          연결 끊김 — 다시 연결하는 중…
        </p>
      )}
      <dl className={status === 'disconnected' ? 'sysinfo-list selectable stale' : 'sysinfo-list selectable'}>
        <Meter label="CPU" percent={data ? data.cpu.usagePercent : null} />
        <Meter
          label="메모리"
          percent={mem ? percentOf(mem.usedBytes, mem.totalBytes) : null}
          detail={mem ? `${formatBytes(mem.usedBytes)} / ${formatBytes(mem.totalBytes)}` : undefined}
        />
        <div className="sysinfo-row">
          <dt>가동 시간</dt>
          <dd className="sysinfo-value">{data ? formatUptime(data.uptimeSeconds) : '—'}</dd>
        </div>
      </dl>
    </div>
  )
}
