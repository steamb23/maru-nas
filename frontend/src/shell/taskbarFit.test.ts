import { describe, expect, it } from 'vitest'
import { TASK_ICON_WIDTH, TASK_LABEL_MIN, TASK_MAX_WIDTH, type TaskbarFit, fitTasks, sameFit } from './taskbarFit'

const gap = 4
const moreSpace = 50

describe('fitTasks', () => {
  it('창이 없으면 제목 모드다', () => {
    expect(fitTasks({ available: 500, gap, moreSpace, natural: [] })).toEqual({
      mode: 'labels',
      cap: TASK_MAX_WIDTH,
      truncated: [],
    })
  })

  it('모두 들어가면 줄이지 않는다', () => {
    // 120 + 150 + 90 + 간격 8 = 368
    expect(fitTasks({ available: 368, gap, moreSpace, natural: [120, 150, 90] })).toEqual({
      mode: 'labels',
      cap: TASK_MAX_WIDTH,
      truncated: [false, false, false],
    })
  })

  it('최대 너비보다 긴 제목은 공간이 남아도 잘린다', () => {
    expect(fitTasks({ available: 1000, gap, moreSpace, natural: [250, 100] })).toEqual({
      mode: 'labels',
      cap: TASK_MAX_WIDTH,
      truncated: [true, false],
    })
  })

  it('긴 제목부터 같은 너비로 줄이고 짧은 제목은 그대로 둔다', () => {
    // 버튼에 쓸 너비 340 - 8 = 332. 80은 그대로, 나머지 둘이 (332 - 80) / 2 = 126
    expect(fitTasks({ available: 340, gap, moreSpace, natural: [180, 160, 80] })).toEqual({
      mode: 'labels',
      cap: 126,
      truncated: [true, true, false],
    })
  })

  it('cap이 정수가 아니면 내려서 넘치지 않게 한다', () => {
    // (301 - 4) / 2 = 148.5
    const fit = fitTasks({ available: 301, gap, moreSpace, natural: [180, 180] })
    expect(fit).toEqual({ mode: 'labels', cap: 148, truncated: [true, true] })
  })

  it('cap이 제목 최소 너비보다 작아지면 아이콘만 그린다', () => {
    const natural = [150, 150]
    const atMin = TASK_LABEL_MIN * 2 + gap
    expect(fitTasks({ available: atMin, gap, moreSpace, natural })).toMatchObject({
      mode: 'labels',
      cap: TASK_LABEL_MIN,
    })
    expect(fitTasks({ available: atMin - 1, gap, moreSpace, natural })).toEqual({ mode: 'icons', visible: 2 })
  })

  it('제목이 짧아 최소 너비보다 좁은 버튼만 있으면 그 너비로 판단한다', () => {
    // 최소 너비보다 좁은 버튼은 줄일 필요가 없으므로 다 들어가면 제목 모드다.
    const natural = [70, 70, 70]
    expect(fitTasks({ available: 218, gap, moreSpace, natural })).toMatchObject({ mode: 'labels' })
  })

  it('아이콘만으로도 넘치면 "더 보기" 버튼 자리를 빼고 들어가는 만큼 보인다', () => {
    const natural = [150, 150, 150, 150, 150]
    const step = TASK_ICON_WIDTH + gap
    // 다섯 개 모두: 5 * 48 - 4 = 236
    expect(fitTasks({ available: 236, gap, moreSpace, natural })).toEqual({ mode: 'icons', visible: 5 })
    // 세 개(140) + 더 보기(50) = 190은 들어가고, 네 개(188) + 50 = 238은 넘친다.
    expect(fitTasks({ available: 235, gap, moreSpace, natural })).toEqual({ mode: 'icons', visible: 3 })
    expect(fitTasks({ available: 3 * step - gap + moreSpace, gap, moreSpace, natural })).toEqual({
      mode: 'icons',
      visible: 3,
    })
    expect(fitTasks({ available: 3 * step - gap + moreSpace - 1, gap, moreSpace, natural })).toEqual({
      mode: 'icons',
      visible: 2,
    })
  })

  it('공간이 없으면 모두 접는다', () => {
    expect(fitTasks({ available: 0, gap, moreSpace, natural: [150, 150] })).toEqual({ mode: 'icons', visible: 0 })
    expect(fitTasks({ available: -30, gap, moreSpace, natural: [150] })).toEqual({ mode: 'icons', visible: 0 })
  })
})

describe('sameFit', () => {
  it('모드와 값이 모두 같아야 같다', () => {
    const labels: TaskbarFit = { mode: 'labels', cap: 120, truncated: [true, false] }
    expect(sameFit(labels, { mode: 'labels', cap: 120, truncated: [true, false] })).toBe(true)
    expect(sameFit(labels, { mode: 'labels', cap: 121, truncated: [true, false] })).toBe(false)
    expect(sameFit(labels, { mode: 'labels', cap: 120, truncated: [true, true] })).toBe(false)
    expect(sameFit(labels, { mode: 'labels', cap: 120, truncated: [true] })).toBe(false)
    expect(sameFit(labels, { mode: 'icons', visible: 2 })).toBe(false)
    expect(sameFit({ mode: 'icons', visible: 2 }, { mode: 'icons', visible: 2 })).toBe(true)
    expect(sameFit({ mode: 'icons', visible: 2 }, { mode: 'icons', visible: 3 })).toBe(false)
  })
})
