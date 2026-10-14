// 해독 문구 효과. 무작위 글자가 한 글자씩 맞춰지며 문장이 나타난다(OMV 매트릭스 효과에 대한 오마주).
// 문구마다 뒤섞임 → 모두 확정 → 유지 → 빈 화면 순서로 지나가고, 마지막 문구 뒤에는 처음으로 돌아간다.
// 문구는 영어로 고정한다(현지화 제외).
import type { Rect } from '../../windows/geometry'
import type { EggEffect, Rng } from './effects'

// 영화 매트릭스(1999) 첫 장면의 메시지로 시작해, 갈수록 sysadmin 드립과 다른 작품 인용으로 선을 넘는다.
// 좁은 화면에서도 한 줄에 들어가도록 MAX_PHRASE_LENGTH 이하의 ASCII만 쓴다(decode.test.ts가 검사).
export const PHRASES = [
  'Wake up, sysadmin...',
  'The Matrix has you...',
  'Follow the white LED.',
  'Knock, knock, sysadmin.',
  'There is no spoon. Only RAID.',
  'RAID is not a backup, Mr. Anderson.',
  'Take the blue pill: reboot and forget.',
  'Take the red pill: read the logs.',
  'Deja vu? Probably a cron job.',
  "The Oracle says: it's DNS.",
  "It's always DNS.",
  'I know kung fu. And chmod.',
  'Free your mind. Then your disk space.',
  'Dodge this: apt full-upgrade.',
  'The spoon was a symlink.',
  'Neo, your uptime is over 9000.',
  'There is no cloud. Only my NAS.',
  "I'm running out of movie quotes.",
  'Seriously. Just log in.',
  '...are you still there?',
  // 스타크래프트 이스터에그의 중국식 레몬 닭고기 레시피(원문 그대로)
  'CHINESE LEMON CHICKEN',
  '1 pound sliced chicken brst.',
  '1 tsp corn starch.',
  '3 tbsp soy sauce(dark).',
  '1 sliced lemon.',
]

export const MAX_PHRASE_LENGTH = 40

// 첫 글자가 확정되기 전 뒤섞이기만 하는 시간
const LEAD_MS = 400
// 앞 글자부터 이 간격으로 확정하되, 글자마다 JITTER_MS 안에서 무작위로 늦춘다.
const CHAR_STEP_MS = 60
const JITTER_MS = 500
// 모두 확정된 뒤 문장을 보여 주는 시간과, 다음 문구 전 빈 화면 시간
const HOLD_MS = 3500
const GAP_MS = 1000
const CURSOR_BLINK_MS = 500

const COLOR = '#4cd964'
const SCRAMBLE_COLOR = '#2a7a3a'
const FONT_FAMILY = 'ui-monospace, "DejaVu Sans Mono", Menlo, Consolas, monospace'
// 고정폭 글꼴의 글자 폭 대략값(em). 글꼴 크기를 정할 때만 쓰고, 그릴 때는 실제 폭을 잰다.
const CHAR_WIDTH_EM = 0.6

export interface PhrasePlan {
  text: string
  // 글자별 확정 시각(문구 시작 기준 ms). 공백은 0
  resolveAt: number[]
  // 모든 글자가 확정되는 시각
  resolvedAt: number
}

export interface DecodeCell {
  ch: string
  done: boolean
}

export interface DecodeFrame {
  // 지금 보여 줄 글자. 빈 화면 동안은 빈 배열
  cells: DecodeCell[]
}

export function planPhrases(phrases: readonly string[], rng: Rng): PhrasePlan[] {
  return phrases.map((text) => {
    const resolveAt = [...text].map((ch, i) => (ch === ' ' ? 0 : LEAD_MS + i * CHAR_STEP_MS + rng() * JITTER_MS))
    return { text, resolveAt, resolvedAt: Math.max(0, ...resolveAt) }
  })
}

const phraseDuration = (p: PhrasePlan) => p.resolvedAt + HOLD_MS + GAP_MS

// randomGlyph는 ASCII 33~126 중 하나다(OMV 매트릭스 효과와 같은 범위).
function randomGlyph(rng: Rng): string {
  return String.fromCharCode(33 + Math.floor(rng() * 94))
}

// decodeFrame은 연출 시작 후 timeMs일 때 화면에 보일 글자다. 확정 전 글자는 rng로 뒤섞는다.
export function decodeFrame(plans: readonly PhrasePlan[], timeMs: number, rng: Rng): DecodeFrame {
  const cycle = plans.reduce((sum, p) => sum + phraseDuration(p), 0)
  if (cycle === 0) return { cells: [] }
  let t = timeMs % cycle
  for (const plan of plans) {
    const duration = phraseDuration(plan)
    if (t >= duration) {
      t -= duration
      continue
    }
    if (t >= plan.resolvedAt + HOLD_MS) return { cells: [] }
    const cells = [...plan.text].map((ch, i) => {
      const done = ch === ' ' || t >= plan.resolveAt[i]
      return { ch: done ? ch : randomGlyph(rng), done }
    })
    return { cells }
  }
  return { cells: [] }
}

// textCenterY는 문구를 그릴 세로 위치다. 로그인 카드 위아래 중 넓은 쪽의 가운데이며, 카드가 없으면 화면 가운데다.
export function textCenterY(height: number, avoid: Rect | null): number {
  if (!avoid) return height / 2
  const above = Math.max(0, avoid.y)
  const belowTop = Math.min(height, avoid.y + avoid.height)
  const below = height - belowTop
  return above >= below ? above / 2 : belowTop + below / 2
}

export function createEffect(rng: Rng): EggEffect {
  const plans = planPhrases(PHRASES, rng)
  const longest = Math.max(...PHRASES.map((p) => p.length)) + 1
  let width = 0
  let height = 0
  let fontSize = 16
  let y = 0

  return {
    resize(w, h, avoid) {
      width = w
      height = h
      y = textCenterY(h, avoid)
      // 가장 긴 문구와 커서가 화면 너비의 85% 안에 들어가게 한다.
      fontSize = Math.max(14, Math.min(40, (w * 0.85) / longest / CHAR_WIDTH_EM))
    },
    draw(ctx, timeMs) {
      ctx.clearRect(0, 0, width, height)

      const { cells } = decodeFrame(plans, timeMs, rng)
      ctx.font = `${fontSize}px ${FONT_FAMILY}`
      ctx.textBaseline = 'middle'
      ctx.shadowColor = COLOR
      ctx.shadowBlur = fontSize * 0.4
      const charWidth = ctx.measureText('M').width
      // 커서 자리까지 포함해 가운데에 둔다. 뒤섞이는 동안에도 글자 수가 같아 위치가 흔들리지 않는다.
      const x0 = (width - charWidth * (cells.length + 1)) / 2
      cells.forEach((cell, i) => {
        ctx.fillStyle = cell.done ? COLOR : SCRAMBLE_COLOR
        ctx.fillText(cell.ch, x0 + i * charWidth, y)
      })
      if (Math.floor(timeMs / CURSOR_BLINK_MS) % 2 === 0) {
        ctx.fillStyle = COLOR
        ctx.fillRect(x0 + cells.length * charWidth, y - fontSize * 0.55, charWidth * 0.9, fontSize * 1.1)
      }
      ctx.shadowBlur = 0
    },
  }
}
