// 창 최대화·최소화 애니메이션 재생. 위치 계산과 설정값은 windowAnim.ts에 있다.
//
// - 최대화·복원: 버튼·더블클릭, 최대화된 창 끌기 핸들러가 captureFrom()으로 바뀌기 전 위치를 남긴 경우에만 재생한다.
//   창 틀이 늘어나는 동안 창 내용은 숨겼다가 끝무렵에 페이드인해, 내용이 늘어나 보이지 않게 한다.
// - 최소화·복원: 작업표시줄, 바탕화면 아이콘 등 어디서 바뀌든 minimized 상태 변화를 보고 재생한다.
//   최소화 애니메이션이 끝날 때까지는 창을 숨기지 않는다.
// 재생 중에 다시 바뀌면 지금 보이는 위치·투명도에서 이어 간다.
import { type RefObject, useCallback, useLayoutEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import type { Rect } from '../windows/geometry'
import {
  CONTENT_FADE_START,
  EASING,
  FALLBACK_SCALE,
  MAXIMIZE_MS,
  MINIMIZE_MS,
  WINDOW_ANIMATIONS,
  fitInside,
  flipTransform,
  sameRect,
  scaleAbout,
} from './windowAnim'

type MinAnim = 'minimize' | 'restore' | null

function animationsEnabled(): boolean {
  return WINDOW_ANIMATIONS && !window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

function rectOf(el: Element): Rect {
  const r = el.getBoundingClientRect()
  return { x: r.left, y: r.top, width: r.width, height: r.height }
}

// minimizeTarget은 최소화한 창이 줄어들어 갈 위치다. 작업표시줄 버튼이 없으면 제자리에서 조금 줄인다.
function minimizeTarget(id: string, layout: Rect): Rect {
  const button = document.querySelector(`.taskbar [data-window-id="${CSS.escape(id)}"]`)
  return button ? fitInside(layout, rectOf(button)) : scaleAbout(layout, FALLBACK_SCALE)
}

export interface WindowAnimation {
  // 창에 hidden을 붙일지. 최소화 애니메이션 중에는 false다.
  hidden: boolean
  // 최소화 애니메이션 중인지. 이때는 창이 클릭을 받지 않는다.
  minimizing: boolean
  // captureFrom은 최대화를 바꾸기 직전에 호출해 지금 위치에서 애니메이션이 시작되게 한다.
  captureFrom: () => void
}

export function useWindowAnimation(
  ref: RefObject<HTMLElement | null>,
  id: string,
  minimized: boolean,
  maximized: boolean,
): WindowAnimation {
  const [prevMinimized, setPrevMinimized] = useState(minimized)
  const [minAnim, setMinAnim] = useState<MinAnim>(null)
  // 렌더 중에 minimized 변화를 감지해, 최소화가 커밋되는 그 렌더에서부터 창을 계속 보이게 한다.
  if (minimized !== prevMinimized) {
    setPrevMinimized(minimized)
    setMinAnim(animationsEnabled() ? (minimized ? 'minimize' : 'restore') : null)
  }

  // 재생 중인 애니메이션. 핸들러는 자기가 아직 현재 애니메이션인지 이 값으로 확인한다.
  const current = useRef<Animation | null>(null)
  // captureFrom이 잰 최대화 전환 전 위치
  const maxFrom = useRef<Rect | null>(null)

  const stop = () => {
    const a = current.current
    current.current = null
    a?.cancel()
  }

  const play = (el: HTMLElement, keyframes: Keyframe[], duration: number, fill: FillMode = 'none', easing = EASING) => {
    const a = el.animate(keyframes, { duration, easing, fill })
    current.current = a
    return a
  }

  const captureFrom = useCallback(() => {
    const el = ref.current
    if (el && animationsEnabled()) maxFrom.current = rectOf(el)
  }, [ref])

  useLayoutEffect(() => {
    const from = maxFrom.current
    maxFrom.current = null
    const el = ref.current
    if (!from || !el) return
    stop()
    const layout = rectOf(el)
    if (sameRect(layout, from)) return
    // transform은 첫 키프레임의 easing을 따르고, 창 내용은 CONTENT_FADE_START부터 선형으로 나타난다.
    play(
      el,
      [
        { offset: 0, transform: flipTransform(layout, from), easing: EASING, '--window-content-opacity': 0 },
        { offset: CONTENT_FADE_START, '--window-content-opacity': 0 },
        { offset: 1, transform: 'none', '--window-content-opacity': 1 },
      ],
      MAXIMIZE_MS,
      'none',
      'linear',
    )
  }, [ref, maximized])

  useLayoutEffect(() => {
    const el = ref.current
    if (!minAnim || !el) return
    // 이어서 재생할 수 있도록 멈추기 전에 지금 보이는 위치와 투명도를 잰다.
    const running = current.current !== null
    const visual = rectOf(el)
    const opacity = running ? Number(getComputedStyle(el).opacity) : 1
    stop()
    const layout = rectOf(el)
    const target = minimizeTarget(id, layout)

    if (minAnim === 'minimize') {
      // 끝난 뒤 hidden이 붙기 전에 창이 한 프레임 다시 보이지 않도록 마지막 프레임을 유지하고,
      // hidden을 같은 프레임에 반영한 뒤 애니메이션을 지운다.
      const a = play(
        el,
        [
          { transform: flipTransform(layout, visual), opacity },
          { transform: flipTransform(layout, target), opacity: 0 },
        ],
        MINIMIZE_MS,
        'forwards',
      )
      a.onfinish = () => {
        if (current.current !== a) return
        current.current = null
        flushSync(() => setMinAnim(null))
        a.cancel()
      }
    } else {
      // 숨겨져 있던 창이면 작업표시줄 버튼에서, 최소화 도중이면 지금 위치에서 시작한다.
      const start = running ? visual : target
      const a = play(
        el,
        [
          { transform: flipTransform(layout, start), opacity: running ? opacity : 0 },
          { transform: 'none', opacity: 1 },
        ],
        MINIMIZE_MS,
      )
      a.onfinish = () => {
        if (current.current !== a) return
        current.current = null
        setMinAnim(null)
      }
    }
  }, [ref, id, minAnim])

  // 창이 닫히면 재생 중인 애니메이션을 멈춘다.
  useLayoutEffect(() => stop, [])

  return { hidden: minimized && minAnim !== 'minimize', minimizing: minimized && minAnim === 'minimize', captureFrom }
}
