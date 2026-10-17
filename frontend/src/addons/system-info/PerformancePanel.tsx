import type { ParseKeys } from 'i18next'
import { type CSSProperties, type KeyboardEvent, type ReactNode, useId, useRef } from 'react'
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

export type Resource = 'cpu' | 'memory' | 'disk' | 'network'
const RESOURCES: readonly Resource[] = ['cpu', 'memory', 'disk', 'network']

const NAME_KEYS: Record<Resource, ParseKeys> = {
  cpu: 'sysinfo.cpu',
  memory: 'sysinfo.memory',
  disk: 'sysinfo.disk',
  network: 'sysinfo.network',
}

interface Props {
  sample: SystemStatsSample | null
  stale: boolean
  // 고른 자원. 창(SystemInfoWindow)이 들고 있어 프로세스 페이지에 다녀와도 남는다.
  resource: Resource
  onResource: (r: Resource) => void
}

interface Stat {
  label: string
  value: string
  // 두 계열 그래프의 계열 키(범례 역할)
  slot?: 1 | 2
}

// View는 자원 하나의 그래프·요약·세부 값이다.
interface View {
  series: ChartSeries[]
  max: number
  maxLabel: string
  format: (v: number) => string
  // 제목 오른쪽의 지금 값(계열이 하나일 때)
  headline?: string
  // 자원 목록의 둘째 줄
  summary: ReactNode
  // 자원 목록 항목의 접근성 이름에 붙일 지금 값
  summaryLabel: string
  stats: Stat[]
}

const pick = (points: readonly StatsPoint[], f: (p: StatsPoint) => number | null) => points.map(f)

