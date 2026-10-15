// 배경 레이어. 로그인 화면과 데스크톱 모두 이 레이어 위에 그린다(App이 화면과 관계없이 하나 둔다).
//
// 배경이 바뀌면 새 배경을 이전 배경 위에 겹쳐 투명도를 올리는 페이드로 바꾼다. 이미지는 다 받아 디코드한 뒤에
// 페이드를 시작해 빈 화면이나 그려지는 중인 그림이 비치지 않게 하고, 받지 못하면 기본 배경으로 바꾼다.
// 페이드 중에 또 바뀌면 지금 보이는 레이어들 위에 새 레이어를 겹쳐 이어 간다. 애니메이션이 꺼져 있으면 바로 바꾼다.
// 어느 레이어를 남길지는 appearance/wallpaperLayers.ts가 정한다.
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { WALLPAPER_FADE_EASING, WALLPAPER_FADE_MS } from '../appearance/wallpaperAnim'
import { type WallpaperLayer, initialLayers, pushLayer } from '../appearance/wallpaperLayers'
import { DEFAULT_VIEW, type WallpaperView, cssBackground } from '../appearance/wallpapers'
import { animationsEnabled } from './useWindowAnimation'

// decoded는 이미지 배경이면 다 받아 디코드할 때까지 기다린다. 받지 못하면 기본 배경이다.
async function decoded(view: WallpaperView): Promise<WallpaperView> {
  if (view.kind !== 'image') return view
  const img = new Image()
  img.src = view.url
  try {
    await img.decode()
    return view
  } catch {
    return DEFAULT_VIEW
  }
}

// view가 null이면 지금 배경을 그대로 둔다.
export function Wallpaper({ view }: { view: WallpaperView | null }) {
  const [layers, setLayers] = useState<readonly WallpaperLayer[]>(() => initialLayers(view ?? DEFAULT_VIEW))
  const nextId = useRef(1)
  // 가장 최근에 요청한 배경의 key. 이미지를 받는 사이에 또 바뀌면 늦게 끝난 쪽을 버린다.
  // 처음에는 비워 두어 첫 레이어(확인하지 않고 그린 힌트)도 받아 확인하게 한다.
  const wanted = useRef<string | null>(null)
  // 새로 올린 레이어의 페이드를 시작할 요소
  const entering = useRef<HTMLDivElement | null>(null)
  const top = layers[layers.length - 1]

  // view는 같은 배경이어도 그릴 때마다 새 객체일 수 있다(힌트는 매번 새로 읽는다). 받는 중인 이미지는 정리 함수로
  // 버리지 않고 key로만 버린다. 정리 함수로 버리면 받는 사이에 다시 그려질 때 그 배경을 잃고, 같은 key라 다시 받지도 않는다.
  useEffect(() => {
    if (!view || view.key === wanted.current) return
    wanted.current = view.key
    void decoded(view).then((ready) => {
      if (wanted.current !== view.key) return
      setLayers((ls) => pushLayer(ls, ready, nextId.current++, animationsEnabled()))
    })
  }, [view])

  // 새 레이어를 투명한 상태에서 올리고, 끝나면 아래 레이어를 지운다.
  useLayoutEffect(() => {
    const el = entering.current
    if (!el || layers.length < 2) return
    const anim = el.animate([{ opacity: 0 }, { opacity: 1 }], {
      duration: WALLPAPER_FADE_MS,
      easing: WALLPAPER_FADE_EASING,
    })
    const id = top.id
    anim.onfinish = () => setLayers((ls) => (ls[ls.length - 1].id === id ? ls.slice(-1) : ls))
  }, [top.id, layers.length])

  return (
    <div className="wallpaper" aria-hidden="true">
      {layers.map((l) => (
        <div
          key={l.id}
          ref={l.id === top.id ? entering : undefined}
          className="wallpaper-layer"
          style={{ background: cssBackground(l.view) }}
        />
      ))}
    </div>
  )
}
