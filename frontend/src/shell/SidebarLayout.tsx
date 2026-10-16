// 창 내용 틀: 왼쪽 사이드바와 오른쪽 내용(제목 머리 + 본문). 설정 앱과 시스템 정보 창이 쓴다.
// 창이 좁으면(narrowWidth 미만) 사이드바를 숨기고, 머리의 메뉴 단추로 사이드바를 내용 위에 덮어 연다.
// 이때 내용은 입력을 받지 않고, 바깥 누르기·Esc·항목 고르기(closeNav)로 닫힌다. 닫히면 메뉴 단추로 포커스를 돌려준다.
//
// 사이드바 항목은 쓰는 쪽이 그린다. 지금 페이지 항목에는 aria-current="page"를 준다(덮어 열 때 그 항목에 포커스).
// 항목(navItems)을 누르면 덮어 연 사이드바를 닫는다. 공통 항목 모양은 .sidebar-nav-item이다.
import { type KeyboardEvent, type ReactNode, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'
import { moveFocus } from './menuFocus'
import { NAV_CLOSE_MS, NAV_HIDDEN, NAV_OPEN_MS, NAV_SHOWN, SCRIM_HIDDEN, SCRIM_SHOWN } from './sidebarAnim'
import { animationsEnabled } from './useWindowAnimation'
import { CLOSE_EASING, EASING } from './windowAnim'

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

interface Props {
  // 뿌리 요소에 더할 클래스(창마다의 스타일)
  className?: string
  // 이 너비(px)보다 좁으면 사이드바를 덮어 여는 배치로 바꾼다
  narrowWidth: number
  // 사이드바의 접근성 이름
  navLabel: string
  // 사이드바 내용
  nav: ReactNode
  // 사이드바 항목의 선택자. 화살표 키로 오가고, 누르면 덮어 연 사이드바를 닫는다(메뉴 단추로 포커스를 돌려준다).
  navItems?: string
  // 머리의 제목(지금 페이지 이름)
  title: ReactNode
  // 머리 아래 본문
  children: ReactNode
}

export function SidebarLayout({ className, narrowWidth, navLabel, nav, navItems = '.sidebar-nav-item', title, children }: Props) {
  const { t } = useTranslation()
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

  // 창 너비로 좁은 배치를 정한다. 넓어지면 덮어 연 사이드바는 닫힌 것으로 본다(다시 좁아져도 열려 있지 않게).
  useLayoutEffect(() => {
    const el = rootRef.current
    if (!el) return
    const measure = () => {
      const n = el.clientWidth < narrowWidth
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
  }, [narrowWidth])

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

  const onNavKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key === 'Escape' && navOpen) {
      e.preventDefault()
      closeNav(true)
      return
    }
    moveFocus(navRef.current, navItems, e)
  }

  const overlay = narrow && (navOpen || navClosing)
  const rootClass = ['sidebar-layout', className, narrow && 'narrow'].filter(Boolean).join(' ')
  const navClass = ['sidebar-nav', overlay && 'overlay', navClosing && 'closing'].filter(Boolean).join(' ')

  return (
    <div ref={rootRef} className={rootClass}>
      <nav
        ref={navRef}
        id={navId}
        className={navClass}
        aria-label={navLabel}
        inert={navClosing}
        onKeyDown={onNavKeyDown}
        onClick={(e) => {
          if (e.target instanceof Element && e.target.closest(navItems)) closeNav(true)
        }}
      >
        {nav}
      </nav>
      {overlay && <div ref={scrimRef} className="sidebar-scrim" onClick={() => closeNav(true)} />}
      <div className="sidebar-main" inert={narrow && navOpen}>
        <header className="sidebar-header">
          {narrow && (
            <button
              ref={menuButtonRef}
              type="button"
              className="sidebar-menu-button"
              aria-label={t('sidebar.showNav')}
              title={t('sidebar.showNav')}
              aria-expanded={navOpen}
              aria-controls={navId}
              onClick={openNav}
            >
              <IconImage name="hamburger-menu-symbolic" size={16} />
            </button>
          )}
          <h2 className="sidebar-title">{title}</h2>
        </header>
        {children}
      </div>
    </div>
  )
}
