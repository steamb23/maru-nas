// 올리기 작업의 상태와 순수 계산(transferStore.ts가 쓴다). 작업 하나는 한 번에 고른(끌어 놓은) 파일들이고, 파일은
// 하나씩 차례로 올린다(도우미의 사용자당 동시 연산 4개 안에서 목록·내려받기에 자리를 남긴다). DOM에 의존하지 않아
// node 환경에서 테스트한다.
import type { ErrorCode } from '../api/client'
import type { RateSample, TransferStatus } from './copyJobs'

// UploadError는 파일 하나를 올리지 못한 이유다(백엔드 오류 코드, 연결 끊김은 network, 그 밖은 unknown).
export type UploadError = ErrorCode | 'network' | 'unknown'

export type UploadFileStatus = 'queued' | 'uploading' | 'done' | 'failed' | 'cancelled'

export interface UploadFile {
  name: string
  size: number
  sent: number
  status: UploadFileStatus
  // overwrite면 같은 이름을 덮어쓴다(사용자가 고름).
  overwrite: boolean
  error?: UploadError
}

export interface UploadJob {
  kind: 'upload'
  id: number
  // target은 올릴 폴더다.
  target: string
  files: readonly UploadFile[]
  status: TransferStatus
  // error는 작업 전체를 멈춘 오류(세션 만료, 보안 확인 취소 등)다.
  error: unknown
  rate: RateSample
}

// NewUpload는 더할 파일이다(파일 내용은 저장소가 같은 순서로 따로 둔다).
export interface NewUpload {
  name: string
  size: number
  overwrite: boolean
}

export function newUploadJob(id: number, target: string, files: readonly NewUpload[]): UploadJob {
  return {
    kind: 'upload',
    id,
    target,
    files: files.map((f) => ({ ...f, sent: 0, status: 'queued' })),
    status: 'queued',
    error: null,
    rate: { at: 0, bytes: 0, rate: null },
  }
}

// nextFile은 다음에 올릴 파일의 자리다. 없으면 -1이다.
export function nextFile(job: UploadJob): number {
  return job.files.findIndex((f) => f.status === 'queued')
}

// withFile은 i번째 파일을 바꾼 작업이다.
export function withFile(job: UploadJob, i: number, change: (f: UploadFile) => UploadFile): UploadJob {
  return { ...job, files: job.files.map((f, k) => (k === i ? change(f) : f)) }
}

export const fileStarted = (job: UploadJob, i: number): UploadJob =>
  withFile(job, i, (f) => ({ ...f, status: 'uploading', sent: 0, error: undefined }))

export const fileProgressed = (job: UploadJob, i: number, sent: number): UploadJob =>
  withFile(job, i, (f) => (f.status === 'uploading' ? { ...f, sent: Math.min(sent, f.size) } : f))

export const fileFinished = (job: UploadJob, i: number): UploadJob => withFile(job, i, (f) => ({ ...f, status: 'done', sent: f.size }))

export const fileFailed = (job: UploadJob, i: number, error: UploadError): UploadJob =>
  withFile(job, i, (f) => ({ ...f, status: 'failed', sent: 0, error }))

// stopped는 올리는 중이거나 기다리는 파일을 모두 취소한 작업이다. status는 끝난 작업의 상태다.
export function stopped(job: UploadJob, error: unknown = null): UploadJob {
  const files = job.files.map((f) => (f.status === 'uploading' || f.status === 'queued' ? { ...f, status: 'cancelled' as const, sent: 0 } : f))
  return { ...job, files, error, status: error !== null ? 'failed' : 'cancelled' }
}

// completed는 모든 파일을 해 본 작업이다. 하지 못한 파일이 있으면 failed다.
export function completed(job: UploadJob): UploadJob {
  return { ...job, status: job.files.some((f) => f.status === 'failed') ? 'failed' : 'done' }
}

// retryable은 다시 올릴 수 있는 파일 수다. overwrite면 같은 이름이 생겨 올리지 못한 파일(file_exists), 아니면 그 밖의
// 실패와 취소한 파일을 센다.
export function retryable(job: UploadJob, overwrite: boolean): number {
  return job.files.filter((f) => retryMatches(f, overwrite)).length
}

function retryMatches(f: UploadFile, overwrite: boolean): boolean {
  if (f.status === 'cancelled') return !overwrite
  if (f.status !== 'failed') return false
  return (f.error === 'file_exists') === overwrite
}

// retried는 retryable의 파일을 다시 기다리게 한 작업이다. overwrite면 그 파일들은 덮어쓴다.
export function retried(job: UploadJob, overwrite: boolean): UploadJob {
  const files = job.files.map((f) =>
    retryMatches(f, overwrite) ? { ...f, status: 'queued' as const, sent: 0, error: undefined, overwrite: overwrite || f.overwrite } : f,
  )
  return { ...job, files, status: 'queued', error: null, rate: { at: 0, bytes: 0, rate: null } }
}

// UploadAmount는 작업의 양이다. 바이트는 이번에 올릴(다시 올리기 전에 끝낸 것 포함) 파일만 센다.
export interface UploadAmount {
  bytes: number
  totalBytes: number
  files: number
  totalFiles: number
}

// uploadAmount는 올린 양이다. 취소한 파일은 빼고 센다.
export function uploadAmount(job: UploadJob): UploadAmount {
  const a = { bytes: 0, totalBytes: 0, files: 0, totalFiles: 0 }
  for (const f of job.files) {
    if (f.status === 'cancelled') continue
    a.totalFiles++
    a.totalBytes += f.size
    a.bytes += f.status === 'done' ? f.size : f.sent
    if (f.status === 'done') a.files++
  }
  return a
}

// uploadProgress는 0~1의 진행률이다. 내용이 없는 파일뿐이면 파일 수로 잰다.
export function uploadProgress(job: UploadJob): number {
  if (job.status === 'done') return 1
  const a = uploadAmount(job)
  if (a.totalBytes > 0) return Math.min(1, a.bytes / a.totalBytes)
  return a.totalFiles > 0 ? a.files / a.totalFiles : 0
}

// uploadRemaining은 지금 속도로 남은 시간(초)이다. 속도를 모르면 null이다.
export function uploadRemaining(job: UploadJob): number | null {
  if (job.status !== 'running' || !job.rate.rate || job.rate.rate <= 0) return null
  const a = uploadAmount(job)
  return Math.max(0, (a.totalBytes - a.bytes) / job.rate.rate)
}

// current는 지금 올리는 파일 이름이다.
export function uploadCurrent(job: UploadJob): string | null {
  return job.files.find((f) => f.status === 'uploading')?.name ?? null
}
