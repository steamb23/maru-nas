// 파일 작업(복사, 올리기)이 띄우는 알림. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 실패는 작업마다 알리고, 완료는 작업 창이 열린 뒤(OPEN_DELAY_MS보다 오래 걸린 작업) 모두 문제없이 끝나 창이 스스로
// 닫힐 때 한 번 알린다. 금방 끝난 작업은 탐색기 목록에 결과가 바로 보이므로 알리지 않는다. 취소(작업 창의 취소,
// 세션 만료, 보안 확인 취소)는 알리지 않는다.
import type { NoticeAction } from '../api/types.gen'
import type { NotifyInput } from '../notifications/notify'
import type { CopyJob } from './copyJobs'
import type { UploadJob } from './uploadJobs'

// 실패 알림의 동작: 이유별 목록과 다시 올리기가 있는 파일 작업 창을 연다.
const OPEN_TRANSFERS: NoticeAction = { kind: 'app', app: 'transfers' }

// failedNotice는 끝난 작업 하나가 알릴 실패다. 실패가 아니면 null이다.
export function failedNotice(job: CopyJob | UploadJob): NotifyInput | null {
  if (job.status !== 'failed') return null
  const count = job.kind === 'copy' ? job.failures.length : job.files.filter((f) => f.status === 'failed').length
  return {
    code: job.kind === 'copy' ? 'filesCopyFailed' : 'filesUploadFailed',
    level: 'error',
    // 항목마다의 실패가 없으면 작업 전체가 멈춘 것이다(문구가 다르다).
    ...(count > 0 && { params: { count } }),
    action: OPEN_TRANSFERS,
  }
}

// finishedNotice는 모두 문제없이 끝난 작업들이 알릴 완료다. 다 한 작업이 없으면(모두 취소) null이다.
export function finishedNotice(jobs: readonly (CopyJob | UploadJob)[]): NotifyInput | null {
  const done = jobs.filter((j) => j.status === 'done')
  if (done.length === 0) return null
  if (done.length > 1) return { code: 'filesDone', level: 'success', params: { count: done.length } }
  const job = done[0]
  if (job.kind === 'copy') return { code: 'filesCopied', level: 'success', params: { count: job.copied } }
  return { code: 'filesUploaded', level: 'success', params: { count: job.files.filter((f) => f.status === 'done').length } }
}
