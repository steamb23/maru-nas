import type {
  AboutResponse,
  ActionRequest,
  AddonCapability,
  AppearancePrefs,
  AppearanceRequest,
  CapabilitiesResponse,
  ChallengeInfo,
  DeviceCodeResponse,
  DrawerPrefs,
  DrawerRequest,
  ElevationRequest,
  ErrorResponse,
  FileAccess,
  FileInfo,
  FileItem,
  FilesCopyFailure,
  FilesCopyProgress,
  FilesCopyRequest,
  FilesCopyResult,
  FilesDeleteRequest,
  FilesListing,
  FilesMkdirRequest,
  FilesPage,
  FilesPrefs,
  FilesPrefsRequest,
  FilesPrepareResponse,
  FilesRenameRequest,
  FilesSearchHit,
  FilesSearchResult,
  FilesTrashItemRequest,
  FilesTrashList,
  FilesTrashRequest,
  FilesTrashRestoreRequest,
  FilesTrashResult,
  FilesTrashVolumes,
  FilesUploadCheckRequest,
  SiteTrashDisks,
  SiteTrashPrepareRequest,
  TrashItem,
  TrashVolume,
  FilesRoot,
  FilesRoots,
  FilesSettings,
  FilesSettingsRequest,
  FilesShare,
  HelperIdentity,
  HelperIdentityResponse,
  HelperStatus,
  ImageInfo,
  LoginResponse,
  OverridableSetting,
  PasskeyAssertion,
  PasskeyChallengeRequest,
  PasskeyConfirmRequest,
  PasskeyDescriptor,
  PasskeyInfo,
  PasskeyLoginEnrollRequest,
  PasskeyLoginEnrollResponse,
  PasskeyLoginFinishRequest,
  PasskeyLoginInfo,
  PasskeyLoginOffer,
  PasskeyLoginOptionsRequest,
  PasskeyLoginRequest,
  PasskeyLoginUnlockResponse,
  PasskeyOptions,
  PasskeyRegisterRequest,
  PasskeyRegistration,
  PasskeyUser,
  Preferences,
  ProfileRequest,
  ProfileResponse,
  ProfileSaveResponse,
  RuntimeSetting,
  SecurityChangeResponse,
  SecurityManageRequest,
  SecurityPolicy,
  SecurityPolicyRequest,
  SecurityPolicyResponse,
  SecurityStatus,
  SecurityUserInfo,
  SecurityVerifyRequest,
  ServerOverrides,
  ServerSettings,
  ServerSettingsRequest,
  SessionInfo,
  SharedFolder,
  Shortcut,
  ShortcutsRequest,
  SiteDefaults,
  SiteDefaultsRequest,
  SiteInfo,
  SiteLogin,
  SiteLoginRequest,
  StartUpgradeCheckRequest,
  StartUpgradeRequest,
  StepUpInfo,
  SystemInfoPrefs,
  SystemInfoRequest,
  SystemResponse,
  TOTPConfirmRequest,
  TOTPInfo,
  TOTPSetupResponse,
  UpgradeInfo,
  Wallpaper,
  WidgetPlacement,
  WidgetsRequest,
} from './types.gen'
import { isArrayOf, isBool, isNum, isObj, isOneOf, isOptional, isStr } from './guards'
import type * as config from './upgrade.gen'

// BASE_URL은 Vite base 설정값이다. 개발 서버는 '/', 빌드는 './'(문서 URL 기준 상대 경로)다.
export const API_BASE = `${import.meta.env.BASE_URL}api`

export type ErrorCode = NonNullable<ErrorResponse['code']>

const ERROR_CODES: readonly ErrorCode[] = [
  'invalid_credentials',
  'rate_limited',
  'challenge_failed',
  'challenge_expired',
  'omv_unavailable',
  'unauthorized',
  'bad_request',
  'rpc_forbidden',
  'admin_required',
  'plugin_missing',
  'omv_error',
  'action_busy',
  'action_failed',
  'action_timeout',
  'config_dirty',
  'image_too_large',
  'unsupported_image',
  'profile_readonly',
  'helper_unavailable',
  'helper_reauth',
  'elevation_failed',
  'elevation_not_allowed',
  'elevation_required',
  'terminal_disabled',
  'security_required',
  'factor_failed',
  'manage_required',
  'passkey_unavailable',
  'passkey_uv_unsupported',
  'security_unavailable',
  'passkey_login_unknown',
  'passkey_login_expired',
  'passkey_login_stale',
  'upgrade_disabled',
  'upgrade_busy',
  'upgrade_outdated',
  'not_found',
  'permission_denied',
  'outside_roots',
  'file_exists',
  'not_dir',
  'is_dir',
  'not_regular',
  'no_space',
  'read_only',
  'helper_busy',
  'transfer_timeout',
  'listing_expired',
  'sort_pending',
  'not_empty',
  'cross_device',
  'root_folder',
  'no_trash',
]

export type RequiredLevel = NonNullable<ErrorResponse['required']>

export class ApiError extends Error {
  readonly status: number
  // code, retryAfterSeconds는 백엔드가 ErrorResponse로 보낸 값이다. 프록시 오류 페이지 등에서는 없다.
  readonly code?: ErrorCode
  readonly retryAfterSeconds?: number
  // required는 security_required일 때 거쳐야 하는 보안 강화 모드의 단계다.
  readonly required?: RequiredLevel

  constructor(status: number, message: string, code?: ErrorCode, retryAfterSeconds?: number, required?: RequiredLevel) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.retryAfterSeconds = retryAfterSeconds
    this.required = required
  }
}

// 로그인되지 않음·세션 만료(401)를 받으면 부르는 함수. 로그인 화면으로 보내는 데 쓴다.
let onUnauthorized: (() => void) | null = null

// setUnauthorizedHandler는 401 처리 함수를 등록하고 해제 함수를 반환한다.
// 로그인 관련 요청(fetchSession, login, verify, logout)의 401에는 부르지 않는다.
// 그 요청들은 401을 "로그인 안 됨"이라는 정상 결과로 직접 처리한다.
export function setUnauthorizedHandler(fn: () => void): () => void {
  onUnauthorized = fn
  return () => {
    if (onUnauthorized === fn) onUnauthorized = null
  }
}

async function toApiError(path: string, res: Response): Promise<ApiError> {
  let code: ErrorCode | undefined
  let retryAfter: number | undefined
  let required: RequiredLevel | undefined
  try {
    const body: unknown = await res.json()
    if (isObj(body)) {
      if (ERROR_CODES.includes(body.code as ErrorCode)) code = body.code as ErrorCode
      if (isNum(body.retryAfterSeconds) && body.retryAfterSeconds > 0) retryAfter = body.retryAfterSeconds
      if (isOneOf(body.required, ['password', 'twoFactor'] as const)) required = body.required
    }
  } catch {
    // JSON이 아닌 오류 응답(프록시 502 페이지 등)
  }
  return new ApiError(res.status, `${path}: HTTP ${res.status}`, code, retryAfter, required)
}

interface RequestOptions {
  signal?: AbortSignal
  body?: unknown
  // image가 있으면 JSON 대신 이미지 바이트를 그 형식으로 보낸다(배경 이미지 올리기).
  image?: Blob
  // true면 401을 받아도 onUnauthorized를 부르지 않는다.
  authRequest?: boolean
  // range가 있으면 Range 머리로 보낸다(GET만, 미리보기의 텍스트 앞부분).
  range?: string
}

async function request(
  method: 'GET' | 'POST' | 'PUT' | 'DELETE',
  path: string,
  opts: RequestOptions = {},
): Promise<Response> {
  const init: RequestInit = { method, signal: opts.signal, credentials: 'same-origin' }
  if (opts.image) {
    // 이미지 올리기만 원시 이미지 본문을 받는다(다른 출처의 폼은 이 형식을 보낼 수 없다).
    init.headers = { 'Content-Type': opts.image.type }
    init.body = opts.image
  } else if (method === 'POST' || method === 'PUT') {
    // 백엔드는 본문이 있는 요청에 application/json만 받는다(CSRF 보조 장치).
    init.headers = { 'Content-Type': 'application/json' }
    init.body = JSON.stringify(opts.body ?? {})
  } else if (opts.range) {
    init.headers = { Range: opts.range }
  }
  const res = await fetch(`${API_BASE}${path}`, init)
  if (!res.ok) {
    const err = await toApiError(path, res)
    if (res.status === 401 && !opts.authRequest) onUnauthorized?.()
    throw err
  }
  return res
}

async function getJSON(path: string, opts?: RequestOptions): Promise<unknown> {
  return (await request('GET', path, opts)).json()
}

async function postJSON(path: string, opts?: RequestOptions): Promise<unknown> {
  return (await request('POST', path, opts)).json()
}

// isSystemResponse는 화면에서 쓰는 필드가 모두 숫자인지 확인한다.
// 프록시가 HTML 오류 페이지 등을 200으로 돌려줘도 창이 깨지지 않게 한다.
export function isSystemResponse(v: unknown): v is SystemResponse {
  return (
    isObj(v) &&
    isObj(v.cpu) &&
    isNum(v.cpu.usagePercent) &&
    isObj(v.memory) &&
    isNum(v.memory.totalBytes) &&
    isNum(v.memory.usedBytes) &&
    isNum(v.memory.availableBytes) &&
    isNum(v.uptimeSeconds)
  )
}

