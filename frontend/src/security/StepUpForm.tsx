// 보안 강화 모드의 확인 폼. 로그인 단계(로그인 카드)와 확인·관리 대화상자(stepUpDialog.tsx)가 함께 쓴다.
//   - verify: 이 세션을 level 단계로 올린다(password면 비밀번호, twoFactor면 등록한 2차 수단 하나)
//   - manage: 비밀번호와(등록한 수단이 있으면) 그 수단으로 관리 창(5분)을 연다
// 계산은 messages.ts, 패스키 창은 webauthn.ts.
import type { ReactNode, RefObject } from 'react'
import { useEffect, useRef, useState } from 'react'
import type { ParseKeys } from 'i18next'
import { useTranslation } from 'react-i18next'
import { manageSecurity, verifySecurity } from '../api/client'
import type { SecurityStatus, SecurityVerifyRequest } from '../api/types.gen'
import { formatClock } from '../shell/elevation'
import { type Factor, type SecurityError, firstFactor, securityError, usableFactors } from './messages'
import { getPasskey, passkeysSupported } from './webauthn'

type Variant = 'login' | 'dialog'

// 로그인 카드에서는 로그인 화면의 모양을, 대화상자에서는 대화상자의 모양을 쓴다.
const CLASSES: Record<Variant, Record<'form' | 'field' | 'input' | 'primary' | 'secondary' | 'hint' | 'error', string>> = {
  login: {
    form: 'login-form',
    field: 'login-field',
    input: '',
    primary: 'login-submit',
    secondary: 'login-secondary',
    hint: 'login-hint',
    error: 'login-error',
  },
  dialog: {
    form: 'step-up-form',
    field: 'step-up-field',
    input: 'step-up-input',
    primary: 'addon-button primary',
    secondary: 'addon-button',
    hint: 'modal-message',
    error: 'step-up-error',
  },
}

const HINT_KEYS: Record<Factor, ParseKeys> = {
  passkey: 'security.stepUp.passkeyHint',
  totp: 'security.stepUp.totpHint',
  recovery: 'security.stepUp.recoveryHint',
}

const SWITCH_KEYS: Record<Factor, ParseKeys> = {
  passkey: 'security.stepUp.switchPasskey',
  totp: 'security.stepUp.switchTotp',
  recovery: 'security.stepUp.switchRecovery',
}

export interface StepUpFormProps {
  variant: Variant
  mode: 'verify' | 'manage'
  // verify의 목표 단계. manage에서는 쓰지 않는다.
  level: 'password' | 'twoFactor'
  // 등록한 2차 수단(SecurityStatus.methods, StepUpInfo.methods)
  methods: readonly Factor[]
  onDone: (status: SecurityStatus) => void
  onCancel: () => void
  cancelLabel: string
  // 첫 입력 상자. 대화상자가 처음 포커스할 곳으로 쓴다.
  inputRef?: RefObject<HTMLInputElement | null>
}

