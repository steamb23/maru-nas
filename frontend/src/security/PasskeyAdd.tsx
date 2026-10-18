// 패스키 등록 대화상자의 내용. 이름을 받은 뒤 브라우저의 패스키 만들기 창을 열어 등록 값을 보내고, 이어서 그 패스키로
// 한 번 더 확인(시험 확인)해 통과해야 백엔드가 저장한다(관리 창 필요). 확인 응답에 사용자 확인(UV)을 켜 주지 않는
// 저장소를 등록할 때 걸러 내기 위해서다. 여는 함수는 securityDialogs.tsx의 addPasskeyDialog.
import type { RefObject } from 'react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { confirmPasskey, registerPasskey } from '../api/client'
import type { PasskeyOptions, SecurityChangeResponse } from '../api/types.gen'
import { formatClock } from '../shell/elevation'
import { passkeyNotSaved, type SecurityError, securityError } from './messages'
import { assertWith, createPasskey, forgetPasskey } from './webauthn'

// MAX_NAME은 백엔드의 패스키 이름 상한(64자)과 같다.
const MAX_NAME = 64

// Leftover는 백엔드가 저장하지 않은 새 패스키가 패스키 저장소에 남았을 때의 안내다.
type Leftover = { signaled: boolean; rpId: string }

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
  const [leftover, setLeftover] = useState<Leftover | null>(null)
  // confirm은 등록 값을 보낸 뒤 기다리는 시험 확인의 옵션이다. 있으면 다시 만들지 않고 확인만 다시 한다.
  const [confirm, setConfirm] = useState<PasskeyOptions | null>(null)
  // pending은 시험 확인을 아직 보내지 않은 새 패스키다. 이대로 닫으면 백엔드가 저장하지 않으므로 저장소에서 지우게 한다.
  const pending = useRef<PasskeyOptions | null>(null)

  useEffect(() => {
    if (error && !busy) inputRef.current?.focus()
  }, [error, busy, inputRef])

  useEffect(
    () => () => {
      const p = pending.current
      const id = p?.credentials[0]?.id
      if (p && id) void forgetPasskey(p.rpId, id)
    },
    [],
  )

  // reject는 백엔드가 거절한 새 패스키를 저장소에서 지우게 하고 안내를 남긴다.
  const reject = async (err: unknown, rpId: string, id: string) => {
    pending.current = null
    setConfirm(null)
    setError(securityError(err, 'passkey'))
    if (passkeyNotSaved(err)) setLeftover({ signaled: await forgetPasskey(rpId, id), rpId })
  }

  const verify = async (opts: PasskeyOptions) => {
    let assertion
    try {
      assertion = await assertWith(opts)
    } catch (err) {
      // 취소했거나 브라우저가 창을 열지 않았다(이어서 여는 창을 막는 브라우저가 있다). 대기가 남아 있어 다시 확인할 수 있다.
      setError(securityError(err, 'passkey'))
      return
    }
    // 보낸 뒤에는 결과를 모르는 동안 닫혀도 지우게 하지 않는다(저장됐을 수 있다).
    pending.current = null
    try {
      onDone(await confirmPasskey(assertion))
    } catch (err) {
      const id = opts.credentials[0]?.id
      if (id) await reject(err, opts.rpId, id)
    }
  }

  const submit = async () => {
    const n = name.trim()
    if (busy || n === '') return
    setBusy(true)
    setError(null)
    setLeftover(null)
    if (confirm) {
      await verify(confirm)
      setBusy(false)
      return
    }
    let created
    try {
      created = await createPasskey()
    } catch (err) {
      setError(securityError(err, 'passkey'))
      setBusy(false)
      return
    }
    try {
      const opts = await registerPasskey(n, created.registration)
      pending.current = opts
      setConfirm(opts)
      await verify(opts)
    } catch (err) {
      await reject(err, created.rpId, created.registration.id)
    }
    setBusy(false)
  }

  let label = t('security.passkeys.create')
  if (busy) label = confirm ? t('security.passkeys.confirming') : t('security.passkeys.creating')
  else if (confirm) label = t('security.passkeys.retryConfirm')

  return (
    <form
      className="step-up-form"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <p className="modal-message">{t(confirm ? 'security.passkeys.confirmIntro' : 'security.passkeys.addIntro')}</p>
      <label className="step-up-field">
        <span>{t('security.passkeys.name')}</span>
        <input
          ref={inputRef}
          className="step-up-input"
          name="passkey-name"
          autoComplete="off"
          maxLength={MAX_NAME}
          value={name}
          disabled={busy || confirm !== null}
          onChange={(e) => setName(e.target.value)}
        />
      </label>
      {error && (
        <p className="step-up-error" role="alert">
          {t(error.key, { remaining: formatClock(error.retryAfter ?? 0) })}
          {leftover && (
            <>
              {' '}
              {leftover.signaled
                ? t('security.passkeys.leftoverSignaled')
                : t('security.passkeys.leftoverManual', { rpId: leftover.rpId })}
            </>
          )}
        </p>
      )}
      <div className="modal-buttons">
        <button type="button" className="addon-button" disabled={busy} onClick={onCancel}>
          {t('dialog.cancel')}
        </button>
        <button type="submit" className="addon-button primary" disabled={busy || name.trim() === ''}>
          {label}
        </button>
      </div>
    </form>
  )
}
