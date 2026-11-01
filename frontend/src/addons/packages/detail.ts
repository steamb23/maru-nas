// 패키지 자세히 보기(설명·변경 기록 모달)의 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../../api/client'

// DescBlock은 긴 설명을 그리는 단위다. 문단은 다시 흐르게 하고, 글머리표는 목록, 들여 쓴 줄은 원문 그대로다.
export type DescBlock = { kind: 'p'; text: string } | { kind: 'ul'; items: string[] } | { kind: 'pre'; text: string }

// 글머리표 줄. 들여쓰기 없는 'o '는 문장일 수 있어 들여 썼을 때만 글머리표로 본다.
const BULLET = /^(\s*)[*+\-•]\s+(.*)$/
const INDENTED_O = /^(\s+)o\s+(.*)$/

function indentOf(line: string): number {
  return line.length - line.trimStart().length
}

// parseDescription은 Debian 긴 설명(백엔드가 다듬은 꼴: 문단은 빈 줄로 나뉘고 줄바꿈·들여쓰기는 원문 그대로)을
// 그릴 블록으로 나눈다. 원문은 72~80열에서 줄을 바꿔 두었으므로 문단과 목록 항목은 줄을 이어 다시 흐르게 한다.
// 목록 항목 다음의 글머리표 없는 줄은 그 항목의 이어지는 줄로 본다(업데이트 목록의 설명은 들여쓰기가 지워져
// 있어 깊이로 가릴 수 없다). 목록 밖에서 들여 쓴 줄(Debian 정책의 "그대로 표시")은 원문 그대로 둔다.
export function parseDescription(text: string): DescBlock[] {
  const blocks: DescBlock[] = []
  for (const para of text.split(/\n[ \t]*\n/)) {
    let words: string[] = []
    let items: string[] | null = null
    let pre: string[] = []
    const flushText = () => {
      if (words.length > 0) blocks.push({ kind: 'p', text: words.join(' ') })
      words = []
    }
    const flushList = () => {
      if (items && items.length > 0) blocks.push({ kind: 'ul', items })
      items = null
    }
    const flushPre = () => {
      if (pre.length > 0) {
        const cut = Math.min(...pre.filter((l) => l.trim() !== '').map(indentOf))
        blocks.push({ kind: 'pre', text: pre.map((l) => l.slice(cut)).join('\n') })
      }
      pre = []
    }
    for (const line of para.split('\n')) {
      if (line.trim() === '') continue
      const bullet = BULLET.exec(line) ?? INDENTED_O.exec(line)
      if (bullet) {
        flushText()
        flushPre()
        items ??= []
        items.push(bullet[2].trim())
      } else if (items) {
        items[items.length - 1] = `${items[items.length - 1]} ${line.trim()}`
      } else if (indentOf(line) > 0) {
        flushText()
        pre.push(line)
      } else {
        flushPre()
        words.push(line.trim())
      }
    }
    flushText()
    flushList()
    flushPre()
  }
  return blocks
}

// ChangelogProblem은 변경 기록을 받지 못한 이유다.
//   - pending: 같은 패키지를 다른 요청이 가져오는 중이다(action_busy). 조금 뒤 다시 묻는다
//   - none: 패키지에 변경 기록이 없다
//   - busy: 업데이트 작업(확인·설치)이 진행 중이다. 작업이 끝나면 다시 묻는다
//   - gone: 패키지가 업데이트 목록에서 사라졌다(이미 설치됨 등)
//   - failed: 그 밖의 실패. 다시 시도할 수 있다
export type ChangelogProblem =
  | { kind: 'pending' | 'none' | 'busy' | 'gone' }
  | { kind: 'failed'; body: ParseKeys }

export function changelogProblem(err: unknown): ChangelogProblem {
  const code = err instanceof ApiError ? err.code : undefined
  switch (code) {
    case 'action_busy':
      return { kind: 'pending' }
    case 'no_changelog':
      return { kind: 'none' }
    case 'job_busy':
      return { kind: 'busy' }
    case 'not_found':
      return { kind: 'gone' }
    case 'download_failed':
      return { kind: 'failed', body: 'packages.detail.failed.download' }
    case 'action_timeout':
      return { kind: 'failed', body: 'packages.detail.failed.timeout' }
  }
  return { kind: 'failed', body: 'packages.detail.failed.other' }
}
