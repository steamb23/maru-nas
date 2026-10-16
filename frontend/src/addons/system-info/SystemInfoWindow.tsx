import { type KeyboardEvent, useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '../../auth/store'
import type { WindowViewProps } from '../types'
import { OverviewPanel } from './OverviewPanel'
import { ProcessesPanel } from './ProcessesPanel'
import { useSysinfoPrefsStore } from './sysinfoPrefsStore'
import { useSystemStats } from './useSystemStats'

type Tab = 'overview' | 'processes'
const TABS: readonly Tab[] = ['overview', 'processes']

// 보기 설정을 불러오지 못했을 때 다시 시도하는 간격
const PREFS_RETRY_MS = 10_000

// SystemInfoWindow는 시스템 정보 창이다. 관리자에게는 [개요] [프로세스] 탭을, 그 밖의 사용자에게는 개요만 보여 준다.
// 데이터(기록 포함)는 탭의 부모인 여기서 받으므로 탭을 바꿔도 그래프 기록과 프로세스 CPU 값이 이어진다.
export function SystemInfoWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const admin = useAuthStore((s) => s.user?.admin === true)
  const { data, status } = useSystemStats(active)
  const [tab, setTab] = useState<Tab>('overview')
  const id = useId()
  const tabRefs = useRef(new Map<Tab, HTMLButtonElement>())

  const prefsStatus = useSysinfoPrefsStore((s) => s.status)
  useEffect(() => {
    const store = useSysinfoPrefsStore.getState()
    void store.load()
    return store.clear
  }, [])
  useEffect(() => {
    if (prefsStatus !== 'error') return
    const timer = setTimeout(() => void useSysinfoPrefsStore.getState().load(), PREFS_RETRY_MS)
    return () => clearTimeout(timer)
  }, [prefsStatus])

  const current: Tab = admin ? tab : 'overview'
  const stale = status === 'disconnected'

  const onTabKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight' && e.key !== 'Home' && e.key !== 'End') return
    e.preventDefault()
    const i = TABS.indexOf(current)
    const next =
      e.key === 'Home'
        ? TABS[0]
        : e.key === 'End'
          ? TABS[TABS.length - 1]
          : TABS[(i + (e.key === 'ArrowLeft' ? -1 : 1) + TABS.length) % TABS.length]
    setTab(next)
    tabRefs.current.get(next)?.focus()
  }

  return (
    <div className="sysinfo">
      {admin && (
        <div className="sysinfo-tabs" role="tablist" aria-label={t('sysinfo.tabs.label')} onKeyDown={onTabKey}>
          {TABS.map((name) => (
            <button
              key={name}
              ref={(el) => {
                if (el) tabRefs.current.set(name, el)
                else tabRefs.current.delete(name)
              }}
              type="button"
              role="tab"
              id={`${id}-tab-${name}`}
              className="sysinfo-tab"
              aria-selected={current === name}
              aria-controls={`${id}-panel`}
              tabIndex={current === name ? 0 : -1}
              onClick={() => setTab(name)}
            >
              {t(`sysinfo.tabs.${name}`)}
            </button>
          ))}
        </div>
      )}
      {stale && (
        <p className="addon-banner sysinfo-banner" role="status">
          {t('sysinfo.disconnected')}
        </p>
      )}
      <div
        className="sysinfo-panel"
        id={`${id}-panel`}
        role={admin ? 'tabpanel' : undefined}
        aria-labelledby={admin ? `${id}-tab-${current}` : undefined}
      >
        {current === 'overview' ? (
          <OverviewPanel sample={data} stale={stale} />
        ) : (
          <ProcessesPanel sample={data} stale={stale} />
        )}
      </div>
    </div>
  )
}
