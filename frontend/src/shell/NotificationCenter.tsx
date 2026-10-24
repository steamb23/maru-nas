// 알림 센터: 화면 오른쪽에 위에서 작업표시줄 위까지 열리는 패널(시안 B). 서버 알림과 이 탭의 클라이언트 알림을
// "오늘"·"이전"으로 묶어 최근 것부터 보인다. 열면 모두 읽음이 되고, 열기 전에 읽지 않았던 항목은 열려 있는 동안
// 표시(점과 굵은 제목)를 남긴다. 바깥 누르기·Esc·알림 버튼으로 닫고, 바탕화면은 막지 않는다.
import { type KeyboardEvent, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Job, Notice, NoticeAction } from '../api/types.gen'
import { formatUptime } from '../addons/common/format'
import { elapsedSeconds, jobHeading } from '../addons/packages/packages'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import { type Item, ageText, buildCenter } from '../notifications/items'
import { LEVEL_ICONS, actionLabel } from '../notifications/messages'
import { type ActionContext, type ClientEvent, actionAvailable } from '../notifications/model'
import { useNotificationStore } from '../notifications/store'
import { useWindowStore } from '../windows/store'
import { NOTIFICATION_CENTER_ID } from './NotificationButton'
import { CENTER_CLOSE_MS, CENTER_CLOSE_TO, CENTER_OPEN_FROM, CENTER_OPEN_MS, CENTER_SHOWN } from './notificationAnim'
import { useNoticeActionContext } from './noticeAction'
import { animationsEnabled } from './useWindowAnimation'
import { CLOSE_EASING, EASING } from './windowAnim'

// 열려 있는 동안 "3분 전" 같은 시각을 다시 그리는 간격.
const NOW_TICK_MS = 30_000

const close = () => useNotificationStore.getState().setCenterOpen(false)

// PACKAGES_ACTION은 백그라운드 작업의 "열기"다(시스템 업데이트 창을 그 작업으로 연다).
const PACKAGES_ACTION: NoticeAction = { kind: 'app', app: 'packages' }

