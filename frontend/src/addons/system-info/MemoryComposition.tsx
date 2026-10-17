import type { ParseKeys } from 'i18next'
import { type KeyboardEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { MemoryDetail } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { formatBytes, formatPercent } from '../common/format'
import { type MemoryPart, type MemorySegment, memorySegments, stepSegment } from './memory'

const PART_KEYS: Record<MemoryPart, ParseKeys> = {
  apps: 'sysinfo.perf.apps',
  shared: 'sysinfo.perf.shared',
  compressed: 'sysinfo.perf.compressed',
  buffers: 'sysinfo.perf.buffers',
  cache: 'sysinfo.perf.cache',
  free: 'sysinfo.perf.free',
}

// 툴팁을 띄운 조각. 포인터는 올린 조각, 키보드는 좌우 화살표로 고른 조각이다(UsageChart의 십자선과 같은 방식).
type Hover = { by: 'pointer' | 'key'; part: MemoryPart } | null

interface Props {
  // 메모리 구성과 전체 크기. 첫 응답 전에는 없다.
  detail: MemoryDetail | undefined
  total: number | undefined
}

// MemoryComposition은 성능 페이지 메모리의 구성 막대(지금 값)다(작업 관리자의 메모리 구성).
// 조각의 이름·크기·비율은 툴팁으로 보인다: 포인터를 올리거나, 막대에 포커스를 두고 좌우 화살표로 고른다.
// 화면 낭독기는 모든 조각을 담은 막대의 접근성 이름으로 읽는다.
export function MemoryComposition({ detail, total }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const [hover, setHover] = useState<Hover>(null)

  const segments = detail && total ? memorySegments(detail, total) : []
  const name = (part: MemoryPart) => t(PART_KEYS[part])
  const describe = (s: MemorySegment) => `${name(s.part)} ${formatBytes(s.bytes, lang)} (${formatPercent(s.percent, lang)})`
  const active = segments.find((s) => s.part === hover?.part)

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape' && hover) {
      e.stopPropagation()
      setHover(null)
      return
    }
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const next = stepSegment(segments, hover?.part ?? null, e.key === 'ArrowLeft' ? -1 : 1)
    setHover(next ? { by: 'key', part: next } : null)
  }

  return (
    <div className="sysinfo-memmap">
      <div className="sysinfo-memmap-title" aria-hidden="true">
        {t('sysinfo.perf.composition')}
      </div>
      <div className="sysinfo-memmap-plot" onPointerLeave={() => setHover((h) => (h?.by === 'pointer' ? null : h))}>
        <div
          className="sysinfo-memmap-bar"
          role="img"
          aria-label={[t('sysinfo.perf.composition'), ...segments.map(describe)].join(', ')}
          tabIndex={0}
          onKeyDown={onKeyDown}
          onBlur={() => setHover((h) => (h?.by === 'key' ? null : h))}
        >
          {segments.map((s) => (
            <div
              key={s.part}
              className={`sysinfo-memmap-seg ${s.slot ? `s${s.slot}` : 'free'}${s.part === active?.part ? ' hover' : ''}`}
              // 조각 사이 틈을 뺀 나머지를 크기에 비례해 나눈다.
              style={{ flexGrow: s.percent }}
              onPointerEnter={() => setHover({ by: 'pointer', part: s.part })}
            />
          ))}
        </div>
        {active && (
          <div
            className="sysinfo-tooltip sysinfo-memmap-tip"
            style={
              active.start + active.percent / 2 > 50
                ? { right: `${100 - active.start - active.percent}%` }
                : { left: `${active.start}%` }
            }
            aria-live={hover?.by === 'key' ? 'polite' : 'off'}
          >
            <div className="sysinfo-tooltip-row">
              <span className={`sysinfo-key swatch ${active.slot ? `s${active.slot}` : 'free'}`} aria-hidden="true" />
              <strong>{formatBytes(active.bytes, lang)}</strong>
              <span className="sysinfo-tooltip-label">
                {name(active.part)} · {formatPercent(active.percent, lang)}
              </span>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
