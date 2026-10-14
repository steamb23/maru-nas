// 창 열기·닫기·최대화·타일링·최소화 애니메이션 재생. 위치 계산과 설정값은 windowAnim.ts에 있다.
//
// - 열기: 창이 처음 그려질 때 조금 작은 크기에서 커지며 나타난다. 최소화된 채로 그려지면 재생하지 않는다.
// - 닫기: 저장소에서는 창이 바로 지워지고, Desktop이 그 창을 닫기 애니메이션이 끝날 때까지 남겨 둔다.
//   closing이 켜지면 줄어들며 사라지고, 끝나면 onClosed로 알려 Desktop이 창을 뺀다.
// - 배치 전환(최대화·타일링과 그 해제): 버튼·더블클릭, 창 끌기 핸들러가 captureFrom()으로 바뀌기 전 위치를
//   남긴 경우에만 재생한다. 타일 그룹의 경계 조절처럼 배치 방식이 그대로인 크기 변화는 재생하지 않는다.
//   창 틀이 늘어나는 동안 창 내용은 숨겼다가 끝무렵에 페이드인해, 내용이 늘어나 보이지 않게 한다.
// - 최소화·복원: 작업표시줄, 바탕화면 아이콘 등 어디서 바뀌든 minimized 상태 변화를 보고 재생한다.
//   최소화 애니메이션이 끝날 때까지는 창을 숨기지 않는다.
// 재생 중에 다시 바뀌면 지금 보이는 위치·투명도에서 이어 간다.
import { type RefObject, useCallback, useLayoutEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import type { Rect } from '../windows/geometry'
import type { WindowLayout } from '../windows/tiling'
import {
  CLOSE_EASING,
  CLOSE_MS,
  CLOSE_SCALE,
  CONTENT_FADE_START,
  EASING,
  FALLBACK_SCALE,
  MAXIMIZE_MS,
  MINIMIZE_MS,
  OPEN_MS,
  OPEN_SCALE,
  WINDOW_ANIMATIONS,
  fitInside,
  flipTransform,
  sameRect,
  scaleAbout,
} from './windowAnim'

type MinAnim = 'minimize' | 'restore' | null

export function animationsEnabled(): boolean {
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
  // captureFrom은 배치 방식(최대화·타일링)을 바꾸기 직전에 호출해 지금 위치에서 애니메이션이 시작되게 한다.
  captureFrom: () => void
}

// 창 id → captureFrom. 창 밖(작업표시줄 메뉴)에서 배치를 바꿀 때 captureWindowLayout으로 부른다.
const layoutCapturers = new Map<string, () => void>()

// captureWindowLayout은 창 id의 captureFrom을 부른다. 그려지지 않은 창이면 아무것도 하지 않는다.
export function captureWindowLayout(id: string) {
  layoutCapturers.get(id)?.()
}

export function useWindowAnimation(
  ref: RefObject<HTMLElement | null>,
  id: string,
  minimized: boolean,
  layout: WindowLayout,
  // 저장소에서 지워져 닫기 애니메이션을 재생할 차례인지
  closing: boolean,
  // 닫기 애니메이션이 끝났거나 재생하지 않을 때 한 번 부른다.
  onClosed: () => void,
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
  // captureFrom이 잰 배치 전환 전 위치
  const layoutFrom = useRef<Rect | null>(null)

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

  // 숨겨진(최소화된) 창은 잴 위치가 없다. 복원과 함께 배치가 바뀌면 복원 애니메이션이 새 배치로 재생된다.
  const captureFrom = useCallback(() => {
    const el = ref.current
    if (el && !el.hidden && animationsEnabled()) layoutFrom.current = rectOf(el)
  }, [ref])

  useLayoutEffect(() => {
    layoutCapturers.set(id, captureFrom)
    return () => {
      if (layoutCapturers.get(id) === captureFrom) layoutCapturers.delete(id)
    }
  }, [id, captureFrom])

  // 처음 그려질 때 커지며 나타난다. minimized는 처음 값만 본다.
  const openedMinimized = useRef(minimized)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el || openedMinimized.current || !animationsEnabled()) return
    const layout = rectOf(el)
    const a = play(el, [
      { transform: flipTransform(layout, scaleAbout(layout, OPEN_SCALE)), opacity: 0 },
      { transform: 'none', opacity: 1 },
    ], OPEN_MS)
    a.onfinish = () => {
      if (current.current === a) current.current = null
    }
  }, [ref])

  useLayoutEffect(() => {
    const from = layoutFrom.current
    layoutFrom.current = null
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
  }, [ref, layout])

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

  useLayoutEffect(() => {
    if (!closing) return
    const el = ref.current
    // 숨겨진(최소화된) 창은 보여 줄 것이 없으므로 바로 끝낸다.
    if (!el || el.hidden || !animationsEnabled()) {
      stop()
      onClosed()
      return
    }
    // 열기·복원 도중에 닫혀도 지금 보이는 위치와 투명도에서 이어 간다.
    const running = current.current !== null
    const visual = rectOf(el)
    const opacity = running ? Number(getComputedStyle(el).opacity) : 1
    stop()
    const layout = rectOf(el)
    // 끝난 뒤 언마운트될 때까지 다시 보이지 않도록 마지막 프레임을 유지한다.
    const a = play(
      el,
      [
        { transform: flipTransform(layout, visual), opacity },
        { transform: flipTransform(layout, scaleAbout(layout, CLOSE_SCALE)), opacity: 0 },
      ],
      CLOSE_MS,
      'forwards',
      CLOSE_EASING,
    )
    a.onfinish = () => {
      if (current.current !== a) return
      onClosed()
    }
  }, [ref, closing, onClosed])

  // 창이 언마운트되면 재생 중인 애니메이션을 멈춘다.
  useLayoutEffect(() => stop, [])

  return { hidden: minimized && minAnim !== 'minimize', minimizing: minimized && minAnim === 'minimize', captureFrom }
}
