import { useSyncExternalStore } from 'react'

// 탭이 보이는지(document.visibilityState). 다른 탭으로 가거나 브라우저 창을 최소화하면 false다.
// 창·위젯의 active에 함께 넣어 보이지 않는 동안 주기 요청을 멈춘다. 다시 보이면 poller가 바로 한 번 가져온다.
const subscribe = (onChange: () => void) => {
  document.addEventListener('visibilitychange', onChange)
  return () => document.removeEventListener('visibilitychange', onChange)
}
const getSnapshot = () => document.visibilityState === 'visible'

export function usePageVisible(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot)
}
