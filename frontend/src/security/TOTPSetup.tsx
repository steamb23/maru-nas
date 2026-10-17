// 인증 앱(TOTP) 등록 대화상자의 내용. 백엔드가 만든 비밀 키를 QR 코드와 글자로 보여 주고, 인증 앱이 보여 준 코드로
// 확인해 저장한다(관리 창 필요). QR 라이브러리(uqr)는 이 대화상자를 열 때만 내려받는다.
// 여는 함수는 securityDialogs.tsx의 setupTOTPDialog.
import type { RefObject } from 'react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { confirmTOTP, setupTOTP } from '../api/client'
import type { SecurityChangeResponse, TOTPSetupResponse } from '../api/types.gen'
import { formatClock } from '../shell/elevation'
import { type SecurityError, groupSecret, qrPath, securityError } from './messages'

interface Qr {
  size: number
  path: string
}

export function TOTPSetup({
  inputRef,
  onDone,
  onCancel,
}: {
  inputRef: RefObject<HTMLInputElement | null>
  onDone: (r: SecurityChangeResponse) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [setup, setSetup] = useState<TOTPSetupResponse | null>(null)
  const [qr, setQr] = useState<Qr | null>(null)
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<SecurityError | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let live = true
    setupTOTP().then(
      async (s) => {
        if (!live) return
        setSetup(s)
        try {
          const { encode } = await import('uqr')
          const r = encode(s.uri, { ecc: 'M', border: 2 })
          if (live) setQr({ size: r.size, path: qrPath(r.data) })
        } catch {
          // QR을 그리지 못해도 비밀 키를 직접 입력할 수 있다.
        }
      },
      (err: unknown) => {
        if (live) setError(securityError(err))
      },
    )
    return () => {
      live = false
    }
  }, [])

  useEffect(() => {
    if (setup && !busy) inputRef.current?.focus()
  }, [setup, busy, error, inputRef])

  const submit = async () => {
    if (busy || code.trim() === '') return
    setBusy(true)
    setError(null)
    try {
      onDone(await confirmTOTP(code.trim()))
    } catch (err) {
      setError(securityError(err, 'totp'))
      setCode('')
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
      <p className="modal-message">{t('security.totp.intro')}</p>
      {setup && (
        <div className="totp-setup">
          {qr && (
            <svg
              className="totp-setup-qr"
              viewBox={`0 0 ${qr.size} ${qr.size}`}
              role="img"
              aria-label={t('security.totp.qrLabel')}
              shapeRendering="crispEdges"
            >
              <rect width={qr.size} height={qr.size} fill="#fff" />
              <path d={qr.path} fill="#000" />
            </svg>
          )}
          <div className="totp-setup-key">
            <span>{t('security.totp.key')}</span>
            <code className="selectable">{groupSecret(setup.secret)}</code>
            {navigator.clipboard && (
              <button
                type="button"
                className="step-up-link"
                onClick={() => navigator.clipboard.writeText(setup.secret).then(() => setCopied(true), () => {})}
              >
                {copied ? t('security.recovery.copied') : t('security.totp.copyKey')}
              </button>
            )}
          </div>
        </div>
      )}
      {setup && (
        <label className="step-up-field">
          <span>{t('security.totp.code')}</span>
          <input
            ref={inputRef}
            className="step-up-input"
            name="code"
            autoComplete="one-time-code"
            inputMode="numeric"
            value={code}
            disabled={busy}
            onChange={(e) => setCode(e.target.value)}
          />
        </label>
      )}
      {!setup && !error && <p className="modal-message">{t('settings.common.loading')}</p>}
      {error && (
        <p className="step-up-error" role="alert">
          {t(error.key, { remaining: formatClock(error.retryAfter ?? 0) })}
        </p>
      )}
      <div className="modal-buttons">
        <button type="button" className="addon-button" disabled={busy} onClick={onCancel}>
          {t('dialog.cancel')}
        </button>
        <button type="submit" className="addon-button primary" disabled={busy || !setup || code.trim() === ''}>
          {busy ? t('security.stepUp.confirming') : t('security.totp.finish')}
        </button>
      </div>
    </form>
  )
}
