// 업그레이드를 요청한 뒤 onmaru가 다시 시작하기를 기다리는 감시. 계산(step)과 저장소는 DOM에 의존하지 않아 node
// 환경에서 테스트한다. 재시작 화면은 RestartScreen.tsx가 그린다.
//
// 업그레이드 서비스는 설치 단계에서 onmaru와 권한 도우미를 다시 시작하고, 세션은 메모리에만 있으므로 모두 끝난다.
// 그래서 감시는 설정 창(페이지)과 따로 모듈 수준에 두고, 그 사이 401로 데스크톱이 내려가도 계속 돈다.
//   - watching: 상태를 2초마다 받는다(페이지가 진행을 보여 준다)
//   - restarting: 설치 단계라 곧 다시 시작한다. 재시작 화면을 보이고 상태를 계속 받는다
//   - down: 연결이 끊겼다. 재시작 화면을 보이고 /api/health를 받는다
// 상태 요청이 401이면(세션이 사라짐 = 다시 시작함) 또는 끊긴 뒤 health가 답하면, 새 프론트 자산을 받도록 페이지를
// 새로 고친다(해시가 바뀐 청크·카탈로그). 다시 로그인하면 업데이트 페이지가 결과를 보인다(resultHint.ts).
import { create } from 'zustand'
import type { UpgradeInfo } from '../api/types.gen'

export type WatchPhase = 'idle' | 'watching' | 'restarting' | 'down'

export interface WatchState {
  phase: WatchPhase
  // target은 업그레이드를 요청한 대상 커밋이다.
  target: string | null
  // since는 지금 phase가 시작된 시각(ms)이다.
  since: number
  // info는 마지막으로 받은 상태다. 감시하는 동안 페이지가 이것을 보여 준다.
  info: UpgradeInfo | null
}

export type WatchEvent =
  | { type: 'status'; info: UpgradeInfo }
  // unauthorized: 상태 요청이 401(세션이 사라짐)
  | { type: 'unauthorized' }
  // unreachable: 상태·health 요청이 연결 실패·5xx·시간 초과
  | { type: 'unreachable' }
  // healthy: health 요청이 답함
  | { type: 'healthy' }

// reload: 페이지를 새로 고친다. done: 감시를 끝낸다(페이지가 결과를 보인다).
export type WatchEffect = 'none' | 'reload' | 'done'

export const idleWatch: WatchState = { phase: 'idle', target: null, since: 0, info: null }

// startWatch는 target으로 업그레이드를 요청한 뒤의 감시 상태다.
export function startWatch(target: string, info: UpgradeInfo | null, now: number): WatchState {
  return { phase: 'watching', target, since: now, info }
}

function enter(state: WatchState, phase: WatchPhase, now: number, info = state.info): WatchState {
  return { ...state, phase, info, since: phase === state.phase ? state.since : now }
}

// step은 감시 상태에 사건 하나를 반영한다.
export function step(state: WatchState, ev: WatchEvent, now: number): { state: WatchState; effect: WatchEffect } {
  if (state.phase === 'idle') return { state, effect: 'none' }
  switch (ev.type) {
    case 'status': {
      const st = ev.info.status
      const ours = !!st?.apply && st.apply.target === state.target
      if (ours && (st.state === 'applied' || st.state === 'apply-failed' || ev.info.interrupted)) {
        return { state: { ...idleWatch, info: ev.info }, effect: 'done' }
      }
      // 설치 단계부터는 재시작 화면을 보인다. 서비스가 아직 요청을 가져가지 않았으면(이전 상태) 그대로 기다린다.
      const installing = ours && st.state === 'applying' && st.stage === 'install'
      const phase = installing || state.phase === 'restarting' ? 'restarting' : 'watching'
      return { state: enter(state, phase, now, ev.info), effect: 'none' }
    }
    case 'unauthorized':
      return { state, effect: 'reload' }
    case 'unreachable':
      return { state: enter(state, 'down', now), effect: 'none' }
    case 'healthy':
      return state.phase === 'down' ? { state, effect: 'reload' } : { state, effect: 'none' }
  }
}

// showsRestartScreen은 재시작 화면을 보일지다.
export function showsRestartScreen(phase: WatchPhase): boolean {
  return phase === 'restarting' || phase === 'down'
}

// STUCK_MS가 지나도록 끊겨 있으면 재시작 화면에 복구 방법을 함께 보인다.
export const STUCK_MS = 5 * 60_000

// POLL_MS는 감시 중 요청 간격이다.
export const POLL_MS = 2000

export interface WatchDeps {
  fetchStatus: (signal: AbortSignal) => Promise<UpgradeInfo>
  fetchHealth: (signal: AbortSignal) => Promise<void>
  // classify는 요청 오류를 사건으로 바꾼다(401이면 unauthorized, 그 밖은 unreachable).
  classify: (err: unknown) => 'unauthorized' | 'unreachable'
  // reload는 결과 힌트를 남기고 페이지를 새로 고친다.
  reload: () => void
  now?: () => number
  schedule?: (fn: () => void, ms: number) => () => void
}

export interface WatchStore extends WatchState {
  // start는 업그레이드를 요청한 뒤(또는 진행 중인 업그레이드를 발견했을 때) 감시를 시작한다. 이미 그 대상을 보고
  // 있으면 아무것도 하지 않는다.
  start: (target: string, info: UpgradeInfo | null) => void
  stop: () => void
}

export function createUpgradeWatch(deps: WatchDeps) {
  const now = deps.now ?? Date.now
  const schedule =
    deps.schedule ??
    ((fn, ms) => {
      const id = setTimeout(fn, ms)
      return () => clearTimeout(id)
    })
  return create<WatchStore>()((set, get) => {
    // generation은 start·stop마다 바뀌어 이전 감시의 늦은 응답을 버린다.
    let generation = 0
    let cancel: (() => void) | null = null
    let inflight: AbortController | null = null

    const apply = (gen: number, ev: WatchEvent) => {
      if (gen !== generation) return
      const { state, effect } = step(get(), ev, now())
      set({ phase: state.phase, target: state.target, since: state.since, info: state.info })
      if (effect === 'reload') {
        generation++
        deps.reload()
      } else if (effect === 'done') {
        generation++
      } else {
        cancel = schedule(() => void tick(gen), POLL_MS)
      }
    }

    const tick = async (gen: number) => {
      cancel = null
      const ctl = new AbortController()
      inflight = ctl
      const stopTimeout = schedule(() => ctl.abort(), 10_000)
      try {
        if (get().phase === 'down') {
          await deps.fetchHealth(ctl.signal)
          apply(gen, { type: 'healthy' })
        } else {
          apply(gen, { type: 'status', info: await deps.fetchStatus(ctl.signal) })
        }
      } catch (err) {
        apply(gen, { type: deps.classify(err) })
      } finally {
        stopTimeout()
        if (inflight === ctl) inflight = null
      }
    }

    const halt = () => {
      generation++
      cancel?.()
      cancel = null
      inflight?.abort()
      inflight = null
    }

    return {
      ...idleWatch,
      start: (target, info) => {
        const s = get()
        if (s.phase !== 'idle' && s.target === target) return
        halt()
        const gen = generation
        set(startWatch(target, info, now()))
        cancel = schedule(() => void tick(gen), POLL_MS)
      },
      stop: () => {
        halt()
        set(idleWatch)
      },
    }
  })
}
