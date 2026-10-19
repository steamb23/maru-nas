// 로그인 직후 패스키로 로그인 켜기 권유(인증 저장소의 passkeyOffer, 2차 인증 권유가 없을 때). 로그인 폼에 남아 있는
// 비밀번호로 이 브라우저의 패스키로 로그인을 켠다(passkeyLoginFlow.ts). 2차 인증 권유와 함께 오면 StepUpForm의
// 확인란이 대신한다.
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { formatUnit } from '../addons/common/format'
import type { SessionInfo } from '../api/types.gen'
import { useLanguage } from '../i18n'
import { type SecurityError, passkeyLoginEnableError } from '../security/messages'
import { enablePasskeyLogin } from '../security/passkeyLoginFlow'
import { dismissPasskeyLoginOffer } from '../security/passkeyLoginStorage'
import { formatClock } from '../shell/elevation'

export function PasskeyOffer({
  username,
  password,
  days,
  onFinish,
}: {
  username: string
  // password는 방금 로그인한 비밀번호다(LoginScreen 상태). 권유를 마치면 LoginScreen이 지운다.
  password: string
  days: number
  // onFinish는 권유를 마친다. 켰으면 새 보증 수준(twoFactor)을 넘긴다.
  onFinish: (assurance?: SessionInfo['assurance']) => void
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<SecurityError | null>(null)
  const enableRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!busy) enableRef.current?.focus()
  }, [busy, error])

  const enable = async () => {
    if (busy) return
    setBusy(true)
    setError(null)
    try {
      const { status } = await enablePasskeyLogin({ username, password })
      onFinish(status.assurance)
    } catch (err) {
      // 비밀번호는 방금 맞았으므로 틀린 값은 패스키 쪽이다.
      setError(passkeyLoginEnableError(err, 'passkey'))
      setBusy(false)
    }
  }

  const never = () => {
    dismissPasskeyLoginOffer(username)
    onFinish()
  }

  return (
    <div className="login-form">
      <p className="login-notice" role="status">
        {t('login.passkeyOffer')}
      </p>
      <p className="login-hint">{t('login.passkeyOfferDesc', { days: formatUnit(days, 'day', lang) })}</p>
      {error && (
        <p className="login-error" role="alert">
          {t(error.key, { remaining: formatClock(error.retryAfter ?? 0) })}
        </p>
      )}
      <button
        ref={enableRef}
        type="button"
        className="login-submit"
        disabled={busy || password === ''}
        onClick={() => void enable()}
      >
        {busy ? t('security.passkeyLogin.enabling') : t('security.passkeyLogin.enable')}
      </button>
      <button type="button" className="login-secondary" disabled={busy} onClick={() => onFinish()}>
        {t('login.passkeyOfferSkip')}
      </button>
      <div className="step-up-others">
        <button type="button" className="step-up-link" disabled={busy} onClick={never}>
          {t('login.passkeyOfferNever')}
        </button>
      </div>
    </div>
  )
}
