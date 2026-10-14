// 애드온 창의 행 조작: 행 버튼, 인라인 확인, 우클릭 메뉴로 연 확인 대화상자.
import { type KeyboardEvent, useId, useLayoutEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { ParseKeys } from 'i18next'
import { IconImage } from '../../icons/IconImage'
import type { ActionErrorCode } from './actionState'
import type { ActionSpec } from './actionSpec'
import type { AddonActions } from './useAddonActions'

const ERROR_KEYS: Partial<Record<ActionErrorCode, ParseKeys>> = {
  action_busy: 'actions.error.action_busy',
  action_failed: 'actions.error.action_failed',
  action_timeout: 'actions.error.action_timeout',
  config_dirty: 'actions.error.config_dirty',
  admin_required: 'actions.error.admin_required',
  plugin_missing: 'actions.error.plugin_missing',
  omv_error: 'actions.error.omv_error',
  omv_unavailable: 'actions.error.omv_error',
}

// focusOnOpen은 열 때 el에 포커스하고, 닫은 뒤 포커스가 갈 곳을 잃었으면(사라진 요소에 있었으면) 열기 전 요소로
// 돌려주는 정리 함수를 반환한다. 닫는 렌더가 끝난 뒤 확인하도록 마이크로태스크로 미룬다.
function focusOnOpen(el: HTMLElement | null): () => void {
  const prev = document.activeElement instanceof HTMLElement ? document.activeElement : null
  el?.focus()
  return () =>
    queueMicrotask(() => {
      const lost = !document.activeElement || document.activeElement === document.body
      if (lost && prev?.isConnected) prev.focus()
    })
}

function errorKey(code: ActionErrorCode): ParseKeys {
  return ERROR_KEYS[code] ?? 'actions.error.unknown'
}

interface RowProps {
  ctl: AddonActions
  target: string
  // 지금 상태에서 쓸 수 있는 조작
  specs: readonly ActionSpec[]
  // 버튼 이름·확인·오류 문구에 쓰는 대상 이름
  label: string
}

// RowActionButtons는 행 끝의 조작 버튼이다. 진행 중이면 버튼 대신 진행 문구를 보여 준다.
export function RowActionButtons({ ctl, target, specs, label }: RowProps) {
  const { t } = useTranslation()
  const st = ctl.state.targets[target]
  if (st?.kind === 'running' || st?.kind === 'settling') {
    const spec = specs.find((s) => s.action === st.action)
    return (
      <span className="row-actions-progress" role="status">
        <span className="row-actions-spinner" aria-hidden="true" />
        {spec ? t(spec.runningKey) : t('actions.working')}
      </span>
    )
  }
  const confirming = ctl.state.confirm?.target === target && ctl.state.confirm.via === 'inline'
  return (
    <span className="row-actions">
      {specs.map((spec) => (
        <button
          key={spec.action}
          type="button"
          className={`row-action-button${confirming && ctl.state.confirm?.action === spec.action ? ' pressed' : ''}`}
          title={t(spec.labelKey)}
          aria-label={t('actions.rowLabel', { action: t(spec.labelKey), name: label })}
          onClick={() => ctl.request(target, spec.action, 'inline')}
        >
          <IconImage name={spec.icon} size={16} />
        </button>
      ))}
    </span>
  )
}

// RowActionPanel은 행 아래 줄에 인라인 확인이나 실패 문구를 그린다. 둘 다 없으면 그리지 않는다.
export function RowActionPanel({ ctl, target, specs, label }: RowProps) {
  const { t } = useTranslation()
  const confirm = ctl.state.confirm
  const spec = confirm?.target === target && confirm.via === 'inline' ? specs.find((s) => s.action === confirm.action) : undefined
  const cancelRef = useRef<HTMLButtonElement>(null)
  const shown = spec !== undefined
  // 열 때 취소에 포커스하고, 닫힐 때 포커스가 갈 곳을 잃었으면 연 버튼으로 돌려준다.
  useLayoutEffect(() => (shown ? focusOnOpen(cancelRef.current) : undefined), [shown])

  if (spec) {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        ctl.cancel()
      }
    }
    return (
      <div className="row-confirm" role="group" aria-label={t(spec.labelKey)} onKeyDown={onKeyDown}>
        <span className="row-confirm-text">{t(spec.confirmKey, { name: label })}</span>
        <button type="button" className="addon-button" ref={cancelRef} onClick={ctl.cancel}>
          {t('actions.cancel')}
        </button>
        <button type="button" className={`addon-button${spec.danger ? ' danger' : ''}`} onClick={ctl.confirm}>
          {t(spec.labelKey)}
        </button>
      </div>
    )
  }

  const st = ctl.state.targets[target]
  if (st?.kind !== 'error') return null
  const failed = specs.find((s) => s.action === st.action)
  return (
    <div className="row-error" role="alert">
      <span>
        {failed && <strong>{t('actions.failedPrefix', { action: t(failed.labelKey) })}</strong>} {t(errorKey(st.code))}
      </span>
      <button
        type="button"
        className="row-action-button"
        title={t('actions.dismiss')}
        aria-label={t('actions.dismiss')}
        onClick={() => ctl.dismiss(target)}
      >
        <IconImage name="window-close-symbolic" size={12} />
      </button>
    </div>
  )
}

