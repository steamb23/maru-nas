import type { ParseKeys } from 'i18next'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { MemoryDetail } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { formatBytes, formatPercent } from '../common/format'
import { MEMORY_PARTS, type MemoryPart, type MemorySegment, type MemorySlot, memorySegments } from './memory'

const PART_KEYS: Record<MemoryPart, ParseKeys> = {
  apps: 'sysinfo.perf.apps',
  shared: 'sysinfo.perf.shared',
  compressed: 'sysinfo.perf.compressed',
  buffers: 'sysinfo.perf.buffers',
  cache: 'sysinfo.perf.cache',
  free: 'sysinfo.perf.free',
}

const swatchClass = (slot: MemorySlot | null) => `sysinfo-key swatch ${slot ? `s${slot}` : 'free'}`

interface Props {
  // 메모리 구성과 전체 크기. 첫 응답 전에는 없다.
  detail: MemoryDetail | undefined
  total: number | undefined
}

// MemoryComposition은 성능 페이지 메모리의 구성 막대(지금 값)와 항목별 크기다(작업 관리자의 메모리 구성).
// 막대 조각에 포인터를 올리면 이름·크기·비율을 보이고, 키보드·화면 낭독기는 막대의 접근성 이름과 아래 목록으로 읽는다.
export function MemoryComposition({ detail, total }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const [hover, setHover] = useState<MemoryPart | null>(null)

  const segments = detail && total ? memorySegments(detail, total) : []
  const name = (part: MemoryPart) => t(PART_KEYS[part])
  const describe = (s: MemorySegment) => `${name(s.part)} ${formatBytes(s.bytes, lang)} (${formatPercent(s.percent, lang)})`
  const active = segments.find((s) => s.part === hover)

  return (
    <div className="sysinfo-memmap">
      <div className="sysinfo-memmap-title" aria-hidden="true">
        {t('sysinfo.perf.composition')}
      </div>
      <div className="sysinfo-memmap-plot" onPointerLeave={() => setHover(null)}>
        <div
          className="sysinfo-memmap-bar"
          role="img"
          aria-label={[t('sysinfo.perf.composition'), ...segments.map(describe)].join(', ')}
        >
          {segments.map((s) => (
            <div
              key={s.part}
              className={`sysinfo-memmap-seg ${s.slot ? `s${s.slot}` : 'free'}${s.part === hover ? ' hover' : ''}`}
              // 조각 사이 틈을 뺀 나머지를 크기에 비례해 나눈다.
              style={{ flexGrow: s.percent }}
              onPointerEnter={() => setHover(s.part)}
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
          >
            <div className="sysinfo-tooltip-row">
              <span className={swatchClass(active.slot)} aria-hidden="true" />
              <strong>{formatBytes(active.bytes, lang)}</strong>
              <span className="sysinfo-tooltip-label">
                {name(active.part)} · {formatPercent(active.percent, lang)}
              </span>
            </div>
          </div>
        )}
      </div>
      <dl className="sysinfo-stats">
        {MEMORY_PARTS.map((p) => (
          <div key={p.part} className="sysinfo-stat">
            <dt>
              <span className={swatchClass(p.slot)} aria-hidden="true" />
              {name(p.part)}
            </dt>
            <dd>{detail ? formatBytes(detail[p.key], lang) : '—'}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
