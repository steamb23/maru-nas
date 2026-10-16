import { type KeyboardEvent, type PointerEvent, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { formatUnit } from '../common/format'
import { type ChartFrame, areaPath, linePath, nearestIndex, pointX, stepIndex } from './chart'
import { HISTORY_MS } from './stats'

export interface ChartSeries {
  id: string
  // 범례·툴팁의 이름. 계열이 하나면 카드 제목이 이름이므로 툴팁에 쓰지 않는다.
  label: string
  // 계열 색 자리(system-info.css의 --sysinfo-series-1, -2). 색은 계열을 따라가고 순위를 따르지 않는다.
  slot: 1 | 2
  // times와 같은 순서의 값. 구할 수 없는 점은 null이다(선을 끊는다).
  values: readonly (number | null)[]
}

interface Props {
  times: readonly number[]
  series: readonly ChartSeries[]
  // 오른쪽 끝(가장 최근 샘플) 시각
  now: number
  // y축 최댓값(0부터)
  max: number
  // 툴팁 값 표시
  format: (v: number) => string
  // 맨 위 격자선의 값 표시. compact면 쓰지 않는다.
  maxLabel?: string
  // 축·키보드 이동이 없는 작은 그래프(코어별). 툴팁은 있다.
  compact?: boolean
  // 꾸밈용 작은 그래프(성능 페이지의 자원 목록). compact에 더해 툴팁과 접근성 이름이 없다(값은 옆 글자로 읽는다).
  decorative?: boolean
  // 높이를 부모에 맞춰 채운다(성능 페이지의 큰 그래프). 없으면 CSS의 고정 높이다.
  fill?: boolean
  // 지금 값을 담은 접근성 이름. decorative면 쓰지 않는다.
  label: string
  // 연결이 끊겨 마지막 값을 흐리게 보여 주는 중인지
  stale?: boolean
}

// 십자선 위치. 포인터는 x 좌표를 기억해 새 샘플이 와도 같은 자리의 가장 가까운 점에 붙고,
// 키보드는 고른 점의 시각을 기억한다(오래되어 빠지면 사라진다).
type Hover = { by: 'pointer'; x: number } | { by: 'key'; t: number } | null

// UsageChart는 최근 60초 사용량 선 그래프다. 계열이 하나면 선 아래를 옅게 채운다.
export function UsageChart(props: Props) {
  const { times, series, now, max, format, maxLabel, decorative, fill, label, stale } = props
  const compact = props.compact || decorative
  const { t } = useTranslation()
  const lang = useLanguage()
  const plotRef = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState<{ width: number; height: number } | null>(null)
  const [hover, setHover] = useState<Hover>(null)

  useLayoutEffect(() => {
    const el = plotRef.current
    if (!el) return
    const measure = () => setSize({ width: el.clientWidth, height: el.clientHeight })
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const frame: ChartFrame | null =
    size && size.width > 0 && size.height > 0
      ? { now, span: HISTORY_MS, max: max > 0 ? max : 1, width: size.width, height: size.height }
      : null
  const samples = (s: ChartSeries) => times.map((ti, i) => ({ t: ti, v: s.values[i] ?? null }))

  let index = -1
  if (frame && hover?.by === 'pointer') index = nearestIndex(times, frame, hover.x)
  if (hover?.by === 'key') index = times.indexOf(hover.t)
  const hoverX = frame && index >= 0 ? pointX(frame, times[index]) : 0

  const onPointerMove = (e: PointerEvent<HTMLDivElement>) => {
    if (e.pointerType === 'touch' && e.buttons === 0) return
    const rect = e.currentTarget.getBoundingClientRect()
    setHover({ by: 'pointer', x: e.clientX - rect.left })
  }
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!frame) return
    if (e.key === 'Escape' && hover) {
      e.stopPropagation()
      setHover(null)
      return
    }
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const next = stepIndex(times, frame, index, e.key === 'ArrowLeft' ? -1 : 1)
    setHover(next >= 0 ? { by: 'key', t: times[next] } : null)
  }

  const ago = index >= 0 ? Math.round((now - times[index]) / 1000) : 0
  const single = series.length === 1
  const rootClass = ['sysinfo-chart', compact && 'compact', decorative && 'decorative', fill && 'fill', stale && 'stale']
    .filter(Boolean)
    .join(' ')

  return (
    <div className={rootClass} aria-hidden={decorative || undefined}>
      <div
        ref={plotRef}
        className="sysinfo-chart-plot"
        role={decorative ? undefined : 'img'}
        aria-label={decorative ? undefined : label}
        tabIndex={compact ? undefined : 0}
        onPointerMove={decorative ? undefined : onPointerMove}
        onPointerLeave={() => setHover((h) => (h?.by === 'pointer' ? null : h))}
        onKeyDown={compact ? undefined : onKeyDown}
        onBlur={() => setHover((h) => (h?.by === 'key' ? null : h))}
      >
        {frame && (
          <svg width={frame.width} height={frame.height} aria-hidden="true">
            {!compact && (
              <>
                <line className="sysinfo-grid" x1={0} x2={frame.width} y1={0.5} y2={0.5} />
                <line className="sysinfo-grid" x1={0} x2={frame.width} y1={frame.height / 2} y2={frame.height / 2} />
              </>
            )}
            <line
              className="sysinfo-grid"
              x1={0}
              x2={frame.width}
              y1={frame.height - 0.5}
              y2={frame.height - 0.5}
            />
            {single && <path className="sysinfo-area s1" d={areaPath(samples(series[0]), frame)} />}
            {series.map((s) => (
              <path key={s.id} className={`sysinfo-line s${s.slot}`} d={linePath(samples(s), frame)} />
            ))}
            {index >= 0 && (
              <>
                <line className="sysinfo-crosshair" x1={hoverX} x2={hoverX} y1={0} y2={frame.height} />
                {series.map((s) => {
                  const v = s.values[index]
                  if (v === null || v === undefined) return null
                  const y = frame.height - (Math.min(Math.max(v, 0), frame.max) / frame.max) * frame.height
                  return <circle key={s.id} className={`sysinfo-dot s${s.slot}`} cx={hoverX} cy={y} r={4} />
                })}
              </>
            )}
          </svg>
        )}
        {!compact && maxLabel && <span className="sysinfo-chart-max">{maxLabel}</span>}
        {!compact && times.length < 2 && <span className="sysinfo-chart-empty">{t('sysinfo.chart.collecting')}</span>}
        {frame && index >= 0 && (
          <div
            className="sysinfo-tooltip"
            style={hoverX > frame.width / 2 ? { right: frame.width - hoverX + 8 } : { left: hoverX + 8 }}
            aria-live={hover?.by === 'key' ? 'polite' : 'off'}
          >
            <div className="sysinfo-tooltip-time">
              {ago <= 0 ? t('sysinfo.chart.now') : t('sysinfo.chart.ago', { time: formatUnit(ago, 'second', lang, 'narrow') })}
            </div>
            {series.map((s) => {
              const v = s.values[index]
              return (
                <div key={s.id} className="sysinfo-tooltip-row">
                  <span className={`sysinfo-key s${s.slot}`} aria-hidden="true" />
                  <strong>{v === null || v === undefined ? '—' : format(v)}</strong>
                  {!single && <span className="sysinfo-tooltip-label">{s.label}</span>}
                </div>
              )
            })}
          </div>
        )}
      </div>
      {!compact && (
        <div className="sysinfo-chart-axis" aria-hidden="true">
          <span>{t('sysinfo.chart.ago', { time: formatUnit(HISTORY_MS / 1000, 'second', lang, 'narrow') })}</span>
          <span>{t('sysinfo.chart.now')}</span>
        </div>
      )}
    </div>
  )
}
