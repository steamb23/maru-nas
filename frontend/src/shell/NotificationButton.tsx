// 작업표시줄 트레이의 알림 버튼. 읽지 않은 알림 수를 배지로 보이고, 누르면 알림 센터(NotificationCenter)를 열고 닫는다.
// 작업표시줄이 트레이 자식의 너비로 배치를 계산하므로(Taskbar의 measureFit) 늘 같은 크기로 그린다.
import type { KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'
import { badgeText, unreadCount, unreadError } from '../notifications/model'
import { useNotificationStore } from '../notifications/store'

// NOTIFICATION_CENTER_ID는 알림 센터 요소의 id다(aria-controls).
export const NOTIFICATION_CENTER_ID = 'notification-center'

export function NotificationButton() {
  const { t } = useTranslation()
  const open = useNotificationStore((s) => s.centerOpen)
  const count = useNotificationStore((s) => unreadCount(s.notices, s.seen, s.events))
  const error = useNotificationStore((s) => unreadError(s.notices, s.seen, s.events))
  const badge = badgeText(count)

  // 알림 센터가 열려 있으면 Esc로 닫는다(센터 안의 Esc는 센터가 받는다).
  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
    if (e.key !== 'Escape' || !open) return
    e.preventDefault()
    useNotificationStore.getState().setCenterOpen(false)
  }

  return (
    <button
      type="button"
      className="notification-button"
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-controls={open ? NOTIFICATION_CENTER_ID : undefined}
      aria-label={count > 0 ? t('notifications.buttonUnread', { count }) : t('notifications.button')}
      title={t('notifications.button')}
      onClick={() => useNotificationStore.getState().setCenterOpen(!open)}
      onKeyDown={onKeyDown}
    >
      <IconImage name="notification-symbolic" size={20} />
      {badge && (
        <span className={error ? 'notification-badge error' : 'notification-badge'} aria-hidden="true">
          {badge}
        </span>
      )}
    </button>
  )
}
