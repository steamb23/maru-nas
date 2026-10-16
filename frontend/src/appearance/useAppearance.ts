// 화면 설정을 화면에 반영하는 훅. App이 화면(로그인·데스크톱)과 관계없이 한 번 부른다.
//
// - 테마: 로그인한 동안은 사용자 값 → 관리자 기본 테마. 로그인 화면은 이 브라우저의 마지막 테마(힌트)를,
//   없으면 관리자 기본 테마를 쓴다(로그아웃했다고 테마가 바뀌어 번쩍이지 않게).
// - 애니메이션: 사용자가 끄면 CSS 전환도 끈다. Web Animations 재생(animationsEnabled)과 같은 기준이다.
// - 배경: 로그인한 동안은 사용자 바탕화면 → 관리자 기본 바탕화면 → 대시보드 기본. 로그인 화면·연결 끊김 화면은
//   로그인 배경. 세션 확인 중에는 마지막 바탕화면(힌트)을 보여 준다(대부분 로그인한 채 새로 고침한 경우다).
//   다만 그 세션이 이미 끝났을 것 같으면(로그아웃했거나 예상 만료 시각이 지남) 로그인 배경(힌트)을 보여 준다.
//   사용자 설정을 받기 전에는 배경을 바꾸지 않는다(null). 받은 뒤 한 번만 바뀌게 한다.
import { useEffect, useMemo } from 'react'
import { siteImageUrl, wallpaperImageUrl } from '../api/client'
import { useAuthStore } from '../auth/store'
import {
  applyAnimations,
  applyTheme,
  markWallpaperHintSignedOut,
  readLoginBackgroundHint,
  readThemeHint,
  readWallpaperHint,
  readWallpaperHintBeforeSession,
  writeLoginBackgroundHint,
  writeThemeHint,
  writeWallpaperHint,
} from './dom'
import { useSiteStore } from './siteStore'
import { allowsAnimations, useAppearanceStore } from './store'
import { effectiveTheme } from './theme'
import { DEFAULT_VIEW, type WallpaperView, imageUrlOf, resolveWallpaper } from './wallpapers'

// useSiteInfoLoader는 공개 전역 정보를 앱이 시작할 때와 로그인 화면·데스크톱으로 넘어갈 때마다 불러온다
// (연결 끊김 화면에서 돌아온 경우 포함). 세션 확인·연결 끊김·2차 인증 중에는 다시 부르지 않는다.
export function useSiteInfoLoader() {
  const status = useAuthStore((s) => s.status)
  const screen = status === 'loggedIn' || status === 'loggedOut' ? status : null
  useEffect(() => {
    void useSiteStore.getState().load()
  }, [screen])
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

// useAppliedAnimations는 화면 설정의 애니메이션 켜기·끄기를 문서에 적용한다. 설정을 받기 전과 로그아웃한 뒤에는
// 켜진 것으로 본다(animationsAllowed와 같다).
export function useAppliedAnimations() {
  const allowed = useAppearanceStore((s) => allowsAnimations(s.prefs))
  useEffect(() => applyAnimations(allowed), [allowed])
}

// WallpaperChoice는 지금 그릴 배경이다. view가 null이면 지금 배경을 그대로 둔다.
// fallback은 view의 이미지를 받지 못했을 때 대신 그릴 배경이다.
export interface WallpaperChoice {
  view: WallpaperView | null
  fallback: WallpaperView
}

// useWallpaperView는 지금 그릴 배경을 정한다.
export function useWallpaperView(): WallpaperChoice {
  const status = useAuthStore((s) => s.status)
  const username = useAuthStore((s) => s.user?.username)
  const expiresAt = useAuthStore((s) => s.expiresAt)
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
  // 바탕화면 힌트에는 세션의 예상 만료 시각을 함께 남긴다(활동으로 연장되면 다시 쓴다).
  useEffect(() => {
    if (desktopView && username) writeWallpaperHint(username, desktopView, expiresAt)
  }, [desktopView, username, expiresAt])
  // 로그인 화면으로 오면(로그아웃, 세션 만료, 끝난 세션으로 새로 고침) 세션이 끝났다고 표시한다.
  useEffect(() => {
    if (status === 'loggedOut') markWallpaperHintSignedOut()
  }, [status])

  switch (status) {
    case 'loggedIn':
      return { view: desktopView ?? readWallpaperHint(username), fallback: DEFAULT_VIEW }
    case 'checking': {
      // 바탕화면 이미지를 받지 못하면 세션이 예상과 달리 끝난 것이다(백엔드 재시작 등). 곧 로그인 화면이 나오므로
      // 기본 배경을 거치지 않고 로그인 배경으로 바로 바꾼다.
      const login = readLoginBackgroundHint()
      return { view: readWallpaperHintBeforeSession() ?? login, fallback: login ?? DEFAULT_VIEW }
    }
    default:
      return { view: loginView ?? readLoginBackgroundHint() ?? (site ? DEFAULT_VIEW : null), fallback: DEFAULT_VIEW }
  }
}
