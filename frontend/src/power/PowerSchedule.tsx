// 전원 모달에서 다시 시작의 "시간 정해서" 화면: 지금 예약, 날짜·시각 고르기, 예약·취소.
// 예약·취소는 권한 도우미의 root 연산이므로 상승하지 않았으면 비밀번호를 묻고 다시 보낸다(scheduleActions.ts).
// 계산은 schedule.ts다.
import { type FormEvent, type RefObject, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import { noticeTimeText } from '../notifications/messages'
import type { PowerDialogStore } from './dialogStore'
import {
  LAST_OFFSET,
  MAX_AHEAD_MS,
  type ScheduleForm,
  checkTarget,
  clockText,
  dateText,
  dayStart,
  initialForm,
  leftText,
  nasWallTime,
  scheduleBlock,
  scheduleError,
  serverNow,
  targetTime,
} from './schedule'
import { cancelSchedule, scheduleRestart } from './scheduleActions'
import type { ScheduleState } from './scheduleData'

// 전원 모달은 데스크톱 범위 대화상자다. 권한 상승도 그 위에 연다.
const DESKTOP = { kind: 'desktop' } as const

// TICK_MS마다 남은 시간과 시각 검사를 다시 계산한다(분 단위 문구라 자주 그릴 필요가 없다).
const TICK_MS = 15_000

// useClientNow는 이 브라우저의 지금 시각이다. TICK_MS마다 바뀐다.
function useClientNow(): number {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), TICK_MS)
    return () => window.clearInterval(id)
  }, [])
  return now
}

interface ViewProps {
  store: PowerDialogStore
  state: ScheduleState
  backRef: RefObject<HTMLButtonElement | null>
  // onScheduled는 예약한 뒤 부른다(모달을 닫는다).
  onScheduled: () => void
}

