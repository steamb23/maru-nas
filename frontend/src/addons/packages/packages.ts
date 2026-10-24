// 시스템 업데이트 창의 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ParseKeys } from 'i18next'
import type { Job, JobOutput, JobsResponse, PackageUpdate } from '../../api/types.gen'
import type { ActionErrorCode } from '../common/actionState'
import { formatUnit } from '../common/format'

// OMV 워크벤치의 업데이트 관리 라우트(8.0부터 같다).
export const UPDATE_SETTINGS_ROUTE = 'system/updatemgmt/settings/software'

// changelogRoute는 패키지 하나의 변경 기록 화면이다(워크벤치가 .deb를 내려받아 보여 준다).
export function changelogRoute(filename: string): string {
  return `system/updatemgmt/updates/changelog/${encodeURIComponent(filename)}`
}

// totalSize는 내려받을 크기의 합이다.
export function totalSize(pkgs: readonly PackageUpdate[]): number {
  return pkgs.reduce((sum, p) => sum + p.size, 0)
}

export type JobKind = Job['kind']
export type JobCode = NonNullable<Job['code']>

// pickJob은 작업 보기에 그릴 작업이다. 진행 중인 작업이 먼저이고, 아니면 열어 달라고 한 작업(알림의 출력 보기),
// 그것도 없으면 이 사용자의 가장 최근 결과다.
export function pickJob(jobs: JobsResponse | null, requested: string | null): Job | null {
  if (!jobs) return null
  if (jobs.current) return jobs.current
  return jobs.recent.find((j) => j.id === requested) ?? jobs.recent[0] ?? null
}

// JOB_HEADINGS는 작업 보기 머리말이다(진행 중이면 종류, 끝났으면 종류와 결과).
const JOB_HEADINGS: Record<JobKind, Record<Job['state'], ParseKeys>> = {
  'apt-update': {
    running: 'packages.job.update.running',
    succeeded: 'packages.job.update.succeeded',
    failed: 'packages.job.update.failed',
    unknown: 'packages.job.update.unknown',
  },
  'apt-upgrade': {
    running: 'packages.job.upgrade.running',
    succeeded: 'packages.job.upgrade.succeeded',
    failed: 'packages.job.upgrade.failed',
    unknown: 'packages.job.upgrade.unknown',
  },
}

export function jobHeading(job: Job): ParseKeys {
  return JOB_HEADINGS[job.kind][job.state]
}

// JOB_REASONS는 실패·결과 모름의 이유와 할 일이다.
const JOB_REASONS: Record<JobCode, ParseKeys> = {
  apt_failed: 'packages.job.reason.aptFailed',
  apt_core_removal: 'packages.job.reason.aptCoreRemoval',
  job_failed: 'packages.job.reason.jobFailed',
  job_lost: 'packages.job.reason.jobLost',
  job_reboot: 'packages.job.reason.jobReboot',
  job_timeout: 'packages.job.reason.jobTimeout',
}

export function jobReason(job: Job): ParseKeys | null {
  return job.code ? JOB_REASONS[job.code] : null
}

// START_ERRORS는 작업을 시작하지 못한 이유다. 적용 대기는 워크벤치에서 적용하도록 안내한다.
const START_ERRORS: Partial<Record<ActionErrorCode, { title: ParseKeys; body: ParseKeys }>> = {
  apt_locked: { title: 'packages.start.locked.title', body: 'packages.start.locked.body' },
  config_dirty: { title: 'packages.start.dirty.title', body: 'packages.start.dirty.body' },
  job_busy: { title: 'packages.start.busy.title', body: 'packages.start.busy.body' },
}

export function startError(code: ActionErrorCode): { title: ParseKeys; body: ParseKeys } {
  return START_ERRORS[code] ?? { title: 'packages.start.failed.title', body: 'packages.start.failed.body' }
}

// 브라우저에 두는 출력의 상한(글자). 넘으면 앞을 줄 경계에서 버린다(오류는 끝에 있다).
export const OUTPUT_LIMIT = 512 * 1024
const OUTPUT_KEEP = 384 * 1024

// OutputBuffer는 작업 하나의 받은 출력이다. next는 다음 요청의 위치(서버의 논리 위치)다.
export interface OutputBuffer {
  jobId: string
  text: string
  next: number
  // 줄 수(화면 표시용). 앞을 버려도 받은 전체 줄 수를 센다.
  lines: number
  // 앞부분이 없다(서버가 상한으로 버렸거나 이 창이 버렸다).
  truncated: boolean
}

export function emptyOutput(jobId: string): OutputBuffer {
  return { jobId, text: '', next: 0, lines: 0, truncated: false }
}

function countLines(s: string): number {
  let n = 0
  for (let i = s.indexOf('\n'); i >= 0; i = s.indexOf('\n', i + 1)) n++
  return n
}

// appendOutput은 받은 조각을 붙인다. 요청한 위치보다 뒤에서 시작하면(서버가 앞을 버림) 앞이 없다고 표시한다.
export function appendOutput(buf: OutputBuffer, out: JobOutput): OutputBuffer {
  let text = buf.text + out.text
  let truncated = buf.truncated || out.from > buf.next
  if (text.length > OUTPUT_LIMIT) {
    let cut = text.length - OUTPUT_KEEP
    const nl = text.indexOf('\n', cut)
    if (nl >= 0) cut = nl + 1
    text = text.slice(cut)
    truncated = true
  }
  return { jobId: buf.jobId, text, next: out.next, lines: buf.lines + countLines(out.text), truncated }
}

// elapsedSeconds는 작업이 걸린(진행 중이면 지금까지의) 초다.
export function elapsedSeconds(job: Job, now: number): number {
  const start = Date.parse(job.startedAt)
  const end = job.endedAt ? Date.parse(job.endedAt) : now
  if (Number.isNaN(start) || Number.isNaN(end)) return 0
  return Math.max(0, Math.round((end - start) / 1000))
}

// formatElapsed는 걸린 시간을 ko "4분 12초", en "4m 12s"처럼 쓴다(한 시간이 넘으면 시와 분).
export function formatElapsed(seconds: number, locale: string): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  const parts: string[] = []
  if (h > 0) parts.push(formatUnit(h, 'hour', locale, 'narrow'))
  if (h > 0 || m > 0) parts.push(formatUnit(m, 'minute', locale, 'narrow'))
  if (h === 0) parts.push(formatUnit(s, 'second', locale, 'narrow'))
  return parts.join(' ')
}