export function isSessionInfo(v: unknown): v is SessionInfo {
  return (
    isObj(v) &&
    isStr(v.username) &&
    v.username !== '' &&
    typeof v.admin === 'boolean' &&
    isOneOf(v.assurance, ['login', 'password', 'twoFactor'] as const) &&
    (v.expiresInSeconds === undefined || (isNum(v.expiresInSeconds) && v.expiresInSeconds >= 0))
  )
}

function isChallengeInfo(v: unknown): v is ChallengeInfo {
  return isObj(v) && isStr(v.kind) && (v.redirectUrl === undefined || isStr(v.redirectUrl))
}

// isLoginResponse는 상태에 맞는 필드가 들어 있는지 확인한다.
export function isLoginResponse(v: unknown): v is LoginResponse {
  if (!isObj(v)) return false
  if (v.status === 'authenticated')
    return (
      isSessionInfo(v.session) &&
      isOptional(
        v.stepUp,
        (x): x is StepUpInfo =>
          isObj(x) && isArrayOf(x.methods, (m): m is StepUpInfo['methods'][number] => isOneOf(m, ['passkey', 'totp', 'recovery'] as const)),
      ) &&
      isOptional(v.passkeyLoginOffer, (x): x is PasskeyLoginOffer => isObj(x) && isNum(x.days))
    )
  if (v.status === 'challengeRequired') return isChallengeInfo(v.challenge)
  return false
}

const CAPABILITY_REASONS: readonly NonNullable<AddonCapability['reason']>[] = ['plugin_missing', 'admin_required']

function isAddonCapability(v: unknown): v is AddonCapability {
  return (
    isObj(v) &&
    isStr(v.id) &&
    typeof v.available === 'boolean' &&
    (v.reason === undefined || CAPABILITY_REASONS.includes(v.reason as NonNullable<AddonCapability['reason']>)) &&
    Array.isArray(v.sources) &&
    v.sources.every(isStr)
  )
}

export function isCapabilitiesResponse(v: unknown): v is CapabilitiesResponse {
  return (
    isObj(v) &&
    Array.isArray(v.addons) &&
    v.addons.every(isAddonCapability) &&
    Array.isArray(v.plugins) &&
    v.plugins.every(isStr)
  )
}

// fetchCapabilities는 플러그인 설치 상태와 사용자 역할로 계산한 애드온별 사용 가능 여부를 가져온다.
export async function fetchCapabilities(signal?: AbortSignal): Promise<CapabilitiesResponse> {
  const data = await getJSON('/capabilities', { signal })
  if (!isCapabilitiesResponse(data)) {
    throw new Error('/capabilities: unexpected response shape')
  }
  return data
}

export interface FetchAddonOptions {
  signal?: AbortSignal
  // 백엔드 캐시를 건너뛰고 다시 가져오게 한다(SMART 새로고침 등). 캐시가 없는 애드온은 무시한다.
  refresh?: boolean
  // 애드온이 정한 조회 파라미터(시스템 정보의 processes=1)
  query?: Record<string, string>
}

// fetchAddon은 애드온 하나의 현황 데이터를 가져온다(GET /api/addons/{id}). guard로 응답 형태를 검사한다.
export async function fetchAddon<T>(
  id: string,
  guard: (v: unknown) => v is T,
  opts: FetchAddonOptions = {},
): Promise<T> {
  const params = new URLSearchParams(opts.query)
  if (opts.refresh) params.set('refresh', '1')
  const search = params.toString()
  const path = `/addons/${encodeURIComponent(id)}${search ? `?${search}` : ''}`
  const data = await getJSON(path, { signal: opts.signal })
  if (!guard(data)) {
    throw new Error(`${path}: unexpected response shape`)
  }
  return data
}

export interface RunAddonActionOptions {
  signal?: AbortSignal
}

// runAddonAction은 애드온 조작 하나를 실행하고 끝날 때까지 기다린다(POST /api/addons/{id}/actions/{action}).
// 백엔드가 OMV 작업이 끝날 때까지 응답하지 않으므로 오래 걸릴 수 있다. 실패하면 ApiError(code)를 던진다.
export async function runAddonAction(
  id: string,
  action: string,
  target: string,
  opts: RunAddonActionOptions = {},
): Promise<void> {
  const body: ActionRequest = { target }
  await request('POST', `/addons/${encodeURIComponent(id)}/actions/${encodeURIComponent(action)}`, {
    body,
    signal: opts.signal,
  })
}

// hasAddonSources는 애드온 응답에 공통으로 들어 있는 출처 필드를 검사한다.
export function hasAddonSources(v: Record<string, unknown>): boolean {
  return isArrayOf(v.sources, isStr) && isOptional(v.failedSources, (x): x is string[] => isArrayOf(x, isStr))
}

export async function fetchSystem(signal?: AbortSignal): Promise<SystemResponse> {
  const data = await getJSON('/system', { signal })
  if (!isSystemResponse(data)) {
    throw new Error('/system: unexpected response shape')
  }
  return data
}

// fetchSession은 현재 브라우저의 대시보드 세션을 묻는다. 로그인되어 있지 않으면 401 ApiError를 던진다.
export async function fetchSession(signal?: AbortSignal): Promise<SessionInfo> {
  const data = await getJSON('/session', { signal, authRequest: true })
  if (!isSessionInfo(data)) {
    throw new Error('/session: unexpected response shape')
  }
  return data
}

async function loginRequest(path: string, body: unknown): Promise<LoginResponse> {
  const data = await postJSON(path, { body, authRequest: true })
  if (!isLoginResponse(data)) {
    throw new Error(`${path}: unexpected response shape`)
  }
  return data
}

export function login(username: string, password: string): Promise<LoginResponse> {
  return loginRequest('/login', { username, password })
}

export function verifyChallenge(response: string): Promise<LoginResponse> {
  return loginRequest('/login/verify', { response })
}

export async function logout(): Promise<void> {
  await request('POST', '/logout', { authRequest: true })
}

// sendActivity는 사용자가 화면을 조작했음을 알려 OMV 세션을 연장하고, 새 만료 시간을 담은
// 세션 정보를 반환한다. 세션이 이미 만료됐으면 401을 받아 onUnauthorized가 불린다.
export async function sendActivity(): Promise<SessionInfo> {
  const data = await postJSON('/session/activity')
  if (!isSessionInfo(data)) {
    throw new Error('/session/activity: unexpected response shape')
  }
  return data
}

export function isWidgetPlacement(v: unknown): v is WidgetPlacement {
  return (
    isObj(v) &&
    isStr(v.addon) &&
    [v.right, v.top, v.cols, v.rows].every((n) => isNum(n) && Number.isInteger(n) && n >= 0)
  )
}

export function isShortcut(v: unknown): v is Shortcut {
  return isObj(v) && isStr(v.app)
}

// isDrawerPrefs는 정렬 방식이 문자열인지만 본다. 모르는 값은 화면이 추천순으로 다룬다(appSort.ts).
export function isDrawerPrefs(v: unknown): v is DrawerPrefs {
  return isObj(v) && isStr(v.sort)
}

// isSystemInfoPrefs는 값이 문자열인지만 본다. 모르는 값은 시스템 정보 창이 기본값으로 다룬다(sysinfoPrefsStore.ts).
export function isSystemInfoPrefs(v: unknown): v is SystemInfoPrefs {
  return isObj(v) && isStr(v.cpuScale) && isStr(v.cpuGraph)
}

// isFilesPrefs는 값이 문자열인지만 본다. 모르는 보기는 탐색기가 목록 보기로 다룬다(files/viewPrefs.ts).
export function isFilesPrefs(v: unknown): v is FilesPrefs {
  return isObj(v) && isStr(v.view)
}

const THEMES = ['auto', 'light', 'dark'] as const

// isWallpaper는 배경 서술자의 모양을 본다. 모르는 프리셋 id는 화면이 기본 배경으로 다룬다(appearance/wallpapers.ts).
export function isWallpaper(v: unknown): v is Wallpaper {
  return isObj(v) && isOneOf(v.kind, ['preset', 'image'] as const) && isOptional(v.preset, isStr)
}

export function isImageInfo(v: unknown): v is ImageInfo {
  return isObj(v) && isStr(v.rev) && v.rev !== ''
}

export function isAppearancePrefs(v: unknown): v is AppearancePrefs {
  return (
    isObj(v) &&
    isOptional(v.theme, (x) => isOneOf(x, THEMES)) &&
    isOptional(v.wallpaper, isWallpaper) &&
    isOptional(v.animations, isBool)
  )
}

export function isPreferences(v: unknown): v is Preferences {
  return (
    isObj(v) &&
    isOptional(v.widgets, (x) => isArrayOf(x, isWidgetPlacement)) &&
    isOptional(v.shortcuts, (x) => isArrayOf(x, isShortcut)) &&
    isOptional(v.drawer, isDrawerPrefs) &&
    isOptional(v.systemInfo, isSystemInfoPrefs) &&
    isOptional(v.files, isFilesPrefs) &&
    isOptional(v.appearance, isAppearancePrefs) &&
    isOptional(v.wallpaperImage, isImageInfo) &&
    isOptional(v.avatarImage, isImageInfo)
  )
}

