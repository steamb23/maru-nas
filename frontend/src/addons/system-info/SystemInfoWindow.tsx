import type { ParseKeys } from 'i18next'
import { type ReactNode, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '../../auth/store'
import { IconImage } from '../../icons/IconImage'
import type { IconName } from '../../icons/resolver'
import { SidebarLayout } from '../../shell/SidebarLayout'
import type { WindowViewProps } from '../types'
import { PerformancePanel, type Resource } from './PerformancePanel'
import { ProcessesPanel } from './ProcessesPanel'
import { useSysinfoPrefsStore } from './sysinfoPrefsStore'
import { useSystemStats } from './useSystemStats'

type Page = 'processes' | 'performance'

// 사이드바 페이지(Windows 11 작업 관리자 순서). 프로세스는 관리자만 본다.
const PAGES: readonly { id: Page; labelKey: ParseKeys; icon: IconName }[] = [
  { id: 'processes', labelKey: 'sysinfo.nav.processes', icon: 'view-list-symbolic' },
  { id: 'performance', labelKey: 'sysinfo.nav.performance', icon: 'utilities-system-monitor-symbolic' },
]

// 창이 이보다 좁으면 사이드바를 숨기고 메뉴 단추로 덮어 연다(SidebarLayout).
// 사이드바와 성능 페이지(자원 목록 + 그래프)가 나란히 들어갈 수 있는 너비다.
const SYSINFO_NARROW_WIDTH = 720

// 보기 설정을 불러오지 못했을 때 다시 시도하는 간격
const PREFS_RETRY_MS = 10_000

// SystemInfoWindow는 시스템 정보 창이다. 관리자에게는 [프로세스] [성능] 사이드바를, 그 밖의 사용자에게는
// 성능 페이지만 보여 준다. 데이터(기록 포함)와 고른 자원은 페이지의 부모인 여기서 들고 있으므로
// 페이지를 바꿔도 그래프 기록과 프로세스 CPU 값이 이어진다.
export function SystemInfoWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const admin = useAuthStore((s) => s.user?.admin === true)
  const { data, status } = useSystemStats(active)
  const [page, setPage] = useState<Page>('performance')
  const [resource, setResource] = useState<Resource>('cpu')

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

  const current: Page = admin ? page : 'performance'
  const stale = status === 'disconnected'
  const body: ReactNode = (
    <div className="sysinfo-body">
      {stale && (
        <p className="addon-banner sysinfo-banner" role="status">
          {t('sysinfo.disconnected')}
        </p>
      )}
      {current === 'performance' ? (
        <PerformancePanel sample={data} stale={stale} resource={resource} onResource={setResource} />
      ) : (
        <ProcessesPanel sample={data} stale={stale} />
      )}
    </div>
  )

  if (!admin) return <div className="sysinfo single">{body}</div>

  const currentPage = PAGES.find((p) => p.id === current) ?? PAGES[1]
  return (
    <SidebarLayout
      className="sysinfo"
      narrowWidth={SYSINFO_NARROW_WIDTH}
      navLabel={t('sysinfo.nav.label')}
      title={t(currentPage.labelKey)}
      nav={PAGES.map((p) => (
        <button
          key={p.id}
          type="button"
          className="sidebar-nav-item"
          aria-current={p.id === current ? 'page' : undefined}
          onClick={() => setPage(p.id)}
        >
          <IconImage name={p.icon} size={16} />
          <span>{t(p.labelKey)}</span>
        </button>
      ))}
    >
      {body}
    </SidebarLayout>
  )
}
