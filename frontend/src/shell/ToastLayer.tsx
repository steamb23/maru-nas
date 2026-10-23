// 토스트: 새 알림을 화면 오른쪽 아래(작업표시줄 위)에 잠깐 띄운다(시안 T). 최근 것이 아래(작업표시줄 쪽)이고,
// 한 번에 TOAST_VISIBLE개까지 보이며 나머지는 "+N개 더"로 접는다. 정보·성공·경고는 시간이 지나면 내려가고
// (읽지 않은 채로 알림 센터에 남는다), 오류는 닫을 때까지 둔다. 포인터나 포커스가 토스트 위에 있으면 멈춘다.
// 닫거나 동작을 누르면 그 알림을 읽음으로 둔다. 토스트는 포커스를 가져가지 않는다.
import { useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { NoticeAction } from '../api/types.gen'
import { IconImage } from '../icons/IconImage'
import { type Item, TOAST_VISIBLE, resolveItem, toastDuration, visibleToasts } from '../notifications/items'
import { LEVEL_ICONS, actionLabel } from '../notifications/messages'
import { type ActionContext, type Toast, actionAvailable } from '../notifications/model'
import { useNotificationStore } from '../notifications/store'
import { TOAST_ENTER_FROM, TOAST_ENTER_MS, TOAST_SHOWN } from './notificationAnim'
import { useNoticeActionContext } from './noticeAction'
import { animationsEnabled } from './useWindowAnimation'
import { EASING } from './windowAnim'

// onAction은 알림의 동작을 실행한다. 데스크톱 크기가 필요해 Desktop이 넘긴다.
export function ToastLayer({ inert, onAction }: { inert: boolean; onAction: (action: NoticeAction) => void }) {
  const { t } = useTranslation()
  const toasts = useNotificationStore((s) => s.toasts)
  const notices = useNotificationStore((s) => s.notices)
  const events = useNotificationStore((s) => s.events)
  const ctx = useNoticeActionContext()
  const [expanded, setExpanded] = useState(false)
  // 포인터가 위에 있거나 안에 포커스가 있으면 시간을 멈춘다.
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  // 접을 것이 없어지면 펼침도 푼다.
  if (expanded && toasts.length <= TOAST_VISIBLE) setExpanded(false)
  const { shown, hidden } = visibleToasts(toasts, expanded)
  const { dismissToast } = useNotificationStore.getState()

  return (
    <section
      className={hovered || focused ? 'toast-layer paused' : 'toast-layer'}
      aria-label={t('notifications.toasts')}
      inert={inert}
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
      onFocus={() => setFocused(true)}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) setFocused(false)
      }}
    >
      {hidden > 0 && (
        <button type="button" className="toast-pill" aria-expanded={false} onClick={() => setExpanded(true)}>
          {t('notifications.more', { count: hidden })}
        </button>
      )}
      {expanded && (
        <div className="toast-controls">
          <button type="button" className="toast-pill" aria-expanded={true} onClick={() => setExpanded(false)}>
            {t('notifications.collapse')}
          </button>
          <button type="button" className="toast-pill" onClick={() => toasts.forEach((x) => dismissToast(x.key))}>
            {t('notifications.closeAll')}
          </button>
        </div>
      )}
      <ol className="toast-list" aria-live="polite">
        {shown.map((toast) => {
          const item = resolveItem(toast.ref, notices, events)
          return (
            item && (
              <ToastCard
                key={toast.key}
                toast={toast}
                item={item}
                ctx={ctx}
                onAction={(action) => {
                  dismissToast(toast.key)
                  onAction(action)
                }}
              />
            )
          )
        })}
      </ol>
    </section>
  )
}

interface CardProps {
  toast: Toast
  item: Item
  ctx: ActionContext
  onAction: (action: NoticeAction) => void
}

function ToastCard({ toast, item, ctx, onAction }: CardProps) {
  const { t } = useTranslation()
  const ref = useRef<HTMLLIElement>(null)
  const { text, action } = item
  const params = { ...text.params }
  const duration = toastDuration(toast.level)

  // 처음 그릴 때 작업표시줄 쪽에서 떠오른다.
  useLayoutEffect(() => {
    if (animationsEnabled()) ref.current?.animate([TOAST_ENTER_FROM, TOAST_SHOWN], { duration: TOAST_ENTER_MS, easing: EASING })
  }, [])

  const { dismissToast, expireToast } = useNotificationStore.getState()
  return (
    <li ref={ref} className={`toast notice-level-${item.level}`} role={item.level === 'error' ? 'alert' : undefined}>
      <IconImage name={LEVEL_ICONS[item.level]} size={20} className="notice-icon" />
      <div className="toast-main">
        <span className="toast-title">{t(text.title, params)}</span>
        {text.body && <span className="toast-body">{t(text.body, params)}</span>}
        {actionAvailable(action, ctx) && (
          <button type="button" className="notice-action toast-action" onClick={() => onAction(action)}>
            {t(actionLabel(action))}
          </button>
        )}
      </div>
      <button
        type="button"
        className="notice-dismiss"
        aria-label={t('notifications.close')}
        title={t('notifications.close')}
        onClick={() => dismissToast(toast.key)}
      >
        <IconImage name="window-close-symbolic" size={16} />
      </button>
      {/* 남은 시간 막대. 이 애니메이션이 끝나면 토스트를 내린다(멈춤은 animation-play-state로 함께 멈춘다).
          애니메이션을 끈 설정에서는 막대만 숨기고 시간은 그대로 잰다. */}
      {duration !== null && (
        <span
          className="toast-timer"
          style={{ animationDuration: `${duration}ms` }}
          aria-hidden="true"
          onAnimationEnd={() => expireToast(toast.key)}
        />
      )}
    </li>
  )
}
