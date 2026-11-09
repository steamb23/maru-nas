// 전원 모달의 내용: 타일(시스템 종료·다시 시작·절전)을 고르면 같은 모달이 확인 단계로 바뀐다. 다시 시작은
// 확인 단계에서 "지금"과 "시간 정해서"(PowerSchedule.tsx)를 고른다.
// 여는 함수는 powerDialog.tsx, 계산은 power.ts·schedule.ts, 단계 상태는 dialogStore.ts다.
import type { ParseKeys } from 'i18next'
import { type KeyboardEvent, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { fetchAddon, runAddonAction } from '../api/client'
import type { PowerInfo } from '../api/types.gen'
import { isSystemStatsResponse } from '../addons/system-info/stats'
import { IconImage } from '../icons/IconImage'
import { ScheduleView } from './PowerSchedule'
import type { PowerDialogStore, PowerWhen } from './dialogStore'
import {
  POWER_ACTIONS,
  POWER_ADDON,
  type PowerAction,
  powerActionSpec,
  powerInfoOf,
  sendOutcome,
  sleepState,
} from './power'
import { scheduleBlock } from './schedule'
import { useScheduleData } from './scheduleData'

// PowerTitle은 모달 제목이다. 확인 단계에서는 그 조작을 묻는다.
export function PowerTitle({ store }: { store: PowerDialogStore }) {
  const { t } = useTranslation()
  const step = store((s) => s.step)
  const when = store((s) => s.when)
  if (!step) return t('power.title')
  const spec = powerActionSpec(step)
  return (
    <span className="power-title">
      <span className={spec.danger ? 'power-title-icon danger' : 'power-title-icon'}>
        <IconImage name={spec.icon} size={16} />
      </span>
      {t(step === 'reboot' && when === 'later' ? 'power.schedule.title' : spec.confirmTitleKey)}
    </span>
  )
}

interface BodyProps {
  store: PowerDialogStore
  onCancel: () => void
  // onSent는 조작을 보낸 뒤(또는 보내다 연결이 끊긴 뒤) 부른다. modeKey는 절전의 대기 방식 이름이다.
  onSent: (action: PowerAction, modeKey?: ParseKeys) => void
  // onScheduled는 다시 시작을 예약한 뒤 부른다.
  onScheduled: () => void
}

// 단추가 단계와 진행 상태를 함께 보아야 하므로 단추 줄도 본문 안에 둔다(권한 상승 대화상자와 같다).
export function PowerBody({ store, onCancel, onSent, onScheduled }: BodyProps) {
  const { t } = useTranslation()
  const { step, when, sending, error } = store()
  const [power, setPower] = useState<{ info?: PowerInfo; failed: boolean }>({ failed: false })
  const schedule = useScheduleData()
  const reasonId = useId()
  const blockId = useId()
  const whenRefs = useRef(new Map<PowerWhen, HTMLButtonElement>())
  const tileRefs = useRef(new Map<PowerAction, HTMLButtonElement>())
  const backRef = useRef<HTMLButtonElement>(null)
  // 확인 단계에서 돌아오면 고른 타일에 포커스를 돌려준다.
  const lastStep = useRef<PowerAction | null>(null)

  // 절전을 쓸 수 있는지는 OMV 전원 관리의 대기 방식으로 정한다(관리자만 받는다).
  useEffect(() => {
    const ctl = new AbortController()
    fetchAddon(POWER_ADDON, isSystemStatsResponse, { query: { power: '1' }, signal: ctl.signal })
      .then((res) => setPower(powerInfoOf(res)))
      .catch(() => {
        if (!ctl.signal.aborted) setPower({ failed: true })
      })
    return () => ctl.abort()
  }, [])

  // 단계가 바뀌면 포커스를 옮긴다. 확인 단계는 "뒤로"(실수로 Enter를 눌러도 보내지 않게), 돌아오면 고른 타일.
  useLayoutEffect(() => {
    if (step) backRef.current?.focus()
    else if (lastStep.current) tileRefs.current.get(lastStep.current)?.focus()
    lastStep.current = step
  }, [step])

  // 예약할 수 없는 세션(상승할 수 없음, 도우미 없음)은 "시간 정해서"를 막고 이유를 보인다.
  const block = scheduleBlock(schedule.data.helper)
  const later = step === 'reboot' && when === 'later' && block === null
  useEffect(() => {
    if (block && store.getState().when === 'later') store.setState({ when: 'now' })
  }, [block, store])

  // 보내다 실패하면 단추가 다시 켜진 뒤(렌더 뒤) "뒤로"에 포커스를 돌려준다. 꺼진 단추에서 포커스를 잃었다.
  // 예약 화면은 자기 단추로 돌려준다(PowerSchedule.tsx).
  useEffect(() => {
    if (error && !sending && !later) backRef.current?.focus()
  }, [error, sending, later])

  const sleep = sleepState(power.info, power.failed)
  const sleepMode = sleep.kind === 'available' ? sleep.modeKey : undefined

  const send = async (action: PowerAction) => {
    if (store.getState().sending) return
    store.setState({ sending: true, error: null })
    let outcome: ReturnType<typeof sendOutcome> = 'sent'
    try {
      await runAddonAction(POWER_ADDON, action, '')
    } catch (err) {
      outcome = sendOutcome(err)
    }
    if (outcome === 'sent') {
      onSent(action, action === 'standby' ? sleepMode : undefined)
      return
    }
    store.setState({ sending: false, error: outcome })
  }

  // 다시 시작의 "지금"·"시간 정해서" 고르기. 방향키로 옮기며 고른다(라디오 그룹).
  const chooseWhen = (next: PowerWhen) => {
    if (store.getState().sending || (next === 'later' && block)) return
    store.setState({ when: next, error: null })
  }
  const onWhenKey = (e: KeyboardEvent) => {
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) return
    e.preventDefault()
    const next: PowerWhen = later ? 'now' : 'later'
    if (next === 'later' && block) return
    chooseWhen(next)
    whenRefs.current.get(next)?.focus()
  }
  const whenSwitch = (
    <>
      <div className="power-when" role="radiogroup" aria-label={t('power.schedule.when')} onKeyDown={onWhenKey}>
        {(['now', 'later'] as const).map((w) => {
          const checked = (w === 'later') === later
          const blocked = w === 'later' && block !== null
          return (
            <button
              key={w}
              ref={(el) => {
                if (el) whenRefs.current.set(w, el)
                else whenRefs.current.delete(w)
              }}
              type="button"
              role="radio"
              className="power-when-option"
              aria-checked={checked}
              tabIndex={checked ? 0 : -1}
              aria-disabled={blocked || undefined}
              aria-describedby={blocked ? blockId : undefined}
              disabled={sending}
              onClick={() => chooseWhen(w)}
            >
              {t(w === 'now' ? 'power.schedule.now' : 'power.schedule.later')}
            </button>
          )
        })}
      </div>
      {block && (
        <p id={blockId} className="power-reason">
          <IconImage name="dialog-warning-symbolic" size={16} />
          <span>{t(block)}</span>
        </p>
      )}
    </>
  )

  if (step && later) {
    return (
      <div className="power-dialog">
        {whenSwitch}
        <ScheduleView store={store} state={schedule} backRef={backRef} onScheduled={onScheduled} />
      </div>
    )
  }

  if (step) {
    const spec = powerActionSpec(step)
    return (
      <div className="power-dialog">
        {step === 'reboot' && whenSwitch}
        <p className="modal-message">{t(spec.confirmMessageKey)}</p>
        {error && (
          <p className="power-error selectable" role="alert">
            <IconImage name="dialog-warning-symbolic" size={16} />
            <span>{t(error)}</span>
          </p>
        )}
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
          <button type="button" className="addon-button danger" disabled={sending} onClick={() => void send(step)}>
            {sending ? (
              <>
                <span className="power-spinner" aria-hidden="true" />
                {t('power.sending')}
              </>
            ) : (
              t(spec.labelKey)
            )}
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="power-dialog">
      <div className="power-tiles">
        {POWER_ACTIONS.map((a) => {
          const isSleep = a.id === 'standby'
          const unavailable = isSleep && sleep.kind !== 'available'
          let detail: string | null = null
          const scheduled = schedule.data.schedule
          if (a.id === 'reboot') {
            if (scheduled?.mode === 'reboot') detail = t('power.schedule.tileScheduled')
            else if (block === null) detail = t('power.schedule.tile')
          }
          if (a.id === 'shutdown' && scheduled && scheduled.mode !== 'reboot') detail = t('power.schedule.tileScheduled')
          if (isSleep) {
            if (sleep.kind === 'loading') detail = t('power.checking')
            else if (sleep.kind === 'unavailable') detail = t('power.unavailable')
            else if (sleep.modeKey) detail = t(sleep.modeKey)
          }
          return (
            <button
              key={a.id}
              ref={(el) => {
                if (el) tileRefs.current.set(a.id, el)
                else tileRefs.current.delete(a.id)
              }}
              type="button"
              className={a.danger ? 'power-tile danger' : 'power-tile'}
              // 쓸 수 없는 절전도 포커스는 받게 두고 이유를 함께 읽힌다.
              aria-disabled={unavailable || undefined}
              aria-describedby={isSleep && sleep.kind === 'unavailable' ? reasonId : undefined}
              onClick={() => {
                if (!unavailable) store.setState({ step: a.id, when: 'now', error: null })
              }}
            >
              <IconImage name={a.icon} size={32} className="power-tile-icon" />
              <span className="power-tile-label">{t(a.labelKey)}</span>
              <span className="power-tile-detail">{detail}</span>
            </button>
          )
        })}
      </div>
      {sleep.kind === 'unavailable' && (
        <p id={reasonId} className="power-reason">
          <IconImage name="dialog-warning-symbolic" size={16} />
          <span>{t(sleep.reasonKey)}</span>
        </p>
      )}
      <p className="modal-message power-notice">{t('power.notice')}</p>
      <div className="modal-buttons">
        <button type="button" className="addon-button" onClick={onCancel}>
          {t('dialog.cancel')}
        </button>
      </div>
    </div>
  )
}
