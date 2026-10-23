// notify는 클라이언트 알림을 하나 띄운다(토스트와 알림 센터). 로그인한 동안이 아니면 버린다.
// 다른 기능(files/ 등)은 이 함수만 import한다. 알림 모듈은 그 기능들을 import하지 않는다.
import type { NotifyInput } from './model'
import { useNotificationStore } from './store'

export type { NotifyInput } from './model'

export function notify(input: NotifyInput): void {
  useNotificationStore.getState().push(input)
}
