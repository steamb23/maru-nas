// 올리기 목록의 상태 계산. 파일은 하나씩 차례로 올린다(도우미의 사용자당 동시 연산 4개 안에서 목록·내려받기에
// 자리를 남긴다). DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ErrorCode } from '../api/client'

export type UploadStatus = 'queued' | 'uploading' | 'done' | 'failed' | 'canceled' | 'skipped'

export interface UploadItem {
  id: number
  // dir은 올릴 폴더, name은 파일 이름이다.
  dir: string
  name: string
  size: number
  sent: number
  status: UploadStatus
  // overwrite면 같은 이름을 덮어쓴다(사용자가 고름).
  overwrite: boolean
  // error는 실패 이유다(백엔드 오류 코드, 연결 끊김은 network, 그 밖은 unknown).
  error?: ErrorCode | 'network' | 'unknown'
  // startedAt은 올리기를 시작한 시각(ms)이다. 남은 시간 계산에 쓴다.
  startedAt?: number
}

export interface UploadState {
  items: readonly UploadItem[]
}

export const emptyUploads: UploadState = { items: [] }

// NewUpload는 더할 파일이다. id는 부르는 쪽이 정한다(올릴 File을 같은 id로 따로 둔다).
export interface NewUpload {
  id: number
  dir: string
  name: string
  size: number
  overwrite: boolean
}

// enqueue는 파일을 목록 끝에 더한다. 끝난 항목은 그대로 둔다(목록을 비우는 것은 clearFinished).
export function enqueue(s: UploadState, files: readonly NewUpload[]): UploadState {
  const added = files.map((f) => ({ ...f, sent: 0, status: 'queued' as const }))
  return { items: [...s.items, ...added] }
}

// nextQueued는 다음에 올릴 항목이다. 올리는 중인 항목이 있으면 없다(하나씩 올린다).
export function nextQueued(s: UploadState): UploadItem | undefined {
  if (s.items.some((i) => i.status === 'uploading')) return undefined
  return s.items.find((i) => i.status === 'queued')
}

function update(s: UploadState, id: number, change: (i: UploadItem) => UploadItem): UploadState {
  return { ...s, items: s.items.map((i) => (i.id === id ? change(i) : i)) }
}

export const started = (s: UploadState, id: number, now: number): UploadState =>
  update(s, id, (i) => ({ ...i, status: 'uploading', sent: 0, startedAt: now, error: undefined }))

export const progressed = (s: UploadState, id: number, sent: number): UploadState =>
  update(s, id, (i) => (i.status === 'uploading' ? { ...i, sent: Math.min(sent, i.size) } : i))

export const finished = (s: UploadState, id: number): UploadState =>
  update(s, id, (i) => ({ ...i, status: 'done', sent: i.size }))

export const failed = (s: UploadState, id: number, error: NonNullable<UploadItem['error']>): UploadState =>
  update(s, id, (i) => ({ ...i, status: 'failed', error }))

export const skipped = (s: UploadState, id: number): UploadState => update(s, id, (i) => ({ ...i, status: 'skipped' }))

// canceled는 올리는 중이거나 기다리는 항목을 취소한다.
export const canceled = (s: UploadState, id: number): UploadState =>
  update(s, id, (i) => (i.status === 'uploading' || i.status === 'queued' ? { ...i, status: 'canceled' } : i))

// retried는 실패·취소한 항목을 다시 기다리게 한다. overwrite면 덮어쓰기로 다시 올린다.
export const retried = (s: UploadState, id: number, overwrite?: boolean): UploadState =>
  update(s, id, (i) =>
    i.status === 'failed' || i.status === 'canceled'
      ? { ...i, status: 'queued', sent: 0, error: undefined, overwrite: overwrite ?? i.overwrite }
      : i,
  )

// clearFinished는 끝난 항목(완료, 건너뜀, 취소)을 목록에서 뺀다. 실패한 항목은 다시 올릴 수 있게 남긴다.
export function clearFinished(s: UploadState): UploadState {
  return { ...s, items: s.items.filter((i) => i.status === 'queued' || i.status === 'uploading' || i.status === 'failed') }
}

// active는 아직 끝나지 않은 항목 수(기다림 + 올리는 중)다.
export function active(s: UploadState): number {
  return s.items.filter((i) => i.status === 'queued' || i.status === 'uploading').length
}

// remainingSeconds는 지금 속도로 남은 시간(초)이다. 3초가 지나기 전이나 보낸 것이 없으면 모른다(null).
export function remainingSeconds(item: UploadItem, now: number): number | null {
  if (item.status !== 'uploading' || item.startedAt === undefined || item.sent <= 0) return null
  const elapsed = (now - item.startedAt) / 1000
  if (elapsed < 3) return null
  const rate = item.sent / elapsed
  return Math.ceil((item.size - item.sent) / rate)
}
