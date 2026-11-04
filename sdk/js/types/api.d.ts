// SPDX-License-Identifier: MIT
// 마루 NAS 애드온 패키지 SDK의 공개 API 타입. 런타임은 호스트가 프레임에 주는 _maru/sdk.js이고(globalThis.maru), 이 파일은
// 타입만 담는다. 런타임(sdk/js/src/client.ts)이 satisfies로 이 형태를 맞춘다.

export type ViewKind = 'window' | 'widget'
export type Theme = 'light' | 'dark'
export type NoticeLevel = 'info' | 'success' | 'warning' | 'error'

/** 호스트가 주는 문맥. 바뀌면 'context' 이벤트가 온다. */
export interface MaruContext {
  readonly addonId: string
  readonly view: ViewKind
  /** 보기를 그려야 하는지(창이 최소화되거나 탭이 숨겨지면 false). false면 주기 요청을 멈춘다. */
  readonly active: boolean
  /** 프레임 크기(px). 위젯이면 칸 수(cols, rows)도 있다. */
  readonly size: { readonly width: number; readonly height: number; readonly cols?: number; readonly rows?: number }
  readonly theme: Theme
  readonly animations: boolean
  /** 화면 언어(BCP 47, 예: ko, en, zh-Hans). */
  readonly language: string
  /** 패키지 번역 표(i18n/<언어>.json 위에 en). */
  readonly messages: Readonly<Record<string, string>>
  /** 로그인한 사용자가 OMV 관리자인지. */
  readonly admin: boolean
  /** 호스트의 SDK 수준. */
  readonly sdkLevel: number
}

/** 요청이 실패한 이유. status는 대시보드 API의 HTTP 상태(호스트가 거절하면 0)다. */
export interface MaruErrorInfo {
  readonly status: number
  /** 대시보드 오류 코드(rpc_forbidden 등) 또는 호스트의 거절 코드(bridge_*). */
  readonly code?: string
  /** 패키지 백엔드가 준 코드(addon_error일 때). */
  readonly addonCode?: string
}

export interface MaruError extends Error, MaruErrorInfo {}

export interface ActionOptions {
  /** 조작 대상(백엔드가 검사한다). */
  target?: string
  /** 조작 입력(JSON 객체, 8KiB까지). */
  input?: Record<string, unknown>
}

export interface ConfirmOptions {
  title?: string
  message: string
  confirmLabel?: string
  danger?: boolean
}

export interface MaruApp {
  /** 지금 문맥. */
  readonly context: MaruContext
  /** 문맥이 바뀌면 부른다(크기, 활성, 테마, 언어). 해제 함수를 돌려준다. */
  on(event: 'context', listener: (context: MaruContext) => void): () => void
  /** 패키지 번역 문구. {{name}}을 params로 바꾼다. 없는 키는 키 그대로다. */
  t(key: string, params?: Record<string, string | number>): string
  /** 패키지 백엔드의 데이터(GET /v1/data). 백엔드가 없는 패키지는 addon_no_backend로 실패한다. */
  data<T = unknown>(query?: Record<string, string>): Promise<T>
  /** manifest rpc.read에 선언한 OMV 읽기 RPC. */
  rpc<T = unknown>(service: string, method: string, params?: unknown): Promise<T>
  /** 조작 실행. 사용자가 이 프레임을 방금 눌렀을 때만 호스트가 받는다. manifest의 confirm이면 호스트가 먼저 묻는다. */
  action(name: string, options?: ActionOptions): Promise<void>
  /** 결과를 돌려주는 조작. */
  query<T = unknown>(name: string, options?: ActionOptions): Promise<T>
  /** 이 사용자의 패키지 저장값(64KiB까지의 JSON). 위젯은 get만 쓸 수 있다. */
  readonly storage: {
    get<T = unknown>(): Promise<T | null>
    set(data: unknown): Promise<void>
  }
  /** OMV 워크벤치의 라우트를 이름 붙은 탭으로 연다(앞 슬래시 없이, 예: storage/smart/devices). */
  openWorkbench(route: string): Promise<void>
  /** 호스트의 확인 대화상자(패키지 이름이 붙는다). 창 보기만. */
  confirm(options: ConfirmOptions): Promise<boolean>
  /** 호스트 알림(토스트·알림 센터). 문구는 패키지 번역 키다. 창 보기만, 분당 3개. */
  notify(level: NoticeLevel, key: string, params?: Record<string, string | number>): Promise<void>
}

export interface MaruNamespace {
  /** 호스트와 연결하고 첫 문맥을 받는다. 여러 번 불러도 같은 앱을 돌려준다. */
  connect(): Promise<MaruApp>
  /** 런타임의 SDK 수준. */
  readonly sdkLevel: number
}
