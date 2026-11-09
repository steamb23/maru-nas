// 예약된 다시 시작·종료가 다가올 때의 경고. 10분 전에 알림을 띄우고, 1분 전부터 데스크톱 위쪽에 카운트다운을
// 보이고, 시각이 되면 전원 감시를 시작해 전원 화면으로 바꾼다. 예약은 서버 알림(rebootScheduled·shutdownScheduled)으로
// 알고 남은 시간은 서버 시계로 센다. 계산은 countdown.ts다.
import type { ParseKeys } from 'i18next'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { NoticePowerSchedule } from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { IconImage } from '../icons/IconImage'
import { notify } from '../notifications/notify'
import { useNotificationStore } from '../notifications/store'
import { refreshNotifications } from '../notifications/useNotifications'
import {
  REFRESH_BEFORE_MS,
  countdownText,
  minutesLeft,
  nextCheck,
  progress,
  scheduledShutdown,
  warningPhase,
} from './countdown'
import { openPowerDialog } from './powerDialog'
import { scheduleError, serverNow } from './schedule'
import { cancelSchedule } from './scheduleActions'
import { usePowerWatch } from './watchStore'

// 예약마다(리비전) 한 번만 하는 일
interface Fired {
  soon: string
  imminent: string
  refresh: string
  due: string
}

const NONE_FIRED: Fired = { soon: '', imminent: '', refresh: '', due: '' }

// inert는 데스크톱 모달이 열려 있을 때 카운트다운의 단추를 막는다(작업표시줄·토스트와 같다).
export function ScheduleWarnings({ inert }: { inert?: boolean }) {
  const { t } = useTranslation()
  const notices = useNotificationStore((s) => s.notices)
  const admin = useAuthStore((s) => s.user?.admin ?? false)
  const sched = useMemo(() => scheduledShutdown(notices), [notices])
  const [now, setNow] = useState(() => serverNow(useNotificationStore.getState().skew))
  const [cancelling, setCancelling] = useState(false)
  const [error, setError] = useState<ParseKeys | null>(null)
  const fired = useRef<Fired>(NONE_FIRED)

  // 남은 시간을 다시 볼 때마다 서버 시계로 새로 계산한다(보이지 않는 탭은 타이머가 늦게 돌므로 횟수로 세지 않는다).
  useEffect(() => {
    if (!sched) {
      fired.current = NONE_FIRED
      return
    }
    let id = 0
    const tick = () => {
      const n = serverNow(useNotificationStore.getState().skew)
      setNow(n)
      id = window.setTimeout(tick, nextCheck(sched.at - n))
    }
    id = window.setTimeout(tick, 0)
    return () => window.clearTimeout(id)
  }, [sched])

  const left = sched ? sched.at - now : Number.POSITIVE_INFINITY
  const phase = sched ? warningPhase(left) : 'later'

  // 예약이 바뀐 직후의 now는 지난 tick의 값일 수 있으므로 남은 시간을 여기서 새로 잰다(알림의 남은 분이 틀리지 않게).
  useEffect(() => {
    if (!sched) return
    const l = sched.at - serverNow(useNotificationStore.getState().skew)
    const ph = warningPhase(l)
    const key = sched.revision
    const f = fired.current
    if (ph === 'soon' && f.soon !== key) {
      fired.current = { ...fired.current, soon: key }
      notify({
        code: sched.reboot ? 'rebootSoon' : 'shutdownSoon',
        level: 'warning',
        params: { count: minutesLeft(l), at: sched.at },
        action: { kind: 'power', page: NoticePowerSchedule },
      })
    }
    // 1분 전과 시각 직전에 알림을 다시 받아 예약이 그대로인지 본다. 취소됐으면 알림이 사라져 경고도 사라진다.
    if (ph === 'imminent' && f.imminent !== key) {
      fired.current = { ...fired.current, imminent: key }
      refreshNotifications()
    }
    if (ph === 'imminent' && l <= REFRESH_BEFORE_MS && f.refresh !== key) {
      fired.current = { ...fired.current, refresh: key }
      refreshNotifications()
    }
    // 시각이 되었는데 예약이 남아 있으면 NAS가 내려갈 것이다. 전원 화면이 내려갔다 돌아오기를 기다린다.
    if (ph === 'due' && f.due !== key) {
      fired.current = { ...fired.current, due: key }
      usePowerWatch.getState().start(sched.reboot ? 'reboot' : 'shutdown')
    }
  }, [sched, now])

  if (!sched || phase !== 'imminent') return null

  const cancel = async () => {
    if (cancelling) return
    setCancelling(true)
    setError(null)
    try {
      await cancelSchedule({ kind: 'desktop' })
    } catch (err) {
      setError(scheduleError(err, 'power.schedule.error.cancelFailed'))
    } finally {
      setCancelling(false)
    }
  }

  return (
    <section className="schedule-banner" aria-label={t('power.warning.label')} inert={inert}>
      <div className="schedule-banner-main">
        <IconImage name={sched.reboot ? 'system-reboot-symbolic' : 'system-shutdown-symbolic'} size={20} />
        {/* 처음 나타날 때 한 번 읽힌다. 시계는 바뀔 때마다 읽히지 않게 밖에 둔다. */}
        <div className="schedule-banner-text" role="alert">
          <strong>{t(sched.reboot ? 'power.warning.rebootTitle' : 'power.warning.shutdownTitle')}</strong>
          <span>{t('power.warning.body')}</span>
          <span>{sched.by ? t('power.warning.by', { by: sched.by }) : t('power.warning.elsewhere')}</span>
          {!admin && <span>{t('power.warning.adminOnly')}</span>}
        </div>
        <span className="schedule-banner-clock" role="timer" aria-label={t('power.warning.remaining')}>
          {countdownText(left)}
        </span>
        {admin && (
          <div className="schedule-banner-buttons">
            <button type="button" className="addon-button" disabled={cancelling} onClick={() => void cancel()}>
              {cancelling ? (
                <>
                  <span className="power-spinner" aria-hidden="true" />
                  {t('power.schedule.cancelling')}
                </>
              ) : (
                t('power.schedule.cancel')
              )}
            </button>
            {sched.reboot && (
              <button
                type="button"
                className="addon-button danger"
                disabled={cancelling}
                onClick={() => openPowerDialog({ step: 'reboot' })}
              >
                {t('power.warning.restartNow')}
              </button>
            )}
          </div>
        )}
      </div>
      {error && (
        <p className="schedule-banner-error" role="alert">
          {t(error)}
        </p>
      )}
      <div className="schedule-banner-bar" aria-hidden="true">
        <div style={{ width: `${progress(left) * 100}%` }} />
      </div>
    </section>
  )
}
