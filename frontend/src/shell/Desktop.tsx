// 데스크톱 셸: 바탕화면 아이콘, 창 레이어, 작업표시줄.
import { useLayoutEffect, useRef, useState } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { APP_IDS } from '../windows/apps'
import type { Size } from '../windows/geometry'
import { stableOrder, useWindowStore } from '../windows/store'
import { DesktopIcon } from './DesktopIcon'
import { Taskbar } from './Taskbar'
import { TileDivider } from './TileDivider'
import { Window } from './Window'

export function Desktop() {
  const areaRef = useRef<HTMLDivElement>(null)
  // 데스크톱 영역(작업표시줄 제외) 크기. 창 위치 제한의 기준이다.
  const [bounds, setBounds] = useState<Size>(() => ({ width: window.innerWidth, height: window.innerHeight }))
  // 창을 옮길 때마다 Desktop 전체가 다시 그려지지 않도록 id 목록만 구독한다.
  const ids = useWindowStore(useShallow((s) => s.windows.map((w) => w.id)))
  // 창은 처음 열린 순서대로 그린다. 앞뒤는 z-index가 정하므로, 작업표시줄에서 순서를 바꿔도
  // 창 DOM이 옮겨지지 않게 한다(옮겨지면 창 안의 스크롤 위치나 iframe이 초기화된다).
  const [order, setOrder] = useState<string[]>([])
  const renderIds = stableOrder(order, ids)
  if (renderIds.length !== order.length || renderIds.some((id, i) => id !== order[i])) setOrder(renderIds)

  useLayoutEffect(() => {
    const el = areaRef.current
    if (!el) return
    const measure = () => {
      const b = { width: el.clientWidth, height: el.clientHeight }
      setBounds(b)
      // 브라우저 창이 줄어 화면 밖으로 밀려난 창을 다시 끌어온다.
      useWindowStore.getState().clampAll(b)
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  return (
    <div className="desktop">
      <div className="desktop-area" ref={areaRef}>
        <ul className="desktop-icons" aria-label="바탕화면">
          {APP_IDS.map((appId) => (
            <li key={appId}>
              <DesktopIcon appId={appId} bounds={bounds} />
            </li>
          ))}
        </ul>
        <TileDivider bounds={bounds} />
        {renderIds.map((id) => (
          <Window key={id} id={id} bounds={bounds} />
        ))}
      </div>
      <Taskbar />
    </div>
  )
}
