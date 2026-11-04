// SPDX-License-Identifier: MIT
// 애드온 패키지 SDK 런타임. 호스트가 프레임 경로의 _maru/sdk.js로 주는 고전 스크립트이며 globalThis.maru를 둔다.
// 진입 HTML의 <head>에서 <script src="./_maru/sdk.js">로 싣는다. 의존성이 없다.
//
// - 실으면 바로 진입 주소의 theme·anim·view로 <html data-theme data-animations data-view>를 둔다(흰 화면 깜빡임 방지)
// - connect()가 호스트에 hello를 보내고 init을 기다린다. 그 뒤 요청은 id로 짝짓는다
// - 프레임 안의 포인터·키 입력을 호스트에 알린다(창 올리기, 세션 유지, 사용자 조작 판정)
import type { ActionOptions, ConfirmOptions, MaruApp, MaruContext, MaruError, MaruNamespace, NoticeLevel } from '../types/api'
import {
  type BridgeError,
  type FrameContext,
  type FrameMessage,
  PROTOCOL_VERSION,
  type RequestMethod,
  type RequestParams,
  SDK_LEVEL,
  interpolate,
  parseHostMessage,
} from './protocol'

// 요청마다 기다리는 시간. 조작은 백엔드 시간 제한(최대 600초)보다 조금 길다.
const TIMEOUTS: Partial<Record<RequestMethod, number>> = {
  action: 660_000,
  query: 660_000,
  confirm: 3_600_000,
}
const DEFAULT_TIMEOUT = 30_000
const CONNECT_TIMEOUT = 15_000

// applyLook은 문맥의 테마·애니메이션·보기·언어를 문서 루트에 둔다(maru.css가 이 속성을 본다).
function applyLook(ctx: Pick<FrameContext, 'theme' | 'animations' | 'view' | 'language'>) {
  const root = document.documentElement
  root.dataset.theme = ctx.theme
  root.style.colorScheme = ctx.theme
  if (ctx.animations) delete root.dataset.animations
  else root.dataset.animations = 'off'
  root.dataset.view = ctx.view
  if (ctx.language) root.lang = ctx.language
}

// 진입 주소의 값으로 먼저 둔다(init을 기다리지 않는다).
function applyInitialLook() {
  const q = new URLSearchParams(location.search)
  const theme = q.get('theme') === 'dark' ? 'dark' : q.get('theme') === 'light' ? 'light' : null
  const view = q.get('view') === 'widget' ? 'widget' : 'window'
  const root = document.documentElement
  if (theme) {
    root.dataset.theme = theme
    root.style.colorScheme = theme
  }
  if (q.get('anim') === '0') root.dataset.animations = 'off'
  root.dataset.view = view
}

class SdkError extends Error implements MaruError {
  readonly status: number
  readonly code?: string
  readonly addonCode?: string
  constructor(e: BridgeError) {
    super(e.message || e.code || 'request failed')
    this.name = 'MaruError'
    this.status = e.status
    this.code = e.code
    this.addonCode = e.addonCode
  }
}

interface Pending {
  resolve: (v: unknown) => void
  reject: (e: unknown) => void
  timer: ReturnType<typeof setTimeout>
}

let connecting: Promise<MaruApp> | null = null

