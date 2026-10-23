// 알림 문구 고르기. 저장소에는 코드와 치환값만 두고 문구 키는 여기서 정한다(언어를 바꾸면 그려진 알림도 바뀐다).
// DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ParseKeys } from 'i18next'
import type { Notice, NoticeAction } from '../api/types.gen'
import type { IconName } from '../icons/resolver'
import type { ClientEvent, ClientEventCode, NoticeLevel } from './model'

export interface NoticeText {
  title: ParseKeys
  // body는 없을 수 있다. params는 title·body·동작 단추 문구에 함께 넘긴다.
  body: ParseKeys | null
  params: Readonly<Record<string, string | number>>
}

// LEVEL_ICONS는 수준마다의 아이콘이다. 색은 CSS(.notice-level-*)가 입힌다.
export const LEVEL_ICONS: Record<NoticeLevel, IconName> = {
  info: 'dialog-information-symbolic',
  success: 'process-completed-symbolic',
  warning: 'dialog-warning-symbolic',
  error: 'dialog-error-symbolic',
}

// serverText는 서버 알림의 문구다. Record라 코드가 늘면 tsc가 빠진 문구를 잡는다.
const SERVER_TEXT: Record<Notice['code'], (n: Notice) => NoticeText> = {
  sample: (n): NoticeText => {
    if (n.count === undefined) return { title: 'notifications.server.sample.title', body: 'notifications.server.sample.body', params: {} }
    return { title: 'notifications.server.sample.title', body: 'notifications.server.sample.bodyCount', params: { count: n.count } }
  },
}

export function serverText(n: Notice): NoticeText {
  return SERVER_TEXT[n.code](n)
}

// 클라이언트 알림의 문구. 수가 없으면(작업 전체가 멈춤) stopped 문구다.
const CLIENT_TEXT: Record<ClientEventCode, { title: ParseKeys; body: ParseKeys; stopped?: ParseKeys }> = {
  filesCopied: { title: 'notifications.client.filesCopied.title', body: 'notifications.client.filesCopied.body' },
  filesUploaded: { title: 'notifications.client.filesUploaded.title', body: 'notifications.client.filesUploaded.body' },
  filesDone: { title: 'notifications.client.filesDone.title', body: 'notifications.client.filesDone.body' },
  filesCopyFailed: {
    title: 'notifications.client.filesCopyFailed.title',
    body: 'notifications.client.filesCopyFailed.body',
    stopped: 'notifications.client.filesCopyFailed.bodyStopped',
  },
  filesUploadFailed: {
    title: 'notifications.client.filesUploadFailed.title',
    body: 'notifications.client.filesUploadFailed.body',
    stopped: 'notifications.client.filesUploadFailed.bodyStopped',
  },
}

export function clientText(e: ClientEvent): NoticeText {
  const text = CLIENT_TEXT[e.code]
  const params = e.params ?? {}
  const body = params.count === undefined ? (text.stopped ?? null) : text.body
  return { title: text.title, body, params }
}

// actionLabel은 동작 단추의 문구다. 파일 작업 창처럼 결과를 자세히 보이는 앱은 "자세히 보기"다.
export function actionLabel(action: NoticeAction): ParseKeys {
  switch (action.kind) {
    case 'app':
      return action.app === 'transfers' ? 'notifications.action.details' : 'notifications.action.open'
    case 'workbench':
      return 'notifications.action.workbench'
    case 'power':
      return 'notifications.action.power'
  }
}
