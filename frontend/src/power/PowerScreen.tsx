// 전원 조작을 보낸 뒤 데스크톱 대신 그리는 화면(다시 시작하는 중, 꺼짐, 절전). 감시(watch.ts)가 NAS가 돌아온
// 것을 보면 페이지를 새로 고친다. 데스크톱을 내리므로 그동안 다른 주기 요청은 없다.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'
import { LanguageSelect } from '../shell/LanguageSelect'
import { formatClock } from '../shell/elevation'
import { screenView } from './watch'
import { usePowerWatch } from './watchStore'

export function PowerScreen() {
  const { t } = useTranslation()
  const state = usePowerWatch()
  // 경과 시간은 이 화면이 처음 보인 때부터 잰다. 오래 걸린다는 판단은 지금 단계가 시작된 때(since)부터 잰다.
  const [shownAt] = useState(Date.now)
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [])
  const view = screenView(state, now)

  return (
    <div className="desktop auth-screen">
      <main className="login-card power-screen" aria-labelledby="power-screen-title" aria-busy={view.icon === 'spinner'}>
        <h1 id="power-screen-title" className="login-title">
          {t(view.titleKey)}
        </h1>
        {view.icon === 'spinner' ? (
          <div className="restart-spinner" aria-hidden="true" />
        ) : (
          <span className="power-screen-icon">
            <IconImage name={view.icon} size={32} />
          </span>
        )}
        <p className="login-hint restart-message" role="status">
          {t(view.messageKey)}
        </p>
        {state.modeKey && (
          <p className="login-hint restart-message">{t('power.screen.mode', { mode: t(state.modeKey) })}</p>
        )}
        <p className="login-hint restart-message restart-elapsed">
          {t('power.screen.elapsed', { time: formatClock((now - shownAt) / 1000) })}
        </p>
        {view.hintKey && (
          <p className="login-error selectable" role="alert">
            {t(view.hintKey)}
          </p>
        )}
        {view.back && (
          // 조작이 처리되지 않은 것 같으면 새로 고쳐 데스크톱으로 돌아간다(세션이 살아 있으면 그대로 들어간다).
          <button type="button" className="addon-button" onClick={() => window.location.reload()}>
            {t('power.screen.back')}
          </button>
        )}
        <LanguageSelect />
      </main>
    </div>
  )
}
