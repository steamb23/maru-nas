// 앱 전체가 함께 쓰는 업그레이드 감시(watch.ts)를 API 클라이언트와 페이지 새로 고침에 연결한다.
import { ApiError, fetchHealth, fetchUpgrade } from '../api/client'
import { setUpgradeHint } from './resultHint'
import { createUpgradeWatch } from './watch'

export const useUpgradeWatch = createUpgradeWatch({
  fetchStatus: fetchUpgrade,
  fetchHealth,
  // 401은 세션이 사라졌다는 뜻이다(maru가 다시 시작함). 그 밖(연결 실패, 프록시의 502 등)은 끊김으로 본다.
  classify: (err) => (err instanceof ApiError && err.status === 401 ? 'unauthorized' : 'unreachable'),
  reload: () => {
    setUpgradeHint()
    window.location.reload()
  },
})
