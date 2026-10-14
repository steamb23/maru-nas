// 로그인 대기 이스터에그 오버레이. 로그인 카드 뒤 배경을 어둡게 덮고 캔버스에 고른 효과를 그린다.
// 효과 계약은 effects.ts, 설정값은 config.ts.
//
// 연출 중에만 마운트하고, 해제하면 페이드아웃 뒤 언마운트해 캔버스 버퍼를 돌려준다.
// 효과 시간은 연출이 시작된 시각(since)부터 실제로 흐른 시간이다. 탭이 숨겨진 동안 시작했거나 숨겨져 있었으면
// 돌아왔을 때 페이드인 없이, 그동안 재생된 지점부터 보인다(OMV 매트릭스 효과처럼).
// 오버레이는 포커스를 받지 않는다. 배경을 누른 포인터 입력은 오버레이가 받아 사라지는 동안에도 흡수하고,
// 카드는 오버레이 위에 있어 연출 중에도 평소처럼 쓸 수 있다.
import { type MouseEvent, type RefObject, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { Rect } from '../../windows/geometry'
import { EGG_FADE_IN_MS, EGG_FADE_OUT_MS, EGG_FPS } from './config'
import { EFFECTS, type EggEffect, pickEffect } from './effects'
import { createFrameGate } from './frameGate'

// mousedown의 기본 동작(포커스 이동)을 막아 입력란의 포커스를 유지한다.
const keepFocus = (e: MouseEvent) => e.preventDefault()

// 이보다 늦게 마운트되면(숨겨진 탭에서 늦게 확인한 경우 등) 이미 재생 중이던 것으로 보고 페이드인하지 않는다.
const LATE_MOUNT_MS = 1000

interface EasterEggProps {
  // 연출이 시작된 시각(performance.now 기준). null이면 연출하지 않는다.
  since: number | null
  // 효과가 피해서 그릴 로그인 카드
  avoid: RefObject<HTMLElement | null>
}

// EasterEgg는 since가 있는 동안 오버레이를 보여 준다. null이 되면 사라지는 애니메이션이 끝난 뒤 언마운트한다.
export function EasterEgg({ since, avoid }: EasterEggProps) {
  // 사라지는 동안에도 마지막 시작 시각으로 그린다.
  const [shown, setShown] = useState<number | null>(null)
  const onLeft = useCallback(() => setShown(null), [])
  if (since !== null && since !== shown) setShown(since)
  if (shown === null) return null
  return <Overlay key={shown} since={shown} leaving={since === null} onLeft={onLeft} avoid={avoid} />
}

interface OverlayProps {
  since: number
  leaving: boolean
  onLeft: () => void
  avoid: EasterEggProps['avoid']
}

function Overlay({ since, leaving, onLeft, avoid }: OverlayProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const firstRef = useRef(true)

  // 처음에는 배경이 어두워지며 나타나고, 해제하면 사라진다. 사라지는 중에 다시 켜지면 지금 투명도에서 되돌아온다.
  // 숨겨진 탭에서는 애니메이션이 끝나지 않을 수 있으므로 바로 언마운트한다.
  useLayoutEffect(() => {
    const el = rootRef.current
    if (!el) return
    if (leaving && document.hidden) {
      onLeft()
      return
    }
    const first = firstRef.current
    firstRef.current = false
    // 이미 재생 중이던 연출이면 페이드인 없이 바로 보인다.
    if (first && !leaving && (document.hidden || performance.now() - since > LATE_MOUNT_MS)) return
    const from = first ? 0 : Number(getComputedStyle(el).opacity)
    el.getAnimations().forEach((a) => a.cancel())
    const to = leaving ? 0 : 1
    const duration = (leaving ? EGG_FADE_OUT_MS : EGG_FADE_IN_MS) * Math.abs(to - from)
    const anim = el.animate([{ opacity: from }, { opacity: to }], { duration, easing: 'ease-out', fill: 'forwards' })
    if (leaving) anim.onfinish = onLeft
  }, [leaving, onLeft, since])

  useEffect(() => {
    const canvas = canvasRef.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return
    let effect: EggEffect | undefined
    let disposed = false
    let width = 0
    let height = 0
    let dpr = 1
    let avoidRect: Rect | null = null
    const avoidEl = avoid.current

    // 캔버스는 화면 전체(fixed, inset 0)라 카드의 화면 좌표가 곧 캔버스 좌표다.
    const resize = () => {
      dpr = window.devicePixelRatio || 1
      width = canvas.clientWidth
      height = canvas.clientHeight
      const pw = Math.round(width * dpr)
      const ph = Math.round(height * dpr)
      // 크기가 같으면 다시 잡지 않는다(다시 잡으면 버퍼를 새로 만들고 화면이 지워진다).
      if (canvas.width !== pw || canvas.height !== ph) {
        canvas.width = pw
        canvas.height = ph
      }
      const r = avoidEl?.getBoundingClientRect()
      avoidRect = r ? { x: r.left, y: r.top, width: r.width, height: r.height } : null
      effect?.resize(width, height, avoidRect)
    }
    resize()
    const observer = new ResizeObserver(resize)
    observer.observe(canvas)
    if (avoidEl) observer.observe(avoidEl)

    pickEffect(EFFECTS, Math.random)?.()
      .then((mod) => {
        if (disposed) return
        effect = mod.createEffect(Math.random)
        effect.resize(width, height, avoidRect)
      })
      .catch((err: unknown) => console.warn('easter egg effect failed to load', err))

    // 탭이 숨겨진 동안은 rAF가 멈춰 그리지 않지만, 시간은 since부터 계속 흐른다.
    const gate = createFrameGate(EGG_FPS)
    let raf = 0
    const frame = (now: number) => {
      raf = requestAnimationFrame(frame)
      if (!gate.tick(now) || !effect) return
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      effect.draw(ctx, Math.max(0, now - since))
    }
    raf = requestAnimationFrame(frame)

    return () => {
      disposed = true
      cancelAnimationFrame(raf)
      observer.disconnect()
      // 언마운트된 캔버스가 GC되기 전에도 버퍼를 바로 돌려준다.
      canvas.width = 0
      canvas.height = 0
    }
  }, [avoid, since])

  return (
    <div ref={rootRef} className="easter-egg" aria-hidden="true" onMouseDown={keepFocus}>
      <canvas ref={canvasRef} />
    </div>
  )
}
