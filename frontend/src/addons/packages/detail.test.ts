import { describe, expect, it } from 'vitest'
import { ApiError } from '../../api/client'
import { changelogProblem, parseDescription } from './detail'

describe('패키지 자세히 보기', () => {
  it('parseDescription은 문단의 줄을 이어 다시 흐르게 한다', () => {
    expect(parseDescription('This package is part of the OpenSSL\nproject.\n\nSecond paragraph.')).toEqual([
      { kind: 'p', text: 'This package is part of the OpenSSL project.' },
      { kind: 'p', text: 'Second paragraph.' },
    ])
  })

  it('parseDescription은 글머리표 줄을 목록으로 바꾸고 이어지는 줄을 항목에 붙인다', () => {
    // dpkg 상태(들여쓰기가 남음)와 업데이트 목록(들여쓰기가 지워짐) 모두
    for (const text of [
      'useful for operations such as:\n * creating keys;\n * creating X.509 certificates,\n   CSRs, and CRLs;',
      'useful for operations such as:\n* creating keys;\n* creating X.509 certificates,\nCSRs, and CRLs;',
    ]) {
      expect(parseDescription(text)).toEqual([
        { kind: 'p', text: 'useful for operations such as:' },
        { kind: 'ul', items: ['creating keys;', 'creating X.509 certificates, CSRs, and CRLs;'] },
      ])
    }
    expect(parseDescription(' - one\n + two\n • three\n o four')).toEqual([
      { kind: 'ul', items: ['one', 'two', 'three', 'four'] },
    ])
  })

  it('parseDescription은 목록 밖의 들여 쓴 줄을 원문 그대로 둔다', () => {
    expect(parseDescription('Example:\n    systemctl status x\n      --no-pager\nAfter.')).toEqual([
      { kind: 'p', text: 'Example:' },
      { kind: 'pre', text: 'systemctl status x\n  --no-pager' },
      { kind: 'p', text: 'After.' },
    ])
    // 들여쓰기 없는 'o'는 문장일 수 있다.
    expect(parseDescription('o is a letter')).toEqual([{ kind: 'p', text: 'o is a letter' }])
    expect(parseDescription('')).toEqual([])
  })

  it('changelogProblem은 오류 코드를 화면 상태로 바꾼다', () => {
    const e = (code: ConstructorParameters<typeof ApiError>[2]) => new ApiError(400, 'x', code)
    expect(changelogProblem(e('action_busy'))).toEqual({ kind: 'pending' })
    expect(changelogProblem(e('no_changelog'))).toEqual({ kind: 'none' })
    expect(changelogProblem(e('job_busy'))).toEqual({ kind: 'busy' })
    expect(changelogProblem(e('not_found'))).toEqual({ kind: 'gone' })
    expect(changelogProblem(e('download_failed'))).toEqual({ kind: 'failed', body: 'packages.detail.failed.download' })
    expect(changelogProblem(e('action_timeout'))).toEqual({ kind: 'failed', body: 'packages.detail.failed.timeout' })
    expect(changelogProblem(e('action_failed'))).toEqual({ kind: 'failed', body: 'packages.detail.failed.other' })
    expect(changelogProblem(new Error('network'))).toEqual({ kind: 'failed', body: 'packages.detail.failed.other' })
  })
})