export function ScheduleView({ store, state, backRef, onScheduled }: ViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data, update, setHelper } = state
  const sending = store((s) => s.sending)
  const error = store((s) => s.error)
  const [busy, setBusy] = useState<'schedule' | 'cancel' | null>(null)
  // 사용자가 고친 날짜·시각. 고치기 전에는 지금 예약(없으면 다음 03:00)을 보인다.
  const [edited, setEdited] = useState<ScheduleForm | null>(null)
  const dateRef = useRef<HTMLSelectElement>(null)
  const submitRef = useRef<HTMLButtonElement>(null)
  const cancelRef = useRef<HTMLButtonElement>(null)
  const lastAction = useRef<'schedule' | 'cancel' | null>(null)
  const now = useClientNow() + data.skew

  // 요청이 끝나면 꺼졌다 켜진 단추에서 잃은 포커스를 돌려준다. 취소에 성공하면 그 단추가 사라지므로 날짜로 간다.
  useLayoutEffect(() => {
    if (sending || !lastAction.current) return
    const active = document.activeElement
    if (active && active !== document.body) return
    if (lastAction.current === 'cancel') (cancelRef.current ?? dateRef.current)?.focus()
    else submitRef.current?.focus()
  }, [sending])

  if (data.status === 'loading') {
    return <p className="modal-message power-loading">{t('power.schedule.loading')}</p>
  }

  const sc = data.schedule
  const scAt = sc ? Date.parse(sc.at) : Number.NaN
  const form = edited ?? initialForm(now, sc)
  const target = targetTime(now, form.offset, form.time)
  const check = checkTarget(target, now)
  const block = scheduleBlock(data.helper)
  const nas = check === 'ok' && target !== null ? nasWallTime(target, lang, data.timeZone, data.serverNow) : null

  let result: string
  switch (check) {
    case 'ok':
      result = t('power.schedule.result', { at: noticeTimeText(target ?? now, lang), left: leftText((target ?? now) - now, lang) })
      break
    case 'empty':
      result = t('power.schedule.error.empty')
      break
    case 'past':
      result = t('power.schedule.error.past', { now: clockText(now, lang) })
      break
    case 'tooSoon':
      result = t('power.schedule.error.tooSoon')
      break
    case 'tooFar':
      result = t('power.schedule.error.tooFar', { limit: noticeTimeText(now + MAX_AHEAD_MS, lang) })
      break
  }

  const begin = (action: 'schedule' | 'cancel'): boolean => {
    if (store.getState().sending) return false
    lastAction.current = action
    setBusy(action)
    store.setState({ sending: true, error: null })
    return true
  }
  const finish = () => {
    setBusy(null)
    store.setState({ sending: false })
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    // 화면의 지금은 TICK_MS마다 바뀌므로 보내기 전에 지금 시각으로 다시 본다.
    if (target === null || checkTarget(target, serverNow(data.skew)) !== 'ok' || block) return
    if (!begin('schedule')) return
    try {
      const res = await scheduleRestart(DESKTOP, target, setHelper)
      if (!res) return
      update(res)
      onScheduled()
    } catch (err) {
      store.setState({ error: scheduleError(err, 'power.schedule.error.failed') })
    } finally {
      finish()
    }
  }

  const cancel = async () => {
    if (!begin('cancel')) return
    try {
      const res = await cancelSchedule(DESKTOP, setHelper)
      if (!res) return
      update(res)
    } catch (err) {
      store.setState({ error: scheduleError(err, 'power.schedule.error.cancelFailed') })
    } finally {
      finish()
    }
  }

  return (
    <form className="power-schedule" onSubmit={(e) => void submit(e)}>
      {sc && !Number.isNaN(scAt) && (
        <div className={sc.mode === 'reboot' ? 'power-current' : 'power-current shutdown'}>
          <IconImage name={sc.mode === 'reboot' ? 'system-reboot-symbolic' : 'system-shutdown-symbolic'} size={16} />
          <div className="power-current-text">
            <span className="power-current-title">
              {t(sc.mode === 'reboot' ? 'power.schedule.rebootAt' : 'power.schedule.shutdownAt', {
                at: noticeTimeText(scAt, lang),
              })}
            </span>
            <span className="power-current-detail">
              {sc.by
                ? t('power.schedule.remainingBy', { left: leftText(scAt - now, lang), by: sc.by })
                : t('power.schedule.remainingElsewhere', { left: leftText(scAt - now, lang) })}
            </span>
          </div>
          <button
            ref={cancelRef}
            type="button"
            className="addon-button"
            disabled={sending || block !== null}
            onClick={() => void cancel()}
          >
            {busy === 'cancel' ? (
              <>
                <span className="power-spinner" aria-hidden="true" />
                {t('power.schedule.cancelling')}
              </>
            ) : (
              t('power.schedule.cancel')
            )}
          </button>
          {sc.mode !== 'reboot' ? (
            <p className="power-current-note">{t('power.schedule.replaceShutdown')}</p>
          ) : (
            !sc.by && <p className="power-current-note">{t('power.schedule.replaceElsewhere')}</p>
          )}
        </div>
      )}
      {data.status === 'error' && (
        <p className="power-reason">
          <IconImage name="dialog-warning-symbolic" size={16} />
          <span>{t('power.schedule.error.load')}</span>
        </p>
      )}
      <div className="power-fields">
        <label className="power-field">
          {t('power.schedule.date')}
          <select
            ref={dateRef}
            className="power-input"
            value={form.offset}
            disabled={sending}
            onChange={(e) => setEdited({ ...form, offset: Number(e.target.value) })}
          >
            {Array.from({ length: LAST_OFFSET + 1 }, (_, offset) => {
              const date = dateText(dayStart(now, offset), lang)
              let label = date
              if (offset === 0) label = t('power.schedule.today', { date })
              else if (offset === 1) label = t('power.schedule.tomorrow', { date })
              return (
                <option key={offset} value={offset}>
                  {label}
                </option>
              )
            })}
          </select>
        </label>
        <label className="power-field">
          {t('power.schedule.time')}
          <input
            type="time"
            className="power-input"
            value={form.time}
            required
            disabled={sending}
            onChange={(e) => setEdited({ ...form, time: e.target.value })}
          />
        </label>
      </div>
      <p className={check === 'ok' ? 'power-result' : 'power-result invalid'} role="status">
        {check !== 'ok' && <IconImage name="dialog-warning-symbolic" size={16} />}
        <span>{result}</span>
      </p>
      {block && (
        <p className="power-reason">
          <IconImage name="dialog-warning-symbolic" size={16} />
          <span>{t(block)}</span>
        </p>
      )}
      {error && (
        <p className="power-error selectable" role="alert">
          <IconImage name="dialog-warning-symbolic" size={16} />
          <span>{t(error)}</span>
        </p>
      )}
      <div className="power-notes">
        {nas && <p>{t('power.schedule.nasTime', { at: nas })}</p>}
        <p>{t('power.schedule.notice')}</p>
      </div>
      <div className="modal-buttons power-confirm-buttons">
        <button
          ref={backRef}
          type="button"
          className="addon-button power-back"
          disabled={sending}
          onClick={() => store.setState({ step: null, error: null })}
        >
          {t('power.back')}
        </button>
        <button
          ref={submitRef}
          type="submit"
          className="addon-button primary"
          disabled={sending || check !== 'ok' || block !== null}
        >
          {busy === 'schedule' ? (
            <>
              <span className="power-spinner" aria-hidden="true" />
              {t('power.schedule.scheduling')}
            </>
          ) : (
            t(sc ? 'power.schedule.change' : 'power.schedule.submit')
          )}
        </button>
      </div>
    </form>
  )
}
