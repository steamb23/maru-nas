// 로그인 상태 저장소. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 로그인 상태는 localStorage·sessionStorage에 저장하지 않는다. 앱을 시작할 때마다
// restore()로 백엔드에 세션 유효 여부를 묻고, 세션 쿠키(HttpOnly)가 살아 있으면 복원한다.
// 그래서 새로고침이나 새 탭에서도 다시 로그인하지 않는다.
import { create } from 'zustand'
import * as client from '../api/client'
import type { ChallengeInfo, LoginResponse, SessionInfo } from '../api/types.gen'
import { initialWindowData, useWindowStore } from '../windows/store'

// checking: 세션 확인 중. offline: 백엔드에 연결할 수 없음.
// loggedOut: 로그인 화면. challenge: 2차 인증 입력 중. loggedIn: 데스크톱.
export type AuthStatus = 'checking' | 'offline' | 'loggedOut' | 'challenge' | 'loggedIn'

// 로그인 화면에 보여 줄 안내. expired: 세션이 만료되어 로그인 화면으로 돌아옴.
export type AuthNotice = 'expired' | null

export interface AuthData {
  status: AuthStatus
  user: SessionInfo | null
  challenge: ChallengeInfo | null
  notice: AuthNotice
  // 창 배치의 주인. 다른 사용자가 로그인하면 창을 모두 닫는다.
  windowsOwner: string | null
  // OMV 세션의 예상 만료 시각(이 브라우저의 Date.now() 기준 ms). 모르거나 자동 로그아웃이 꺼져 있으면 null.
  expiresAt: number | null
}

export interface AuthActions {
  // restore는 백엔드에 세션을 물어 상태를 정한다.
  restore: () => Promise<void>
  // login·verify는 실패하면 ApiError 등을 그대로 던진다. 문구는 화면이 정한다(messages.ts).
  login: (username: string, password: string) => Promise<void>
  verify: (response: string) => Promise<void>
  // cancelChallenge는 2차 인증을 그만두고 로그인 화면으로 돌아간다.
  cancelChallenge: () => void
  logout: () => Promise<void>
  // expire는 세션이 만료됐을 때(다른 요청이 401을 받음) 로그인 화면으로 보낸다. 창 배치는 남긴다.
  expire: () => void
  // sessionUpdated는 활동 신호·세션 확인 응답으로 받은 새 세션 정보(만료 시간)를 반영한다.
  sessionUpdated: (info: SessionInfo) => void
}

export type AuthStore = AuthData & AuthActions

// AuthApi는 저장소가 쓰는 API 호출이다. 테스트에서 바꿔 끼운다.
export interface AuthApi {
  fetchSession: () => Promise<SessionInfo>
  login: (username: string, password: string) => Promise<LoginResponse>
  verifyChallenge: (response: string) => Promise<LoginResponse>
  logout: () => Promise<void>
}

export interface AuthDeps {
  api: AuthApi
  // resetWindows는 창을 모두 닫는다.
  resetWindows: () => void
  // now는 현재 시각(ms)이다. 기본값은 Date.now.
  now?: () => number
}

export const initialAuthData: AuthData = {
  status: 'checking',
  user: null,
  challenge: null,
  notice: null,
  windowsOwner: null,
  expiresAt: null,
}

export function createAuthStore({ api, resetWindows, now = Date.now }: AuthDeps) {
  return create<AuthStore>()((set, get) => {
    const expiresAt = (info: SessionInfo) =>
      info.expiresInSeconds === undefined ? null : now() + info.expiresInSeconds * 1000

    const signedIn = (user: SessionInfo) => {
      const owner = get().windowsOwner
      if (owner !== null && owner !== user.username) resetWindows()
      set({
        status: 'loggedIn',
        user,
        challenge: null,
        notice: null,
        windowsOwner: user.username,
        expiresAt: expiresAt(user),
      })
    }

    const handleLogin = (res: LoginResponse) => {
      if (res.status === 'authenticated' && res.session) signedIn(res.session)
      else if (res.status === 'challengeRequired' && res.challenge)
        set({ status: 'challenge', challenge: res.challenge, notice: null })
    }

    return {
      ...initialAuthData,

      restore: async () => {
        // 연결 끊김 화면에서 다시 시도할 때는 화면을 바꾸지 않는다(깜빡이지 않게).
        if (get().status !== 'offline') set({ status: 'checking' })
        try {
          signedIn(await api.fetchSession())
        } catch (err) {
          if (err instanceof client.ApiError && err.status === 401) set({ status: 'loggedOut', user: null })
          else set({ status: 'offline' })
        }
      },

      login: async (username, password) => {
        handleLogin(await api.login(username, password))
      },

      verify: async (response) => {
        try {
          handleLogin(await api.verifyChallenge(response))
        } catch (err) {
          // 대기 중 로그인이 만료됐으면 처음부터 다시 로그인해야 한다.
          if (err instanceof client.ApiError && err.code === 'challenge_expired') {
            set({ status: 'loggedOut', challenge: null })
          }
          throw err
        }
      },

      cancelChallenge: () => {
        if (get().status === 'challenge') set({ status: 'loggedOut', challenge: null })
      },

      logout: async () => {
        try {
          await api.logout()
        } catch {
          // 백엔드에 닿지 않아도 화면은 로그인 화면으로 보낸다. 세션은 백엔드에서 만료된다.
        }
        resetWindows()
        set({ status: 'loggedOut', user: null, challenge: null, notice: null, windowsOwner: null, expiresAt: null })
      },

      expire: () => {
        if (get().status !== 'loggedIn') return
        set({ status: 'loggedOut', user: null, notice: 'expired', expiresAt: null })
      },

      sessionUpdated: (info) => {
        const user = get().user
        if (get().status !== 'loggedIn' || user?.username !== info.username) return
        set({ user: info, expiresAt: expiresAt(info) })
      },
    }
  })
}

export const useAuthStore = createAuthStore({
  api: client,
  resetWindows: () => useWindowStore.setState(initialWindowData),
})
