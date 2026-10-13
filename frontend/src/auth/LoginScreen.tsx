// 로그인 화면과 서버 연결 끊김 화면. 인증은 백엔드가 OMV에 위임한다.
import { type FormEvent, useEffect, useRef, useState } from 'react'
import { IconImage } from '../icons/IconImage'
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

  const inChallenge = status === 'challenge' && challenge !== null
  const redirectOnly = inChallenge && !!challenge.redirectUrl

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
  const errorText = error && (locked ? `${error.message} (${remaining}초)` : error.message)

  return (
    <div className="desktop auth-screen">
      <main className="login-card" aria-labelledby="login-title">
        <div className="login-avatar">
          <IconImage name="avatar-default-symbolic" size={40} />
        </div>
        <h1 id="login-title" className="login-title">
          OMV 데스크톱
        </h1>

        {notice === 'expired' && !error && (
          <p className="login-notice" role="status">
            세션이 만료되었습니다. 다시 로그인하세요.
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
              <p className="login-hint">이 계정의 2차 인증은 OpenMediaVault 화면에서 진행해야 합니다.</p>
            ) : (
              <>
                <p className="login-hint">2차 인증 코드를 입력하세요.</p>
                <label className="login-field">
                  <span>인증 코드</span>
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
                  {busy ? '확인 중…' : '확인'}
                </button>
              </>
            )}
            <button type="button" className="login-secondary" onClick={onCancel} disabled={busy}>
              처음으로
            </button>
          </form>
        ) : (
          <form className="login-form" onSubmit={onLogin}>
            <label className="login-field">
              <span>사용자 이름</span>
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
              <span>비밀번호</span>
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
              {busy ? '로그인 중…' : '로그인'}
            </button>
          </form>
        )}
        <p className="login-footnote">OpenMediaVault 계정으로 로그인합니다.</p>
      </main>
    </div>
  )
}

// OfflineScreen은 앱을 시작할 때 백엔드에 연결할 수 없으면 보여 준다. 주기적으로 다시 시도한다.
export function OfflineScreen() {
  useEffect(() => {
    const id = window.setInterval(() => void restore(), OFFLINE_RETRY_MS)
    return () => window.clearInterval(id)
  }, [])

  return (
    <div className="desktop auth-screen">
      <main className="login-card" aria-labelledby="offline-title">
        <h1 id="offline-title" className="login-title">
          연결 끊김
        </h1>
        <p className="login-hint">대시보드 서버에 연결할 수 없습니다. 잠시 후 다시 연결합니다.</p>
        <button type="button" className="login-submit" onClick={() => void restore()}>
          지금 다시 연결
        </button>
      </main>
    </div>
  )
}
