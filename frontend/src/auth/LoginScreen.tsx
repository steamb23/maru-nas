// 로그인 화면과 서버 연결 끊김 화면. 인증은 백엔드가 OMV에 위임한다.
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
import { IDLE_DELAY_MS, easterEggEnabled } from './easterEgg/config'
import { EasterEgg } from './easterEgg/EasterEgg'
import { type LoginError, loginErrorMessage } from './messages'
import { useAuthStore } from './store'

const { login, verify, cancelChallenge, restore } = useAuthStore.getState()

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
  const notice = useAuthStore((s) => s.notice)

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<LoginError | null>(null)
  const [remaining, lock] = useCountdown()
  const passwordRef = useRef<HTMLInputElement>(null)
  const codeRef = useRef<HTMLInputElement>(null)
  const cardRef = useRef<HTMLElement>(null)

  // 처음 그려질 때(앱 시작, 로그아웃, 세션 만료) 카드가 줄어들며 나타난다.
  useLayoutEffect(() => enterLogin(cardRef.current), [])

  const inChallenge = status === 'challenge' && challenge !== null
  const redirectOnly = inChallenge && !!challenge.redirectUrl
  // 대기 이스터에그는 일반 로그인 폼에서만 나온다(2차 인증 중, 로그인 요청 중에는 끔).
  // 대기 연출은 관리자가 끄지 않았을 때만 쓴다. 전역 정보를 받기 전이나 받지 못했으면 켜진 것으로 본다.
  const eggAllowed = useSiteStore((s) => s.info?.loginEasterEgg ?? true)
  const idleSince = useIdle(easterEggEnabled(eggAllowed) && !inChallenge && !busy, IDLE_DELAY_MS)

  useEffect(() => {
    if (inChallenge) codeRef.current?.focus()
  }, [inChallenge])

  // 요청 중에는 입력칸을 disabled 대신 readOnly로 둔다. disabled면 실패 직후 focus()가 무시된다.
  const run = async (action: () => Promise<void>, onFail?: () => void) => {
    setBusy(true)
    setError(null)
    try {
      await action()
    } catch (err) {
      const e = loginErrorMessage(err)
      setError(e)
      if (e.retryAfterSeconds) lock(e.retryAfterSeconds)
      onFail?.()
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
      async () => {
        await verify(code.trim())
        setPassword('')
      },
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

        {notice === 'expired' && !error && (
          <p className="login-notice" role="status">
            {t('login.sessionExpired')}
          </p>
        )}
        {errorText && (
          <p className="login-error" role="alert">
            {errorText}
          </p>
        )}

        {inChallenge ? (
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
              {busy ? t('login.signingIn') : t('login.signIn')}
            </button>
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
