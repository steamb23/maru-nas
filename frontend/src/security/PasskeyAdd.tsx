// 패스키 등록 대화상자의 내용. 이름을 받은 뒤 브라우저의 패스키 만들기 창을 열고, 결과를 백엔드에 등록한다(관리 창 필요).
// 여는 함수는 securityDialogs.tsx의 addPasskeyDialog.
import type { RefObject } from 'react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { registerPasskey } from '../api/client'
import type { SecurityChangeResponse } from '../api/types.gen'
import { formatClock } from '../shell/elevation'
import { type SecurityError, securityError } from './messages'
import { createPasskey } from './webauthn'

// MAX_NAME은 백엔드의 패스키 이름 상한(64자)과 같다.
const MAX_NAME = 64

export function PasskeyAdd({
  defaultName,
  inputRef,
  onDone,
  onCancel,
}: {
  defaultName: string
  inputRef: RefObject<HTMLInputElement | null>
  onDone: (r: SecurityChangeResponse) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(defaultName)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<SecurityError | null>(null)

  useEffect(() => {
    if (error && !busy) inputRef.current?.focus()
  }, [error, busy, inputRef])

  const submit = async () => {
    const n = name.trim()
    if (busy || n === '') return
    setBusy(true)
    setError(null)
    try {
      onDone(await registerPasskey(n, await createPasskey()))
    } catch (err) {
      setError(securityError(err, 'passkey'))
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
      <p className="modal-message">{t('security.passkeys.addIntro')}</p>
      <label className="step-up-field">
        <span>{t('security.passkeys.name')}</span>
        <input
          ref={inputRef}
          className="step-up-input"
          name="passkey-name"
          autoComplete="off"
          maxLength={MAX_NAME}
          value={name}
          disabled={busy}
          onChange={(e) => setName(e.target.value)}
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
        <button type="submit" className="addon-button primary" disabled={busy || name.trim() === ''}>
          {busy ? t('security.passkeys.creating') : t('security.passkeys.create')}
        </button>
      </div>
    </form>
  )
}
