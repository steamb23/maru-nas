// 작업표시줄 오른쪽 끝의 프로필 버튼과 메뉴.
import { type KeyboardEvent, useEffect, useId, useRef, useState } from 'react'
import type { SessionInfo } from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { IconImage } from '../icons/IconImage'
import { PROFILE_MENU_ITEMS, type ProfileMenuItemId, groupItems, nextIndex, visibleItems } from './profileMenu'

const { logout } = useAuthStore.getState()

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
  const user = useAuthStore((s) => s.user)
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const menuId = useId()

  const close = (focusButton: boolean) => {
    setOpen(false)
    if (focusButton) buttonRef.current?.focus()
  }

  // 열려 있는 동안 메뉴 밖을 누르면 닫는다. 누른 곳의 동작(창 클릭 등)은 그대로 둔다.
  useEffect(() => {
    if (!open) return
    const onPointerDown = (e: PointerEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false)
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
      setOpen(false)
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
        aria-label={`사용자 메뉴 (${user.username})`}
        title={user.username}
        onClick={() => setOpen((v) => !v)}
      >
        <ProfileAvatar user={user} size={28} />
      </button>
      {open && (
        <div id={menuId} ref={menuRef} className="profile-menu" role="menu" onKeyDown={onMenuKeyDown}>
          <div className="profile-menu-header">
            <ProfileAvatar user={user} size={36} />
            <div className="profile-menu-user">
              <span className="profile-menu-name">{user.username}</span>
              <span className="profile-menu-role">{user.admin ? '관리자' : '사용자'}</span>
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
                  <span>{item.label}</span>
                </button>
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
