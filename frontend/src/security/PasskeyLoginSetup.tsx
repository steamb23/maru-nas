// 패스키로 로그인을 이 브라우저에서 켜는 대화상자의 내용. 비밀번호를 받아 패스키 확인(PRF 함께 요청) 한 번으로
// 켠다(passkeyLoginFlow.ts의 enablePasskeyLogin). 비밀번호와 패스키를 함께 확인하므로 관리 창은 필요 없고, 세션은
// twoFactor로 오른다. 여는 함수는 securityDialogs.tsx의 enablePasskeyLoginDialog.
import type { RefObject } from 'react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { formatUnit } from '../addons/common/format'
import type { SecurityStatus } from '../api/types.gen'
import { useLanguage } from '../i18n'
import { formatClock } from '../shell/elevation'
import { type SecurityError, passkeyLoginEnableError } from './messages'
import { enablePasskeyLogin } from './passkeyLoginFlow'

export function PasskeyLoginSetup({
  username,
  days,
  inputRef,
  onDone,
  onCancel,
}: {
  username: string
  // days는 지금 켜면 쓸 수 있는 기간(일)이다(정책의 passkeyLoginDays).
  days: number
  inputRef: RefObject<HTMLInputElement | null>
  onDone: (status: SecurityStatus) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<SecurityError | null>(null)

  // 실패한 뒤(입력 상자가 다시 켜진 뒤) 비밀번호 칸에 포커스한다. 처음 포커스는 대화상자(initialFocus)가 한다.
  useEffect(() => {
    if (error && !busy) inputRef.current?.focus()
  }, [error, busy, inputRef])

  const submit = async () => {
    if (busy || password === '') return
    setBusy(true)
    setError(null)
    try {
      const { status } = await enablePasskeyLogin({ username, password })
      onDone(status)
    } catch (err) {
      setError(passkeyLoginEnableError(err))
      setPassword('')
      setBusy(false)
    }
  }

  return (
    <form
      className="step-up-form"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <p className="modal-message">{t('security.passkeyLogin.intro', { days: formatUnit(days, 'day', lang) })}</p>
      <label className="step-up-field">
        <span>{t('security.stepUp.password')}</span>
        <input
          ref={inputRef}
          className="step-up-input"
          name="password"
          type="password"
          autoComplete="current-password"
          value={password}
          disabled={busy}
          onChange={(e) => setPassword(e.target.value)}
        />
      </label>
      {error && (
        <p className="step-up-error" role="alert">
          {t(error.key, { remaining: formatClock(error.retryAfter ?? 0) })}
        </p>
      )}
      <div className="modal-buttons">
        <button type="button" className="addon-button" disabled={busy} onClick={onCancel}>
          {t('dialog.cancel')}
        </button>
        <button type="submit" className="addon-button primary" disabled={busy || password === ''}>
          {busy ? t('security.passkeyLogin.enabling') : t('security.passkeyLogin.enable')}
        </button>
      </div>
    </form>
  )
}
