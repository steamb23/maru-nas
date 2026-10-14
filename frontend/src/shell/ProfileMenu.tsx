// 작업표시줄 오른쪽 끝의 프로필 버튼과 메뉴.
import { type KeyboardEvent, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { SessionInfo } from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { IconImage } from '../icons/IconImage'
import { PROFILE_MENU_ITEMS, type ProfileMenuItemId, groupItems, nextIndex, visibleItems } from './profileMenu'
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

// 메뉴 항목 동작. profileMenu.ts의 항목 id마다 하나씩 둔다.
const actions: Record<ProfileMenuItemId, () => void> = {
  logout: () => void logout(),
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
  const user = useAuthStore((s) => s.user)
  const [open, setOpen] = useState(false)
  // 닫기 애니메이션 중이면 true. 그동안 메뉴를 그려 두되 클릭·포커스는 받지 않는다.
  const [closing, setClosing] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const menuId = useId()

  const show = () => {
    setOpen(true)
    setClosing(false)
  }
  const hide = () => {
    setOpen(false)
    setClosing(animationsEnabled())
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
    const items = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])
    const current = items.indexOf(document.activeElement as HTMLButtonElement)
    const next = nextIndex(current, items.length, e.key)
    if (next !== current) {
      e.preventDefault()
      items[next]?.focus()
    }
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
              {group.map((item) => (
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
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
