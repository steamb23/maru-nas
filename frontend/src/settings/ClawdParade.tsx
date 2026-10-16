// 정보 페이지 대기 이스터에그. Claude Code에 대한 경의로, 정보 페이지를 띄워 둔 채 입력 없이 CLAWD_IDLE_MS가
// 지나면 Clawd가 페이지 영역의 왼쪽·오른쪽 가장자리에서 한 마리씩 기어 들어와 쉬거나 배회한다. 아무 곳이나 누르면
// 모두 놀라 점프한 뒤 가까운 가장자리로 도망친다. 움직임 계산은 about/clawdParade.ts, 그림은 about/clawd.ts.
//
// 레이어는 .settings-body(제목 머리 아래 페이지 영역)에 붙어 내용과 함께 스크롤되지 않고, 포인터를 받지 않는다.
// 창이 비활성이 되거나(최소화·닫는 중·탭 숨김) 애니메이션이 꺼져 있으면 연출 없이 바로 없앤다.
import { useCallback, useEffect, useRef, useState } from 'react'
import { useIdle } from '../shell/useIdle'
import { animationsEnabled } from '../shell/useWindowAnimation'
import { CLAWD_COLS, CLAWD_ROWS, clawdPaths } from './about/clawd'
import { CLAWD_HEIGHT, CLAWD_IDLE_MS, CLAWD_WIDTH, createParade } from './about/clawdParade'

// 프레임 사이가 이보다 길면(브라우저가 잠시 멈춘 경우) 이만큼만 진행해 한 번에 멀리 건너뛰지 않는다.
const MAX_STEP_MS = 100

interface ClawdParadeProps {
  // 창이 최소화되거나 닫히는 중이거나 탭이 보이지 않으면 false
  active: boolean
}

// ClawdParade는 대기 시간이 지나면 Clawd를 보여 준다. 입력으로 대기가 풀리면 흩어지게 하고, 모두 나가면 언마운트한다.
export function ClawdParade({ active }: ClawdParadeProps) {
  const enabled = active && animationsEnabled()
  const since = useIdle(enabled, CLAWD_IDLE_MS)
  // 흩어지는 동안에도 마지막 시작 시각으로 그린다.
  const [shown, setShown] = useState<number | null>(null)
  const onGone = useCallback(() => setShown(null), [])
  if (since !== null && since !== shown) setShown(since)
  else if (!enabled && shown !== null) setShown(null)
  if (shown === null || !enabled) return null
  return <Parade key={shown} scatter={since === null} onGone={onGone} />
}

interface ParadeProps {
  scatter: boolean
  onGone: () => void
}

function Parade({ scatter, onGone }: ParadeProps) {
  const layerRef = useRef<HTMLDivElement>(null)
  const [parade] = useState(() => createParade(Math.random))
  // 그릴 Clawd의 id. 등장·퇴장할 때만 바뀌고, 위치와 자세는 프레임마다 요소에 직접 쓴다.
  const [ids, setIds] = useState<readonly number[]>([])
  const elements = useRef(new Map<number, HTMLDivElement>())

  useEffect(() => {
    if (scatter) parade.scatter()
  }, [parade, scatter])

  useEffect(() => {
    const layer = layerRef.current
    if (!layer) return
    const resize = () => parade.resize(layer.clientWidth, layer.clientHeight)
    resize()
    const observer = new ResizeObserver(resize)
    observer.observe(layer)

    let raf = 0
    let last = performance.now()
    let shownKey = ''
    const frame = (now: number) => {
      parade.step(Math.min(MAX_STEP_MS, Math.max(0, now - last)))
      last = now
      if (parade.done) {
        onGone()
        return
      }
      const key = parade.clawds.map((c) => c.id).join(',')
      if (key !== shownKey) {
        shownKey = key
        setIds(parade.clawds.map((c) => c.id))
      }
      for (const c of parade.clawds) {
        // 처음 그려지기 전(다음 렌더 전)에는 요소가 없다. 다음 프레임에 자리를 잡는다.
        const el = elements.current.get(c.id)
        if (!el) continue
        // 정수 px로 옮겨 도트가 흐려지지 않게 한다. 아래쪽 줄일수록 앞에 그린다.
        el.style.transform = `translate(${Math.round(c.x)}px, ${Math.round(c.y)}px)`
        el.style.zIndex = String(Math.round(c.lane))
        if (el.dataset.pose !== c.pose) {
          // 자세가 처음 정해지면 CSS가 요소를 보이게 한다(.clawd[data-pose]).
          el.dataset.pose = c.pose
          const { body, eyes } = clawdPaths(c.pose)
          el.querySelector('.clawd-body')?.setAttribute('d', body)
          el.querySelector('.clawd-eyes')?.setAttribute('d', eyes)
        }
      }
      raf = requestAnimationFrame(frame)
    }
    raf = requestAnimationFrame(frame)

    return () => {
      cancelAnimationFrame(raf)
      observer.disconnect()
    }
  }, [parade, onGone])

  return (
    <div ref={layerRef} className="clawd-layer" aria-hidden="true">
      {ids.map((id) => (
        <div
          key={id}
          className="clawd"
          // 위치(transform)·z-index는 프레임마다 직접 쓰므로 여기에는 크기만 둔다.
          style={{ width: CLAWD_WIDTH, height: CLAWD_HEIGHT }}
          ref={(el) => {
            if (!el) return
            elements.current.set(id, el)
            return () => {
              elements.current.delete(id)
            }
          }}
        >
          <svg viewBox={`0 0 ${CLAWD_COLS} ${CLAWD_ROWS}`}>
            <path className="clawd-body" />
            <path className="clawd-eyes" />
          </svg>
        </div>
      ))}
    </div>
  )
}
