// 로그인 대기 이스터에그 오버레이. 로그인 카드 뒤 배경을 어둡게 덮고 캔버스에 고른 효과를 그린다.
// 효과 계약은 effects.ts, 설정값은 config.ts.
//
// 연출 중에만 마운트하고, 해제하면 페이드아웃 뒤 언마운트해 캔버스 버퍼를 돌려준다.
// 오버레이는 포커스를 받지 않는다. 배경을 누른 포인터 입력은 오버레이가 받아 사라지는 동안에도 흡수하고,
// 카드는 오버레이 위에 있어 연출 중에도 평소처럼 쓸 수 있다.
import { type MouseEvent, type RefObject, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { Rect } from '../../windows/geometry'
import { EGG_FADE_IN_MS, EGG_FADE_OUT_MS, EGG_FPS } from './config'
import { EFFECTS, type EggEffect, pickEffect } from './effects'
import { createFrameGate } from './frameGate'

// mousedown의 기본 동작(포커스 이동)을 막아 입력란의 포커스를 유지한다.
const keepFocus = (e: MouseEvent) => e.preventDefault()

interface EasterEggProps {
  active: boolean
  // 효과가 피해서 그릴 로그인 카드
  avoid: RefObject<HTMLElement | null>
}

// EasterEgg는 active인 동안 오버레이를 보여 준다. active가 false가 되면 사라지는 애니메이션이 끝난 뒤 언마운트한다.
export function EasterEgg({ active, avoid }: EasterEggProps) {
  const [mounted, setMounted] = useState(false)
  const onLeft = useCallback(() => setMounted(false), [])
  if (active && !mounted) setMounted(true)
  if (!mounted) return null
  return <Overlay leaving={!active} onLeft={onLeft} avoid={avoid} />
}

function Overlay({ leaving, onLeft, avoid }: { leaving: boolean; onLeft: () => void; avoid: EasterEggProps['avoid'] }) {
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
    const from = first ? 0 : Number(getComputedStyle(el).opacity)
    el.getAnimations().forEach((a) => a.cancel())
    const to = leaving ? 0 : 1
    const duration = (leaving ? EGG_FADE_OUT_MS : EGG_FADE_IN_MS) * Math.abs(to - from)
    const anim = el.animate([{ opacity: from }, { opacity: to }], { duration, easing: 'ease-out', fill: 'forwards' })
    if (leaving) anim.onfinish = onLeft
  }, [leaving, onLeft])

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

    // 효과를 내려받은 뒤부터 시간이 흐른다. 탭이 숨겨지면 rAF가 멈추고 useIdle이 연출을 끈다.
    const gate = createFrameGate(EGG_FPS)
    let time = 0
    let raf = 0
    const frame = (now: number) => {
      raf = requestAnimationFrame(frame)
      const step = gate.tick(now)
      if (step === null || !effect) return
      time += step
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      effect.draw(ctx, time)
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
  }, [avoid])

  return (
    <div ref={rootRef} className="easter-egg" aria-hidden="true" onMouseDown={keepFocus}>
      <canvas ref={canvasRef} />
    </div>
  )
}
