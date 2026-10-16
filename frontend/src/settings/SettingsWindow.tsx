// 설정 앱 창. 왼쪽 사이드바(사용자 카드, 그룹별 페이지 목록)와 오른쪽 페이지 내용으로 나뉜다.
// 사이드바 틀과 좁은 창의 덮어 열기는 shell/SidebarLayout이 맡는다(pages.ts의 SETTINGS_NARROW_WIDTH 미만).
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { WindowViewProps } from '../addons/types'
import { useAuthStore } from '../auth/store'
import { IconImage } from '../icons/IconImage'
import { ProfileAvatar } from '../shell/ProfileMenu'
import { SidebarLayout } from '../shell/SidebarLayout'
import { useSettingsNavStore } from './navStore'
import { SETTINGS_PAGE_VIEWS } from './pageViews'
import { SETTINGS_NARROW_WIDTH, groupPages, resolvePage, visiblePages } from './pages'

const { show } = useSettingsNavStore.getState()

export function SettingsWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const admin = user?.admin === true
  const requested = useSettingsNavStore((s) => s.page)
  const pages = useMemo(() => visiblePages({ admin }), [admin])
  const groups = useMemo(() => groupPages(pages), [pages])
  const current = resolvePage(requested, pages)
  const PageView = SETTINGS_PAGE_VIEWS[current.id]

  return (
    <SidebarLayout
      className="settings"
      narrowWidth={SETTINGS_NARROW_WIDTH}
      navLabel={t('settings.nav')}
      navItems=".settings-user, .sidebar-nav-item"
      title={t(current.labelKey)}
      nav={
        <>
          {/* 사용자 카드. 누르면 프로필 페이지(pages.ts의 nav: 'card')로 간다. */}
          {user && (
            <button
              type="button"
              className="settings-user"
              title={t('profile.openProfile')}
              aria-current={current.id === 'profile' ? 'page' : undefined}
              onClick={() => show('profile')}
            >
              <ProfileAvatar user={user} size={40} />
              <span className="settings-user-text">
                <span className="settings-user-name">{user.username}</span>
                <span className="settings-user-role">{admin ? t('profile.roleAdmin') : t('profile.roleUser')}</span>
              </span>
            </button>
          )}
          {groups.map((g) => (
            <div
              key={g.id}
              className="settings-nav-group"
              role="group"
              aria-label={g.labelKey ? t(g.labelKey) : undefined}
            >
              {g.labelKey && (
                <div className="settings-nav-heading" aria-hidden="true">
                  {t(g.labelKey)}
                </div>
              )}
              {g.pages.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  className="sidebar-nav-item"
                  aria-current={p.id === current.id ? 'page' : undefined}
                  onClick={() => show(p.id)}
                >
                  <IconImage name={p.icon} size={16} />
                  <span>{t(p.labelKey)}</span>
                </button>
              ))}
            </div>
          ))}
        </>
      }
    >
      {/* 페이지 위에 겹쳐 그리는 레이어(정보 페이지의 Clawd)의 기준 상자. 스크롤 컨테이너 밖에 있어야
          레이어가 내용과 함께 스크롤되지 않는다. */}
      <div className="settings-body">
        <div className="settings-page" key={current.id}>
          <PageView active={active} />
        </div>
      </div>
    </SidebarLayout>
  )
}
