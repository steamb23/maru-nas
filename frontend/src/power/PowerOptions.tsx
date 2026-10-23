// 전원 모달의 내용: 타일(시스템 종료·다시 시작·절전)을 고르면 같은 모달이 확인 단계로 바뀐다.
// 여는 함수는 powerDialog.tsx, 계산은 power.ts, 단계 상태는 dialogStore.ts다.
import type { ParseKeys } from 'i18next'
import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { fetchAddon, runAddonAction } from '../api/client'
import type { PowerInfo } from '../api/types.gen'
import { isSystemStatsResponse } from '../addons/system-info/stats'
import { IconImage } from '../icons/IconImage'
import type { PowerDialogStore } from './dialogStore'
import {
  POWER_ACTIONS,
  POWER_ADDON,
  type PowerAction,
  powerActionSpec,
  powerInfoOf,
  sendOutcome,
  sleepState,
} from './power'

// PowerTitle은 모달 제목이다. 확인 단계에서는 그 조작을 묻는다.
export function PowerTitle({ store }: { store: PowerDialogStore }) {
  const { t } = useTranslation()
  const step = store((s) => s.step)
  if (!step) return t('power.title')
  const spec = powerActionSpec(step)
  return (
    <span className="power-title">
      <span className={spec.danger ? 'power-title-icon danger' : 'power-title-icon'}>
        <IconImage name={spec.icon} size={16} />
      </span>
      {t(spec.confirmTitleKey)}
    </span>
  )
}

interface BodyProps {
  store: PowerDialogStore
  onCancel: () => void
  // onSent는 조작을 보낸 뒤(또는 보내다 연결이 끊긴 뒤) 부른다. modeKey는 절전의 대기 방식 이름이다.
  onSent: (action: PowerAction, modeKey?: ParseKeys) => void
}

// 단추가 단계와 진행 상태를 함께 보아야 하므로 단추 줄도 본문 안에 둔다(권한 상승 대화상자와 같다).
export function PowerBody({ store, onCancel, onSent }: BodyProps) {
  const { t } = useTranslation()
  const { step, sending, error } = store()
  const [power, setPower] = useState<{ info?: PowerInfo; failed: boolean }>({ failed: false })
  const reasonId = useId()
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

  if (step) {
    const spec = powerActionSpec(step)
    return (
      <div className="power-dialog">
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
                if (!unavailable) store.setState({ step: a.id, error: null })
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