export function isAboutResponse(v: unknown): v is AboutResponse {
  return (
    isObj(v) &&
    isStr(v.version) &&
    isStr(v.goVersion) &&
    isOptional(v.revision, isStr) &&
    isOptional(v.revisionTime, isStr) &&
    isOptional(v.modified, isBool) &&
    isOptional(v.omvVersion, isStr) &&
    isOptional(v.omvVersionName, isStr)
  )
}

// fetchAbout은 온마루 버전과 빌드 정보다(설정의 정보 페이지).
export async function fetchAbout(signal?: AbortSignal): Promise<AboutResponse> {
  const data = await getJSON('/about', { signal })
  if (!isAboutResponse(data)) throw new Error('/about: unexpected response shape')
  return data
}

// 진행 중인 설정 요청. 동시에 부른 fetchPreferences는 이 요청 하나를 함께 기다린다.
let preferencesRequest: Promise<Preferences> | null = null

// fetchPreferences는 로그인한 사용자의 대시보드 설정(위젯 배치, 바로가기 등)을 가져온다.
// 로그인 직후 설정 저장소 여럿이 함께 부르므로 동시에 부른 요청은 하나로 합친다. 끝나면 다음 부름은 새로 묻는다.
export function fetchPreferences(): Promise<Preferences> {
  preferencesRequest ??= (async () => {
    const data = await getJSON('/preferences')
    if (!isPreferences(data)) {
      throw new Error('/preferences: unexpected response shape')
    }
    return data
  })().finally(() => {
    preferencesRequest = null
  })
  return preferencesRequest
}

// saveWidgets는 위젯 배치 전체를 저장한다. 다른 설정은 바꾸지 않는다.
export async function saveWidgets(widgets: readonly WidgetPlacement[]): Promise<void> {
  const body: WidgetsRequest = { widgets: [...widgets] }
  await request('PUT', '/preferences/widgets', { body })
}

// saveShortcuts는 바로가기 목록 전체를 저장한다. 다른 설정은 바꾸지 않는다.
export async function saveShortcuts(shortcuts: readonly Shortcut[]): Promise<void> {
  const body: ShortcutsRequest = { shortcuts: [...shortcuts] }
  await request('PUT', '/preferences/shortcuts', { body })
}

// saveDrawer는 "모든 앱" 화면 설정 전체를 저장한다. 다른 설정은 바꾸지 않는다.
export async function saveDrawer(drawer: DrawerPrefs): Promise<void> {
  const body: DrawerRequest = { drawer: { ...drawer } }
  await request('PUT', '/preferences/drawer', { body })
}

// saveSystemInfoPrefs는 시스템 정보 창 설정 전체를 저장한다. 다른 설정은 바꾸지 않는다.
export async function saveSystemInfoPrefs(prefs: SystemInfoPrefs): Promise<void> {
  const body: SystemInfoRequest = { systemInfo: { ...prefs } }
  await request('PUT', '/preferences/system-info', { body })
}

// saveFilesPrefs는 파일 탐색기 보기 설정 전체를 저장한다. 다른 설정은 바꾸지 않는다.
export async function saveFilesPrefs(prefs: FilesPrefs): Promise<void> {
  const body: FilesPrefsRequest = { files: { ...prefs } }
  await request('PUT', '/preferences/files', { body })
}

// saveAppearance는 화면 설정 전체를 저장한다. 다른 설정은 바꾸지 않는다.
export async function saveAppearance(appearance: AppearancePrefs): Promise<void> {
  const body: AppearanceRequest = { appearance: { ...appearance } }
  await request('PUT', '/preferences/appearance', { body })
}

async function putImage(path: string, image: Blob): Promise<ImageInfo> {
  const data = await (await request('PUT', path, { image })).json()
  if (!isImageInfo(data)) throw new Error(`${path}: unexpected response shape`)
  return data
}

// uploadWallpaperImage는 바탕화면 이미지를 올린다. 백엔드가 바탕화면을 이 이미지로 바꾼다.
export function uploadWallpaperImage(image: Blob): Promise<ImageInfo> {
  return putImage('/preferences/wallpaper-image', image)
}

// deleteWallpaperImage는 올린 바탕화면 이미지를 지운다. 바탕화면이 그 이미지였으면 기본값을 따르게 된다.
export async function deleteWallpaperImage(): Promise<void> {
  await request('DELETE', '/preferences/wallpaper-image')
}

// 전역 배경 이미지 자리 이름(백엔드 prefs.SiteImage).
export type SiteImageName = 'login-background' | 'default-wallpaper'

// imageUrl은 배경 이미지 주소다. 리비전을 붙여 이미지가 바뀌면 브라우저가 새로 받게 한다.
export function wallpaperImageUrl(rev: string): string {
  return `${API_BASE}/preferences/wallpaper-image?v=${encodeURIComponent(rev)}`
}

export function siteImageUrl(name: SiteImageName, rev: string): string {
  return `${API_BASE}/site/images/${name}?v=${encodeURIComponent(rev)}`
}

export function isSiteInfo(v: unknown): v is SiteInfo {
  return (
    isObj(v) &&
    isOptional(v.loginBackground, isWallpaper) &&
    isBool(v.loginEasterEgg) &&
    isOptional(v.defaultTheme, (x) => isOneOf(x, THEMES)) &&
    isOptional(v.defaultWallpaper, isWallpaper) &&
    isObj(v.images) &&
    isOptional(v.images.loginBackground, isImageInfo) &&
    isOptional(v.images.defaultWallpaper, isImageInfo)
  )
}

// fetchSite는 로그인 화면이 쓰는 공개 전역 정보(로그인 배경, 기본 화면 등)다. 로그인 전에도 받는다.
export async function fetchSite(signal?: AbortSignal): Promise<SiteInfo> {
  const data = await getJSON('/site', { signal, authRequest: true })
  if (!isSiteInfo(data)) throw new Error('/site: unexpected response shape')
  return data
}

// saveSiteLogin은 로그인 화면 설정 전체를 저장한다(관리자).
export async function saveSiteLogin(login: SiteLogin): Promise<void> {
  const body: SiteLoginRequest = { login: { ...login } }
  await request('PUT', '/site/login', { body })
}

// saveSiteDefaults는 기본 화면 전체를 저장한다(관리자).
export async function saveSiteDefaults(defaults: SiteDefaults): Promise<void> {
  const body: SiteDefaultsRequest = { defaults: { ...defaults } }
  await request('PUT', '/site/defaults', { body })
}

// uploadSiteImage는 전역 배경 이미지를 올린다(관리자). 그 자리의 배경이 이 이미지로 바뀐다.
export function uploadSiteImage(name: SiteImageName, image: Blob): Promise<ImageInfo> {
  return putImage(`/site/images/${name}`, image)
}

// deleteSiteImage는 전역 배경 이미지를 지운다(관리자).
export async function deleteSiteImage(name: SiteImageName): Promise<void> {
  await request('DELETE', `/site/images/${name}`)
}

const isOverridable = (v: unknown): v is OverridableSetting =>
  isObj(v) && isStr(v.value) && isStr(v.base) && isBool(v.overridden)

const isRuntimeSetting = (v: unknown): v is RuntimeSetting =>
  isObj(v) &&
  isStr(v.flag) &&
  isStr(v.env) &&
  isStr(v.value) &&
  isOneOf(v.source, ['default', 'env', 'flag', 'systemd'] as const)

export function isServerSettings(v: unknown): v is ServerSettings {
  return (
    isObj(v) &&
    isOverridable(v.workbenchUrl) &&
    isOverridable(v.smartRefreshAt) &&
    isArrayOf(v.runtime, isRuntimeSetting)
  )
}

// fetchServerSettings는 웹에서 덮어쓸 수 있는 가동 설정과 가동 설정 현황이다(관리자).
export async function fetchServerSettings(signal?: AbortSignal): Promise<ServerSettings> {
  const data = await getJSON('/site/server', { signal })
  if (!isServerSettings(data)) throw new Error('/site/server: unexpected response shape')
  return data
}

// saveServerSettings는 덮어쓰기 값 전체를 저장한다(관리자). 없는 값은 플래그·환경 변수 값으로 돌아간다.
export async function saveServerSettings(server: ServerOverrides): Promise<void> {
  const body: ServerSettingsRequest = { server: { ...server } }
  await request('PUT', '/site/server', { body })
}

// avatarImageUrl은 프로필 사진 주소다. 리비전을 붙여 사진이 바뀌면 브라우저가 새로 받게 한다.
export function avatarImageUrl(rev: string): string {
  return `${API_BASE}/preferences/avatar-image?v=${encodeURIComponent(rev)}`
}

// uploadAvatarImage는 프로필 사진을 바꾼다. 브라우저에서 잘라 줄인 이미지를 보낸다(백엔드 상한 2MiB).
export function uploadAvatarImage(image: Blob): Promise<ImageInfo> {
  return putImage('/preferences/avatar-image', image)
}

// deleteAvatarImage는 프로필 사진을 지운다.
export async function deleteAvatarImage(): Promise<void> {
  await request('DELETE', '/preferences/avatar-image')
}

