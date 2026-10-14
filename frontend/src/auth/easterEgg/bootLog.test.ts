import { describe, expect, it } from 'vitest'
import { type LogLine, SCRIPT, SPINNER_FRAMES, bootFrame, formatTimespan, lineSegments, planBoot } from './bootLog'

const text = (line: LogLine) =>
  lineSegments(line)
    .map((s) => s.text)
    .join('')

describe('SCRIPT', () => {
  it('실패처럼 보이는 상태 없이 ASCII 문구만 쓴다', () => {
    for (const step of SCRIPT) {
      const s = step.kind === 'job' ? `${step.name} ${step.limit} ${step.done ?? ''}` : step.text
      expect(s, s).toMatch(/^[\x20-\x7e]+$/)
      expect(s, s).not.toMatch(/FAIL|ERROR|TIME|DEPEND/)
    }
  })

  it('마지막은 끝나지 않는 start job이다', () => {
    const last = SCRIPT.at(-1)
    expect(last?.kind).toBe('job')
    expect(last?.kind === 'job' && last.done).toBeFalsy()
  })
})

describe('formatTimespan', () => {
  it('systemd처럼 0인 단위를 빼고 쓴다', () => {
    expect(formatTimespan(0)).toBe('0s')
    expect(formatTimespan(5)).toBe('5s')
    expect(formatTimespan(89)).toBe('1min 29s')
    expect(formatTimespan(3600)).toBe('1h')
    expect(formatTimespan(90061)).toBe('1d 1h 1min 1s')
  })
})

describe('bootFrame', () => {
  const plan = planBoot(SCRIPT, () => 0.5)
  const jobAt = plan.steps.find((s) => s.step.kind === 'job' && s.step.name === 'Sysadmin Motivation')!.at
  const lastAt = plan.steps.at(-1)!.at

  it('시작하면 첫 줄만 보인다', () => {
    expect(bootFrame(plan, 0).map(text)).toEqual(['[  OK  ] Created slice System Slice.'])
  })

  it('줄은 시간 순서대로 쌓인다', () => {
    const counts = [0, 1000, 5000, 10_000].map((t) => bootFrame(plan, t).length)
    expect(counts).toEqual([...counts].sort((a, b) => a - b))
    expect(counts[3]).toBeGreaterThan(counts[0])
  })

  it('start job은 도는 별과 경과 시간을 보이다가 OK로 바뀐다', () => {
    const running = bootFrame(plan, jobAt + 2500).at(-1)!
    expect(running.kind).toBe('job')
    expect(text(running)).toMatch(/^\[[ *]{6}\] A start job is running for Sysadmin Motivation \(44s \/ 1min 30s\)$/)
    expect(SPINNER_FRAMES).toContain(text(running).slice(1, 7))
    const done = bootFrame(plan, jobAt + 5000).at(-1)!
    expect(text(done)).toBe('[  OK  ] Started Sysadmin Motivation (just in time).')
  })

  it('마지막 job은 점점 빨라지며 끝없이 돌다가, 끝나면 빈 화면 뒤 다시 부팅한다', () => {
    const early = bootFrame(plan, lastAt + 3500).at(-1)!
    const late = bootFrame(plan, lastAt + 70_000).at(-1)!
    expect(text(early)).toContain('(1s / no limit)')
    expect(text(late)).toMatch(/\(\d+d [^)]*\/ no limit\)/)
    expect(bootFrame(plan, lastAt + 75_000)).toEqual([])
    expect(bootFrame(plan, plan.cycle).map(text)).toEqual(['[  OK  ] Created slice System Slice.'])
  })
})
