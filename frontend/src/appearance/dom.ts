// 화면 설정을 문서에 적용하고, 첫 그림 깜빡임을 줄이는 힌트를 브라우저에 둔다(DOM·localStorage).
// 컴포넌트와 main.tsx에서만 import한다. 계산은 theme.ts·wallpapers.ts가 한다.
//
// 힌트는 이 브라우저에서 마지막으로 쓴 값이다. 새로 고침하면 서버 응답을 받기 전까지 힌트로 그리고,
// 받은 뒤에는 서버 값을 따른다. 저장소에 접근할 수 없으면(사생활 보호 모드 등) 힌트 없이 동작한다.
import { type ThemeMode, toThemeMode } from './theme'
import {
  type WallpaperHint,
  type WallpaperView,
  toWallpaperHint,
  toWallpaperView,
  viewBeforeSessionCheck,
} from './wallpapers'

const THEME_KEY = 'onmaru.theme'
const LOGIN_BACKGROUND_KEY = 'onmaru.loginBackground'
const WALLPAPER_KEY = 'onmaru.wallpaper'

// applyTheme은 <html data-theme>로 테마를 정한다. auto는 속성을 지워 브라우저(OS) 설정을 따르게 한다.
export function applyTheme(mode: ThemeMode) {
  const root = document.documentElement
  if (mode === 'auto') delete root.dataset.theme
  else root.dataset.theme = mode
}

// applyAnimations는 화면 설정에서 애니메이션을 끄면 <html data-animations="off">를 둔다. base.css가 이 속성으로
// CSS 전환을 모두 끈다. 시스템의 '동작 줄이기'는 CSS가 미디어 쿼리로 직접 따르므로 여기서 보지 않는다.
export function applyAnimations(allowed: boolean) {
  const root = document.documentElement
  if (allowed) delete root.dataset.animations
  else root.dataset.animations = 'off'
}

function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, value: string) {
  try {
    if (localStorage.getItem(key) !== value) localStorage.setItem(key, value)
  } catch {
    // 힌트는 없어도 된다.
  }
}

function readJSON(key: string): unknown {
  try {
    return JSON.parse(read(key) ?? 'null')
  } catch {
    return null
  }
}

export function readThemeHint(): ThemeMode | null {
  return toThemeMode(read(THEME_KEY))
}

export function writeThemeHint(mode: ThemeMode) {
  write(THEME_KEY, mode)
}

export function readLoginBackgroundHint(): WallpaperView | null {
  return toWallpaperView(readJSON(LOGIN_BACKGROUND_KEY))
}

export function writeLoginBackgroundHint(view: WallpaperView) {
  write(LOGIN_BACKGROUND_KEY, JSON.stringify(view))
}

// 바탕화면 힌트는 사용자 이름, 세션의 예상 만료 시각과 함께 둔다(wallpapers.ts의 WallpaperHint).
function readHint(): WallpaperHint | null {
  return toWallpaperHint(readJSON(WALLPAPER_KEY))
}

function writeHint(hint: WallpaperHint) {
  write(WALLPAPER_KEY, JSON.stringify(hint))
}

// readWallpaperHint는 로그인한 사용자의 마지막 바탕화면이다. 다른 사용자가 로그인하면 쓰지 않는다.
export function readWallpaperHint(username: string | undefined): WallpaperView | null {
  const hint = readHint()
  return hint && hint.user === username ? hint.view : null
}

// readWallpaperHintBeforeSession은 세션을 확인하는 동안 그릴 마지막 바탕화면이다. 세션이 끝났을 것 같으면 null.
export function readWallpaperHintBeforeSession(): WallpaperView | null {
  return viewBeforeSessionCheck(readHint(), Date.now())
}

// writeWallpaperHint는 지금 바탕화면을 남긴다. until은 세션의 예상 만료 시각(모르면 null)이다.
export function writeWallpaperHint(username: string, view: WallpaperView, until: number | null) {
  writeHint({ user: username, view, until })
}

// markWallpaperHintSignedOut은 세션이 끝났다고 표시한다(로그아웃, 세션 만료). 다음에 새로 고침하면 세션을
// 확인하는 동안 바탕화면 대신 로그인 배경을 그린다. 바탕화면은 남겨 두어 다시 로그인한 직후에 쓴다.
export function markWallpaperHintSignedOut() {
  const hint = readHint()
  if (hint && hint.until !== 0) writeHint({ ...hint, until: 0 })
}
