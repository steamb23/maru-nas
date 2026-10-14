// 작업표시줄 오른쪽 끝의 프로필 버튼과 메뉴.
import { type KeyboardEvent, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { SessionInfo } from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { setLanguage, useLanguage } from '../i18n'
import { SUPPORTED_LANGUAGES, languageName } from '../i18n/languages'
import { IconImage } from '../icons/IconImage'
import {
  PROFILE_MENU_ITEMS,
  type ProfileMenuActionId,
  groupItems,
  nextIndex,
  submenuKey,
  visibleItems,
} from './profileMenu'
import { animationsEnabled } from './useWindowAnimation'
import { CLOSE_EASING, EASING } from './windowAnim'

const { logout } = useAuthStore.getState()

// 메뉴는 프로필 버튼 쪽(오른쪽 아래, index.css의 transform-origin)에서 조금 아래·작은 상태로 떠오르며 나타나고,
// 닫을 때는 더 짧게 살짝 가라앉으며 사라진다. 창 애니메이션과 같은 켜기/끄기·동작 줄이기 설정을 따른다.
const MENU_OPEN_MS = 160
const MENU_CLOSE_MS = 110
const MENU_OPEN_FROM: Keyframe = { opacity: 0, transform: 'translateY(8px) scale(0.96)' }
const MENU_CLOSE_TO: Keyframe = { opacity: 0, transform: 'translateY(4px) scale(0.98)' }
const MENU_SHOWN: Keyframe = { opacity: 1, transform: 'none' }

// 메뉴 항목 동작. profileMenu.ts의 action 항목 id마다 하나씩 둔다.
const actions: Record<ProfileMenuActionId, () => void> = {
  logout: () => void logout(),
}

// moveFocus는 화살표·Home·End 키로 root 안의 selector 항목 사이에서 포커스를 옮긴다. 옮겼으면 true.
function moveFocus(root: HTMLElement | null, selector: string, e: KeyboardEvent<HTMLElement>): boolean {
  const items = Array.from(root?.querySelectorAll<HTMLButtonElement>(selector) ?? [])
  const current = items.indexOf(document.activeElement as HTMLButtonElement)
  const next = nextIndex(current, items.length, e.key)
  if (next === current) return false
  e.preventDefault()
  items[next]?.focus()
  return true
}

// ProfileAvatar는 사용자 아바타다. 지금은 기본 아이콘만 그린다.
// 나중에 개인 설정에서 이미지·이니셜로 바꿀 수 있도록 아바타 그리기는 여기에만 둔다.
export function ProfileAvatar({ size }: { user: SessionInfo; size: number }) {
  return (
    <span className="profile-avatar" style={{ width: size, height: size }}>
      <IconImage name="avatar-default-symbolic" size={Math.round(size * 0.6)} />
    </span>
  )
}

export function ProfileMenu() {
  const { t } = useTranslation()
  const lang = useLanguage()
  const user = useAuthStore((s) => s.user)
  const [open, setOpen] = useState(false)
  // 언어 하위 메뉴가 열려 있으면 true.
  const [subOpen, setSubOpen] = useState(false)
  // 하위 메뉴만 닫는 애니메이션 중이면 true. 메뉴의 closing과 같은 방식이다.
  const [subClosing, setSubClosing] = useState(false)
  // 닫기 애니메이션 중이면 true. 그동안 메뉴를 그려 두되 클릭·포커스는 받지 않는다.
  const [closing, setClosing] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const menuId = useId()
  const subId = useId()
  const subRef = useRef<HTMLDivElement>(null)
  const subButtonRef = useRef<HTMLButtonElement>(null)

  const show = () => {
    setOpen(true)
    setClosing(false)
    setSubOpen(false)
    setSubClosing(false)
  }
  const hide = () => {
    setOpen(false)
    setClosing(animationsEnabled())
  }
  const showSub = () => {
    setSubOpen(true)
    setSubClosing(false)
  }
  const hideSub = () => {
    if (!subOpen) return
    setSubOpen(false)
    setSubClosing(animationsEnabled())
  }
  const closeSub = () => {
    hideSub()
    subButtonRef.current?.focus()
  }
  const close = (focusButton: boolean) => {
    hide()
    if (focusButton) buttonRef.current?.focus()
  }

  // 열기·닫기 애니메이션. 닫는 중에 다시 열면 지금 보이는 투명도·위치에서 이어 간다.
  useLayoutEffect(() => {
    const el = menuRef.current
    if (!el || !animationsEnabled()) return
    const style = getComputedStyle(el)
    const from: Keyframe = { opacity: style.opacity, transform: style.transform }
    const running = el.getAnimations().length > 0
    el.getAnimations().forEach((a) => a.cancel())
    if (open) {
      el.animate([running ? from : MENU_OPEN_FROM, MENU_SHOWN], { duration: MENU_OPEN_MS, easing: EASING })
    } else {
      // 끝난 뒤 메뉴가 지워지는 렌더링까지 원래 모습이 한 프레임 비치지 않도록 마지막 상태를 유지한다.
      const anim = el.animate([from, MENU_CLOSE_TO], {
        duration: MENU_CLOSE_MS,
        easing: CLOSE_EASING,
        fill: 'forwards',
      })
      anim.onfinish = () => setClosing(false)
    }
  }, [open])

  // 열려 있는 동안 메뉴 밖을 누르면 닫는다. 누른 곳의 동작(창 클릭 등)은 그대로 둔다.
  useEffect(() => {
    if (!open) return
    const onPointerDown = (e: PointerEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) hide()
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    return () => document.removeEventListener('pointerdown', onPointerDown, true)
  }, [open])

  // 열리면 첫 항목에 포커스한다.
  useEffect(() => {
    if (open) menuRef.current?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus()
  }, [open])

  // 하위 메뉴는 메뉴와 같은 모양으로 열리고 닫히며, 열리면 선택된 언어에 포커스한다.
  // 메뉴 전체를 닫을 때는 하위 메뉴가 메뉴 안에 있으므로 따로 닫지 않고 메뉴와 함께 사라진다.
  useLayoutEffect(() => {
    const el = subRef.current
    if (!el) return
    if (subOpen) el.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus()
    if (!animationsEnabled()) return
    const style = getComputedStyle(el)
    const from: Keyframe = { opacity: style.opacity, transform: style.transform }
    const running = el.getAnimations().length > 0
    el.getAnimations().forEach((a) => a.cancel())
    if (subOpen) {
      el.animate([running ? from : MENU_OPEN_FROM, MENU_SHOWN], { duration: MENU_OPEN_MS, easing: EASING })
    } else {
      const anim = el.animate([from, MENU_CLOSE_TO], {
        duration: MENU_CLOSE_MS,
        easing: CLOSE_EASING,
        fill: 'forwards',
      })
      anim.onfinish = () => setSubClosing(false)
    }
  }, [subOpen])

  if (!user) return null
  const groups = groupItems(visibleItems(PROFILE_MENU_ITEMS, user))

  const onMenuKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      close(true)
      return
    }
    if (e.key === 'Tab') {
      hide()
      return
    }
    if (document.activeElement === subButtonRef.current && submenuKey(e.key, false) === 'open') {
      e.preventDefault()
      showSub()
      return
    }
    // 메뉴의 다른 항목으로 옮기면 하위 메뉴를 닫는다.
    if (moveFocus(menuRef.current, '[role="menuitem"]', e)) hideSub()
  }

  // 하위 메뉴의 키 입력은 메뉴까지 올리지 않는다. Tab만 메뉴로 올려 메뉴 전체를 닫는다.
  const onSubKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Tab') return
    e.stopPropagation()
    if (submenuKey(e.key, true) === 'close') {
      e.preventDefault()
      closeSub()
      return
    }
    moveFocus(subRef.current, '[role="menuitemradio"]', e)
  }

  return (
    <div className="profile" ref={rootRef}>
      <button
        ref={buttonRef}
        type="button"
        className="profile-button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-label={t('profile.menu', { username: user.username })}
        title={user.username}
        onClick={() => (open ? hide() : show())}
      >
        <ProfileAvatar user={user} size={28} />
      </button>
      {(open || closing) && (
        <div
          id={menuId}
          ref={menuRef}
          className={closing ? 'profile-menu closing' : 'profile-menu'}
          role="menu"
          inert={closing}
          onKeyDown={onMenuKeyDown}
        >
          <div className="profile-menu-header">
            <ProfileAvatar user={user} size={36} />
            <div className="profile-menu-user">
              <span className="profile-menu-name">{user.username}</span>
              <span className="profile-menu-role">{user.admin ? t('profile.roleAdmin') : t('profile.roleUser')}</span>
            </div>
          </div>
          {groups.map((group) => (
            <div key={group[0].group} className="profile-menu-group" role="group">
              {group.map((item) =>
                item.kind === 'action' ? (
                  <button
                    key={item.id}
                    type="button"
                    role="menuitem"
                    className="profile-menu-item"
                    onClick={() => {
                      close(true)
                      actions[item.id]()
                    }}
                  >
                    <IconImage name={item.icon} size={16} />
                    <span>{t(item.labelKey)}</span>
                  </button>
                ) : (
                  <div key={item.id} className="profile-submenu-anchor">
                    <button
                      ref={subButtonRef}
                      type="button"
                      role="menuitem"
                      className="profile-menu-item"
                      aria-haspopup="menu"
                      aria-expanded={subOpen}
                      aria-controls={subOpen ? subId : undefined}
                      onClick={() => (subOpen ? hideSub() : showSub())}
                    >
                      <IconImage name="pan-start-symbolic" size={16} className="profile-menu-chevron" />
                      <IconImage name={item.icon} size={16} />
                      <span>{t(item.labelKey)}</span>
                      <span className="profile-menu-value">{languageName(lang)}</span>
                    </button>
                    {(subOpen || subClosing) && (
                      <div
                        id={subId}
                        ref={subRef}
                        className={subClosing ? 'profile-menu profile-submenu closing' : 'profile-menu profile-submenu'}
                        role="menu"
                        inert={subClosing}
                        aria-label={t(item.labelKey)}
                        onKeyDown={onSubKeyDown}
                      >
                        {SUPPORTED_LANGUAGES.map((code) => (
                          <button
                            key={code}
                            type="button"
                            role="menuitemradio"
                            aria-checked={code === lang}
                            lang={code}
                            className="profile-menu-item"
                            onClick={() => {
                              close(true)
                              setLanguage(code)
                            }}
                          >
                            <span className="profile-menu-check">
                              {code === lang && <IconImage name="object-select-symbolic" size={16} />}
                            </span>
                            <span>{languageName(code)}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                ),
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