// onAction은 알림의 동작을 실행한다. 데스크톱 크기가 필요해 Desktop이 넘긴다. job은 작업 결과의 작업 id다.
export function NotificationCenter({ onAction }: { onAction: (action: NoticeAction, job?: string) => void }) {
  const { t } = useTranslation()
  const open = useNotificationStore((s) => s.centerOpen)
  const notices = useNotificationStore((s) => s.notices)
  const events = useNotificationStore((s) => s.events)
  const fresh = useNotificationStore((s) => s.fresh)
  const tasks = useNotificationStore((s) => s.tasks)
  // 백그라운드 작업 구역은 시스템 업데이트 창이 닫혀 있을 때만 보인다(창이 열려 있으면 창이 진행을 보인다).
  const packagesOpen = useWindowStore((s) => s.windows.some((w) => w.appId === 'packages'))
  const shownTasks = packagesOpen ? [] : tasks
  const ctx = useNoticeActionContext()
  const titleId = useId()
  const panelRef = useRef<HTMLElement>(null)
  // 닫기 애니메이션 중이면 true. 그동안 패널을 그려 두되 클릭·포커스는 받지 않는다.
  const [closing, setClosing] = useState(false)
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    setClosing(!open && animationsEnabled())
  }

  // 열기·닫기 애니메이션. 닫는 중에 다시 열면 지금 보이는 투명도·위치에서 이어 간다.
  useLayoutEffect(() => {
    const el = panelRef.current
    if (!el || !animationsEnabled()) return
    const style = getComputedStyle(el)
    const from: Keyframe = { opacity: style.opacity, transform: style.transform }
    const running = el.getAnimations().length > 0
    el.getAnimations().forEach((a) => a.cancel())
    if (open) {
      el.animate([running ? from : CENTER_OPEN_FROM, CENTER_SHOWN], { duration: CENTER_OPEN_MS, easing: EASING })
    } else {
      const anim = el.animate([from, CENTER_CLOSE_TO], { duration: CENTER_CLOSE_MS, easing: CLOSE_EASING, fill: 'forwards' })
      anim.onfinish = () => setClosing(false)
    }
  }, [open])

  // 열려 있는 동안 패널과 알림 버튼 밖을 누르면 닫는다(누른 곳의 동작은 그대로).
  // 열리면 패널에 포커스한다(Tab으로 안의 단추로 간다).
  useEffect(() => {
    if (!open) return
    panelRef.current?.focus()
    const onPointerDown = (e: PointerEvent) => {
      const target = e.target as Element
      if (!panelRef.current?.contains(target) && !target.closest?.('.notification-button')) close()
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    return () => document.removeEventListener('pointerdown', onPointerDown, true)
  }, [open])

  if (!open && !closing) return null

  // Esc는 닫고 포커스를 알림 버튼으로 돌려준다.
  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key !== 'Escape') return
    e.preventDefault()
    close()
    document.querySelector<HTMLButtonElement>('.notification-button')?.focus()
  }

  const run = (item: Item, action: NoticeAction) => {
    useNotificationStore.getState().markRead(item.ref)
    close()
    onAction(action, item.job)
  }
  const openTask = (job: Job) => {
    close()
    onAction(PACKAGES_ACTION, job.id)
  }

  const empty = notices.length === 0 && events.length === 0 && shownTasks.length === 0
  // "모두 지우기"는 지울 수 있는 알림(클라이언트 알림, 작업 결과)이 있을 때만 보인다.
  const clearable = events.length > 0 || notices.some((n) => n.dismissible)

  return (
    <section
      id={NOTIFICATION_CENTER_ID}
      ref={panelRef}
      className={closing ? 'notification-center closing' : 'notification-center'}
      role="dialog"
      aria-labelledby={titleId}
      tabIndex={-1}
      inert={closing}
      onKeyDown={onKeyDown}
    >
      <header className="notification-center-header">
        <h2 id={titleId}>{t('notifications.title')}</h2>
        {clearable && (
          <button type="button" className="notification-clear" onClick={() => useNotificationStore.getState().clearAll()}>
            <IconImage name="edit-clear-all-symbolic" size={16} />
            {t('notifications.clearAll')}
          </button>
        )}
      </header>
      {empty ? (
        <div className="notification-empty">
          <span className="notification-empty-icon">
            <IconImage name="notification-symbolic" size={28} />
          </span>
          <p className="notification-empty-title">{t('notifications.empty')}</p>
          <p className="notification-empty-hint">{t('notifications.emptyHint')}</p>
        </div>
      ) : (
        <CenterList
          notices={notices}
          events={events}
          tasks={shownTasks}
          fresh={fresh}
          ctx={ctx}
          onRun={run}
          onOpenTask={openTask}
        />
      )}
    </section>
  )
}

interface ListProps {
  notices: readonly Notice[]
  events: readonly ClientEvent[]
  tasks: readonly Job[]
  fresh: ReadonlySet<string>
  ctx: ActionContext
  onRun: (item: Item, action: NoticeAction) => void
  onOpenTask: (job: Job) => void
}

// CenterList는 "오늘"·"이전" 묶음이다. 센터를 열 때마다 새로 그려지므로 그때의 시각으로 시작하고,
// 열려 있는 동안 NOW_TICK_MS마다 "3분 전" 같은 시각을 새로 고친다.
function CenterList({ notices, events, tasks, fresh, ctx, onRun, onOpenTask }: ListProps) {
  const { t } = useTranslation()
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const tick = window.setInterval(() => setNow(Date.now()), NOW_TICK_MS)
    return () => window.clearInterval(tick)
  }, [])
  const groups = buildCenter(notices, events, now)
  const section = (label: string, items: Item[]) =>
    items.length > 0 && (
      <section className="notification-group" aria-label={label}>
        <h3 className="notification-group-title">{label}</h3>
        {items.map((item) => (
          <NoticeCard
            key={item.key}
            item={item}
            fresh={fresh.has(item.key)}
            now={now}
            ctx={ctx}
            onAction={(action) => onRun(item, action)}
          />
        ))}
      </section>
    )
  return (
    <div className="notification-list">
      {tasks.length > 0 && (
        <section className="notification-group" aria-label={t('notifications.tasks.title')}>
          <h3 className="notification-group-title">{t('notifications.tasks.title')}</h3>
          {tasks.map((job) => (
            <TaskCard key={job.id} job={job} now={now} onOpen={() => onOpenTask(job)} />
          ))}
        </section>
      )}
      {section(t('notifications.today'), groups.today)}
      {section(t('notifications.earlier'), groups.earlier)}
    </div>
  )
}

