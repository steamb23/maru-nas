// 로그인 화면과 서버 연결 끊김 화면. 인증은 백엔드가 OMV에 위임한다. 이 브라우저에 켠 패스키로 로그인이 있으면
// 그 사용자마다 패스키로 로그인 단추를 둔다(풀어 낸 비밀번호는 화면 상태에 넣지 않는다).
import { type FormEvent, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import lockupUrl from '../assets/logo/onmaru-lockup.svg?url'
import { useSiteStore } from '../appearance/siteStore'
import { BRAND_NAME, BRAND_ROMANIZED, BRAND_TAGLINE } from '../brand'
import { useLanguage } from '../i18n'
import { LanguageSelect } from '../shell/LanguageSelect'
import { enterLogin } from '../shell/screenTransition'
import { useIdle } from '../shell/useIdle'
import { formatUnit } from '../addons/common/format'
import type { SessionInfo } from '../api/types.gen'
import { type PasskeyLoginEntry, isPasskeyLoginGone } from '../security/passkeyLogin'
import { PasskeyLoginBrokenError, PrfUnsupportedError, forgetOnError, unlockPasskeyLogin } from '../security/passkeyLoginFlow'
import { activePasskeyLogins } from '../security/passkeyLoginStorage'
import { StepUpForm } from '../security/StepUpForm'
import { passkeysSupported } from '../security/webauthn'
import { hasUpgradeHint } from '../upgrade/resultHint'
import { IDLE_DELAY_MS, easterEggEnabled } from './easterEgg/config'
import { EasterEgg } from './easterEgg/EasterEgg'
import { type LoginError, loginErrorMessage } from './messages'
import { PasskeyOffer } from './PasskeyOffer'
import { useAuthStore } from './store'

const { login, verify, completePasskeyLogin, cancelChallenge, finishStepUp, restore } = useAuthStore.getState()

// passkeyEntries는 이 브라우저에 켠 패스키로 로그인 항목이다. 패스키를 쓸 수 없는 접속(평문 HTTP)에서는 없다.
const passkeyEntries = (): PasskeyLoginEntry[] => (passkeysSupported() ? activePasskeyLogins() : [])

// OFFLINE_RETRY_MS마다 백엔드 연결을 다시 시도한다.
const OFFLINE_RETRY_MS = 5000


// useCountdown은 잠금이 끝나는 시각(ms)까지 남은 초를 1초마다 갱신한다. 지나면 0이다.
// lock(초)을 부르면 지금부터 카운트다운을 시작한다.
function useCountdown(): [remaining: number, lock: (seconds: number) => void] {
  const [{ until, now }, setState] = useState({ until: 0, now: 0 })
  const remaining = Math.max(0, Math.ceil((until - now) / 1000))
  const active = remaining > 0
  // 잠금이 시작·끝날 때만 타이머를 바꾼다.
  useEffect(() => {
    if (!active) return
    const id = window.setInterval(() => setState((s) => ({ ...s, now: Date.now() })), 1000)
    return () => window.clearInterval(id)
  }, [active])
  const lock = (seconds: number) => {
    const t = Date.now()
    setState({ until: t + seconds * 1000, now: t })
  }
  return [remaining, lock]
}

export function LoginScreen() {
  const { t } = useTranslation()
  const lang = useLanguage()
  const status = useAuthStore((s) => s.status)
  const challenge = useAuthStore((s) => s.challenge)
  const stepUp = useAuthStore((s) => s.stepUp)
  const passkeyOffer = useAuthStore((s) => s.passkeyOffer)
  const user = useAuthStore((s) => s.user)
  const notice = useAuthStore((s) => s.notice)
  // 업그레이드로 다시 시작해 새로 고친 화면이면(upgrade/resultHint.ts) 세션 만료 대신 그 안내를 보인다.
  const [upgraded] = useState(hasUpgradeHint)

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<LoginError | null>(null)
  // 처음 그릴 때와 패스키로 로그인이 실패한 뒤에 다시 읽는다(실패로 지워진 항목을 뺀다).
  const [entries, setEntries] = useState(passkeyEntries)
  // 패스키로 로그인 중인 항목의 등록 ID
  const [unlocking, setUnlocking] = useState<string | null>(null)
  const [remaining, lock] = useCountdown()
  const passwordRef = useRef<HTMLInputElement>(null)
  const codeRef = useRef<HTMLInputElement>(null)
  const cardRef = useRef<HTMLElement>(null)

  // 처음 그려질 때(앱 시작, 로그아웃, 세션 만료) 카드가 줄어들며 나타난다.
  useLayoutEffect(() => enterLogin(cardRef.current), [])

  const inChallenge = status === 'challenge' && challenge !== null
  // 로그인 직후 단계: 2차 인증 권유, 패스키로 로그인 켜기 권유, 또는 둘 다
  const inStepUp = status === 'stepUp' && user !== null && (stepUp !== null || passkeyOffer !== null)
  const redirectOnly = inChallenge && !!challenge.redirectUrl
  // 대기 이스터에그는 일반 로그인 폼에서만 나온다(2차 인증 중, 로그인 요청 중에는 끔).
  // 대기 연출은 관리자가 끄지 않았을 때만 쓴다. 전역 정보를 받기 전이나 받지 못했으면 켜진 것으로 본다.
  const eggAllowed = useSiteStore((s) => s.info?.loginEasterEgg ?? true)
  const idleSince = useIdle(easterEggEnabled(eggAllowed) && !inChallenge && !inStepUp && !busy, IDLE_DELAY_MS)

  useEffect(() => {
    if (inChallenge) codeRef.current?.focus()
  }, [inChallenge])

  // 로그인 직후 단계에서 세션이 끝나 로그인 폼으로 돌아오면 권유에 쓰려고 남겨 둔 비밀번호를 지운다.
  const prevStatus = useRef(status)
  useEffect(() => {
    if (prevStatus.current === 'stepUp' && status === 'loggedOut') setPassword('')
    prevStatus.current = status
  }, [status])

  // 요청 중에는 입력칸을 disabled 대신 readOnly로 둔다. disabled면 실패 직후 focus()가 무시된다.
  const run = async (action: () => Promise<void>, onFail?: (err: unknown) => void) => {
    setBusy(true)
    setError(null)
    try {
      await action()
    } catch (err) {
      const e = loginErrorMessage(err)
      setError(e)
      if (e.retryAfterSeconds) lock(e.retryAfterSeconds)
      onFail?.(err)
    } finally {
      setBusy(false)
    }
  }

  const onLogin = (e: FormEvent) => {
    e.preventDefault()
    void run(
      () => login(username, password),
      () => {
        setPassword('')
        passwordRef.current?.focus()
      },
    )
  }

  const onVerify = (e: FormEvent) => {
    e.preventDefault()
    void run(
      // 비밀번호는 지우지 않는다. 코드 확인 뒤에도 패스키로 로그인 켜기를 권할 수 있고, 권유를 마치거나 화면을
      // 떠날 때 지운다.
      () => verify(code.trim()),
      () => {
        setCode('')
        codeRef.current?.focus()
      },
    )
  }

  const onCancel = () => {
    setCode('')
    setPassword('')
    setError(null)
    cancelChallenge()
  }

  // onPasskeyLogin은 이 브라우저의 항목으로 비밀번호를 풀어 로그인을 마친다. 풀어 낸 비밀번호는 이 안에만 둔다.
  const onPasskeyLogin = (entry: PasskeyLoginEntry) => {
    setUnlocking(entry.enrollment)
    void run(
      async () => {
        const { password: saved, rotated } = await unlockPasskeyLogin(entry)
        try {
          await completePasskeyLogin(saved, rotated)
        } catch (err) {
          forgetOnError(err, entry)
          throw err
        }
      },
      (err) => {
        setEntries(passkeyEntries())
        // 그 등록을 더 쓸 수 없으면(해제, 만료, 비밀번호 변경, 풀지 못함, PRF 결과 없음) 항목이 지워졌으므로 비밀번호로
        // 로그인하게 한다.
        if (isPasskeyLoginGone(err) || err instanceof PasskeyLoginBrokenError || err instanceof PrfUnsupportedError) {
          setUsername(entry.username)
          setPassword('')
          passwordRef.current?.focus()
        }
      },
    ).finally(() => setUnlocking(null))
  }

  // finishOffer는 로그인 직후 단계를 마치고 데스크톱으로 간다. 권유에 쓰던 비밀번호를 지운다.
  const finishOffer = (assurance?: SessionInfo['assurance']) => {
    setPassword('')
    void finishStepUp(assurance)
  }

  const locked = remaining > 0
  const message = error && t(error.key, error.params)
  const errorText =
    message && (locked ? t('login.withCountdown', { message, remaining: formatUnit(remaining, 'second', lang) }) : message)

  return (
    <div className="desktop auth-screen">
      <main ref={cardRef} className="login-card" aria-labelledby="login-title">
        {/* 로그인 전에는 누구의 프로필인지 알 수 없으므로 사용자 아바타 대신 묶음 로고를 제목으로 둔다 */}
        <h1 id="login-title" className="login-brand">
          <img
            className="login-lockup"
            src={lockupUrl}
            width={210}
            height={80}
            alt={`${BRAND_NAME} (${BRAND_ROMANIZED})`}
            draggable={false}
          />
        </h1>
        <p className="login-tagline" lang="en">
          {BRAND_TAGLINE}
        </p>

        {upgraded && !error ? (
          <p className="login-notice" role="status">
            {t('login.upgraded')}
          </p>
        ) : (
          notice === 'expired' &&
          !error && (
            <p className="login-notice" role="status">
              {t('login.sessionExpired')}
            </p>
          )
        )}
        {errorText && (
          <p className="login-error" role="alert">
            {errorText}
          </p>
        )}

        {inStepUp ? (
          // 로그인은 마쳤다. 보안 강화 모드의 2차 인증이나 패스키로 로그인 켜기를 권하고, 건너뛰면 그대로 데스크톱에
          // 들어간다. 둘 다 권하면 2차 인증의 패스키 확인에서 함께 켠다.
          stepUp ? (
            <>
              <p className="login-notice" role="status">
                {t('login.stepUpPrompt')}
              </p>
              <StepUpForm
                variant="login"
                mode="verify"
                level="twoFactor"
                methods={stepUp.methods}
                passkeyLogin={passkeyOffer && password !== '' ? { username: user.username, password } : undefined}
                onDone={(st) => finishOffer(st.assurance)}
                onCancel={() => finishOffer()}
                cancelLabel={t('login.skipStepUp')}
              />
            </>
          ) : (
            passkeyOffer && (
              <PasskeyOffer username={user.username} password={password} days={passkeyOffer.days} onFinish={finishOffer} />
            )
          )
        ) : inChallenge ? (
          <form className="login-form" onSubmit={onVerify}>
            {redirectOnly ? (
              <p className="login-hint">{t('login.twoFactorRedirect')}</p>
            ) : (
              <>
                <p className="login-hint">{t('login.twoFactorPrompt')}</p>
                <label className="login-field">
                  <span>{t('login.code')}</span>
                  <input
                    ref={codeRef}
                    name="code"
                    autoComplete="one-time-code"
                    inputMode="numeric"
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    readOnly={busy}
                    required
                  />
                </label>
                <button type="submit" className="login-submit" disabled={busy || locked || code.trim() === ''}>
                  {busy ? t('login.verifying') : t('login.verify')}
                </button>
              </>
            )}
            <button type="button" className="login-secondary" onClick={onCancel} disabled={busy}>
              {t('login.startOver')}
            </button>
          </form>
        ) : (
          <form className="login-form" onSubmit={onLogin}>
            <label className="login-field">
              <span>{t('login.username')}</span>
              <input
                name="username"
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                readOnly={busy}
                autoFocus
                required
              />
            </label>
            <label className="login-field">
              <span>{t('login.password')}</span>
              <input
                ref={passwordRef}
                name="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                readOnly={busy}
                required
              />
            </label>
            <button
              type="submit"
              className="login-submit"
              disabled={busy || locked || username.trim() === '' || password === ''}
            >
              {busy && unlocking === null ? t('login.signingIn') : t('login.signIn')}
            </button>
            {entries.map((entry) => (
              <button
                key={entry.enrollment}
                type="button"
                className="login-secondary"
                disabled={busy || locked}
                onClick={() => onPasskeyLogin(entry)}
              >
                {unlocking === entry.enrollment ? t('login.passkeyWaiting') : t('login.passkeySignIn', { user: entry.username })}
              </button>
            ))}
          </form>
        )}
        <p className="login-footnote">{t('login.footnote')}</p>
        <LanguageSelect />
      </main>
      <EasterEgg since={idleSince} avoid={cardRef} />
    </div>
  )
}

// OfflineScreen은 앱을 시작할 때 백엔드에 연결할 수 없으면 보여 준다. 주기적으로 다시 시도한다.
export function OfflineScreen() {
  const { t } = useTranslation()
  useEffect(() => {
    const id = window.setInterval(() => void restore(), OFFLINE_RETRY_MS)
    return () => window.clearInterval(id)
  }, [])

  return (
    <div className="desktop auth-screen">
      <main className="login-card" aria-labelledby="offline-title">
        <h1 id="offline-title" className="login-title">
          {t('offline.title')}
        </h1>
        <p className="login-hint">{t('offline.message')}</p>
        <button type="button" className="login-submit" onClick={() => void restore()}>
          {t('offline.retry')}
        </button>
        <LanguageSelect />
      </main>
    </div>
  )
}
