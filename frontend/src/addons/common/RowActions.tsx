// 애드온 창의 행 조작: 행 버튼, 실패 표시, 조작 확인 모달(행 버튼·우클릭 메뉴 공통).
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { ParseKeys } from 'i18next'
import { IconImage } from '../../icons/IconImage'
import { Modal } from '../../shell/Modal'
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
  const confirm = ctl.state.confirm?.target === target ? ctl.state.confirm : null
  return (
    <span className="row-actions">
      {specs.map((spec) => (
        <button
          key={spec.action}
          type="button"
          className={`row-action-button${confirm?.action === spec.action ? ' pressed' : ''}`}
          title={t(spec.labelKey)}
          aria-label={t('actions.rowLabel', { action: t(spec.labelKey), name: label })}
          onClick={() => ctl.request(target, spec.action)}
        >
          <IconImage name={spec.icon} size={16} />
        </button>
      ))}
    </span>
  )
}

// RowActionPanel은 행 아래 줄에 마지막 조작의 실패 문구를 그린다. 실패가 없으면 그리지 않는다.
export function RowActionPanel({ ctl, target, specs }: RowProps) {
  const { t } = useTranslation()
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

// ActionConfirmDialog는 조작 확인 모달이다. 행 버튼과 우클릭 메뉴 어느 쪽으로 열어도 같고, 그 창만 막는다.
// 확인할 대상이 목록에서 사라지면 닫힌다.
export function ActionConfirmDialog({ ctl, lookup }: DialogProps) {
  const { t } = useTranslation()
  const confirm = ctl.state.confirm
  const found = confirm ? lookup(confirm.target) : null
  const spec = found?.specs.find((s) => s.action === confirm?.action)
  const cancelRef = useRef<HTMLButtonElement>(null)

  return (
    <Modal
      open={spec !== undefined}
      scope="window"
      role="alertdialog"
      title={spec && t(spec.labelKey)}
      initialFocus={cancelRef}
      onCancel={ctl.cancel}
      footer={
        spec && (
          <>
            <button type="button" className="addon-button" ref={cancelRef} onClick={ctl.cancel}>
              {t('dialog.cancel')}
            </button>
            <button type="button" className={`addon-button${spec.danger ? ' danger' : ''}`} onClick={ctl.confirm}>
              {t(spec.labelKey)}
            </button>
          </>
        )
      }
    >
      {spec && found && <p className="modal-message">{t(spec.confirmKey, { name: found.label })}</p>}
    </Modal>
  )
}
