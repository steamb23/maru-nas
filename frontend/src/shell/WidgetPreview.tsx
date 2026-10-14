import { type Ref, useImperativeHandle, useLayoutEffect, useRef } from 'react'
import type { PxRect } from '../widgets/grid'
import { animationsEnabled } from './useWindowAnimation'
import { type SpringRect, enterRect, springAt, springSettled, stepSpring } from './widgetPreviewAnim'
import { flipTransform, sameRect } from './windowAnim'

// WidgetPreviewHandle은 끄는 쪽(WidgetFrame)이 포인터가 움직일 때마다 부른다. 렌더 없이 가이드만 움직인다.
export interface WidgetPreviewHandle {
  // 놓을 자리(위젯 레이어 기준 픽셀)로 옮긴다. null이면 놓을 수 없는 자리라 숨기기만 하고,
  // 다시 보일 때 마지막 자리에서 이어서 움직인다.
  moveTo: (rect: PxRect | null) => void
}

interface Props {
  // 처음 그려질 때의 놓을 자리. 놓을 수 있는 자리를 아직 만나지 않았으면 null
  initial: PxRect | null
  ref?: Ref<WidgetPreviewHandle>
}

interface State {
  // 놓을 자리. 아직 놓을 수 있는 자리를 만나지 않았으면 null
  target: PxRect | null
  // 지금 보이는 자리와 속도. 애니메이션을 끄면 null(바로 놓을 자리에 그린다)
  spring: SpringRect | null
  // 진행 중인 requestAnimationFrame 번호(없으면 0)와 지난 프레임 시각
  frame: number
  time: number
}

// apply는 지금 보이는 자리를 transform으로 그린다.
function apply(e: HTMLElement, s: State) {
  if (!s.target) return
  const t = s.spring ? flipTransform(s.target, s.spring) : 'none'
  e.style.transform = t === 'none' ? '' : t
}

// run은 스프링이 멈출 때까지 프레임마다 진행한다. 이미 돌고 있으면 그대로 둔다(목표만 바뀐다).
function run(e: HTMLElement, s: State) {
  if (s.frame) return
  s.time = 0
  const tick = (now: number) => {
    s.frame = 0
    if (!s.spring || !s.target) return
    // 첫 프레임은 지난 시각이 없으므로 한 프레임(60Hz)으로 본다.
    const dt = s.time ? (now - s.time) / 1000 : 1 / 60
    s.time = now
    s.spring = stepSpring(s.spring, s.target, dt)
    const settled = springSettled(s.spring, s.target)
    if (settled) s.spring = springAt(s.target)
    apply(e, s)
    if (!settled) s.frame = requestAnimationFrame(tick)
  }
  s.frame = requestAnimationFrame(tick)
}

// moveTo는 가이드를 rect로 옮긴다. null이면 숨기기만 한다(WidgetPreviewHandle.moveTo).
function moveTo(e: HTMLElement, s: State, rect: PxRect | null) {
  // className은 React가 바꾸지 않는 고정값이라 여기서 바꾼 hidden이 렌더로 지워지지 않는다.
  e.classList.toggle('hidden', rect === null)
  if (rect && !(s.target && sameRect(s.target, rect))) {
    const first = s.target === null
    s.target = rect
    Object.assign(e.style, { left: `${rect.x}px`, top: `${rect.y}px`, width: `${rect.width}px`, height: `${rect.height}px` })
    if (!animationsEnabled()) {
      s.spring = null
      apply(e, s)
      return
    }
    // 처음 나타날 때는 조금 작은 자리에서 커지고, 투명도는 CSS 전환(.hidden)이 맡는다.
    if (first) s.spring = springAt(enterRect(rect))
    // 레이아웃 자리가 바뀌었으므로 같은 프레임 안에 transform도 바꿔야 한 프레임 튀지 않는다.
    apply(e, s)
  }
  // 자리가 같아도 멈추지 않은 스프링이 있으면 이어서 돌린다(effect를 정리했다가 다시 실행한 경우 등).
  if (s.spring) run(e, s)
}

// WidgetPreview는 위젯을 끌거나 크기를 바꾸는 동안 놓으면 들어갈 자리를 보여 주는 가이드다.
// 모양은 타일링 가이드(TilePreview)를 따른다. 놓을 칸이 바뀌면 임계 감쇠 스프링으로 따라간다.
// 칸이 바뀔 때마다 애니메이션을 새로 시작하면 속도가 끊겨 빠르게 끌 때 멈칫거리므로, 위치와 속도를 이어 가며
// requestAnimationFrame으로 transform만 바꾼다. 요소의 레이아웃 자리(left·top·width·height)는 항상 놓을 자리이고,
// transform이 지금 보이는 자리(FLIP)를 만든다.
export function WidgetPreview({ initial, ref }: Props) {
  const el = useRef<HTMLDivElement>(null)
  const state = useRef<State>({ target: null, spring: null, frame: 0, time: 0 })

  useImperativeHandle(
    ref,
    () => ({
      moveTo: (rect) => {
        if (el.current) moveTo(el.current, state.current, rect)
      },
    }),
    [],
  )

  // initial은 끌기를 시작할 때 한 번 정해진다(이후 이동은 핸들의 moveTo가 한다).
  useLayoutEffect(() => {
    const e = el.current
    if (!e) return
    const s = state.current
    // 숨긴 상태(.hidden)의 스타일을 먼저 계산시켜야 처음 보일 때 투명도 전환이 재생된다.
    getComputedStyle(e).getPropertyValue('opacity')
    moveTo(e, s, initial)
    return () => {
      // 번호도 지워야 다시 실행될 때 run이 루프를 새로 시작한다.
      cancelAnimationFrame(s.frame)
      s.frame = 0
    }
  }, [initial])

  return <div ref={el} className="tile-preview widget-preview hidden" aria-hidden="true" />
}