interface CardProps {
  item: Item
  // fresh면 센터를 열기 전에 읽지 않았던 알림이다.
  fresh: boolean
  now: number
  ctx: ActionContext
  onAction: (action: NoticeAction) => void
}

function NoticeCard({ item, fresh, now, ctx, onAction }: CardProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { text, action } = item
  const params = { ...text.params }
  // 지울 수 있는 것은 클라이언트 알림과 서버의 작업 결과 알림이다.
  const { ref } = item
  const dismiss = !item.dismissible
    ? null
    : ref.kind === 'client'
      ? () => useNotificationStore.getState().dismissEvent(ref.key)
      : () => useNotificationStore.getState().dismissNotice(ref.id)
  return (
    <article className={`notice-card notice-level-${item.level}${fresh ? ' fresh' : ''}`}>
      <IconImage name={LEVEL_ICONS[item.level]} size={20} className="notice-icon" />
      <div className="notice-main">
        <h4 className="notice-title">{t(text.title, params)}</h4>
        {text.body && <p className="notice-body">{t(text.body, params)}</p>}
        {actionAvailable(action, ctx) && (
          <div className="notice-actions">
            <button type="button" className="notice-action" onClick={() => onAction(action)}>
              {t(actionLabel(action, item.job !== undefined))}
            </button>
          </div>
        )}
      </div>
      <div className="notice-side">
        {(item.at !== null || fresh) && (
          <span className="notice-time">
            {fresh && <span className="notice-dot" aria-hidden="true" />}
            {item.at !== null && ageText(item.at, now, lang)}
          </span>
        )}
        {dismiss && (
          <button
            type="button"
            className="notice-dismiss"
            aria-label={t('notifications.dismiss')}
            title={t('notifications.dismiss')}
            onClick={dismiss}
          >
            <IconImage name="window-close-symbolic" size={16} />
          </button>
        )}
      </div>
    </article>
  )
}

// TaskCard는 진행 중인 서버 백그라운드 작업이다(시안 N). 관리자 세션이 없어 출력을 기다리는 중이면 점선 카드로 보인다.
function TaskCard({ job, now, onOpen }: { job: Job; now: number; onOpen: () => void }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const heading = t(jobHeading(job))
  const sub = job.waiting
    ? t('packages.job.waiting')
    : t('notifications.tasks.sub', { owner: job.owner, elapsed: formatUptime(elapsedSeconds(job, now), lang, 'narrow') })
  return (
    <article className={job.waiting ? 'notice-card notice-task waiting' : 'notice-card notice-task'} role="status">
      <IconImage
        name={job.waiting ? 'dialog-information-symbolic' : 'software-update-available-symbolic'}
        size={20}
        className="notice-icon"
      />
      <div className="notice-main">
        <h4 className="notice-title">
          {heading}
          {job.waiting && <span className="badge notice-task-tag">{t('packages.job.waitingTag')}</span>}
        </h4>
        <p className="notice-body">{sub}</p>
        {!job.waiting && (
          <div className="meter notice-task-meter" role="progressbar" aria-label={heading}>
            <div className="meter-fill" />
          </div>
        )}
        {job.lastLine && <p className="notice-task-line">{job.lastLine}</p>}
        <div className="notice-actions">
          <button type="button" className="notice-action" onClick={onOpen}>
            {t('notifications.tasks.open')}
          </button>
        </div>
      </div>
    </article>
  )
}
