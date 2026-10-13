import { useEffect } from 'react'
import { setUnauthorizedHandler } from './api/client'
import { LoginScreen, OfflineScreen } from './auth/LoginScreen'
import { useAuthStore } from './auth/store'
import { useSessionKeeper } from './auth/useSessionKeeper'
import { Desktop } from './shell/Desktop'
import { installNativeGuard } from './shell/nativeGuard'

const { restore, expire } = useAuthStore.getState()

// SignedIn은 로그인한 동안만 그린다. 로그아웃·세션 만료로 언마운트되면 창과 주기 요청도 멈춘다.
function SignedIn() {
  useSessionKeeper()
  return <Desktop />
}

export default function App() {
  // 데스크톱뿐 아니라 로그인 화면 등 앱 전체에 적용되도록 여기서 건다.
  useEffect(() => installNativeGuard(document), [])
  // 다른 요청이 401을 받으면(세션 만료 등) 로그인 화면으로 보낸다.
  useEffect(() => setUnauthorizedHandler(expire), [])
  useEffect(() => void restore(), [])

  const status = useAuthStore((s) => s.status)
  switch (status) {
    case 'loggedIn':
      return <SignedIn />
    case 'checking':
      // 세션을 확인하는 짧은 동안은 바탕화면 배경만 보여 준다.
      return <div className="desktop" />
    case 'offline':
      return <OfflineScreen />
    default:
      return <LoginScreen />
  }
}
