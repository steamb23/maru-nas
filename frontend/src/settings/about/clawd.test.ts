import { describe, expect, it } from 'vitest'
import { CLAWD_COLS, CLAWD_ROWS, type ClawdPose, clawdBitmap, clawdPaths } from './clawd'

const POSES: ClawdPose[] = ['stand', 'blink', 'walk-a', 'walk-b', 'startled', 'flee-a', 'flee-b']

// cells는 그림에서 칸 종류가 ch인 칸의 [열, 행] 목록이다.
function cells(rows: readonly string[], ch: string): [number, number][] {
  return rows.flatMap((row, y) => [...row].flatMap((c, x) => (c === ch ? [[x, y] as [number, number]] : [])))
}

// pathCells는 rowPath가 만든 path(행마다 1칸 높이 사각형)를 다시 칸 목록으로 푼다.
function pathCells(d: string): [number, number][] {
  return [...d.matchAll(/M(\d+) (\d+)h(\d+)v1h-\d+z/g)].flatMap(([, x, y, w]) =>
    Array.from({ length: Number(w) }, (_, i) => [Number(x) + i, Number(y)] as [number, number]),
  )
}

describe('clawdBitmap', () => {
  it('기본 자세는 참고 그림(12×8)과 같다', () => {
    expect(clawdBitmap('stand')).toEqual([
      '..########..',
      '..#o####o#..',
      '############',
      '############',
      '..########..',
      '..########..',
      '..#.#..#.#..',
      '..#.#..#.#..',
    ])
  })

  it('모든 자세가 같은 크기다', () => {
    for (const pose of POSES) {
      const rows = clawdBitmap(pose)
      expect(rows).toHaveLength(CLAWD_ROWS)
      for (const row of rows) expect(row).toMatch(new RegExp(`^[#o.]{${CLAWD_COLS}}$`))
    }
  })

  it('쉬는 자세와 놀란 자세는 좌우 대칭이다', () => {
    for (const pose of ['stand', 'blink', 'startled'] as const) {
      for (const row of clawdBitmap(pose)) expect([...row].reverse().join('')).toBe(row)
    }
  })

  it('깜빡이면 눈이 몸통으로 덮인다', () => {
    expect(cells(clawdBitmap('blink'), 'o')).toEqual([])
    expect(cells(clawdBitmap('blink'), '#')).toHaveLength(cells(clawdBitmap('stand'), '#').length + 2)
  })

  it('걷기 두 자세는 다리 쌍을 번갈아 든다', () => {
    const feet = (pose: ClawdPose) => cells(clawdBitmap(pose), '#').filter(([, y]) => y === CLAWD_ROWS - 1)
    expect(feet('stand').map(([x]) => x)).toEqual([2, 4, 7, 9])
    expect(feet('walk-a').map(([x]) => x)).toEqual([4, 9])
    expect(feet('walk-b').map(([x]) => x)).toEqual([2, 7])
    expect(feet('flee-a')).toEqual(feet('walk-a'))
    expect(feet('flee-b')).toEqual(feet('walk-b'))
  })

  it('놀라면 팔을 위로 들고 눈이 세로 2칸이 된다', () => {
    const rows = clawdBitmap('startled')
    // 팔: 0–2행의 바깥 열과 2행의 어깨. 기본 자세의 3행 팔은 없다.
    expect(rows.map((row) => row.slice(0, 2))).toEqual(['#.', '#.', '##', '..', '..', '..', '..', '..'])
    expect(cells(rows, 'o')).toEqual([
      [3, 1],
      [8, 1],
      [3, 2],
      [8, 2],
    ])
  })
})

describe('clawdPaths', () => {
  it('path가 그림의 몸통·눈 칸을 그대로 덮는다', () => {
    for (const pose of POSES) {
      const rows = clawdBitmap(pose)
      const { body, eyes } = clawdPaths(pose)
      expect(pathCells(body)).toEqual(cells(rows, '#'))
      expect(pathCells(eyes)).toEqual(cells(rows, 'o'))
    }
  })

  it('같은 자세는 같은 객체를 돌려준다', () => {
    expect(clawdPaths('stand')).toBe(clawdPaths('stand'))
  })
})
