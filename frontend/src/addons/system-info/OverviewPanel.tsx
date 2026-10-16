import type { HTMLAttributes, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { useContextMenuTrigger } from '../../shell/useContextMenuTrigger'
import { formatBytes, formatPercent, formatRate, formatUptime } from '../common/format'
import { type ChartSeries, UsageChart } from './UsageChart'
import { niceMax } from './chart'
import { cpuGraphMenuItems } from './menus'
import { type StatsPoint, latest } from './stats'
import { useSysinfoPrefsStore } from './sysinfoPrefsStore'
import type { SystemStatsSample } from './useSystemStats'

interface Props {
  sample: SystemStatsSample | null
  stale: boolean
}

interface CardProps {
  title: string
  // 제목 오른쪽의 지금 값
  value?: ReactNode
  className?: string
  children: ReactNode
}

function Card({ title, value, className, children, ...rest }: CardProps & HTMLAttributes<HTMLElement>) {
  return (
    <section className={className ? `sysinfo-card ${className}` : 'sysinfo-card'} {...rest}>
      <header className="sysinfo-card-head">
        <h3 className="sysinfo-card-title">{title}</h3>
        {value !== undefined && <div className="sysinfo-card-value">{value}</div>}
      </header>
      {children}
    </section>
  )
}

// Legend는 두 계열 그래프의 범례와 지금 값이다(선 모양 키, 값 먼저).
function Legend({ items }: { items: { slot: 1 | 2; label: string; value: string }[] }) {
  return (
    <ul className="sysinfo-legend">
      {items.map((i) => (
        <li key={i.label}>
          <span className={`sysinfo-key s${i.slot}`} aria-hidden="true" />
          <span className="sysinfo-legend-value">{i.value}</span>
          <span className="sysinfo-legend-label">{i.label}</span>
        </li>
      ))}
    </ul>
  )
}

const pick = (points: readonly StatsPoint[], f: (p: StatsPoint) => number | null) => points.map(f)

// OverviewPanel은 개요 탭이다: 가동 시간과 CPU·메모리·네트워크·디스크 사용량 그래프(최근 60초).
export function OverviewPanel({ sample, stale }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const cpuGraph = useSysinfoPrefsStore((s) => s.cpuGraph)
  const cpuMenu = useContextMenuTrigger({
    items: () => {
      const s = useSysinfoPrefsStore.getState()
      return cpuGraphMenuItems(s.cpuGraph, (g) => s.set({ cpuGraph: g }), s.status !== 'ready')
    },
  })

  const res = sample?.response
  const history = sample?.history ?? null
  const points = history?.points ?? []
  const times = points.map((p) => p.t)
  const now = history?.last.sampledAtMs ?? 0
  const point = latest(history)
  const pct = (v: number | null | undefined) => (v === null || v === undefined ? '—' : formatPercent(v, lang))
  const rate = (v: number | null | undefined) => (v === null || v === undefined ? '—' : formatRate(v, lang))
  const percentChart = (id: string, label: string, values: (number | null)[], compact = false) => (
    <UsageChart
      times={times}
      series={[{ id, label, slot: 1, values }]}
      now={now}
      max={100}
      format={(v) => formatPercent(v, lang)}
      maxLabel={formatPercent(100, lang)}
      compact={compact}
      label={`${label} ${pct(values[values.length - 1])}`}
      stale={stale}
    />
  )
  const rateChart = (title: string, series: ChartSeries[]) => {
    const peak = Math.max(0, ...series.flatMap((s) => s.values.map((v) => v ?? 0)))
    const max = niceMax(peak)
    const summary = series.map((s) => `${s.label} ${rate(s.values[s.values.length - 1])}`).join(', ')
    return (
      <UsageChart
        times={times}
        series={series}
        now={now}
        max={max}
        format={(v) => formatRate(v, lang)}
        maxLabel={formatRate(max, lang)}
        label={`${title}: ${summary}`}
        stale={stale}
      />
    )
  }

  const cores = res?.cpu.cores ?? []
  const perCore = cpuGraph === 'cores' && cores.length > 0
  const mem = res?.memory

  return (
    <div className="sysinfo-overview">
      <p className="sysinfo-uptime">
        <span className="sysinfo-uptime-label">{t('sysinfo.uptime')}</span>
        <span>{res ? formatUptime(res.uptimeSeconds, lang) : '—'}</span>
      </p>
      <div className="sysinfo-cards">
        <Card
          title={t('sysinfo.cpu')}
          value={pct(point?.cpu)}
          className={perCore ? 'wide' : undefined}
          {...cpuMenu.handlers}
        >
          {perCore ? (
            <div className="sysinfo-cores">
              {cores.map((c) => {
                const values = pick(points, (p) => p.cores.get(c.id) ?? null)
                const name = t('sysinfo.core', { id: c.id })
                return (
                  <div key={c.id} className="sysinfo-core">
                    <div className="sysinfo-core-head">
                      <span>{name}</span>
                      <span className="sysinfo-core-value">{pct(point?.cores.get(c.id))}</span>
                    </div>
                    {percentChart(`core-${c.id}`, name, values, true)}
                  </div>
                )
              })}
            </div>
          ) : (
            percentChart(
              'cpu',
              t('sysinfo.cpu'),
              pick(points, (p) => p.cpu),
            )
          )}
        </Card>
        <Card title={t('sysinfo.memory')} value={pct(point?.memory)}>
          <div className="sysinfo-card-detail">
            {mem ? `${formatBytes(mem.usedBytes, lang)} / ${formatBytes(mem.totalBytes, lang)}` : '—'}
          </div>
          {percentChart(
            'memory',
            t('sysinfo.memory'),
            pick(points, (p) => p.memory),
          )}
        </Card>
        <Card title={t('sysinfo.network')}>
          <Legend
            items={[
              { slot: 1, label: t('sysinfo.receive'), value: rate(point?.rx) },
              { slot: 2, label: t('sysinfo.send'), value: rate(point?.tx) },
            ]}
          />
          {rateChart(t('sysinfo.network'), [
            { id: 'rx', label: t('sysinfo.receive'), slot: 1, values: pick(points, (p) => p.rx) },
            { id: 'tx', label: t('sysinfo.send'), slot: 2, values: pick(points, (p) => p.tx) },
          ])}
        </Card>
        <Card title={t('sysinfo.disk')}>
          <Legend
            items={[
              { slot: 1, label: t('sysinfo.read'), value: rate(point?.read) },
              { slot: 2, label: t('sysinfo.write'), value: rate(point?.write) },
            ]}
          />
          {rateChart(t('sysinfo.disk'), [
            { id: 'read', label: t('sysinfo.read'), slot: 1, values: pick(points, (p) => p.read) },
            { id: 'write', label: t('sysinfo.write'), slot: 2, values: pick(points, (p) => p.write) },
          ])}
        </Card>
      </div>
    </div>
  )
}