export function isProfileResponse(v: unknown): v is ProfileResponse {
  return (
    isObj(v) &&
    isStr(v.username) &&
    isOptional(v.email, isStr) &&
    isOptional(v.tags, (x) => isArrayOf(x, isStr)) &&
    isOptional(v.groups, (x) => isArrayOf(x, isStr)) &&
    isOptional(v.systemAccount, isBool) &&
    isOptional(v.readOnlyReason, (x) => isOneOf(x, ['system', 'locked'] as const))
  )
}

// fetchProfile은 로그인한 사용자 자신의 OMV 계정 정보다(설정의 프로필 페이지).
export async function fetchProfile(signal?: AbortSignal): Promise<ProfileResponse> {
  const data = await getJSON('/profile', { signal })
  if (!isProfileResponse(data)) throw new Error('/profile: unexpected response shape')
  return data
}

// saveProfile은 자기 계정의 전자 우편·태그를 바꾼다(없는 값은 그대로). applied가 false면 OMV에 적용 대기가
// 남았다(일반 사용자).
export async function saveProfile(change: ProfileRequest): Promise<ProfileSaveResponse> {
  const body: ProfileRequest = { ...change }
  const data = await (await request('PUT', '/profile', { body })).json()
  if (!isObj(data) || !isBool(data.applied)) throw new Error('/profile: unexpected response shape')
  return { applied: data.applied }
}

const HELPER_STATES = ['disabled', 'unavailable', 'none', 'reauth', 'active'] as const

export function isHelperStatus(v: unknown): v is HelperStatus {
  return isObj(v) && isOneOf(v.state, HELPER_STATES) && isBool(v.canElevate) && isNum(v.elevatedSeconds)
}

function isHelperIdentity(v: unknown): v is HelperIdentity {
  return (
    isObj(v) &&
    isNum(v.uid) &&
    isNum(v.gid) &&
    isStr(v.user) &&
    isArrayOf(v.groups, (g): g is HelperIdentity['groups'][number] => isObj(g) && isNum(g.id) && isStr(g.name))
  )
}

export function isHelperIdentityResponse(v: unknown): v is HelperIdentityResponse {
  return isObj(v) && isHelperIdentity(v.user) && isOptional(v.root, isHelperIdentity)
}

// fetchHelperStatus는 이 세션의 권한 도우미 상태(상승 여부·남은 시간 포함)다.
export async function fetchHelperStatus(signal?: AbortSignal): Promise<HelperStatus> {
  const data = await getJSON('/helper', { signal })
  if (!isHelperStatus(data)) throw new Error('/helper: unexpected response shape')
  return data
}

// fetchHelperIdentity는 도우미가 이 사용자 권한으로(상승 중이면 root로도) 실행한 worker의 실제 권한이다.
export async function fetchHelperIdentity(signal?: AbortSignal): Promise<HelperIdentityResponse> {
  const data = await getJSON('/helper/identity', { signal })
  if (!isHelperIdentityResponse(data)) throw new Error('/helper/identity: unexpected response shape')
  return data
}

// elevate는 비밀번호를 다시 확인해 이 세션에 root 권한 상승을 붙인다(관리자). 비밀번호는 도우미가 직접 확인한다.
export async function elevate(password: string): Promise<HelperStatus> {
  const body: ElevationRequest = { password }
  const data = await postJSON('/helper/elevation', { body })
  if (!isHelperStatus(data)) throw new Error('/helper/elevation: unexpected response shape')
  return data
}

// dropElevation은 상승을 바로 끝낸다.
export async function dropElevation(): Promise<HelperStatus> {
  const data = await (await request('DELETE', '/helper/elevation')).json()
  if (!isHelperStatus(data)) throw new Error('/helper/elevation: unexpected response shape')
  return data
}

// 대시보드에서 업그레이드(GET·POST /api/upgrade*, 관리자). 상태는 업그레이드 서비스의 상태 파일 그대로다(upgrade.gen.ts).

const UPGRADE_DISABLED_REASONS = ['not-enabled', 'no-helper', 'state-dir'] as const

// isNullable은 값이 없거나(undefined·null) guard를 통과하는지다. Go 포인터 필드는 null로 온다.
function isNullable<T>(v: unknown, guard: (x: unknown) => x is T): v is T | undefined | null {
  return v === undefined || v === null || guard(v)
}

function isLocalizedText(v: unknown): v is config.LocalizedText {
  return isObj(v) && isStr(v.en) && isStr(v.ko)
}

function isUpgradeReason(v: unknown): v is config.UpgradeReason {
  return isObj(v) && isStr(v.id) && isLocalizedText(v.text) && isStr(v.command)
}

function isUpgradeQuestion(v: unknown): v is config.UpgradeQuestion {
  return (
    isObj(v) &&
    isStr(v.name) &&
    isOneOf(v.kind, ['input', 'confirm'] as const) &&
    isLocalizedText(v.title) &&
    isLocalizedText(v.help) &&
    isStr(v.default) &&
    isBool(v.dash) &&
    isBool(v.emptyOK) &&
    isNullable(v.error, isLocalizedText)
  )
}

function isUpgradePlan(v: unknown): v is config.UpgradePlan {
  return (
    isObj(v) &&
    isStr(v.version) &&
    isBool(v.web) &&
    isArrayOf(v.manual, isUpgradeReason) &&
    isArrayOf(v.questions, isUpgradeQuestion) &&
    isArrayOf(v.notes, isLocalizedText)
  )
}

function isUpgradeCommit(v: unknown): v is config.UpgradeCommit {
  return isObj(v) && isStr(v.hash) && isStr(v.author) && isStr(v.time) && isStr(v.subject)
}

function isUpgradeCheck(v: unknown): v is config.UpgradeCheck {
  return (
    isObj(v) &&
    isStr(v.mode) &&
    isBool(v.upToDate) &&
    isBool(v.web) &&
    isArrayOf(v.commits, isUpgradeCommit) &&
    isArrayOf(v.manual, isUpgradeReason) &&
    isNullable(v.plan, isUpgradePlan)
  )
}

function isUpgradeStatus(v: unknown): v is config.UpgradeStatus {
  return (
    isObj(v) &&
    isStr(v.state) &&
    isStr(v.updatedAt) &&
    isNullable(v.check, isUpgradeCheck) &&
    isNullable(v.apply, (a): a is config.UpgradeApply => isObj(a) && isStr(a.target) && isStr(a.user)) &&
    isNullable(v.error, (e): e is config.UpgradeError => isObj(e) && isLocalizedText(e.text))
  )
}

export function isUpgradeInfo(v: unknown): v is UpgradeInfo {
  return (
    isObj(v) &&
    isBool(v.enabled) &&
    isOptional(v.disabledReason, (r) => isOneOf(r, UPGRADE_DISABLED_REASONS)) &&
    isBool(v.pending) &&
    isBool(v.pendingStalled) &&
    isBool(v.interrupted) &&
    isStr(v.version) &&
    isNullable(v.installed, (r): r is config.InstallRecord => isObj(r) && isStr(r.version) && isStr(r.revision)) &&
    isNullable(v.status, isUpgradeStatus)
  )
}

async function upgradeInfo(path: string, res: Promise<unknown>): Promise<UpgradeInfo> {
  const data = await res
  if (!isUpgradeInfo(data)) throw new Error(`${path}: unexpected response shape`)
  return { ...data, installed: data.installed ?? undefined, status: data.status ?? undefined }
}

// fetchUpgrade는 대시보드에서 업그레이드의 상태다.
export function fetchUpgrade(signal?: AbortSignal): Promise<UpgradeInfo> {
  return upgradeInfo('/upgrade', getJSON('/upgrade', { signal }))
}

// requestUpgradeCheck는 업그레이드 서비스에 확인을 요청한다(상승이 필요 없다).
export function requestUpgradeCheck(mode: StartUpgradeCheckRequest['mode']): Promise<UpgradeInfo> {
  const body: StartUpgradeCheckRequest = { mode }
  return upgradeInfo('/upgrade/check', postJSON('/upgrade/check', { body }))
}

// applyUpgrade는 마지막 확인의 대상 커밋으로 업그레이드를 요청한다. 상승하지 않았으면 elevation_required다.
export function applyUpgrade(target: string, settings: Record<string, string>): Promise<UpgradeInfo> {
  const body: StartUpgradeRequest = { target, settings }
  return upgradeInfo('/upgrade/apply', postJSON('/upgrade/apply', { body }))
}

// fetchHealth는 백엔드가 응답하는지 본다(로그인 필요 없음). 업그레이드로 다시 시작하기를 기다릴 때 쓴다.
export async function fetchHealth(signal?: AbortSignal): Promise<void> {
  const data = await getJSON('/health', { signal, authRequest: true })
  if (!isObj(data) || data.status !== 'ok') throw new Error('/health: unexpected response shape')
}

// 보안 강화 모드(GET·POST /api/security*, 관리자 정책은 /api/site/security)

const ASSURANCES = ['login', 'password', 'twoFactor'] as const
const LEVELS = ['off', 'password', 'twoFactor'] as const
const FACTORS = ['passkey', 'totp', 'recovery'] as const

