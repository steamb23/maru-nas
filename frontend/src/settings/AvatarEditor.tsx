// 프로필 사진 자르기 편집기. 정사각 보기 영역에 사진을 그리고 원 밖은 어둡게 덮는다. 끌거나 화살표 키로 위치를,
// 슬라이더·휠·+/- 키로 확대를 바꾼다. 저장하면 보이는 부분을 512px 정사각형으로 그려(WebP, 지원하지 않으면 PNG)
// 올린다. 계산은 profile/avatarCrop.ts에 있다.
import type { ParseKeys } from 'i18next'
import { type KeyboardEvent, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { uploadAvatarImage } from '../api/client'
import type { ImageInfo } from '../api/types.gen'
import {
  AVATAR_OUTPUT_SIZE,
  type Crop,
  INITIAL_CROP,
  MAX_ZOOM,
  MIN_ZOOM,
  moveBy,
  sourceRect,
  zoomTo,
} from '../profile/avatarCrop'
import { useAvatarStore } from '../profile/store'
import { usePointerDrag } from '../shell/usePointerDrag'
import { uploadErrorKey } from './imageUpload'

// 보기 영역의 한 변(px). AvatarEditor.css의 .avatar-editor-view와 맞춘다.
export const AVATAR_EDITOR_VIEW = 256
const VIEW = AVATAR_EDITOR_VIEW
// 키보드로 한 번에 옮기는 거리(px)와 확대 배율
const KEY_STEP = 8
const ZOOM_STEP = 1.1

// toBlob은 캔버스를 WebP로, 브라우저가 WebP를 만들지 못하면 PNG로 바꾼다.
async function toBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  const encode = (type: string) => new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, type, 0.9))
  const webp = await encode('image/webp')
  if (webp?.type === 'image/webp') return webp
  const png = await encode('image/png')
  if (!png) throw new Error('canvas encode failed')
  return png
}

// draw는 보기 영역에 보이는 부분을 canvas 전체에 그린다.
function draw(canvas: HTMLCanvasElement, bitmap: ImageBitmap, crop: Crop) {
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const r = sourceRect({ width: bitmap.width, height: bitmap.height }, VIEW, crop)
  ctx.imageSmoothingQuality = 'high'
  ctx.clearRect(0, 0, canvas.width, canvas.height)
  ctx.drawImage(bitmap, r.x, r.y, r.width, r.height, 0, 0, canvas.width, canvas.height)
}

export function AvatarEditor({ bitmap, onDone }: { bitmap: ImageBitmap; onDone: (info: ImageInfo | null) => void }) {
  const { t } = useTranslation()
  const img = { width: bitmap.width, height: bitmap.height }
  const [crop, setCrop] = useState<Crop>(INITIAL_CROP)
  const [busy, setBusy] = useState(false)
  const [errorKey, setErrorKey] = useState<ParseKeys | null>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const viewRef = useRef<HTMLDivElement>(null)
  // 끌기를 시작할 때의 자르기. 끄는 동안은 여기서부터 옮긴다.
  const startCrop = useRef(crop)
  // 휠 처리기(네이티브 리스너)가 지금 자르기를 읽는다.
  const cropRef = useRef(crop)
  useLayoutEffect(() => {
    cropRef.current = crop
  })

  useLayoutEffect(() => {
    const c = canvasRef.current
    if (!c) return
    const side = Math.round(VIEW * window.devicePixelRatio)
    if (c.width !== side) c.width = c.height = side
    draw(c, bitmap, crop)
  }, [bitmap, crop])

  // 휠로 확대한다. 대화상자 본문이 스크롤되지 않게 기본 동작을 막아야 하므로 passive가 아닌 리스너로 단다.
  useEffect(() => {
    const el = viewRef.current
    if (!el) return
    const size = { width: bitmap.width, height: bitmap.height }
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      const z = cropRef.current.zoom * (e.deltaY < 0 ? ZOOM_STEP : 1 / ZOOM_STEP)
      setCrop(zoomTo(size, VIEW, cropRef.current, z))
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [bitmap])

  const drag = usePointerDrag({
    threshold: 0,
    onStart: () => {
      if (busy) return false
      startCrop.current = cropRef.current
    },
    onMove: ({ dx, dy }) => setCrop(moveBy(img, VIEW, startCrop.current, dx, dy)),
    onEnd: () => {},
  })

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const moves: Record<string, [number, number]> = {
      ArrowLeft: [-KEY_STEP, 0],
      ArrowRight: [KEY_STEP, 0],
      ArrowUp: [0, -KEY_STEP],
      ArrowDown: [0, KEY_STEP],
    }
    if (moves[e.key]) {
      e.preventDefault()
      setCrop(moveBy(img, VIEW, crop, ...moves[e.key]))
    } else if (e.key === '+' || e.key === '=') {
      e.preventDefault()
      setCrop(zoomTo(img, VIEW, crop, crop.zoom * ZOOM_STEP))
    } else if (e.key === '-') {
      e.preventDefault()
      setCrop(zoomTo(img, VIEW, crop, crop.zoom / ZOOM_STEP))
    }
  }

  // save는 보이는 부분을 그려 올린다. 대화상자가 그사이 닫혀도 올린 사진이 바로 보이게 저장소를 여기서 바꾼다.
  const save = async () => {
    setBusy(true)
    setErrorKey(null)
    try {
      const out = document.createElement('canvas')
      out.width = out.height = AVATAR_OUTPUT_SIZE
      draw(out, bitmap, crop)
      const info = await uploadAvatarImage(await toBlob(out))
      useAvatarStore.getState().uploaded(info)
      onDone(info)
    } catch (err) {
      setErrorKey(uploadErrorKey(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="avatar-editor">
      <div
        ref={viewRef}
        className="avatar-editor-view"
        tabIndex={0}
        role="group"
        aria-label={t('settings.profile.editor.view')}
        onKeyDown={onKeyDown}
        {...drag}
      >
        <canvas ref={canvasRef} className="avatar-editor-canvas" />
        <span className="avatar-editor-mask" aria-hidden="true" />
      </div>
      <label className="avatar-editor-zoom">
        <span>{t('settings.profile.editor.zoom')}</span>
        <input
          type="range"
          min={MIN_ZOOM}
          max={MAX_ZOOM}
          step={0.01}
          value={crop.zoom}
          disabled={busy}
          onChange={(e) => setCrop(zoomTo(img, VIEW, crop, Number(e.target.value)))}
        />
      </label>
      <p className={errorKey ? 'settings-hint error' : 'settings-hint'} role={errorKey ? 'alert' : undefined}>
        {errorKey ? t(errorKey) : t('settings.profile.editor.hint')}
      </p>
      <div className="modal-buttons">
        <button type="button" className="addon-button" disabled={busy} onClick={() => onDone(null)}>
          {t('dialog.cancel')}
        </button>
        <button type="button" className="addon-button primary" disabled={busy} onClick={() => void save()}>
          {busy ? t('settings.image.uploading') : t('settings.profile.editor.save')}
        </button>
      </div>
    </div>
  )
}