export function StepUpForm({ variant, mode, level, methods, onDone, onCancel, cancelLabel, inputRef }: StepUpFormProps) {
  const { t } = useTranslation()
  const c = CLASSES[variant]
  const passkeys = passkeysSupported()
  const needPassword = mode === 'manage' || level === 'password'
  const needFactor = mode === 'manage' ? methods.length > 0 : level === 'twoFactor'
  const [factor, setFactor] = useState<Factor | null>(() => (needFactor ? firstFactor(methods, passkeys) : null))
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<SecurityError | null>(null)
  const localRef = useRef<HTMLInputElement>(null)
  const firstRef = inputRef ?? localRef

  // 처음 그릴 때와 실패한 뒤(입력 상자가 다시 켜진 뒤) 첫 입력 상자에 포커스한다.
  useEffect(() => {
    if (!busy) firstRef.current?.focus()
  }, [error, busy, factor, firstRef])

  // 확인할 수 있는 수단이 없다(등록하지 않았거나, 패스키만 있는데 이 브라우저가 쓸 수 없다).
  if (needFactor && factor === null) {
    return (
      <div className={c.form}>
        <p className={c.hint}>{t(methods.length === 0 ? 'security.stepUp.noMethods' : 'security.stepUp.passkeyUnsupported')}</p>
        <Buttons variant={variant}>
          <button type="button" className={c.secondary} onClick={onCancel}>
            {cancelLabel}
          </button>
        </Buttons>
      </div>
    )
  }

  const codeFactor = factor === 'totp' || factor === 'recovery'
  const ready = !(needPassword && password === '') && !(needFactor && codeFactor && code.trim() === '')

  const submit = async () => {
    if (busy || !ready) return
    setBusy(true)
    setError(null)
    const tried = needPassword && needFactor ? 'both' : needPassword ? 'password' : (factor ?? undefined)
    try {
      let proof: SecurityVerifyRequest | undefined
      if (needFactor && factor) {
        proof = factor === 'passkey' ? { method: 'passkey', passkey: await getPasskey() } : { method: factor, code: code.trim() }
      }
      let status: SecurityStatus
      if (mode === 'manage') status = await manageSecurity({ password, factor: proof })
      else if (proof) status = await verifySecurity(proof)
      else status = await verifySecurity({ method: 'password', password })
      onDone(status)
    } catch (err) {
      setError(securityError(err, tried))
      setCode('')
      if (needPassword) setPassword('')
      setBusy(false)
    }
  }

  const hint =
    mode === 'manage'
      ? t(needFactor ? 'security.stepUp.manageHint' : 'security.stepUp.managePasswordHint')
      : needPassword
        ? t('security.stepUp.passwordHint')
        : t(HINT_KEYS[factor as Factor])
  const others = needFactor ? usableFactors(methods, passkeys).filter((f) => f !== factor) : []
  const submitLabel = busy
    ? t('security.stepUp.confirming')
    : factor === 'passkey'
      ? t('security.stepUp.usePasskey')
      : t('security.stepUp.confirm')

  return (
    <form
      className={c.form}
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <p className={c.hint}>{hint}</p>
      {needPassword && (
        <label className={c.field}>
          <span>{t('security.stepUp.password')}</span>
          <input
            ref={firstRef}
            className={c.input}
            name="password"
            type="password"
            autoComplete="current-password"
            value={password}
            disabled={busy}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
      )}
      {needFactor && codeFactor && (
        <label className={c.field}>
          <span>{t(factor === 'totp' ? 'security.stepUp.code' : 'security.stepUp.recoveryCode')}</span>
          <input
            ref={needPassword ? undefined : firstRef}
            className={c.input}
            name={factor === 'totp' ? 'code' : 'recovery-code'}
            autoComplete={factor === 'totp' ? 'one-time-code' : 'off'}
            inputMode={factor === 'totp' ? 'numeric' : 'text'}
            autoCapitalize="none"
            spellCheck={false}
            value={code}
            disabled={busy}
            onChange={(e) => setCode(e.target.value)}
          />
        </label>
      )}
      {needFactor && mode === 'manage' && factor === 'passkey' && (
        <p className={c.hint}>{t('security.stepUp.managePasskeyHint')}</p>
      )}
      {error && (
        <p className={c.error} role="alert">
          {t(error.key, { remaining: formatClock(error.retryAfter ?? 0) })}
        </p>
      )}
      {others.length > 0 && (
        <div className="step-up-others">
          {others.map((f) => (
            <button
              key={f}
              type="button"
              className="step-up-link"
              disabled={busy}
              onClick={() => {
                setFactor(f)
                setCode('')
                setError(null)
              }}
            >
              {t(SWITCH_KEYS[f])}
            </button>
          ))}
        </div>
      )}
      <Buttons variant={variant}>
        {variant === 'dialog' && (
          <button type="button" className={c.secondary} disabled={busy} onClick={onCancel}>
            {cancelLabel}
          </button>
        )}
        <button type="submit" className={c.primary} disabled={busy || !ready}>
          {submitLabel}
        </button>
        {variant === 'login' && (
          <button type="button" className={c.secondary} disabled={busy} onClick={onCancel}>
            {cancelLabel}
          </button>
        )}
      </Buttons>
    </form>
  )
}

// Buttons는 단추 줄이다. 대화상자는 오른쪽에 나란히, 로그인 카드는 폼 안에 세로로 둔다.
function Buttons({ variant, children }: { variant: Variant; children: ReactNode }) {
  return variant === 'dialog' ? <div className="modal-buttons">{children}</div> : <>{children}</>
}
