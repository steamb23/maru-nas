// 권한 상승 대화상자의 내용(제목, 비밀번호 폼). 여는 함수는 elevationDialog.tsx, 계산은 elevation.ts.
import type { RefObject } from 'react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, elevate } from '../api/client'
import type { HelperStatus } from '../api/types.gen'
import { requestStepUp } from '../security/stepUpDialog'
import type { DialogScope } from './dialogs'
import { type ElevationError, elevationError, formatClock } from './elevation'

export function ElevationTitle() {
  const { t } = useTranslation()
  return t('elevation.title')
}

// 단추가 입력 상태(진행 중, 오류)를 함께 보아야 하므로 단추 줄도 본문 안의 폼에 둔다(Enter로 제출).
export function ElevationForm({
  scope,
  inputRef,
  onDone,
  onCancel,
}: {
  scope: DialogScope
  inputRef: RefObject<HTMLInputElement | null>
  onDone: (status: HelperStatus) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<ElevationError | null>(null)

  // 실패하면 입력 상자가 다시 켜진 뒤(렌더 뒤) 포커스를 돌려준다. 꺼진 상자에는 포커스할 수 없다.
  useEffect(() => {
    if (error && !busy) inputRef.current?.focus()
  }, [error, busy, inputRef])

  const submit = async () => {
    if (busy || password === '') return
    setBusy(true)
    setError(null)
    try {
      onDone(await elevate(password))
    } catch (err) {
      // 보안 강화 정책이 상승 전에 확인을 요구하면 확인한 뒤 같은 비밀번호로 다시 시도한다.
      if (err instanceof ApiError && err.code === 'security_required') {
        if (await requestStepUp(scope, { level: err.required, target: 'elevation' })) {
          try {
            onDone(await elevate(password))
            return
          } catch (again) {
            setError(elevationError(again))
          }
        }
        setBusy(false)
        return
      }
      setError(elevationError(err))
      setPassword('')
      setBusy(false)
    }
  }

  return (
    <form
      className="elevation-form"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <p className="modal-message">{t('elevation.description')}</p>
      <label className="elevation-field">
        <span>{t('elevation.password')}</span>
        <input
          ref={inputRef}
          className="elevation-input"
          name="password"
          type="password"
          autoComplete="current-password"
          value={password}
          disabled={busy}
          onChange={(e) => setPassword(e.target.value)}
        />
      </label>
      {error && (
        <p className="elevation-error" role="alert">
          {t(error.key, { remaining: formatClock(error.retryAfter ?? 0) })}
        </p>
      )}
      <div className="modal-buttons">
        <button type="button" className="addon-button" disabled={busy} onClick={onCancel}>
          {t('dialog.cancel')}
        </button>
        <button type="submit" className="addon-button primary" disabled={busy || password === ''}>
          {busy ? t('elevation.confirming') : t('elevation.confirm')}
        </button>
      </div>
    </form>
  )
}