interface DialogProps {
  ctl: AddonActions
  // 대상의 조작 정의와 표시 이름. 대상이 사라졌으면 null을 돌려준다.
  lookup: (target: string) => { specs: readonly ActionSpec[]; label: string } | null
}

// ActionConfirmDialog는 우클릭 메뉴로 고른 조작의 확인 대화상자다. 창 본문 위를 덮어 그 창만 막는다
// (.window 기준으로 절대 위치를 잡으므로 창 본문을 스크롤해도 제자리에 있다).
export function ActionConfirmDialog({ ctl, lookup }: DialogProps) {
  const { t } = useTranslation()
  const confirm = ctl.state.confirm
  const found = confirm?.via === 'dialog' ? lookup(confirm.target) : null
  const spec = found?.specs.find((s) => s.action === confirm?.action)
  const boxRef = useRef<HTMLDivElement>(null)
  const cancelRef = useRef<HTMLButtonElement>(null)
  const idBase = useId()
  const open = spec !== undefined

  // 열 때 취소에 포커스하고, 닫으면 열기 전 포커스로 돌려준다.
  useLayoutEffect(() => (open ? focusOnOpen(cancelRef.current) : undefined), [open])

  if (!spec || !found) return null

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.stopPropagation()
      ctl.cancel()
      return
    }
    if (e.key !== 'Tab') return
    // 포커스를 대화상자 안에 가둔다.
    const buttons = boxRef.current?.querySelectorAll('button')
    if (!buttons || buttons.length === 0) return
    const first = buttons[0]
    const last = buttons[buttons.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }

  return (
    <div className="action-dialog-backdrop" onPointerDown={(e) => e.target === e.currentTarget && ctl.cancel()}>
      <div
        ref={boxRef}
        className="action-dialog"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={`${idBase}-title`}
        aria-describedby={`${idBase}-text`}
        onKeyDown={onKeyDown}
      >
        <h2 id={`${idBase}-title`} className="action-dialog-title">
          {t(spec.labelKey)}
        </h2>
        <p id={`${idBase}-text`} className="action-dialog-text">
          {t(spec.confirmKey, { name: found.label })}
        </p>
        <div className="action-dialog-buttons">
          <button type="button" className="addon-button" ref={cancelRef} onClick={ctl.cancel}>
            {t('actions.cancel')}
          </button>
          <button type="button" className={`addon-button${spec.danger ? ' danger' : ''}`} onClick={ctl.confirm}>
            {t(spec.labelKey)}
          </button>
        </div>
      </div>
    </div>
  )
}
