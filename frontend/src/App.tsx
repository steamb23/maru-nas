import { useEffect } from 'react'
import { useCapabilities } from './addons/useCapabilities'
import { setUnauthorizedHandler } from './api/client'
import {
  useAppearanceLoader,
  useAppliedAnimations,
  useAppliedTheme,
  useSiteInfoLoader,
  useWallpaperView,
} from './appearance/useAppearance'
import { LoginScreen, OfflineScreen } from './auth/LoginScreen'
import { useAuthStore } from './auth/store'
import { useAvatarLoader } from './profile/useAvatar'
import { useSessionKeeper } from './auth/useSessionKeeper'
import { Desktop } from './shell/Desktop'
import { installNativeGuard } from './shell/nativeGuard'
import { Wallpaper } from './shell/Wallpaper'
import { PowerScreen } from './power/PowerScreen'
import { usePowerWatch } from './power/watchStore'
import { RestartScreen } from './upgrade/RestartScreen'
import { showsRestartScreen } from './upgrade/watch'
import { useUpgradeWatch } from './upgrade/watchStore'

const { restore, expire } = useAuthStore.getState()

// SignedIn은 로그인한 동안만 그린다. 로그아웃·세션 만료로 언마운트되면 창과 주기 요청도 멈춘다.
function SignedIn() {
  useSessionKeeper()
  useCapabilities()
  useAppearanceLoader()
  useAvatarLoader()
  return <Desktop />
}

// Screen은 인증 상태에 맞는 화면이다. 배경 레이어는 App이 화면과 관계없이 그 아래에 둔다.
function Screen() {
  const status = useAuthStore((s) => s.status)
  switch (status) {
    case 'loggedIn':
      return <SignedIn />
    case 'checking':
      // 세션을 확인하는 짧은 동안은 배경만 보여 준다.
      return <div className="desktop" />
    case 'offline':
      return <OfflineScreen />
    default:
      return <LoginScreen />
  }
}

// Root는 업그레이드로 다시 시작하는 동안이면 재시작 화면, 전원 조작을 보낸 뒤면 전원 화면, 아니면 인증 상태의
// 화면이다. 두 화면은 401로 세션이 끝나도 남아 있다가 maru가 돌아오면 페이지를 새로 고친다(upgrade/watch.ts,
// power/watch.ts).
function Root() {
  const restarting = useUpgradeWatch((s) => showsRestartScreen(s.phase))
  const powering = usePowerWatch((s) => s.phase !== 'idle')
  if (restarting) return <RestartScreen />
  return powering ? <PowerScreen /> : <Screen />
}

export default function App() {
  // 데스크톱뿐 아니라 로그인 화면 등 앱 전체에 적용되도록 여기서 건다.
  useEffect(() => installNativeGuard(document), [])
  // 다른 요청이 401을 받으면(세션 만료 등) 로그인 화면으로 보낸다.
  useEffect(() => setUnauthorizedHandler(expire), [])
  useEffect(() => void restore(), [])
  // 화면 설정(테마, 배경, 애니메이션)은 로그인 화면에도 적용한다.
  useSiteInfoLoader()
  useAppliedTheme()
  useAppliedAnimations()
  const wallpaper = useWallpaperView()

  return (
    <>
      <Wallpaper view={wallpaper.view} fallback={wallpaper.fallback} />
      <Root />
    </>
  )
}