function connect(): Promise<MaruApp> {
  if (connecting) return connecting
  connecting = new Promise<MaruApp>((resolve, reject) => {
    if (window.parent === window) {
      reject(new SdkError({ status: 0, code: 'bridge_closed', message: 'not running inside the Maru NAS dashboard' }))
      return
    }
    let ctx: FrameContext | null = null
    let nextId = 1
    const pending = new Map<number, Pending>()
    const listeners = new Set<(c: MaruContext) => void>()

    const post = (msg: FrameMessage) => window.parent.postMessage(msg, '*')

    const request = <M extends RequestMethod>(method: M, params: RequestParams[M]): Promise<unknown> =>
      new Promise((res, rej) => {
        const id = nextId++
        const timer = setTimeout(() => {
          pending.delete(id)
          rej(new SdkError({ status: 0, code: 'bridge_timeout', message: `${method} timed out` }))
        }, TIMEOUTS[method] ?? DEFAULT_TIMEOUT)
        pending.set(id, { resolve: res, reject: rej, timer })
        post({ maru: PROTOCOL_VERSION, type: 'request', id, method, params })
      })

    const app: MaruApp = {
      get context() {
        return ctx as MaruContext
      },
      on(event, listener) {
        if (event !== 'context') return () => {}
        listeners.add(listener)
        return () => listeners.delete(listener)
      },
      t(key, params) {
        const text = ctx?.messages[key]
        return text === undefined ? key : interpolate(text, params)
      },
      data: <T>(query?: Record<string, string>) => request('data', { query }) as Promise<T>,
      rpc: <T>(service: string, method: string, params?: unknown) => request('rpc', { service, method, params }) as Promise<T>,
      action: async (name: string, options: ActionOptions = {}) => {
        await request('action', { name, target: options.target, input: options.input })
      },
      query: <T>(name: string, options: ActionOptions = {}) =>
        request('query', { name, target: options.target, input: options.input }) as Promise<T>,
      storage: {
        get: <T>() => request('storage.get', {}) as Promise<T | null>,
        set: async (data: unknown) => {
          await request('storage.set', { data })
        },
      },
      openWorkbench: async (route: string) => {
        await request('openWorkbench', { route })
      },
      confirm: (options: ConfirmOptions) => request('confirm', { ...options }) as Promise<boolean>,
      notify: async (level: NoticeLevel, key: string, params?: Record<string, string | number>) => {
        await request('notify', { level, key, params })
      },
    }

    const fail = setTimeout(
      () => reject(new SdkError({ status: 0, code: 'bridge_timeout', message: 'the dashboard did not answer' })),
      CONNECT_TIMEOUT,
    )

    window.addEventListener('message', (e: MessageEvent) => {
      if (e.source !== window.parent) return
      const msg = parseHostMessage(e.data)
      if (!msg) return
      switch (msg.type) {
        case 'init':
          if (ctx) return
          ctx = msg.context
          applyLook(ctx)
          clearTimeout(fail)
          resolve(app)
          return
        case 'context':
          if (!ctx) return
          ctx = { ...ctx, ...msg.context }
          applyLook(ctx)
          for (const l of listeners) l(ctx as MaruContext)
          return
        case 'response': {
          const p = pending.get(msg.id)
          if (!p) return
          pending.delete(msg.id)
          clearTimeout(p.timer)
          if (msg.ok) p.resolve(msg.result)
          else p.reject(new SdkError(msg.error))
          return
        }
      }
    })

    // 프레임 안의 입력은 호스트 문서까지 가지 않으므로 알린다. 포인터는 누를 때마다(창 올리기·사용자 조작 판정),
    // 키·휠은 1초에 한 번(세션 유지)이다.
    let lastKey = 0
    document.addEventListener(
      'pointerdown',
      () => post({ maru: PROTOCOL_VERSION, type: 'input', kind: 'pointer' }),
      { capture: true, passive: true },
    )
    const onKey = (e: Event) => {
      const now = Date.now()
      // Enter·Space는 단추를 누르는 입력이라 사용자 조작 판정에 바로 쓰이게 늘 보낸다.
      const press = e instanceof KeyboardEvent && (e.key === 'Enter' || e.key === ' ')
      if (!press && now - lastKey < 1000) return
      lastKey = now
      post({ maru: PROTOCOL_VERSION, type: 'input', kind: 'key' })
    }
    document.addEventListener('keydown', onKey, { capture: true, passive: true })
    document.addEventListener('wheel', onKey, { capture: true, passive: true })

    post({ maru: PROTOCOL_VERSION, type: 'hello', sdk: SDK_LEVEL })
  })
  return connecting
}

const api = { connect, sdkLevel: SDK_LEVEL } satisfies MaruNamespace

applyInitialLook()
Object.defineProperty(globalThis, 'maru', { value: Object.freeze(api), writable: false, configurable: false })