// passkeyLoginDays는 숫자인지만 본다. 백엔드가 모르는 값을 정해진 값으로 바꿔 보낸다.
function isSecurityPolicy(v: unknown): v is SecurityPolicy {
  return (
    isObj(v) &&
    isOptional(v.terminal, (x) => isOneOf(x, LEVELS)) &&
    isOptional(v.elevation, (x) => isOneOf(x, LEVELS)) &&
    isOptional(v.files, (x) => isOneOf(x, LEVELS)) &&
    isOptional(v.filesScope, (x) => isOneOf(x, ['all', 'writes'] as const)) &&
    isOptional(v.passkeyLoginDays, (x): x is SecurityPolicy['passkeyLoginDays'] => isNum(x))
  )
}

function isPasskeyInfo(v: unknown): v is PasskeyInfo {
  return (
    isObj(v) &&
    isStr(v.id) &&
    isStr(v.name) &&
    isStr(v.created) &&
    isOptional(v.lastUsed, isStr) &&
    isOptional(v.backupEligible, isBool)
  )
}

function isPasskeyLoginInfo(v: unknown): v is PasskeyLoginInfo {
  return (
    isObj(v) &&
    isStr(v.id) &&
    isStr(v.passkey) &&
    isStr(v.created) &&
    isStr(v.expiresAt) &&
    isOptional(v.lastUsed, isStr)
  )
}

export function isSecurityStatus(v: unknown): v is SecurityStatus {
  return (
    isObj(v) &&
    isOneOf(v.assurance, ASSURANCES) &&
    isNum(v.manageSeconds) &&
    isBool(v.passkeysAvailable) &&
    isArrayOf(v.passkeys, isPasskeyInfo) &&
    isOptional(v.totp, (x): x is TOTPInfo => isObj(x) && isStr(x.created)) &&
    isNum(v.recoveryCodes) &&
    isArrayOf(v.methods, (x): x is SecurityStatus['methods'][number] => isOneOf(x, FACTORS)) &&
    isSecurityPolicy(v.policy) &&
    isArrayOf(v.passkeyLogins, isPasskeyLoginInfo)
  )
}

function isSecurityChange(v: unknown): v is SecurityChangeResponse {
  return isObj(v) && isSecurityStatus(v.status) && isOptional(v.recoveryCodes, (x): x is string[] => isArrayOf(x, isStr))
}

function isPasskeyDescriptor(v: unknown): v is PasskeyDescriptor {
  return isObj(v) && isStr(v.id) && isOptional(v.transports, (x): x is string[] => isArrayOf(x, isStr))
}

export function isPasskeyOptions(v: unknown): v is PasskeyOptions {
  return (
    isObj(v) &&
    isStr(v.challenge) &&
    isStr(v.rpId) &&
    isNum(v.timeoutMs) &&
    isArrayOf(v.credentials, isPasskeyDescriptor) &&
    isOptional(v.rpName, isStr) &&
    isOptional(v.user, (x): x is PasskeyUser => isObj(x) && isStr(x.id) && isStr(x.name) && isStr(x.displayName)) &&
    isOptional(v.algorithms, (x): x is number[] => isArrayOf(x, isNum))
  )
}

function isSecurityUserInfo(v: unknown): v is SecurityUserInfo {
  return (
    isObj(v) &&
    isStr(v.username) &&
    isNum(v.passkeys) &&
    isBool(v.totp) &&
    isNum(v.recoveryCodes) &&
    isOptional(v.unreadable, isBool)
  )
}

function isSecurityPolicyResponse(v: unknown): v is SecurityPolicyResponse {
  return (
    isObj(v) &&
    isSecurityPolicy(v.policy) &&
    isOptional(v.autoLogoutMinutes, isNum) &&
    isOptional(v.passkeyRpId, isStr)
  )
}

// isPasskeyLoginEnrollResponse는 만료 시각을 읽을 수 있는지도 본다(브라우저 항목의 만료로 쓴다).
function isPasskeyLoginEnrollResponse(v: unknown): v is PasskeyLoginEnrollResponse {
  return (
    isObj(v) &&
    isStr(v.id) &&
    v.id !== '' &&
    isStr(v.share) &&
    isStr(v.expiresAt) &&
    Number.isFinite(Date.parse(v.expiresAt)) &&
    isSecurityStatus(v.status)
  )
}

async function securityStatusFrom(path: string, res: Promise<unknown>): Promise<SecurityStatus> {
  const data = await res
  if (!isSecurityStatus(data)) throw new Error(`${path}: unexpected response shape`)
  return data
}

async function securityChangeFrom(path: string, res: Promise<unknown>): Promise<SecurityChangeResponse> {
  const data = await res
  if (!isSecurityChange(data)) throw new Error(`${path}: unexpected response shape`)
  return data
}

// fetchSecurity는 로그인한 사용자 자신의 보안 상태(보증 수준, 등록한 수단, 정책)다.
export function fetchSecurity(signal?: AbortSignal): Promise<SecurityStatus> {
  return securityStatusFrom('/security', getJSON('/security', { signal }))
}

// verifySecurity는 세션 중 확인(비밀번호 재확인 또는 2차 수단)으로 이 세션의 보증 수준을 올린다.
export function verifySecurity(req: SecurityVerifyRequest): Promise<SecurityStatus> {
  return securityStatusFrom('/security/verify', postJSON('/security/verify', { body: req }))
}

// manageSecurity는 비밀번호와(등록한 수단이 있으면) 그 수단을 다시 확인해 관리 창을 연다.
export function manageSecurity(req: SecurityManageRequest): Promise<SecurityStatus> {
  return securityStatusFrom('/security/manage', postJSON('/security/manage', { body: req }))
}

// passkeyChallenge는 패스키 확인·등록에 쓸 옵션(challenge 포함)이다.
export async function passkeyChallenge(purpose: PasskeyChallengeRequest['purpose']): Promise<PasskeyOptions> {
  const body: PasskeyChallengeRequest = { purpose }
  const data = await postJSON('/security/passkeys/challenge', { body })
  if (!isPasskeyOptions(data)) throw new Error('/security/passkeys/challenge: unexpected response shape')
  return data
}

// registerPasskey는 새 패스키의 등록 값을 보낸다. 백엔드는 아직 저장하지 않고, 그 패스키로 시험 확인할 옵션을 준다.
export async function registerPasskey(name: string, credential: PasskeyRegistration): Promise<PasskeyOptions> {
  const body: PasskeyRegisterRequest = { name, credential }
  const data = await postJSON('/security/passkeys', { body })
  if (!isPasskeyOptions(data)) throw new Error('/security/passkeys: unexpected response shape')
  return data
}

// confirmPasskey는 새 패스키로 한 시험 확인을 보낸다. 통과하면 백엔드가 패스키를 저장한다.
export function confirmPasskey(passkey: PasskeyAssertion): Promise<SecurityChangeResponse> {
  const body: PasskeyConfirmRequest = { passkey }
  return securityChangeFrom('/security/passkeys/confirm', postJSON('/security/passkeys/confirm', { body }))
}

export async function deletePasskey(id: string): Promise<SecurityChangeResponse> {
  const path = `/security/passkeys/${encodeURIComponent(id)}`
  return securityChangeFrom(path, (await request('DELETE', path)).json())
}

// 패스키로 로그인(.claude/rules/security.md). 비밀번호 암호문은 브라우저에만 있고, 서버는 키 조각(share)만 준다.

// enrollPasskeyLogin은 비밀번호와 그 패스키(PRF를 함께 요청한 확인)를 보내 이 브라우저의 패스키로 로그인을 켠다.
// 응답의 share는 키를 만드는 데만 쓰고 저장하지 않는다. 세션은 twoFactor로 오른다.
export async function enrollPasskeyLogin(req: PasskeyLoginEnrollRequest): Promise<PasskeyLoginEnrollResponse> {
  const data = await postJSON('/security/passkey-logins', { body: req })
  if (!isPasskeyLoginEnrollResponse(data)) throw new Error('/security/passkey-logins: unexpected response shape')
  return data
}

// deletePasskeyLogin은 패스키로 로그인 등록 하나를 해제한다(관리 창은 필요 없다).
export async function deletePasskeyLogin(id: string): Promise<SecurityStatus> {
  const path = `/security/passkey-logins/${encodeURIComponent(id)}`
  return securityStatusFrom(path, (await request('DELETE', path)).json())
}

// passkeyLoginOptions는 로그인 화면에서 브라우저 항목의 등록으로 확인 옵션(그 패스키 하나만 허용)을 받는다.
// 로그인 단계 쿠키를 연다. 없는 등록은 passkey_login_unknown, 만료는 passkey_login_expired다.
export async function passkeyLoginOptions(username: string, enrollment: string): Promise<PasskeyOptions> {
  const body: PasskeyLoginOptionsRequest = { username, enrollment }
  const data = await postJSON('/login/passkey/options', { body, authRequest: true })
  if (!isPasskeyOptions(data)) throw new Error('/login/passkey/options: unexpected response shape')
  return data
}

