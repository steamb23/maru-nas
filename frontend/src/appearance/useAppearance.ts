// 화면 설정을 화면에 반영하는 훅. App이 화면(로그인·데스크톱)과 관계없이 한 번 부른다.
//
// - 테마: 로그인한 동안은 사용자 값 → 관리자 기본 테마. 로그인 화면은 이 브라우저의 마지막 테마(힌트)를,
//   없으면 관리자 기본 테마를 쓴다(로그아웃했다고 테마가 바뀌어 번쩍이지 않게).
// - 배경: 로그인한 동안은 사용자 바탕화면 → 관리자 기본 바탕화면 → 대시보드 기본. 로그인 화면·연결 끊김 화면은
//   로그인 배경. 세션 확인 중에는 마지막 바탕화면(힌트)을 보여 준다(대부분 로그인한 채 새로 고침한 경우다).
//   사용자 설정을 받기 전에는 배경을 바꾸지 않는다(null). 받은 뒤 한 번만 바뀌게 한다.
import { useEffect, useMemo } from 'react'
import { siteImageUrl, wallpaperImageUrl } from '../api/client'
import { useAuthStore } from '../auth/store'
import {
  applyTheme,
  readLoginBackgroundHint,
  readThemeHint,
  readWallpaperHint,
  writeLoginBackgroundHint,
  writeThemeHint,
  writeWallpaperHint,
} from './dom'
import { useSiteStore } from './siteStore'
import { useAppearanceStore } from './store'
import { effectiveTheme } from './theme'
import { DEFAULT_VIEW, type WallpaperView, imageUrlOf, resolveWallpaper } from './wallpapers'

// useSiteInfo는 공개 전역 정보를 앱이 시작할 때와 로그인·로그아웃할 때 불러온다.
export function useSiteInfoLoader() {
  const status = useAuthStore((s) => s.status)
  const signedIn = status === 'loggedIn'
  useEffect(() => {
    void useSiteStore.getState().load()
  }, [signedIn])
}

// useAppearanceLoader는 로그인한 동안 사용자 화면 설정을 불러오고, 실패하면 다시 시도한다. SignedIn에서 부른다.
export function useAppearanceLoader() {
  const failed = useAppearanceStore((s) => s.status === 'error')
  useEffect(() => {
    const { load, clear } = useAppearanceStore.getState()
    void load()
    return clear
  }, [])
  useEffect(() => {
    if (!failed) return
    const id = window.setTimeout(() => void useAppearanceStore.getState().load(), 10_000)
    return () => window.clearTimeout(id)
  }, [failed])
}

// useAppliedTheme은 지금 화면에 맞는 테마를 문서에 적용한다.
export function useAppliedTheme() {
  const status = useAuthStore((s) => s.status)
  const ready = useAppearanceStore((s) => s.status === 'ready')
  const userTheme = useAppearanceStore((s) => s.prefs.theme)
  const siteTheme = useSiteStore((s) => s.info?.defaultTheme)
  const siteLoaded = useSiteStore((s) => s.info !== null)
  useEffect(() => {
    if (status === 'loggedIn') {
      if (!ready) return
      const mode = effectiveTheme(userTheme, siteTheme)
      applyTheme(mode)
      writeThemeHint(mode)
      return
    }
    // 로그인 화면: 힌트가 있으면 그대로, 없으면 관리자 기본 테마.
    const hint = readThemeHint()
    if (hint) applyTheme(hint)
    else if (siteLoaded) applyTheme(effectiveTheme(undefined, siteTheme))
  }, [status, ready, userTheme, siteTheme, siteLoaded])
}

// useWallpaperView는 지금 그릴 배경이다. null이면 지금 배경을 그대로 둔다.
export function useWallpaperView(): WallpaperView | null {
  const status = useAuthStore((s) => s.status)
  const username = useAuthStore((s) => s.user?.username)
  const site = useSiteStore((s) => s.info)
  const ready = useAppearanceStore((s) => s.status === 'ready')
  const userWallpaper = useAppearanceStore((s) => s.prefs.wallpaper)
  const userImage = useAppearanceStore((s) => s.image)

  const loginView = useMemo(() => {
    if (!site) return null
    return resolveWallpaper([
      {
        wallpaper: site.loginBackground,
        imageUrl: imageUrlOf(site.images.loginBackground, (rev) => siteImageUrl('login-background', rev)),
      },
    ])
  }, [site])

  const desktopView = useMemo(() => {
    if (!ready || !site) return null
    return resolveWallpaper([
      { wallpaper: userWallpaper, imageUrl: imageUrlOf(userImage, wallpaperImageUrl) },
      {
        wallpaper: site.defaultWallpaper,
        imageUrl: imageUrlOf(site.images.defaultWallpaper, (rev) => siteImageUrl('default-wallpaper', rev)),
      },
    ])
  }, [ready, site, userWallpaper, userImage])

  useEffect(() => {
    if (loginView) writeLoginBackgroundHint(loginView)
  }, [loginView])
  useEffect(() => {
    if (desktopView && username) writeWallpaperHint(username, desktopView)
  }, [desktopView, username])

  switch (status) {
    case 'loggedIn':
      return desktopView ?? readWallpaperHint(username)
    case 'checking':
      return readWallpaperHint() ?? readLoginBackgroundHint()
    default:
      return loginView ?? readLoginBackgroundHint() ?? (site ? DEFAULT_VIEW : null)
  }
}
