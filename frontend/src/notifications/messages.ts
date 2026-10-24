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
  aptUpdates: (n): NoticeText => ({
    title: 'notifications.server.aptUpdates.title',
    body: 'notifications.server.aptUpdates.body',
    params: { count: n.count ?? n.items?.length ?? 0 },
  }),
  rebootRequired: (): NoticeText => ({
    title: 'notifications.server.rebootRequired.title',
    body: 'notifications.server.rebootRequired.body',
    params: {},
  }),
  jobSucceeded: (n) => jobText(n),
  jobFailed: (n) => jobText(n),
  jobUnknown: (n) => jobText(n),
}

type JobKind = NonNullable<Notice['job']>['kind']
type JobCode = NonNullable<NonNullable<Notice['job']>['code']>
type EndState = 'succeeded' | 'failed' | 'unknown'

// JOB_TITLES는 작업 종류와 결과마다의 제목이다.
const JOB_TITLES: Record<JobKind, Record<EndState, ParseKeys>> = {
  'apt-update': {
    succeeded: 'notifications.server.jobs.aptUpdate.succeeded.title',
    failed: 'notifications.server.jobs.aptUpdate.failed.title',
    unknown: 'notifications.server.jobs.aptUpdate.unknown.title',
  },
  'apt-upgrade': {
    succeeded: 'notifications.server.jobs.aptUpgrade.succeeded.title',
    failed: 'notifications.server.jobs.aptUpgrade.failed.title',
    unknown: 'notifications.server.jobs.aptUpgrade.unknown.title',
  },
}

// JOB_SUMMARIES는 성공한 작업의 요약 문구다(수가 있을 때, 없을 때).
const JOB_SUMMARIES: Record<JobKind, { count: ParseKeys; none: ParseKeys }> = {
  'apt-update': {
    count: 'notifications.server.jobs.aptUpdate.succeeded.body',
    none: 'notifications.server.jobs.aptUpdate.succeeded.bodyNone',
  },
  'apt-upgrade': {
    count: 'notifications.server.jobs.aptUpgrade.succeeded.body',
    none: 'notifications.server.jobs.aptUpgrade.succeeded.bodyNone',
  },
}

// JOB_CODE_TEXT는 실패·결과 모름의 이유다.
export const JOB_CODE_TEXT: Record<JobCode, ParseKeys> = {
  apt_failed: 'notifications.server.jobs.code.aptFailed',
  apt_core_removal: 'notifications.server.jobs.code.aptCoreRemoval',
  job_failed: 'notifications.server.jobs.code.jobFailed',
  job_lost: 'notifications.server.jobs.code.jobLost',
  job_reboot: 'notifications.server.jobs.code.jobReboot',
  job_timeout: 'notifications.server.jobs.code.jobTimeout',
}

// jobText는 작업 결과 알림의 문구다. 알림 코드가 결과를, job이 종류와 이유를 정한다.
function jobText(n: Notice): NoticeText {
  const kind: JobKind = n.job?.kind ?? 'apt-upgrade'
  const state: EndState = n.code === 'jobSucceeded' ? 'succeeded' : n.code === 'jobFailed' ? 'failed' : 'unknown'
  const title = JOB_TITLES[kind][state]
  if (state === 'succeeded') {
    const summary = JOB_SUMMARIES[kind]
    return n.count ? { title, body: summary.count, params: { count: n.count } } : { title, body: summary.none, params: {} }
  }
  const code = n.job?.code
  return { title, body: code ? JOB_CODE_TEXT[code] : null, params: {} }
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