// passkeyLoginUnlock은 패스키 확인을 보내고, 맞으면 비밀번호를 풀 키 조각(share)과 회전할 다음 조각(nextShare)을 받는다
// (둘 다 base64url).
export async function passkeyLoginUnlock(passkey: PasskeyAssertion): Promise<PasskeyLoginUnlockResponse> {
  const body: PasskeyLoginRequest = { passkey }
  const data = await postJSON('/login/passkey', { body, authRequest: true })
  if (!isObj(data) || !isStr(data.share) || !isStr(data.nextShare)) throw new Error('/login/passkey: unexpected response shape')
  return { share: data.share, nextShare: data.nextShare }
}

// finishPasskeyLogin은 풀어 낸 비밀번호로 로그인을 마친다. 응답은 일반 로그인과 같다(twoFactor 세션). rotated는 항목을
// 다음 조각용으로 바꿔 저장했는지이며, 그러면 백엔드가 회전을 확정한다. 비밀번호가 바뀌었으면 백엔드가 등록을 지우고
// passkey_login_stale을 준다.
export function finishPasskeyLogin(password: string, rotated: boolean): Promise<LoginResponse> {
  const body: PasskeyLoginFinishRequest = { password, rotated }
  return loginRequest('/login/passkey/finish', body)
}

// discardPasskeyLogin은 브라우저가 풀지 못한 등록을 서버에서 지우게 한다(204). passkeyLoginUnlock 뒤에만 쓸 수 있다.
export async function discardPasskeyLogin(): Promise<void> {
  const body: PasskeyLoginFinishRequest = { discard: true }
  await request('POST', '/login/passkey/finish', { body, authRequest: true })
}

export async function setupTOTP(): Promise<TOTPSetupResponse> {
  const data = await postJSON('/security/totp/setup')
  if (!isObj(data) || !isStr(data.secret) || !isStr(data.uri)) throw new Error('/security/totp/setup: unexpected response shape')
  return { secret: data.secret, uri: data.uri }
}

export function confirmTOTP(code: string): Promise<SecurityChangeResponse> {
  const body: TOTPConfirmRequest = { code }
  return securityChangeFrom('/security/totp', postJSON('/security/totp', { body }))
}

export async function deleteTOTP(): Promise<SecurityChangeResponse> {
  return securityChangeFrom('/security/totp', (await request('DELETE', '/security/totp')).json())
}

export function regenerateRecoveryCodes(): Promise<SecurityChangeResponse> {
  return securityChangeFrom('/security/recovery-codes', postJSON('/security/recovery-codes'))
}

// createDeviceCode는 기기 추가 코드를 만든다(관리 창 필요). 등록한 패스키가 없는 새 기기의 관리 확인이 등록한 수단
// 대신 받는다(SecurityManageRequest.deviceCode).
export async function createDeviceCode(): Promise<DeviceCodeResponse> {
  const data = await postJSON('/security/device-code')
  if (!isObj(data) || !isStr(data.code) || !isNum(data.expiresInSeconds)) throw new Error('/security/device-code: unexpected response shape')
  return { code: data.code, expiresInSeconds: data.expiresInSeconds }
}

// fetchSecurityUsers는 인증 수단 기록이 있는 사용자 목록이다(관리자).
export async function fetchSecurityUsers(signal?: AbortSignal): Promise<SecurityUserInfo[]> {
  const data = await getJSON('/security/users', { signal })
  if (!isArrayOf(data, isSecurityUserInfo)) throw new Error('/security/users: unexpected response shape')
  return data
}

// resetSecurityUser는 사용자의 인증 수단을 모두 지우고 그 사용자의 세션을 끝낸다(관리자, 2차 인증을 거친 세션).
export async function resetSecurityUser(username: string): Promise<void> {
  await request('DELETE', `/security/users/${encodeURIComponent(username)}`)
}

export async function fetchSecurityPolicy(signal?: AbortSignal): Promise<SecurityPolicyResponse> {
  const data = await getJSON('/site/security', { signal })
  if (!isSecurityPolicyResponse(data)) throw new Error('/site/security: unexpected response shape')
  return data
}

export async function saveSecurityPolicy(policy: SecurityPolicy): Promise<void> {
  const body: SecurityPolicyRequest = { security: policy }
  await request('PUT', '/site/security', { body })
}

// 파일 탐색기. 파일은 권한 도우미가 로그인한 사용자의 권한으로 다루고, 백엔드는 보이는 폴더 밖의 경로를 거른다.

const isFilesRoot = (v: unknown): v is FilesRoot => isObj(v) && isStr(v.name) && isStr(v.path)

const isFilesShare = (v: unknown): v is FilesShare =>
  isObj(v) && isStr(v.uuid) && isStr(v.name) && isStr(v.path) && isBool(v.reachable)

export function isFilesRoots(v: unknown): v is FilesRoots {
  return (
    isObj(v) &&
    isOptional(v.home, isFilesRoot) &&
    isArrayOf(v.shares, isFilesShare) &&
    isArrayOf(v.disks, isFilesRoot) &&
    isBool(v.fallback) &&
    isOptional(v.sharesUpdatedAt, isStr)
  )
}

export function isFilesListing(v: unknown): v is FilesListing {
  return isObj(v) && isStr(v.id) && isStr(v.path) && isNum(v.total) && isBool(v.truncated) && isBool(v.writable)
}

const FILE_TYPES = ['dir', 'file', 'symlink', 'other'] as const

export function isFileItem(v: unknown): v is FileItem {
  return (
    isObj(v) &&
    isStr(v.name) &&
    isOneOf(v.type, [...FILE_TYPES, 'unknown'] as const) &&
    isOneOf(v.state, ['ok', 'pending', 'gone', 'error'] as const) &&
    isOptional(v.size, isNum) &&
    isOptional(v.modTime, isNum) &&
    isOptional(v.locked, isBool) &&
    isOptional(v.linkType, (x: unknown) => isOneOf(x, FILE_TYPES)) &&
    isOptional(v.badName, isBool)
  )
}

export function isFilesPage(v: unknown): v is FilesPage {
  return isObj(v) && isNum(v.total) && isNum(v.offset) && isArrayOf(v.items, isFileItem)
}

const isFileAccess = (v: unknown): v is FileAccess => isObj(v) && isBool(v.read) && isBool(v.write)

export function isFileInfo(v: unknown): v is FileInfo {
  return (
    isObj(v) &&
    isStr(v.path) &&
    isStr(v.name) &&
    isOneOf(v.type, FILE_TYPES) &&
    isNum(v.size) &&
    isNum(v.modTime) &&
    isNum(v.mode) &&
    isNum(v.uid) &&
    isNum(v.gid) &&
    isOptional(v.owner, isStr) &&
    isOptional(v.group, isStr) &&
    isOptional(v.acl, isBool) &&
    isOptional(v.access, isFileAccess) &&
    isOptional(v.shareUuid, isStr) &&
    isOptional(v.target, isStr)
  )
}

const isSharedFolder = (v: unknown): v is SharedFolder => isObj(v) && isStr(v.uuid) && isStr(v.name) && isStr(v.path)

export function isFilesSettings(v: unknown): v is FilesSettings {
  return isObj(v) && isArrayOf(v.shares, isSharedFolder) && isOptional(v.sharesUpdatedAt, isStr) && isBool(v.showDisks)
}

const q = encodeURIComponent

// fetchFilesRoots는 파일 탐색기 사이드바에 처음 보이는 폴더(홈, 공유 폴더, 디스크)다.
export async function fetchFilesRoots(signal?: AbortSignal): Promise<FilesRoots> {
  const data = await getJSON('/files/roots', { signal })
  if (!isFilesRoots(data)) throw new Error('/files/roots: unexpected response shape')
  return data
}

// openListing은 폴더의 이름 목록을 연다. 같은 세션이 연 목록이 그대로면 서버가 다시 읽지 않는다(refresh면 다시 읽는다).
export async function openListing(path: string, refresh = false, signal?: AbortSignal): Promise<FilesListing> {
  const data = await getJSON(`/files/list?path=${q(path)}${refresh ? '&refresh=1' : ''}`, { signal })
  if (!isFilesListing(data)) throw new Error('/files/list: unexpected response shape')
  return data
}

export interface ListingRange {
  offset: number
  limit: number
  sort: 'name' | 'mtime' | 'size'
  desc: boolean
  query: string
  // namesOnly면 정보 없이 이름만 받는다(도우미를 부르지 않는다. 스크롤 위치 말풍선).
  namesOnly?: boolean
}

// fetchListingRange는 목록의 보기(정렬·찾기)에서 범위 하나다. 정보가 없는 항목은 서버가 읽어 채운다.
export async function fetchListingRange(id: string, r: ListingRange, signal?: AbortSignal): Promise<FilesPage> {
  const params = new URLSearchParams({
    offset: String(r.offset),
    limit: String(r.limit),
    sort: r.sort,
    order: r.desc ? 'desc' : 'asc',
  })
  if (r.query) params.set('q', r.query)
  if (r.namesOnly) params.set('names', '1')
  const data = await getJSON(`/files/list/${q(id)}?${params}`, { signal })
  if (!isFilesPage(data)) throw new Error('/files/list/{id}: unexpected response shape')
  return data
}

// prepareListing은 정보가 없는 항목을 한 묶음 더 읽는다(날짜·크기순 준비). filled가 total이 될 때까지 부른다.
export async function prepareListing(id: string, signal?: AbortSignal): Promise<FilesPrepareResponse> {
  const data = await postJSON(`/files/list/${q(id)}/prepare`, { signal })
  if (!isObj(data) || !isNum(data.filled) || !isNum(data.total)) {
    throw new Error('/files/list/{id}/prepare: unexpected response shape')
  }
  return { filled: data.filled, total: data.total }
}