// PerformancePanel은 성능 페이지다(Windows 11 작업 관리자 방식). 왼쪽 자원 목록(작은 그래프와 지금 값)에서
// 고른 자원을 오른쪽에 큰 그래프(최근 60초)와 세부 값으로 보여 준다.
export function PerformancePanel({ sample, stale, resource, onResource }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const id = useId()
  const tabRefs = useRef(new Map<Resource, HTMLButtonElement>())
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
  const none = '—'
  const pct = (v: number | null | undefined) => (v === null || v === undefined ? none : formatPercent(v, lang))
  const rate = (v: number | null | undefined) => (v === null || v === undefined ? none : formatRate(v, lang))
  const bytes = (v: number | undefined) => (v === undefined ? none : formatBytes(v, lang))
  const count = (v: number | undefined) => (v === undefined ? none : v.toLocaleString(lang))

  const percentView = (key: Resource, values: (number | null)[], v: number | null | undefined) => ({
    series: [{ id: key, label: t(NAME_KEYS[key]), slot: 1 as const, values }],
    max: 100,
    maxLabel: formatPercent(100, lang, 0),
    format: (x: number) => formatPercent(x, lang),
    headline: pct(v),
  })
  const rateView = (a: ChartSeries, b: ChartSeries, av: number | null | undefined, bv: number | null | undefined) => {
    const peak = Math.max(0, ...[...a.values, ...b.values].map((v) => v ?? 0))
    const max = niceMax(peak)
    return {
      series: [a, b],
      max,
      maxLabel: formatRate(max, lang, 0),
      format: (x: number) => formatRate(x, lang),
      // 두 값은 늘 두 줄로 둔다(값 길이에 따라 항목 높이가 바뀌지 않게).
      summary: (
        <>
          <span className="sysinfo-resource-rate">
            <span className="sysinfo-key s1" aria-hidden="true" />
            {rate(av)}
          </span>
          <span className="sysinfo-resource-rate">
            <span className="sysinfo-key s2" aria-hidden="true" />
            {rate(bv)}
          </span>
        </>
      ),
      summaryLabel: `${a.label} ${rate(av)}, ${b.label} ${rate(bv)}`,
      stats: [
        { label: a.label, value: rate(av), slot: 1 as const },
        { label: b.label, value: rate(bv), slot: 2 as const },
      ],
    }
  }

  const mem = res?.memory
  const views: Record<Resource, View> = {
    cpu: {
      ...percentView(
        'cpu',
        pick(points, (p) => p.cpu),
        point?.cpu,
      ),
      summary: pct(point?.cpu),
      summaryLabel: pct(point?.cpu),
      stats: [
        { label: t('sysinfo.perf.utilization'), value: pct(point?.cpu) },
        { label: t('sysinfo.perf.cores'), value: count(res?.cpu.cores.length) },
        { label: t('sysinfo.perf.processCount'), value: count(res?.tasks?.processes) },
        { label: t('sysinfo.perf.threads'), value: count(res?.tasks?.threads) },
        { label: t('sysinfo.uptime'), value: res ? formatUptime(res.uptimeSeconds, lang, 'narrow') : none },
      ],
    },
    memory: {
      ...percentView(
        'memory',
        pick(points, (p) => p.memory),
        point?.memory,
      ),
      // 둘째 줄은 사용량/전체다. 좁은 페이지(자원 목록이 한 줄)에서는 빼고 백분율만 보인다(.sysinfo-resource-extra).
      summary: mem ? (
        <>
          <span>{pct(point?.memory)}</span>
          <span className="sysinfo-resource-extra">
            {bytes(mem.usedBytes)} / {bytes(mem.totalBytes)}
          </span>
        </>
      ) : (
        none
      ),
      summaryLabel: mem ? `${pct(point?.memory)}, ${bytes(mem.usedBytes)} / ${bytes(mem.totalBytes)}` : none,
      stats: [
        { label: t('sysinfo.perf.inUse'), value: bytes(mem?.usedBytes) },
        { label: t('sysinfo.perf.available'), value: bytes(mem?.availableBytes) },
        { label: t('sysinfo.perf.total'), value: bytes(mem?.totalBytes) },
      ],
    },
    disk: rateView(
      { id: 'read', label: t('sysinfo.read'), slot: 1, values: pick(points, (p) => p.read) },
      { id: 'write', label: t('sysinfo.write'), slot: 2, values: pick(points, (p) => p.write) },
      point?.read,
      point?.write,
    ),
    network: rateView(
      { id: 'rx', label: t('sysinfo.receive'), slot: 1, values: pick(points, (p) => p.rx) },
      { id: 'tx', label: t('sysinfo.send'), slot: 2, values: pick(points, (p) => p.tx) },
      point?.rx,
      point?.tx,
    ),
  }

  const onListKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = RESOURCES.indexOf(resource)
    let next: Resource | null = null
    if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') next = RESOURCES[(i - 1 + RESOURCES.length) % RESOURCES.length]
    else if (e.key === 'ArrowDown' || e.key === 'ArrowRight') next = RESOURCES[(i + 1) % RESOURCES.length]
    else if (e.key === 'Home') next = RESOURCES[0]
    else if (e.key === 'End') next = RESOURCES[RESOURCES.length - 1]
    if (!next) return
    e.preventDefault()
    onResource(next)
    tabRefs.current.get(next)?.focus()
  }

  const view = views[resource]
  const name = t(NAME_KEYS[resource])
  const cores = res?.cpu.cores ?? []
  const perCore = resource === 'cpu' && cpuGraph === 'cores' && cores.length > 0

  return (
    <div className="sysinfo-perf">
      <div
        className="sysinfo-resources"
        role="tablist"
        aria-orientation="vertical"
        aria-label={t('sysinfo.perf.resources')}
        onKeyDown={onListKey}
      >
        {RESOURCES.map((r) => {
          const v = views[r]
          return (
            <button
              key={r}
              ref={(el) => {
                if (el) tabRefs.current.set(r, el)
                else tabRefs.current.delete(r)
              }}
              type="button"
              role="tab"
              id={`${id}-tab-${r}`}
              className="sysinfo-resource"
              aria-selected={r === resource}
              aria-controls={`${id}-panel`}
              aria-label={`${t(NAME_KEYS[r])}: ${v.summaryLabel}`}
              tabIndex={r === resource ? 0 : -1}
              onClick={() => onResource(r)}
            >
              <UsageChart
                times={times}
                series={v.series}
                now={now}
                max={v.max}
                format={v.format}
                decorative
                label=""
                stale={stale}
              />
              <span className="sysinfo-resource-text">
                <span className="sysinfo-resource-name">{t(NAME_KEYS[r])}</span>
                <span className="sysinfo-resource-value">{v.summary}</span>
              </span>
            </button>
          )
        })}
      </div>
      <div className="sysinfo-pane" role="tabpanel" id={`${id}-panel`} aria-labelledby={`${id}-tab-${resource}`}>
        <div className="sysinfo-pane-head">
          <h3 className="sysinfo-pane-title">{name}</h3>
          {view.headline && <span className="sysinfo-pane-value">{view.headline}</span>}
        </div>
        {/* CPU는 우클릭 메뉴(Shift+F10, 길게 누르기)로 전체 사용률과 코어별 그래프를 바꾼다. */}
        <div className="sysinfo-pane-chart" {...(resource === 'cpu' ? cpuMenu.handlers : {})}>
          {perCore ? (
            // 작은 그래프는 포커스를 받지 않으므로, 키보드로 메뉴를 열 자리와 값 요약을 격자에 둔다.
            <div
              className="sysinfo-cores"
              role="group"
              tabIndex={0}
              // 정사각형에 가까운 격자(4코어 2×2, 8코어 3열)로 그래프 영역을 채운다. 좁으면 CSS가 열을 줄인다.
              style={{ '--cores-cols': Math.ceil(Math.sqrt(cores.length)) } as CSSProperties}
              aria-label={cores
                .map((c) => `${t('sysinfo.core', { id: c.id })} ${pct(point?.cores.get(c.id))}`)
                .join(', ')}
            >
              {cores.map((c) => {
                const coreName = t('sysinfo.core', { id: c.id })
                const values = pick(points, (p) => p.cores.get(c.id) ?? null)
                return (
                  <div key={c.id} className="sysinfo-core">
                    <div className="sysinfo-core-head">
                      <span>{coreName}</span>
                      <span className="sysinfo-core-value">{pct(point?.cores.get(c.id))}</span>
                    </div>
                    <UsageChart
                      times={times}
                      series={[{ id: `core-${c.id}`, label: coreName, slot: 1, values }]}
                      now={now}
                      max={100}
                      format={(x) => formatPercent(x, lang)}
                      compact
                      label={`${coreName} ${pct(point?.cores.get(c.id))}`}
                      stale={stale}
                    />
                  </div>
                )
              })}
            </div>
          ) : (
            <UsageChart
              times={times}
              series={view.series}
              now={now}
              max={view.max}
              format={view.format}
              maxLabel={view.maxLabel}
              fill
              label={`${name}: ${view.summaryLabel}`}
              stale={stale}
            />
          )}
        </div>
        <dl className="sysinfo-stats">
          {view.stats.map((s) => (
            <div key={s.label} className="sysinfo-stat">
              <dt>
                {s.slot && <span className={`sysinfo-key s${s.slot}`} aria-hidden="true" />}
                {s.label}
              </dt>
              <dd>{s.value}</dd>
            </div>
          ))}
        </dl>
      </div>
    </div>
  )
}
