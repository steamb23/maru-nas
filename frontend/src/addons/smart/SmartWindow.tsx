import { useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { SmartDisk } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { useCapabilitiesStore } from '../capabilities'
import { formatBytes, formatCelsius, formatDateTime } from '../common/format'
import type { WindowViewProps } from '../types'
import { SmartDetailView } from './SmartDetail'
import { sortDisks, statusLevel } from './smart'
import { useSmartDetails } from './useSmartDetail'
import { useRefreshReady, useSmart } from './useSmart'

// 이보다 좁은 창은 목록과 상세를 한 번에 하나만 보인다(디스크를 고르면 상세, "디스크 목록"으로 돌아감).
const SMART_NARROW_WIDTH = 600

function DiskRow({ disk, selected, onSelect }: { disk: SmartDisk; selected: boolean; onSelect: () => void }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const level = statusLevel(disk.status)
  return (
    <button type="button" className="smart-disk" aria-current={selected ? 'true' : undefined} onClick={onSelect}>
      <span className={`status-dot ${level}`} aria-hidden="true" />
      <span className="smart-disk-text">
        <span className="smart-disk-line">
          <span className="smart-disk-name">{disk.device}</span>
          {disk.status !== 'good' && <span className={`badge level-${level}`}>{t(`smart.status.${disk.status}`)}</span>}
          {!disk.monitored && <span className="badge">{t('smart.notMonitored')}</span>}
          {disk.celsius !== undefined && <span className="smart-disk-temp">{formatCelsius(disk.celsius, lang)}</span>}
        </span>
        <span className="smart-disk-model">
          {disk.model}
          {disk.sizeBytes > 0 && ` · ${formatBytes(disk.sizeBytes, lang)}`}
        </span>
      </span>
    </button>
  )
}

// SmartWindow는 왼쪽에 디스크 목록, 오른쪽에 고른 디스크의 SMART 상세를 보인다. 목록은 백엔드가 보관한 결과이며
// 새로고침 단추로 다시 가져온다(디스크를 깨우므로 자동으로는 하루 한 번만 다시 가져온다). 상세는 디스크를 고를
// 때만 읽으므로 처음에는 아무 디스크도 고르지 않는다.
export function SmartWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data: sample, status, refresh, refreshing } = useSmart(active)
  const data = sample?.response
  const disks = data ? sortDisks(data.disks) : []
  // 마지막 확인 뒤 1분 동안은 백엔드가 다시 가져오지 않으므로 버튼을 막아 둔다.
  const ready = useRefreshReady(sample?.refreshAfterMs)
  const canRefresh = !refreshing && ready && status !== 'loading'
  const workbench = useCapabilitiesStore((s) => Boolean(s.caps?.workbenchUrl))

  const rootRef = useRef<HTMLDivElement>(null)
  const [narrow, setNarrow] = useState(false)
  useLayoutEffect(() => {
    const el = rootRef.current
    if (!el) return
    const measure = () => setNarrow(el.clientWidth < SMART_NARROW_WIDTH)
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const [selected, setSelected] = useState<string | null>(null)
  // 목록에서 사라진 디스크는 고르지 않은 것으로 본다.
  const disk = selected === null ? undefined : disks.find((d) => d.device === selected)
  const details = useSmartDetails(disk ? disk.device : null)
  const showList = !narrow || !disk
  const showDetail = !narrow || disk !== undefined

  return (
    <div ref={rootRef} className={narrow ? 'smart-window narrow' : 'smart-window'}>
      {showList && (
        <nav className="smart-list" aria-label={t('smart.detail.listLabel')}>
          <div className="smart-list-head">
            <span className="addon-card-sub">
              {data ? t('smart.checkedAt', { time: formatDateTime(data.checkedAt, lang) }) : t('smart.checking')}
            </span>
            <button type="button" className="addon-button" onClick={refresh} disabled={!canRefresh}>
              {refreshing ? t('smart.refreshing') : t('smart.refresh')}
            </button>
          </div>
          {status === 'disconnected' && (
            <p className="addon-banner" role="status">
              {t('addonWindow.disconnected')}
            </p>
          )}
          {data && disks.length === 0 && <p className="addon-empty">{t('smart.noDisks')}</p>}
          <ul className={status === 'disconnected' ? 'smart-disks stale' : 'smart-disks'}>
            {disks.map((d) => (
              <li key={d.device}>
                <DiskRow disk={d} selected={d.device === disk?.device} onSelect={() => setSelected(d.device)} />
              </li>
            ))}
          </ul>
          <p className="smart-list-hint">{t('smart.wakeHint')}</p>
        </nav>
      )}
      {showDetail && (
        <section className="smart-detail" aria-label={disk ? disk.device : undefined}>
          {disk && details.state ? (
            <SmartDetailView
              key={disk.device}
              disk={disk}
              state={details.state}
              onReload={details.reload}
              onBack={narrow ? () => setSelected(null) : undefined}
              onRefreshList={canRefresh ? refresh : undefined}
              workbench={workbench}
            />
          ) : (
            <div className="smart-state">
              <p className="smart-state-title">{t('smart.detail.pick')}</p>
              <p className="smart-state-text">{t('smart.detail.pickHint')}</p>
            </div>
          )}
        </section>
      )}
    </div>
  )
}