// findInListing은 목록의 보기(정렬·찾기)에서 이름 하나의 자리다. 보기에 없으면 -1이다(만들거나 이름을 바꾼 항목 고르기).
export async function findInListing(
  id: string,
  name: string,
  view: Pick<ListingRange, 'sort' | 'desc' | 'query'>,
  signal?: AbortSignal,
): Promise<number> {
  const params = new URLSearchParams({ name, sort: view.sort, order: view.desc ? 'desc' : 'asc' })
  if (view.query) params.set('q', view.query)
  const data = await getJSON(`/files/list/${q(id)}/find?${params}`, { signal })
  if (!isObj(data) || !isNum(data.index)) throw new Error('/files/list/{id}/find: unexpected response shape')
  return data.index
}

async function fileItemFrom(path: string, res: Promise<unknown>): Promise<FileItem> {
  const data = await res
  if (!isFileItem(data)) throw new Error(`${path}: unexpected response shape`)
  return data
}

// makeFolder는 path에 폴더를 만든다. 같은 이름이 있으면 file_exists다.
export function makeFolder(path: string): Promise<FileItem> {
  const body: FilesMkdirRequest = { path }
  return fileItemFrom('/files/mkdir', postJSON('/files/mkdir', { body }))
}

// renameFile은 이름을 바꾸거나 다른 폴더로 옮긴다(같은 디스크 안). 대상이 있으면 file_exists이고 덮어쓰지 않는다.
export function renameFile(from: string, to: string): Promise<FileItem> {
  const body: FilesRenameRequest = { from, to }
  return fileItemFrom('/files/rename', postJSON('/files/rename', { body }))
}

// deleteFile은 파일이나 폴더를 지운다. 되돌릴 수 없다. recursive면 폴더 안의 항목까지 지운다.
export async function deleteFile(path: string, recursive: boolean): Promise<void> {
  const body: FilesDeleteRequest = { path, recursive }
  await request('POST', '/files/delete', { body })
}

// 복사(다른 디스크로 끌어 놓기). 권한 도우미가 복사하고, 진행은 요청이 열려 있는 동안 text/event-stream으로 온다.

// parseCopyEvents는 받은 글자 buffer에서 끝난 사건(빈 줄로 끝나는 블록의 data 줄)을 꺼내고 남은 글자를 돌려준다.
// JSON이 아닌 사건은 건너뛴다.
export function parseCopyEvents(buffer: string): { events: unknown[]; rest: string } {
  const events: unknown[] = []
  const blocks = buffer.replaceAll('\r\n', '\n').split('\n\n')
  const rest = blocks.pop() ?? ''
  for (const block of blocks) {
    const data = block
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice(5).replace(/^ /, ''))
      .join('\n')
    if (data === '') continue
    try {
      events.push(JSON.parse(data))
    } catch {
      // 프록시가 끼워 넣은 것 등
    }
  }
  return { events, rest }
}

const isCopyProgress = (v: unknown): v is FilesCopyProgress =>
  isObj(v) &&
  isNum(v.files) &&
  isNum(v.bytes) &&
  isNum(v.totalFiles) &&
  isNum(v.totalBytes) &&
  isOptional(v.scanning, isBool) &&
  isOptional(v.current, isStr)

// copyResultOf는 결과 모양을 확인한다. 모르는 실패 코드는 helper_unavailable로 둔다.
function copyResultOf(v: unknown): FilesCopyResult | null {
  if (!isObj(v) || !isArrayOf(v.copied, isStr) || !Array.isArray(v.failed) || !isNum(v.skipped)) return null
  const failed: FilesCopyFailure[] = []
  for (const f of v.failed as unknown[]) {
    if (!isObj(f) || !isStr(f.name)) return null
    failed.push({ name: f.name, code: ERROR_CODES.includes(f.code as ErrorCode) ? (f.code as ErrorCode) : 'helper_unavailable' })
  }
  return { copied: v.copied, failed, skipped: v.skipped }
}

// copyFiles는 req.dir 안의 이름들을 req.to 폴더에 같은 이름으로 복사한다. 진행은 onProgress로 받는다. 시작하기 전의
// 실패와, 시작한 뒤 멈춘 실패는 ApiError로 던지고, 이름마다의 실패는 결과의 failed에 담긴다. signal로 끊으면
// 서버와 도우미가 하던 항목을 지우고 멈춘다(이미 복사한 항목은 남는다).
export async function copyFiles(
  req: FilesCopyRequest,
  onProgress: (p: FilesCopyProgress) => void,
  signal?: AbortSignal,
): Promise<FilesCopyResult> {
  const res = await request('POST', '/files/copy', { body: req, signal })
  if (!res.body) throw new Error('/files/copy: no response body')
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader()
  let buffer = ''
  try {
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      const parsed = parseCopyEvents(buffer + value)
      buffer = parsed.rest
      for (const ev of parsed.events) {
        if (!isObj(ev)) continue
        if (isCopyProgress(ev.progress)) {
          onProgress(ev.progress)
          continue
        }
        const result = copyResultOf(ev.result)
        if (result) return result
        if (isObj(ev.error)) {
          const status = isNum(ev.status) && ev.status >= 400 ? ev.status : 503
          if (status === 401) onUnauthorized?.()
          throw apiErrorOf('/files/copy', status, ev.error)
        }
      }
    }
  } finally {
    reader.releaseLock()
  }
  throw new TypeError('/files/copy: stream ended before the result')
}

const isSearchHit = (v: unknown): v is FilesSearchHit =>
  isObj(v) && isStr(v.path) && isOneOf(v.type, [...FILE_TYPES, 'unknown'] as const)

const isSearchResult = (v: unknown): v is FilesSearchResult =>
  isObj(v) && isNum(v.dirs) && isOptional(v.skipped, isNum) && isOptional(v.truncated, isBool)

// SearchProgress는 검색 응답의 진행 하나다(새로 찾은 항목과 지금까지 읽은 폴더 수).
export interface SearchProgress {
  hits: FilesSearchHit[]
  dirs: number
  skipped: number
}

// searchFiles는 path 폴더와 그 아래에서 이름에 query가 든 항목을 찾는다(대소문자 무시). 찾은 항목은 onProgress로
// 오는 대로 받는다. 시작하기 전의 실패와 시작한 뒤 멈춘 실패는 ApiError로 던진다. signal로 끊으면 서버와 도우미가 멈춘다.
export async function searchFiles(
  path: string,
  query: string,
  onProgress: (p: SearchProgress) => void,
  signal?: AbortSignal,
): Promise<FilesSearchResult> {
  const url = `/files/search?path=${q(path)}&q=${q(query)}`
  const res = await request('GET', url, { signal })
  if (!res.body) throw new Error('/files/search: no response body')
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader()
  let buffer = ''
  try {
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      const parsed = parseCopyEvents(buffer + value)
      buffer = parsed.rest
      for (const ev of parsed.events) {
        if (!isObj(ev)) continue
        if (isSearchResult(ev.result)) return ev.result
        if (isObj(ev.error)) {
          const status = isNum(ev.status) && ev.status >= 400 ? ev.status : 503
          if (status === 401) onUnauthorized?.()
          throw apiErrorOf('/files/search', status, ev.error)
        }
        if (isNum(ev.dirs)) {
          const hits = Array.isArray(ev.hits) ? (ev.hits as unknown[]).filter(isSearchHit) : []
          onProgress({ hits, dirs: ev.dirs, skipped: isNum(ev.skipped) ? ev.skipped : 0 })
        }
      }
    }
  } finally {
    reader.releaseLock()
  }
  throw new TypeError('/files/search: stream ended before the result')
}

// 휴지통. 권한 도우미가 trash-put과 같은 곳·형식으로 다룬다(서버의 trash-list·trash-restore와 같은 휴지통).

const isTrashResult = (v: unknown): v is FilesTrashResult => isObj(v) && isStr(v.trash) && isStr(v.name)

const isTrashVolume = (v: unknown): v is TrashVolume =>
  isObj(v) && isStr(v.top) && isOneOf(v.kind, ['home', 'shared', 'own', 'none'] as const) && isBool(v.shared)

const isTrashItem = (v: unknown): v is TrashItem =>
  isObj(v) &&
  isStr(v.trash) &&
  isStr(v.name) &&
  isStr(v.path) &&
  isNum(v.deletedAt) &&
  isOneOf(v.type, FILE_TYPES) &&
  isNum(v.size) &&
  isNum(v.modTime) &&
  isOptional(v.badName, isBool)

// trashFile은 항목 하나를 휴지통으로 옮긴다. 그 디스크에 휴지통이 없으면 no_trash다.
export async function trashFile(path: string): Promise<FilesTrashResult> {
  const body: FilesTrashRequest = { path }
  const data = await postJSON('/files/trash', { body })
  if (!isTrashResult(data)) throw new Error('/files/trash: unexpected response shape')
  return data
}

