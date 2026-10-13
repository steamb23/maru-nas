import { useEffect } from 'react'
import { Desktop } from './shell/Desktop'
import { installNativeGuard } from './shell/nativeGuard'

export default function App() {
  // 데스크톱뿐 아니라 이후 로그인 화면 등 앱 전체에 적용되도록 여기서 건다.
  useEffect(() => installNativeGuard(document), [])
  return <Desktop />
}
