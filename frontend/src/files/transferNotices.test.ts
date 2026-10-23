import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import { type CopyJob, newJob } from './copyJobs'
import { failedNotice, finishedNotice } from './transferNotices'
import { type UploadJob, newUploadJob } from './uploadJobs'

const copy = (patch: Partial<CopyJob>): CopyJob => ({ ...newJob(1, '/home/a', ['x', 'y'], '/srv/d', 1), ...patch })

const upload = (statuses: UploadJob['files'][number]['status'][], patch: Partial<UploadJob> = {}): UploadJob => {
  const job = newUploadJob(2, '/srv/d', statuses.map((_, i) => ({ name: `f${i}`, size: 1, overwrite: false })))
  return { ...job, files: job.files.map((f, i) => ({ ...f, status: statuses[i] })), ...patch }
}

const open = { kind: 'app', app: 'transfers' }

describe('failedNotice', () => {
  it('하지 못한 항목 수와 함께 알린다', () => {
    const failures = [{ item: 'x', err: new ApiError(409, 'x', 'file_exists') }]
    expect(failedNotice(copy({ status: 'failed', failures }))).toEqual({
      code: 'filesCopyFailed',
      level: 'error',
      params: { count: 1 },
      action: open,
    })
    expect(failedNotice(upload(['done', 'failed', 'failed'], { status: 'failed' }))).toEqual({
      code: 'filesUploadFailed',
      level: 'error',
      params: { count: 2 },
      action: open,
    })
  })

  it('작업 전체가 멈췄으면 수 없이 알린다', () => {
    expect(failedNotice(copy({ status: 'failed', error: new Error('x') }))?.params).toBeUndefined()
  })

  it('실패가 아니면 알리지 않는다', () => {
    expect(failedNotice(copy({ status: 'done' }))).toBeNull()
    expect(failedNotice(copy({ status: 'cancelled' }))).toBeNull()
    expect(failedNotice(upload(['cancelled'], { status: 'cancelled' }))).toBeNull()
  })
})

describe('finishedNotice', () => {
  it('작업 하나는 종류와 항목 수로 알린다', () => {
    expect(finishedNotice([copy({ status: 'done', copied: 2 })])).toEqual({ code: 'filesCopied', level: 'success', params: { count: 2 } })
    expect(finishedNotice([upload(['done', 'done', 'done'], { status: 'done' })])).toEqual({
      code: 'filesUploaded',
      level: 'success',
      params: { count: 3 },
    })
  })

  it('여럿이면 다 한 작업 수로 알린다', () => {
    const jobs = [copy({ status: 'done', copied: 2 }), upload(['done'], { status: 'done' }), copy({ status: 'cancelled' })]
    expect(finishedNotice(jobs)).toEqual({ code: 'filesDone', level: 'success', params: { count: 2 } })
  })

  it('모두 취소했으면 알리지 않는다', () => {
    expect(finishedNotice([copy({ status: 'cancelled' })])).toBeNull()
    expect(finishedNotice([])).toBeNull()
  })
})
