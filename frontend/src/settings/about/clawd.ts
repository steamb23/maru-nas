// Clawd 도트 그림(DOM 없음). 정보 페이지 대기 이스터에그(settings/ClawdParade.tsx)가 그린다.
//
// Clawd는 Anthropic Claude Code의 마스코트다. Claude Code에 대한 경의로, Claude 앱 입력창 위의 Clawd를
// 정사각 칸 12×8 격자로 그대로 옮겼다. 외부 이미지 파일은 들이지 않고 그림을 여기 데이터로만 둔다.
// 문제가 되면 이 이스터에그를 통째로 지운다(이 폴더의 clawd*.ts, ClawdParade.tsx·.css, AboutPage의 한 줄).

export const CLAWD_COLS = 12
export const CLAWD_ROWS = 8

// 자세. 걷기·도망은 다리 두 쌍(열 2·7, 열 4·9)을 번갈아 든다.
export type ClawdPose = 'stand' | 'blink' | 'walk-a' | 'walk-b' | 'startled' | 'flee-a' | 'flee-b'

// 칸: '#' 몸통, 'o' 눈, '.' 빈 칸
const BODY = '#'
const EYE = 'o'

// 기본 자세. 몸통 열 2–9, 눈 열 3·8, 팔은 2–3행에서 양옆으로 2칸씩, 다리는 열 2·4·7·9.
const STAND_TOP = ['..########..', '..#o####o#..', '############', '############', '..########..', '..########..']
// 놀란 자세의 윗부분: 팔을 만세하듯 위로 꺾어 들고(2행 어깨에서 0행까지) 눈이 세로 2칸으로 커진다.
const STARTLED_TOP = ['#.########.#', '#.#o####o#.#', '###o####o###', '..########..', '..########..', '..########..']
const BLINK_TOP = STAND_TOP.map((row) => row.replaceAll(EYE, BODY))

const LEGS = ['..#.#..#.#..', '..#.#..#.#..']
// 다리 2·7을 든 모습과 4·9를 든 모습
const LEGS_A = ['..#.#..#.#..', '....#....#..']
const LEGS_B = ['..#.#..#.#..', '..#....#....']

const BITMAPS: Record<ClawdPose, readonly string[]> = {
  stand: [...STAND_TOP, ...LEGS],
  blink: [...BLINK_TOP, ...LEGS],
  'walk-a': [...STAND_TOP, ...LEGS_A],
  'walk-b': [...STAND_TOP, ...LEGS_B],
  startled: [...STARTLED_TOP, ...LEGS],
  'flee-a': [...STARTLED_TOP, ...LEGS_A],
  'flee-b': [...STARTLED_TOP, ...LEGS_B],
}

// clawdBitmap은 자세의 그림을 행 문자열(CLAWD_ROWS개, 각 CLAWD_COLS자)로 돌려준다.
export function clawdBitmap(pose: ClawdPose): readonly string[] {
  return BITMAPS[pose]
}

export interface ClawdPaths {
  body: string
  eyes: string
}

// rowPath는 행마다 같은 칸이 이어진 구간을 1칸 높이의 사각형 하나로 묶은 path다(viewBox 0 0 12 8).
function rowPath(rows: readonly string[], cell: string): string {
  let d = ''
  rows.forEach((row, y) => {
    for (let x = 0; x < row.length; ) {
      if (row[x] !== cell) {
        x++
        continue
      }
      let end = x
      while (row[end] === cell) end++
      d += `M${x} ${y}h${end - x}v1h${x - end}z`
      x = end
    }
  })
  return d
}

const cache = new Map<ClawdPose, ClawdPaths>()

// clawdPaths는 자세를 몸통·눈 두 SVG path로 바꾼다. 자세마다 한 번만 계산한다.
export function clawdPaths(pose: ClawdPose): ClawdPaths {
  let paths = cache.get(pose)
  if (!paths) {
    const rows = BITMAPS[pose]
    paths = { body: rowPath(rows, BODY), eyes: rowPath(rows, EYE) }
    cache.set(pose, paths)
  }
  return paths
}
