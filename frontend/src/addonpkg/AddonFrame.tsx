// AddonFrame은 애드온 패키지의 창·위젯 보기를 샌드박스 프레임으로 그리고 브리지를 잇는다.
//
// - sandbox="allow-scripts"만 준다(allow-same-origin 없음). 프레임은 불투명 출처라 대시보드·OMV의 쿠키와 저장소에
//   닿지 못한다. 파일은 세션·패키지 토큰이 든 경로(uiBase)로 받는다
// - 메시지는 event.source가 이 프레임일 때만 받고, 애드온 id는 메시지가 아니라 이 컴포넌트의 id로 정한다
// - 프레임이 스스로 다른 문서로 이동하면(두 번째 load) 브리지를 끊고 프레임을 새로 만든다
// - 프레임 안의 입력은 호스트 문서까지 오지 않으므로, SDK가 알린 입력을 iframe 요소의 합성 pointerdown(창 올리기,
//   바깥 누르기로 닫는 메뉴, 대기 판정)과 활동 신호(세션 유지)로 바꾼다
import {
  type FrameContext,
  type HostMessage,
  PROTOCOL_VERSION,
  SDK_LEVEL,
  type Theme,
  type ViewKind,
  parseFrameMessage,
} from '@maru-sdk/protocol'
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { fetchAddonMessages } from '../api/client'
import type { GridSize } from '../api/types.gen'
import { reportActivity } from '../auth/activity'
import { useAuthStore } from '../auth/store'
import { useLanguage } from '../i18n'
import { selectActiveId, useWindowStore } from '../windows/store'
import { BridgeLimits, admit, gestureAllowed, parseParams } from './bridge'
import { type PackageAppId, localizedText, usePackageCatalog } from './catalog'
import { BridgeFailure, handleRequest, toBridgeError } from './hostServices'

interface Props {
  id: PackageAppId
  view: ViewKind
  active: boolean
  // windowId는 창 보기의 창이다(사용자 조작 판정, 확인 대화상자).
  windowId?: string
  // grid는 위젯 보기의 칸 수다.
  grid?: GridSize
}

// messagesCache는 패키지 번역의 캐시다(id·rev·언어). 탭이 살아 있는 동안 둔다.
const messagesCache = new Map<string, Promise<Record<string, string>>>()

function loadMessages(id: string, rev: string, lang: string): Promise<Record<string, string>> {
  const key = `${id}\u0000${rev}\u0000${lang}`
  let p = messagesCache.get(key)
  if (!p) {
    p = fetchAddonMessages(id, lang).catch(() => {
      messagesCache.delete(key)
      return {}
    })
    messagesCache.set(key, p)
  }
  return p
}

// readTheme은 지금 호스트 테마다(<html data-theme>, 없으면 시스템 설정).
function readTheme(): Theme {
  const set = document.documentElement.dataset.theme
  if (set === 'light' || set === 'dark') return set
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

// readAnimations는 애니메이션을 켜도 되는지다(화면 설정과 시스템의 동작 줄이기).
function readAnimations(): boolean {
  return (
    document.documentElement.dataset.animations !== 'off' && !window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

// useHostLook은 호스트의 테마·애니메이션 설정을 따라간다.
function useHostLook(): { theme: Theme; animations: boolean } {
  const [look, setLook] = useState(() => ({ theme: readTheme(), animations: readAnimations() }))
  useEffect(() => {
    const update = () => setLook({ theme: readTheme(), animations: readAnimations() })
    const mo = new MutationObserver(update)
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'data-animations'] })
    const queries = [window.matchMedia('(prefers-color-scheme: dark)'), window.matchMedia('(prefers-reduced-motion: reduce)')]
    for (const q of queries) q.addEventListener('change', update)
    return () => {
      mo.disconnect()
      for (const q of queries) q.removeEventListener('change', update)
    }
  }, [])
  return look
}

