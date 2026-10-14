// 범용 모달 대화상자. 범위(창·데스크톱)의 호스트에 그리고, 열려 있는 동안 그 범위를 inert로 막는다.
//
// - 창 범위: 창 본문(.window-body)만 막는다. 제목 표시줄(끌기·최소화·닫기)과 크기 조절, 다른 창은 쓸 수 있다
// - 데스크톱 범위: 창·위젯·아이콘·작업표시줄을 모두 막는다
// 틀(범위·차단·포커스·애니메이션·쌓임)만 여기서 다루고, 내용·단추·모양은 쓰는 쪽이 정한다.
// 명령형으로 여는 방법은 dialogs.ts, 확인·알림 같은 기본 형태는 dialogPresets.tsx에 있다.
import {
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
  type RefObject,
  type SyntheticEvent,
  memo,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import { createPortal } from 'react-dom'
import {
  BACKDROP_HIDDEN,
  BACKDROP_SHOWN,
  DIALOG_CLOSE_MS,
  DIALOG_CLOSE_TO,
  DIALOG_OPEN_FROM,
  DIALOG_OPEN_MS,
  DIALOG_SHOWN,
} from './dialogAnim'
import { type ModalScope, useModalLayer } from './modalScope'
import { isUnder } from './modalStack'
import { animationsEnabled } from './useWindowAnimation'
import { CLOSE_EASING, EASING } from './windowAnim'

export interface ModalSize {
  width?: number | string
  maxWidth?: number | string
  height?: number | string
  maxHeight?: number | string
}

// ModalOptions는 열림 상태와 내용을 뺀 모양·동작 설정이다. 명령형 대화상자(dialogs.ts)도 같은 설정을 받는다.
export interface ModalOptions {
  // 기본: 창 보기 안에서는 그 창, 밖에서는 데스크톱
  scope?: ModalScope
  // 머리말. 있으면 aria-labelledby로 이어지고, 없으면 aria-label을 준다.
  title?: ReactNode
  'aria-label'?: string
  // 단추 줄. 없으면 그리지 않는다.
  footer?: ReactNode
  // Esc·배경 누르기로 onCancel을 부를지(기본 true). onCancel이 없으면 사용자가 닫을 수 없다.
  closeOnEscape?: boolean
  closeOnBackdrop?: boolean
  role?: 'dialog' | 'alertdialog'
  // 열 때 포커스할 요소. 없으면 첫 포커스 가능한 요소, 그것도 없으면 상자. 내용에서 autoFocus를 쓰지 말고
  // 이것을 쓴다(autoFocus는 Modal이 열기 전 포커스를 기억하기 전에 옮겨, 닫은 뒤 돌려줄 곳을 잃는다).
  initialFocus?: RefObject<HTMLElement | null>
  // 기본 너비 340px. 범위보다 커지지 않는다.
  size?: ModalSize
  // dim: 범위를 어둡게 덮는다. clear: 막기만 하고 어둡게 하지 않는다.
  backdrop?: 'dim' | 'clear'
  // 상자에 덧붙이는 클래스
  className?: string
  // 닫는 애니메이션까지 끝난 뒤 부른다.
  onClosed?: () => void
}

export interface ModalProps extends ModalOptions {
  open: boolean
  onCancel?: () => void
  // 본문. 길면 본문만 스크롤된다.
  children?: ReactNode
}

const DEFAULT_WIDTH = 340

const FOCUSABLE = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'

// focusables는 root 안에서 Tab으로 갈 수 있는 요소다(비활성·숨김·inert 제외).
function focusables(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (el) => !el.matches(':disabled') && !el.closest('[inert]') && el.getClientRects().length > 0,
  )
}

// 모달은 포털로 그리지만 React 이벤트는 React 트리를 따라 올라간다. 모달을 그린 쪽(행의 우클릭 메뉴,
// 길게 누르기, 끌기 등)이 모달 안의 입력을 받지 않도록 여기서 멈춘다. 문서에 건 capture 단계 처리기
// (세션 연장, 메뉴 바깥 누르기)는 영향을 받지 않는다.
const stop = (e: SyntheticEvent) => e.stopPropagation()

