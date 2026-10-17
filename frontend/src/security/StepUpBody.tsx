// 확인·관리 대화상자(stepUpDialog.tsx)의 내용. 열린 뒤 지금 보안 상태(GET /api/security)를 받아 필요한 확인을 고르고
// StepUpForm을 그린다. 보안 대화상자들이 함께 쓰는 제목(SecurityDialogTitle)도 둔다.
import type { ParseKeys } from 'i18next'
import type { RefObject } from 'react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { type RequiredLevel, fetchSecurity } from '../api/client'
import type { SecurityStatus } from '../api/types.gen'
import { type SecurityTarget, satisfies, securityError } from './messages'
import { StepUpForm } from './StepUpForm'

export interface StepUpRequest {
  // level은 거쳐야 하는 단계다(ErrorResponse.required). 없으면 target 기능의 정책을 따르고, 그것도 없으면
  // 등록한 수단이 있으면 2차 인증, 없으면 비밀번호 재확인이다(설정의 "지금 확인").
  level?: RequiredLevel
  target?: SecurityTarget
}

export function SecurityDialogTitle({ k }: { k: ParseKeys }) {
  const { t } = useTranslation()
  return t(k)
}

type Load = { state: 'loading' } | { state: 'error'; key: ParseKeys } | { state: 'ready'; data: SecurityStatus }

export function StepUpBody({
  mode,
  req,
  inputRef,
  onDone,
  onCancel,
}: {
  mode: 'verify' | 'manage'
  req: StepUpRequest
  inputRef: RefObject<HTMLInputElement | null>
  onDone: (s: SecurityStatus) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [load, setLoad] = useState<Load>({ state: 'loading' })

  useEffect(() => {
    const ctl = new AbortController()
    fetchSecurity(ctl.signal).then(
      (data) => setLoad({ state: 'ready', data }),
      (err: unknown) => {
        if (!ctl.signal.aborted) setLoad({ state: 'error', key: securityError(err).key })
      },
    )
    return () => ctl.abort()
  }, [])

  const data = load.state === 'ready' ? load.data : null
  const level = data
    ? (req.level ?? (req.target && data.policy[req.target]) ?? (data.methods.length > 0 ? 'twoFactor' : 'password'))
    : null
  // 이미 거친 단계면 묻지 않는다(다른 탭에서 확인했거나 정책이 바뀌었다). 세션 중 확인에서 off는 할 일이 없다.
  const already = mode === 'verify' && data !== null && level !== null && (level === 'off' || satisfies(data.assurance, level))
  useEffect(() => {
    if (already && data) onDone(data)
  }, [already, data, onDone])

  if (data === null || already || level === null || level === 'off') {
    return (
      <>
        <p className="modal-message" role={load.state === 'error' ? 'alert' : 'status'}>
          {load.state === 'error' ? t(load.key) : t('settings.common.loading')}
        </p>
        <div className="modal-buttons">
          <button type="button" className="addon-button" onClick={onCancel}>
            {t('dialog.cancel')}
          </button>
        </div>
      </>
    )
  }
  return (
    <StepUpForm
      variant="dialog"
      mode={mode}
      level={level}
      methods={data.methods}
      onDone={onDone}
      onCancel={onCancel}
      cancelLabel={t('dialog.cancel')}
      inputRef={inputRef}
    />
  )
}