export function AddonFrame({ id, view, active, windowId, grid }: Props) {
  const { t } = useTranslation()
  const pkg = usePackageCatalog((s) => s.packages.find((p) => p.id === id))
  const lang = useLanguage()
  const admin = useAuthStore((s) => s.user?.admin ?? false)
  const look = useHostLook()
  const iframeRef = useRef<HTMLIFrameElement>(null)
  // reloadKey가 바뀌면 프레임을 새로 만든다(스스로 이동한 프레임, 다시 불러오기).
  const [reloadKey, setReloadKey] = useState(0)
  const [size, setSize] = useState({ width: 0, height: 0 })
  // loaded는 받은 번역과 그 키(id·rev·언어)다. 키가 지금과 다르면 아직 받는 중이다.
  const [loaded, setLoaded] = useState<{ key: string; messages: Record<string, string> } | null>(null)

  const entry = pkg ? (view === 'window' ? pkg.views.window?.entry : pkg.views.widget?.entry) : undefined
  const uiBase = pkg?.uiBase
  const pkgId = pkg?.id
  const rev = pkg?.rev
  // 테마는 처음 그릴 때만 주소에 넣는다(그 뒤 바뀌면 context로 알린다. 주소가 바뀌면 프레임이 다시 뜬다).
  const [initialLook] = useState(look)
  const src = useMemo(() => {
    if (!uiBase || !entry) return null
    const url = new URL(uiBase + entry, document.baseURI)
    url.searchParams.set('theme', initialLook.theme)
    url.searchParams.set('anim', initialLook.animations ? '1' : '0')
    url.searchParams.set('view', view)
    return url.href
  }, [uiBase, entry, initialLook, view])

  const messagesKey = pkgId && rev ? `${pkgId}/${rev}/${lang}` : ''
  useEffect(() => {
    if (!pkgId || !rev) return
    let live = true
    loadMessages(pkgId, rev, lang).then((m) => live && setLoaded({ key: `${pkgId}/${rev}/${lang}`, messages: m }))
    return () => {
      live = false
    }
  }, [pkgId, rev, lang])
  const messages = loaded && loaded.key === messagesKey ? loaded.messages : null

  // 지금 문맥. 메시지 처리기는 ref로 최신 값을 읽는다(처리기를 다시 걸지 않게).
  const context: FrameContext | null = pkg
    ? {
        addonId: pkg.id,
        view,
        active,
        size: { ...size, cols: grid?.cols, rows: grid?.rows },
        theme: look.theme,
        animations: look.animations,
        language: lang,
        messages: messages ?? {},
        admin,
        sdkLevel: SDK_LEVEL,
      }
    : null
  const contextRef = useRef(context)
  const pkgRef = useRef(pkg)
  useLayoutEffect(() => {
    contextRef.current = context
    pkgRef.current = pkg
  })

  // 브리지 상태(프레임 하나마다, 다시 만들면 새로).
  const state = useRef({ ready: false, loads: 0, lastInput: null as number | null, limits: new BridgeLimits() })

  const post = useCallback((msg: HostMessage) => {
    // 불투명 출처라 targetOrigin을 정할 수 없다. 받는 쪽은 이 프레임의 contentWindow뿐이다.
    iframeRef.current?.contentWindow?.postMessage(msg, '*')
  }, [])

  // 문맥이 바뀌면(크기, 활성, 테마, 언어) 알린다.
  const sentRef = useRef<string>('')
  useEffect(() => {
    if (!context || !state.current.ready || messages === null) return
    const json = JSON.stringify(context)
    if (json === sentRef.current) return
    sentRef.current = json
    post({ maru: PROTOCOL_VERSION, type: 'context', context })
  })

  // 프레임 크기.
  useEffect(() => {
    const el = iframeRef.current
    if (!el) return
    const ro = new ResizeObserver(() => setSize({ width: Math.round(el.clientWidth), height: Math.round(el.clientHeight) }))
    ro.observe(el)
    return () => ro.disconnect()
  }, [reloadKey, src])

  // 입력 전달: iframe 요소에 합성 pointerdown을 보내 창 올리기와 바깥 누르기 처리를 그대로 쓴다.
  const relayPointer = useCallback(() => {
    const el = iframeRef.current
    if (!el) return
    el.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, cancelable: true, pointerType: 'mouse' }))
  }, [])

  useEffect(() => {
    const el = iframeRef.current
    if (!el) return
    // 프레임으로 포커스가 들어오면(다른 프레임에서 옮겨 와도) 창을 앞으로 가져온다.
    const onFocus = () => relayPointer()
    el.addEventListener('focus', onFocus)
    return () => el.removeEventListener('focus', onFocus)
  }, [relayPointer, reloadKey, src])

  useEffect(() => {
    state.current = { ready: false, loads: 0, lastInput: null, limits: new BridgeLimits() }
    sentRef.current = ''
    const onMessage = (e: MessageEvent) => {
      const el = iframeRef.current
      if (!el || e.source !== el.contentWindow) return
      const msg = parseFrameMessage(e.data)
      if (!msg) return
      const st = state.current
      if (msg.type === 'input') {
        st.lastInput = Date.now()
        if (msg.kind === 'pointer') relayPointer()
        else reportActivity()
        return
      }
      if (msg.type === 'hello') {
        const ctx = contextRef.current
        if (!ctx) return
        // 번역을 받은 뒤 init을 보낸다(오래 걸리면 빈 표로 먼저 보낸다).
        const send = (m: Record<string, string>) => {
          const cur = contextRef.current
          if (!cur) return
          const full = { ...cur, messages: m }
          st.ready = true
          sentRef.current = JSON.stringify(full)
          post({ maru: PROTOCOL_VERSION, type: 'init', context: full })
        }
        const p = pkgRef.current
        if (!p) return
        let sent = false
        const timer = window.setTimeout(() => {
          sent = true
          send(contextRef.current?.messages ?? {})
        }, 3000)
        loadMessages(p.id, p.rev, ctx.language).then((m) => {
          window.clearTimeout(timer)
          if (!sent) send(m)
        })
        return
      }
      // request
      const p = pkgRef.current
      const ctx = contextRef.current
      if (!p || !ctx || !st.ready) return
      const reply = (ok: boolean, payload: unknown) =>
        post(
          ok
            ? { maru: PROTOCOL_VERSION, type: 'response', id: msg.id, ok: true, result: payload }
            : { maru: PROTOCOL_VERSION, type: 'response', id: msg.id, ok: false, error: toBridgeError(payload) },
        )
      let bytes = 0
      try {
        bytes = JSON.stringify(e.data).length
      } catch {
        bytes = Infinity
      }
      const gesture = () =>
        gestureAllowed({
          activation: navigator.userActivation?.isActive ?? true,
          focused: document.activeElement === el,
          front: view === 'window' && !!windowId && selectActiveId(useWindowStore.getState()) === windowId && !el.closest('[inert]'),
          lastInput: st.lastInput,
          now: Date.now(),
        })
      const rejected = admit(msg.method, view, bytes, gesture)
      if (rejected) {
        reply(false, new BridgeFailure({ status: 0, code: rejected, message: `request ${msg.method} rejected` }))
        return
      }
      const params = parseParams(msg.method, msg.params)
      if (!params) {
        reply(false, new BridgeFailure({ status: 0, code: 'bridge_invalid', message: `invalid ${msg.method} params` }))
        return
      }
      const limited = st.limits.start(msg.method, Date.now())
      if (limited) {
        reply(false, new BridgeFailure({ status: 0, code: limited, message: `request ${msg.method} rejected` }))
        return
      }
      const limits = st.limits
      handleRequest(msg.method, params, {
        pkg: p,
        scope: view === 'window' && windowId ? { kind: 'window', windowId } : null,
        language: ctx.language,
        messages: ctx.messages,
      })
        .then(
          (result) => state.current.limits === limits && reply(true, result),
          (err: unknown) => state.current.limits === limits && reply(false, err),
        )
        .finally(() => limits.done())
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [post, relayPointer, view, windowId, reloadKey, src])

  const onLoad = () => {
    const st = state.current
    st.loads++
    // 두 번째 load는 프레임이 스스로 다른 문서로 이동한 것이다. 브리지를 끊고 처음 문서로 다시 만든다.
    if (st.loads > 1) setReloadKey((k) => k + 1)
  }

  if (!pkg || !src) {
    return (
      <div className="addonpkg-unavailable" role="status">
        {t('addonPackages.unavailable')}
      </div>
    )
  }
  const name = localizedText(pkg.name, lang)
  return (
    <iframe
      key={`${src}#${reloadKey}`}
      ref={iframeRef}
      className={`addonpkg-frame addonpkg-${view}`}
      src={src}
      title={t('addonPackages.frameTitle', { name })}
      sandbox="allow-scripts"
      allow=""
      referrerPolicy="no-referrer"
      loading="eager"
      // 위젯은 틀이 끌기·크기 조절·창 열기를 맡으므로 프레임은 입력을 받지 않는다(Tab도 들어가지 않는다).
      inert={view === 'widget'}
      tabIndex={view === 'widget' ? -1 : 0}
      style={{ colorScheme: look.theme }}
      onLoad={onLoad}
    />
  )
}
