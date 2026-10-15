// 설정 앱 창. 왼쪽 사이드바(사용자 카드, 그룹별 페이지 목록)와 오른쪽 페이지 내용으로 나뉜다.
// 창이 좁으면(pages.ts의 SETTINGS_NARROW_WIDTH 미만) 사이드바를 숨기고, 내용 머리의 메뉴 단추로
// 사이드바를 내용 위에 덮어 연다. 이때 내용은 입력을 받지 않고, 바깥 누르기·Esc·페이지 고르기로 닫힌다.
import { type KeyboardEvent, useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { WindowViewProps } from '../addons/types'
import { useAuthStore } from '../auth/store'
import { IconImage } from '../icons/IconImage'
import { moveFocus } from '../shell/menuFocus'
import { ProfileAvatar } from '../shell/ProfileMenu'
import { animationsEnabled } from '../shell/useWindowAnimation'
import { CLOSE_EASING, EASING } from '../shell/windowAnim'
import { NAV_CLOSE_MS, NAV_HIDDEN, NAV_OPEN_MS, NAV_SHOWN, SCRIM_HIDDEN, SCRIM_SHOWN } from './navAnim'
import { useSettingsNavStore } from './navStore'
import { SETTINGS_PAGE_VIEWS } from './pageViews'
import { SETTINGS_NARROW_WIDTH, type SettingsPageId, groupPages, resolvePage, visiblePages } from './pages'

const { show, reset } = useSettingsNavStore.getState()

// play는 el을 지금 보이는 모습에서 to로 움직인다. 열기·닫기가 겹치면 이어서 움직인다.
function play(el: Element | null, from: Keyframe, to: Keyframe, opening: boolean, onFinish?: () => void) {
  if (!el) return
  const style = getComputedStyle(el)
  const running = el.getAnimations().length > 0
  const start: Keyframe = running ? { opacity: style.opacity, transform: style.transform } : from
  el.getAnimations().forEach((a) => a.cancel())
  const anim = el.animate([start, to], {
    duration: opening ? NAV_OPEN_MS : NAV_CLOSE_MS,
    easing: opening ? EASING : CLOSE_EASING,
    // 닫을 때는 끝난 뒤 지워지는 렌더링까지 마지막 모습을 유지한다.
    fill: opening ? 'none' : 'forwards',
  })
  if (onFinish) anim.onfinish = onFinish
}

export function SettingsWindow(_props: WindowViewProps) {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const admin = user?.admin === true
  const requested = useSettingsNavStore((s) => s.page)
  const pages = useMemo(() => visiblePages({ admin }), [admin])
  const groups = useMemo(() => groupPages(pages), [pages])
  const current = resolvePage(requested, pages)
  const navId = useId()
  const rootRef = useRef<HTMLDivElement>(null)
  const navRef = useRef<HTMLElement>(null)
  const scrimRef = useRef<HTMLDivElement>(null)
  const menuButtonRef = useRef<HTMLButtonElement>(null)
  const [narrow, setNarrow] = useState(false)
  // 좁은 창에서 사이드바가 덮어 열려 있으면 true. 닫는 애니메이션 동안은 navClosing.
  const [navOpen, setNavOpen] = useState(false)
  const [navClosing, setNavClosing] = useState(false)
  const prevOpen = useRef(false)
  // 닫은 뒤 메뉴 단추로 포커스를 돌려줄지. 닫는 순간에는 내용이 아직 inert라 포커스할 수 없어 그린 뒤에 옮긴다.
  const focusButton = useRef(false)

  // 창이 닫히면 다음에 기본 페이지로 열리게 한다.
  useEffect(() => reset, [])

  // 창 너비로 좁은 배치를 정한다. 넓어지면 덮어 연 사이드바는 닫힌 것으로 본다(다시 좁아져도 열려 있지 않게).
  useLayoutEffect(() => {
    const el = rootRef.current
    if (!el) return
    const measure = () => {
      const n = el.clientWidth < SETTINGS_NARROW_WIDTH
      setNarrow(n)
      if (!n) {
        setNavOpen(false)
        setNavClosing(false)
      }
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // 덮어 여는 사이드바의 열기·닫기 애니메이션. 열리면 지금 페이지 항목에 포커스한다.
  useLayoutEffect(() => {
    if (prevOpen.current === navOpen) return
    prevOpen.current = navOpen
    if (navOpen) navRef.current?.querySelector<HTMLElement>('[aria-current="page"]')?.focus()
    else if (focusButton.current) menuButtonRef.current?.focus()
    focusButton.current = false
    if (!animationsEnabled()) return
    if (navOpen) {
      play(navRef.current, NAV_HIDDEN, NAV_SHOWN, true)
      play(scrimRef.current, SCRIM_HIDDEN, SCRIM_SHOWN, true)
    } else {
      play(navRef.current, NAV_SHOWN, NAV_HIDDEN, false, () => setNavClosing(false))
      play(scrimRef.current, SCRIM_SHOWN, SCRIM_HIDDEN, false)
    }
  }, [navOpen])

  // 사이드바가 다시 제자리에 그려질 때 닫기 애니메이션의 마지막 모습(fill)이 남지 않게 지운다.
  useLayoutEffect(() => {
    if (!navClosing && !navOpen) navRef.current?.getAnimations().forEach((a) => a.cancel())
  }, [navClosing, navOpen])

  const openNav = () => {
    setNavOpen(true)
    setNavClosing(false)
  }
  const closeNav = (returnFocus: boolean) => {
    if (!navOpen) return
    setNavOpen(false)
    setNavClosing(animationsEnabled())
    focusButton.current = returnFocus
  }

  const select = (id: SettingsPageId) => {
    show(id)
    closeNav(true)
  }

  const onNavKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key === 'Escape' && navOpen) {
      e.preventDefault()
      closeNav(true)
      return
    }
    moveFocus(navRef.current, '.settings-nav-item', e)
  }

  const overlay = narrow && (navOpen || navClosing)
  const PageView = SETTINGS_PAGE_VIEWS[current.id]
  const navClass = ['settings-nav', overlay && 'overlay', navClosing && 'closing'].filter(Boolean).join(' ')

  return (
    <div ref={rootRef} className={narrow ? 'settings narrow' : 'settings'}>
      <nav
        ref={navRef}
        id={navId}
        className={navClass}
        aria-label={t('settings.nav')}
        inert={navClosing}
        onKeyDown={onNavKeyDown}
      >
        {user && (
          <div className="settings-user">
            <ProfileAvatar user={user} size={40} />
            <div className="settings-user-text">
              <span className="settings-user-name">{user.username}</span>
              <span className="settings-user-role">{admin ? t('profile.roleAdmin') : t('profile.roleUser')}</span>
            </div>
          </div>
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
                className="settings-nav-item"
                aria-current={p.id === current.id ? 'page' : undefined}
                onClick={() => select(p.id)}
              >
                <IconImage name={p.icon} size={16} />
                <span>{t(p.labelKey)}</span>
              </button>
            ))}
          </div>
        ))}
      </nav>
      {overlay && <div ref={scrimRef} className="settings-scrim" onClick={() => closeNav(true)} />}
      <div className="settings-main" inert={narrow && navOpen}>
        <header className="settings-header">
          {narrow && (
            <button
              ref={menuButtonRef}
              type="button"
              className="settings-menu-button"
              aria-label={t('settings.showNav')}
              title={t('settings.showNav')}
              aria-expanded={navOpen}
              aria-controls={navId}
              onClick={openNav}
            >
              <IconImage name="hamburger-menu-symbolic" size={16} />
            </button>
          )}
          <h2 className="settings-title">{t(current.labelKey)}</h2>
        </header>
        <div className="settings-page" key={current.id}>
          <PageView />
        </div>
      </div>
    </div>
  )
}
