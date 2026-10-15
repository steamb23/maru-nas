// 작업표시줄 오른쪽 끝의 프로필 버튼과 메뉴.
import { type KeyboardEvent, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { avatarImageUrl } from '../api/client'
import type { SessionInfo } from '../api/types.gen'
import { useCapabilitiesStore } from '../addons/capabilities'
import { useAuthStore } from '../auth/store'
import { useAvatarStore } from '../profile/store'
import { setLanguage, useLanguage } from '../i18n'
import { SUPPORTED_LANGUAGES, languageName } from '../i18n/languages'
import { IconImage } from '../icons/IconImage'
import type { SettingsPageId } from '../settings/pages'
import { useDialogs } from './dialogPresets'
import { openLink } from './launchApp'
import {
  PROFILE_MENU_ITEMS,
  type ProfileMenuActionId,
  groupItems,
  submenuKey,
  visibleItems,
} from './profileMenu'
import { MENU_CLOSE_MS, MENU_CLOSE_TO, MENU_OPEN_FROM, MENU_OPEN_MS, MENU_SHOWN } from './menuAnim'
import { moveFocus } from './menuFocus'
import { animationsEnabled } from './useWindowAnimation'
import { CLOSE_EASING, EASING } from './windowAnim'

const { logout } = useAuthStore.getState()

// 메뉴는 프로필 버튼 쪽(오른쪽 아래, ProfileMenu.css의 transform-origin)에서 나타나고 사라진다(menuAnim.ts).

// ProfileAvatar는 사용자 아바타다. 프로필 사진이 있으면 원형으로 그리고, 없거나 받지 못하면 기본 아이콘을 그린다.
// 아바타 그리기는 여기에만 둔다(작업표시줄, 프로필 메뉴, 설정 사이드바, 프로필 페이지).
export function ProfileAvatar({ size }: { user: SessionInfo; size: number }) {
  const image = useAvatarStore((s) => s.image)
  const url = image ? avatarImageUrl(image.rev) : null
  // 받지 못한 사진 주소. 같은 주소는 다시 시도하지 않고 기본 아이콘을 그린다.
  const [broken, setBroken] = useState<string | null>(null)
  if (url && broken !== url) {
    return (
      <img
        className="profile-avatar profile-avatar-image"
        src={url}
        width={size}
        height={size}
        alt=""
        draggable={false}
        onError={() => setBroken(url)}
      />
    )
  }
  return (
    <span className="profile-avatar" style={{ width: size, height: size }}>
      <IconImage name="avatar-default-symbolic" size={Math.round(size * 0.6)} />
    </span>
  )
}

// onOpenSettings는 설정 창을 그 페이지로 연다. 창 위치 계산에 데스크톱 크기가 필요해 Desktop이 넘긴다.
export function ProfileMenu({ onOpenSettings }: { onOpenSettings: (page: SettingsPageId) => void }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const user = useAuthStore((s) => s.user)
  // OMV 웹 UI 주소가 설정되어 있으면 "OMV 워크벤치 열기"를 보인다.
  const workbench = useCapabilitiesStore((s) => Boolean(s.caps?.workbenchUrl))
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
  const dialogs = useDialogs({ scope: 'desktop' })

  // 메뉴 항목 동작. profileMenu.ts의 action 항목 id마다 하나씩 둔다.
  const actions: Record<ProfileMenuActionId, () => void> = {
    // 팝업 차단을 피하려고 클릭 처리 안에서 바로 연다.
    workbench: () => openLink('workbench'),
    // 로그아웃하면 열린 창이 모두 닫히므로 데스크톱 범위 모달로 한 번 묻는다.
    logout: async () => {
      const ok = await dialogs.confirm({
        title: t('profile.logout'),
        message: t('profile.logoutConfirm'),
        confirmLabel: t('profile.logout'),
      })
      if (ok) void logout()
    },
  }

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
  const groups = groupItems(visibleItems(PROFILE_MENU_ITEMS, { admin: user.admin, workbench }))

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
          {/* 머리글을 누르면 설정의 프로필 페이지를 연다. 화살표로 옮겨 갈 수 있게 메뉴 항목으로 둔다. */}
          <button
            type="button"
            role="menuitem"
            className="profile-menu-header"
            title={t('profile.openProfile')}
            onClick={() => {
              close(false)
              onOpenSettings('profile')
            }}
          >
            <ProfileAvatar user={user} size={36} />
            <span className="profile-menu-user">
              <span className="profile-menu-name">{user.username}</span>
              <span className="profile-menu-role">{user.admin ? t('profile.roleAdmin') : t('profile.roleUser')}</span>
            </span>
            <IconImage name="pan-end-symbolic" size={16} className="profile-menu-header-chevron" />
          </button>
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
