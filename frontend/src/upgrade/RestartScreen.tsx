// 업그레이드로 maru가 다시 시작하는 동안 데스크톱 대신 그리는 화면. 감시(watch.ts)가 설치 단계에 들어가거나 연결이
// 끊기면 보이고, maru가 돌아오면 감시가 페이지를 새로 고친다. 데스크톱을 내리므로 그동안 다른 주기 요청은 없다.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LanguageSelect } from '../shell/LanguageSelect'
import { formatClock } from '../shell/elevation'
import { STUCK_MS } from './watch'
import { useUpgradeWatch } from './watchStore'

export function RestartScreen() {
  const { t } = useTranslation()
  const phase = useUpgradeWatch((s) => s.phase)
  const since = useUpgradeWatch((s) => s.since)
  // 경과 시간은 이 화면이 처음 보인 때부터 잰다. 돌아오지 않는다는 판단은 끊긴 때(since)부터 잰다.
  const [shownAt] = useState(Date.now)
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [])
  const stuck = phase === 'down' && now - since > STUCK_MS

  return (
    <div className="desktop auth-screen">
      <main className="login-card" aria-labelledby="restart-title" aria-busy="true">
        <h1 id="restart-title" className="login-title">
          {t('restart.title')}
        </h1>
        <div className="restart-spinner" aria-hidden="true" />
        <p className="login-hint restart-message" role="status">
          {phase === 'down' ? t('restart.waiting') : t('restart.installing')}
        </p>
        <p className="login-hint restart-message restart-elapsed">{t('restart.elapsed', { time: formatClock((now - shownAt) / 1000) })}</p>
        {stuck && (
          <p className="login-error selectable" role="alert">
            {t('restart.stuck')}
          </p>
        )}
        <LanguageSelect />
      </main>
    </div>
  )
}