// fetchTrashVolumes는 홈 폴더와 디스크마다 휴지통 종류와, 휴지통의 항목 수(정보를 읽지 않고 센 값)다.
export async function fetchTrashVolumes(signal?: AbortSignal): Promise<FilesTrashVolumes> {
  const data = await getJSON('/files/trash/volumes', { signal })
  if (!isObj(data) || !isArrayOf(data.volumes, isTrashVolume) || !isNum(data.items)) {
    throw new Error('/files/trash/volumes: unexpected response shape')
  }
  return { volumes: data.volumes, items: data.items }
}

// fetchTrash는 사용자의 모든 휴지통 항목이다(지운 시각이 늦은 것부터).
export async function fetchTrash(signal?: AbortSignal): Promise<FilesTrashList> {
  const data = await getJSON('/files/trash', { signal })
  if (!isObj(data) || !isArrayOf(data.items, isTrashItem) || !isBool(data.truncated)) {
    throw new Error('/files/trash: unexpected response shape')
  }
  return { items: data.items, truncated: data.truncated }
}

// restoreTrash는 휴지통 항목을 to(원래 경로나 같은 폴더의 새 이름)로 되돌린다. 있으면 file_exists다.
export function restoreTrash(trash: string, name: string, to: string): Promise<FileItem> {
  const body: FilesTrashRestoreRequest = { trash, name, to }
  return fileItemFrom('/files/trash/restore', postJSON('/files/trash/restore', { body }))
}

// purgeTrash는 휴지통 항목 하나를 영구 삭제한다.
export async function purgeTrash(trash: string, name: string): Promise<void> {
  const body: FilesTrashItemRequest = { trash, name }
  await request('POST', '/files/trash/purge', { body })
}

// emptyTrash는 사용자의 모든 휴지통을 비운다.
export async function emptyTrash(): Promise<void> {
  await request('POST', '/files/trash/empty')
}

const isSiteTrashDisk = (d: unknown): d is SiteTrashDisks['disks'][number] =>
  isObj(d) && isStr(d.name) && isStr(d.path) && isBool(d.shared)

const isSiteTrashDisks = (v: unknown): v is SiteTrashDisks => isObj(v) && isArrayOf(v.disks, isSiteTrashDisk)

// fetchSiteTrash는 디스크마다 공유 휴지통(.Trash)이 있는지다(관리자).
export async function fetchSiteTrash(signal?: AbortSignal): Promise<SiteTrashDisks> {
  const data = await getJSON('/site/files/trash', { signal })
  if (!isSiteTrashDisks(data)) throw new Error('/site/files/trash: unexpected response shape')
  return data
}

// prepareSiteTrash는 디스크 맨 위에 공유 휴지통 .Trash(1777)를 만든다(관리자, root 권한 상승 필요).
export async function prepareSiteTrash(disk: string): Promise<SiteTrashDisks> {
  const body: SiteTrashPrepareRequest = { disk }
  const data = await postJSON('/site/files/trash', { body })
  if (!isSiteTrashDisks(data)) throw new Error('/site/files/trash: unexpected response shape')
  return data
}

// fetchFileInfo는 정보 패널 내용(소유자, 그룹, 모드, ACL, 내 권한)이다.
export async function fetchFileInfo(path: string, signal?: AbortSignal): Promise<FileInfo> {
  const data = await getJSON(`/files/info?path=${q(path)}`, { signal })
  if (!isFileInfo(data)) throw new Error('/files/info: unexpected response shape')
  return data
}

// fileDownloadUrl은 내려받기 주소다. 브라우저의 내려받기 관리자가 받도록 <a download>로 연다.
export function fileDownloadUrl(path: string): string {
  return `${API_BASE}/files/download?path=${q(path)}`
}

// filePreviewUrl은 미리보기 주소다. 브라우저가 그리는 형식(이미지, 동영상, 오디오, PDF)으로 온다(<img>, <video> 등의 src).
export function filePreviewUrl(path: string): string {
  return `${API_BASE}/files/preview?path=${q(path)}`
}

// fetchPreviewBytes는 미리보기 내용의 앞 limit바이트다(텍스트 보기). size는 정보로 아는 파일 크기이고, 그보다 크면
// 범위로 앞부분만 받는다(크기 0인 파일은 범위를 만족하지 못하므로 범위 없이 받는다).
export async function fetchPreviewBytes(path: string, size: number, limit: number, signal?: AbortSignal): Promise<Uint8Array> {
  const range = size > limit ? `bytes=0-${limit - 1}` : undefined
  const res = await request('GET', `/files/preview?path=${q(path)}`, { signal, range })
  const bytes = new Uint8Array(await res.arrayBuffer())
  return bytes.length > limit ? bytes.subarray(0, limit) : bytes
}

export interface UploadOptions {
  overwrite: boolean
  onProgress: (sent: number) => void
  signal: AbortSignal
}

// checkUpload은 올리기의 사전 확인이다. 본문 없이 알 수 있는 거절(세션, 보안 확인, 이름, 범위)을 본문을 보내기 전에
// 받는다. PUT의 거절은 백엔드가 남은 본문을 받은 뒤에 답하므로 본문을 다 보낸 뒤에야 온다. 사용성을 위한 것이고 PUT이
// 모든 검사를 다시 한다.
export async function checkUpload(path: string, signal?: AbortSignal): Promise<void> {
  const body: FilesUploadCheckRequest = { path }
  await request('POST', '/files/upload/check', { body, signal })
}

// uploadFile은 파일 하나를 path에 올린다. 본문을 보내기 전에 checkUpload로 먼저 확인하므로, withStepUp으로 감싸면
// 보안 확인 대화상자가 본문을 보내기 전에 뜨고 확인 뒤 둘 다 다시 한다. 진행률을 받으려고 XHR로 보낸다(fetch는 올리기
// 진행을 알리지 않는다). 백엔드는 원시 본문(application/octet-stream)만 받는다(CSRF 보조 장치). 취소하면 AbortError다.
export async function uploadFile(path: string, file: Blob, opts: UploadOptions): Promise<FileItem> {
  await checkUpload(path, opts.signal)
  return sendUpload(path, file, opts)
}

function sendUpload(path: string, file: Blob, opts: UploadOptions): Promise<FileItem> {
  const url = `/files/upload?path=${q(path)}${opts.overwrite ? '&overwrite=1' : ''}`
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    const abort = () => xhr.abort()
    opts.signal.addEventListener('abort', abort, { once: true })
    const done = () => opts.signal.removeEventListener('abort', abort)
    xhr.open('PUT', `${API_BASE}${url}`)
    xhr.setRequestHeader('Content-Type', 'application/octet-stream')
    xhr.upload.onprogress = (e) => opts.onProgress(e.loaded)
    xhr.onload = () => {
      done()
      let body: unknown = null
      try {
        body = JSON.parse(xhr.responseText)
      } catch {
        // JSON이 아닌 응답(프록시 오류 페이지 등)
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        if (isFileItem(body)) resolve(body)
        else reject(new Error(`${url}: unexpected response shape`))
        return
      }
      const err = apiErrorOf(url, xhr.status, body)
      if (xhr.status === 401) onUnauthorized?.()
      reject(err)
    }
    xhr.onerror = () => {
      done()
      reject(new TypeError(`${url}: network error`))
    }
    xhr.onabort = () => {
      done()
      reject(new DOMException('upload aborted', 'AbortError'))
    }
    if (opts.signal.aborted) {
      done()
      reject(new DOMException('upload aborted', 'AbortError'))
      return
    }
    xhr.send(file)
  })
}

// apiErrorOf는 이미 읽은 오류 본문으로 ApiError를 만든다(XHR 응답용, toApiError와 같은 규칙).
function apiErrorOf(path: string, status: number, body: unknown): ApiError {
  let code: ErrorCode | undefined
  let retryAfter: number | undefined
  let required: RequiredLevel | undefined
  if (isObj(body)) {
    if (ERROR_CODES.includes(body.code as ErrorCode)) code = body.code as ErrorCode
    if (isNum(body.retryAfterSeconds) && body.retryAfterSeconds > 0) retryAfter = body.retryAfterSeconds
    if (isOneOf(body.required, ['password', 'twoFactor'] as const)) required = body.required
  }
  return new ApiError(status, `${path}: HTTP ${status}`, code, retryAfter, required)
}

// fetchFilesSettings는 파일 탐색기 전역 설정(공유 폴더 스냅숏, 디스크 최상위 보기)이다(관리자).
export async function fetchFilesSettings(signal?: AbortSignal): Promise<FilesSettings> {
  const data = await getJSON('/site/files', { signal })
  if (!isFilesSettings(data)) throw new Error('/site/files: unexpected response shape')
  return data
}

export async function saveFilesSettings(showDisks: boolean): Promise<FilesSettings> {
  const body: FilesSettingsRequest = { showDisks }
  const data = await (await request('PUT', '/site/files', { body })).json()
  if (!isFilesSettings(data)) throw new Error('/site/files: unexpected response shape')
  return data
}

// refreshFilesShares는 공유 폴더 스냅숏을 지금 OMV에서 받는다(관리자).
export async function refreshFilesShares(): Promise<FilesSettings> {
  const data = await postJSON('/site/files/refresh')
  if (!isFilesSettings(data)) throw new Error('/site/files/refresh: unexpected response shape')
  return data
}
