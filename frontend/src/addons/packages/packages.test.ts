import { describe, expect, it } from 'vitest'
import type { Job, JobOutput } from '../../api/types.gen'
import {
  OUTPUT_LIMIT,
  appendOutput,
  changelogRoute,
  elapsedSeconds,
  emptyOutput,
  jobHeading,
  jobReason,
  pickJob,
  startError,
  totalSize,
} from './packages'

const job = (over: Partial<Job> = {}): Job => ({
  id: '20261024-101500-1a2b3c4d',
  kind: 'apt-upgrade',
  state: 'running',
  owner: 'admin',
  mine: true,
  startedAt: '2026-10-24T10:15:00Z',
  outputEnd: 0,
  ...over,
})

const out = (from: number, text: string, over: Partial<JobOutput> = {}): JobOutput => ({
  from,
  next: from + text.length,
  text,
  more: false,
  state: 'running',
  ...over,
})

describe('시스템 업데이트', () => {
  it('changelogRoute는 파일 이름을 라우트 한 단계로 인코딩한다', () => {
    expect(changelogRoute('pool/main/o/openmediavault/openmediavault_8.5.12-1_all.deb')).toBe(
      'system/updatemgmt/updates/changelog/pool%2Fmain%2Fo%2Fopenmediavault%2Fopenmediavault_8.5.12-1_all.deb',
    )
  })

  it('totalSize는 크기를 더한다', () => {
    expect(totalSize([])).toBe(0)
    expect(
      totalSize([
        { name: 'a', version: '1', size: 10 },
        { name: 'b', version: '1', size: 5 },
      ]),
    ).toBe(15)
  })

  it('pickJob은 진행 중인 작업, 요청한 결과, 최근 결과 순으로 고른다', () => {
    const running = job()
    const a = job({ id: 'a', state: 'succeeded' })
    const b = job({ id: 'b', state: 'failed' })
    expect(pickJob(null, null)).toBeNull()
    expect(pickJob({ current: running, recent: [a, b] }, 'b')).toBe(running)
    expect(pickJob({ recent: [a, b] }, 'b')).toBe(b)
    expect(pickJob({ recent: [a, b] }, 'gone')).toBe(a)
    expect(pickJob({ recent: [] }, null)).toBeNull()
  })

  it('작업 머리말과 이유를 종류·결과·코드로 고른다', () => {
    expect(jobHeading(job({ kind: 'apt-update' }))).toBe('packages.job.update.running')
    expect(jobHeading(job({ state: 'failed' }))).toBe('packages.job.upgrade.failed')
    expect(jobReason(job({ state: 'failed', code: 'apt_core_removal' }))).toBe('packages.job.reason.aptCoreRemoval')
    expect(jobReason(job())).toBeNull()
  })

  it('startError는 모르는 코드를 일반 실패로 둔다', () => {
    expect(startError('apt_locked').title).toBe('packages.start.locked.title')
    expect(startError('job_busy').body).toBe('packages.start.busy.body')
    expect(startError('omv_error').title).toBe('packages.start.failed.title')
  })

  it('appendOutput은 커서를 따라 붙이고 서버가 앞을 버린 틈을 표시한다', () => {
    let buf = emptyOutput('j')
    buf = appendOutput(buf, out(0, 'a\nb\n'))
    buf = appendOutput(buf, out(4, 'c'))
    expect(buf).toMatchObject({ text: 'a\nb\nc', next: 5, lines: 2, truncated: false })
    buf = appendOutput(buf, out(100, 'd\n'))
    expect(buf).toMatchObject({ text: 'a\nb\ncd\n', next: 102, truncated: true })
  })

  it('appendOutput은 상한을 넘으면 앞을 줄 경계에서 버린다', () => {
    const line = 'x'.repeat(99) + '\n'
    let buf = emptyOutput('j')
    let from = 0
    for (let i = 0; i < 7; i++) {
      const text = line.repeat(1000)
      buf = appendOutput(buf, out(from, text))
      from += text.length
    }
    expect(buf.text.length).toBeLessThanOrEqual(OUTPUT_LIMIT)
    expect(buf.text.startsWith('x')).toBe(true)
    expect(buf.truncated).toBe(true)
    expect(buf.lines).toBe(7000)
    expect(buf.next).toBe(from)
  })

  it('elapsedSeconds는 끝났으면 끝난 시각까지 잰다', () => {
    const now = Date.parse('2026-10-24T10:19:12Z')
    expect(elapsedSeconds(job(), now)).toBe(252)
    expect(elapsedSeconds(job({ endedAt: '2026-10-24T10:16:00Z' }), now)).toBe(60)
    expect(elapsedSeconds(job({ startedAt: 'bad' }), now)).toBe(0)
  })
})