let nextKey = 1

export function Modal({
  open,
  onCancel,
  children,
  scope,
  title,
  footer,
  closeOnEscape = true,
  closeOnBackdrop = true,
  role = 'dialog',
  initialFocus,
  size,
  backdrop = 'dim',
  className,
  onClosed,
  'aria-label': ariaLabel,
}: ModalProps) {
  const layer = useModalLayer(scope)
  const host = layer?.host ?? null
  const [key] = useState(() => nextKey++)
  const idBase = useId()
  const backdropRef = useRef<HTMLDivElement>(null)
  const boxRef = useRef<HTMLDivElement>(null)
  const onClosedRef = useRef(onClosed)
  useLayoutEffect(() => {
    onClosedRef.current = onClosed
  })

  // 닫는 애니메이션 중이면 true. 그동안 상자는 마지막 내용을 그대로 둔다(ModalBox).
  const [closing, setClosing] = useState(false)
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    setClosing(!open && host !== null)
  }

  // 열려 있는 동안 범위의 스택에 올린다. 닫기 시작하면 바로 내려 범위의 차단을 푼다.
  const push = layer?.push
  const pop = layer?.pop
  useLayoutEffect(() => {
    if (!open || !push || !pop) return
    push(key)
    return () => pop(key)
  }, [open, push, pop, key])

  // 열기: 포커스를 옮기고 애니메이션을 재생한다. 닫히면(언마운트 포함) 포커스가 갈 곳을 잃었을 때
  // 열기 전 요소로 돌려준다. 범위의 inert가 풀린 뒤여야 하므로 다음 작업으로 미룬다.
  // 개발 모드의 StrictMode는 효과를 정리했다 곧바로 다시 실행하므로, 열기 전 요소는 처음 한 번만 기억하고
  // 미뤄 둔 복원 시점에 다시 열려 있으면 돌려주지 않는다.
  const ready = open && host !== null
  // 미뤄 둔 복원이 읽는 최신 상태. 렌더링에는 쓰지 않는다.
  const focusRef = useRef<{ mounted: boolean; ready: boolean; prev: HTMLElement | null }>({
    mounted: false,
    ready: false,
    prev: null,
  })
  useLayoutEffect(() => {
    const st = focusRef.current
    st.mounted = true
    return () => {
      st.mounted = false
    }
  }, [])
  useLayoutEffect(() => {
    focusRef.current.ready = ready
  })
  useLayoutEffect(() => {
    const box = boxRef.current
    const bd = backdropRef.current
    if (!ready || !box || !bd) return
    const st = focusRef.current
    // 이미 상자 안에 포커스가 있으면(다시 실행) 기억해 둔 요소를 그대로 둔다.
    const active = document.activeElement
    if (active instanceof HTMLElement && !box.contains(active)) st.prev = active
    if (!box.contains(document.activeElement)) (initialFocus?.current ?? focusables(box)[0] ?? box).focus()
    if (animationsEnabled()) {
      for (const el of [box, bd]) el.getAnimations().forEach((a) => a.cancel())
      box.animate([DIALOG_OPEN_FROM, DIALOG_SHOWN], { duration: DIALOG_OPEN_MS, easing: EASING })
      bd.animate([BACKDROP_HIDDEN, BACKDROP_SHOWN], { duration: DIALOG_OPEN_MS, easing: EASING })
    }
    return () => {
      window.setTimeout(() => {
        if (st.mounted && st.ready) return
        const prev = st.prev
        st.prev = null
        const active = document.activeElement
        const lost = !active || active === document.body || box.contains(active)
        if (lost && prev?.isConnected) prev.focus()
      }, 0)
    }
  }, [ready, initialFocus])

  // 닫기: 지금 보이는 상태에서 사라지는 애니메이션을 재생하고, 끝나면 그리기를 멈춘다.
  useLayoutEffect(() => {
    if (!closing) return
    const box = boxRef.current
    const bd = backdropRef.current
    const finish = () => {
      setClosing(false)
      onClosedRef.current?.()
    }
    if (!animationsEnabled() || !box || !bd) {
      finish()
      return
    }
    const boxStyle = getComputedStyle(box)
    const from: Keyframe = { opacity: boxStyle.opacity, transform: boxStyle.transform }
    const bdFrom: Keyframe = { opacity: getComputedStyle(bd).opacity }
    for (const el of [box, bd]) el.getAnimations().forEach((a) => a.cancel())
    const timing: KeyframeAnimationOptions = { duration: DIALOG_CLOSE_MS, easing: CLOSE_EASING, fill: 'forwards' }
    const anim = box.animate([from, DIALOG_CLOSE_TO], timing)
    bd.animate([bdFrom, BACKDROP_HIDDEN], timing)
    anim.onfinish = finish
    // 닫는 도중 다시 열리면 끝내지 않는다(열기 애니메이션이 이 애니메이션을 취소한다).
    return () => {
      anim.onfinish = null
    }
  }, [closing])

  if (!layer || !host || (!open && !closing)) return null

  // 닫는 중이거나 다른 모달 아래에 깔리면 입력을 받지 않는다.
  const under = isUnder(layer.stack, key)
  const inert = !open || under

  const onBackdropDown = (e: PointerEvent<HTMLDivElement>) => {
    e.stopPropagation()
    if (e.target === e.currentTarget && closeOnBackdrop && onCancel) onCancel()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    e.stopPropagation()
    if (e.key === 'Escape') {
      if (closeOnEscape && onCancel) {
        e.preventDefault()
        onCancel()
      }
      return
    }
    if (e.key !== 'Tab') return
    // 포커스를 상자 안에서 돌린다(창 범위에서는 다른 창이 막히지 않으므로 필요하다).
    const box = boxRef.current
    if (!box) return
    const items = focusables(box)
    if (items.length === 0) {
      e.preventDefault()
      box.focus()
      return
    }
    const first = items[0]
    const last = items[items.length - 1]
    const active = document.activeElement
    if (e.shiftKey && (active === first || active === box)) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && (active === last || !box.contains(active))) {
      e.preventDefault()
      first.focus()
    }
  }

  const boxStyle: CSSProperties = {
    width: size?.width ?? DEFAULT_WIDTH,
    maxWidth: size?.maxWidth,
    height: size?.height,
    maxHeight: size?.maxHeight,
  }

  return createPortal(
    <div
      ref={backdropRef}
      className={`modal-backdrop ${backdrop}${under ? ' under' : ''}`}
      inert={inert}
      onPointerDown={onBackdropDown}
      onPointerUp={stop}
      onPointerMove={stop}
      onPointerCancel={stop}
      onClick={stop}
      onDoubleClick={stop}
      onContextMenu={stop}
      onKeyDown={onKeyDown}
    >
      <ModalBox
        frozen={!open}
        boxRef={boxRef}
        idBase={idBase}
        role={role}
        ariaLabel={ariaLabel}
        className={className}
        style={boxStyle}
        title={title}
        footer={footer}
      >
        {children}
      </ModalBox>
    </div>,
    host,
  )
}

interface BoxProps {
  // 닫는 중이면 true. 다시 그리지 않아 마지막 내용이 남는다(닫히면 쓰는 쪽이 내용을 지울 수 있으므로).
  frozen: boolean
  boxRef: RefObject<HTMLDivElement | null>
  idBase: string
  role: 'dialog' | 'alertdialog'
  ariaLabel?: string
  className?: string
  style: CSSProperties
  title?: ReactNode
  footer?: ReactNode
  children?: ReactNode
}

const ModalBox = memo(
  function ModalBox({ boxRef, idBase, role, ariaLabel, className, style, title, footer, children }: BoxProps) {
    const titleId = `${idBase}-title`
    return (
      <div
        ref={boxRef}
        className={className ? `modal ${className}` : 'modal'}
        role={role}
        aria-modal="true"
        aria-labelledby={title ? titleId : undefined}
        aria-label={title ? undefined : ariaLabel}
        tabIndex={-1}
        style={style}
      >
        {title && (
          <h2 id={titleId} className="modal-title">
            {title}
          </h2>
        )}
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-buttons">{footer}</div>}
      </div>
    )
  },
  (_prev, next) => next.frozen,
)
