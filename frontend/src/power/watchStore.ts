// 앱 전체가 함께 쓰는 전원 감시(watch.ts)를 API 클라이언트와 페이지 새로 고침에 연결한다.
import { fetchHealth } from '../api/client'
import { createPowerWatch } from './watch'

export const usePowerWatch = createPowerWatch({
  fetchHealth,
  reload: () => window.location.reload(),
})
