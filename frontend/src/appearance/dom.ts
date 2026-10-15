// 화면 설정을 문서에 적용하고, 첫 그림 깜빡임을 줄이는 힌트를 브라우저에 둔다(DOM·localStorage).
// 컴포넌트와 main.tsx에서만 import한다. 계산은 theme.ts·wallpapers.ts가 한다.
//
// 힌트는 이 브라우저에서 마지막으로 쓴 값이다. 새로 고침하면 서버 응답을 받기 전까지 힌트로 그리고,
// 받은 뒤에는 서버 값을 따른다. 저장소에 접근할 수 없으면(사생활 보호 모드 등) 힌트 없이 동작한다.
import { type ThemeMode, toThemeMode } from './theme'
import { type WallpaperView, toWallpaperView } from './wallpapers'

const THEME_KEY = 'onmaru.theme'
const LOGIN_BACKGROUND_KEY = 'onmaru.loginBackground'
const WALLPAPER_KEY = 'onmaru.wallpaper'

// applyTheme은 <html data-theme>로 테마를 정한다. auto는 속성을 지워 브라우저(OS) 설정을 따르게 한다.
export function applyTheme(mode: ThemeMode) {
  const root = document.documentElement
  if (mode === 'auto') delete root.dataset.theme
  else root.dataset.theme = mode
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

// 바탕화면 힌트는 사용자 이름과 함께 둔다. 다른 사용자가 로그인하면 쓰지 않는다.
export function readWallpaperHint(username?: string): WallpaperView | null {
  const v = readJSON(WALLPAPER_KEY)
  if (typeof v !== 'object' || v === null) return null
  const { user, view } = v as { user?: unknown; view?: unknown }
  if (username !== undefined && user !== username) return null
  return toWallpaperView(view)
}

export function writeWallpaperHint(username: string, view: WallpaperView) {
  write(WALLPAPER_KEY, JSON.stringify({ user: username, view }))
}
