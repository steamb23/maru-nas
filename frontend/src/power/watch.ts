// 전원 조작을 보낸 뒤 NAS가 내려갔다가 돌아오기를 기다리는 감시. 계산(step, screenView)과 저장소는 DOM에
// 의존하지 않아 node 환경에서 테스트한다. 화면은 PowerScreen.tsx가 그린다.
//
// 조작을 보내면 App이 데스크톱 대신 전원 화면을 그리고, 감시가 /api/health를 받는다.
//   - sent: 조작을 보냈다. health가 실패할 때까지(시스템이 내려갈 때까지) 기다린다
//   - down: 연결이 끊겼다. health가 답하면(다시 켜지거나 절전에서 깨어남) 페이지를 새로 고친다
// 새로 고치면 세션을 다시 확인하므로, 다시 시작한 뒤에는(세션이 메모리에만 있다) 로그인 화면이 나온다.
// 업그레이드 감시(upgrade/watch.ts)와 같은 방식이지만 업그레이드 결과 힌트는 남기지 않는다.
import type { ParseKeys } from 'i18next'
import { create } from 'zustand'
import type { IconName } from '../icons/resolver'
import type { PowerAction } from './power'

export type PowerPhase = 'idle' | 'sent' | 'down'

export interface PowerWatchState {
  phase: PowerPhase
  action: PowerAction | null
  // modeKey는 절전의 대기 방식 표시 이름이다(절전일 때만, 모르면 null).
  modeKey: ParseKeys | null
  // since는 지금 phase가 시작된 시각(ms)이다.
  since: number
}

// healthy: health 요청이 답함. unreachable: 연결 실패·5xx·시간 초과
export type PowerWatchEvent = { type: 'healthy' } | { type: 'unreachable' }

export const idlePowerWatch: PowerWatchState = { phase: 'idle', action: null, modeKey: null, since: 0 }

// step은 감시 상태에 사건 하나를 반영한다. reload면 페이지를 새로 고친다.
export function step(
  state: PowerWatchState,
  ev: PowerWatchEvent,
  now: number,
): { state: PowerWatchState; effect: 'none' | 'reload' } {
  switch (state.phase) {
    case 'idle':
      return { state, effect: 'none' }
    case 'sent':
      return ev.type === 'unreachable'
        ? { state: { ...state, phase: 'down', since: now }, effect: 'none' }
        : { state, effect: 'none' }
    case 'down':
      return ev.type === 'healthy' ? { state, effect: 'reload' } : { state, effect: 'none' }
  }
}

// POLL_MS는 health 요청 간격, REQUEST_TIMEOUT_MS는 요청 하나의 시간 제한이다(절전하면 응답 없이 걸린다).
export const POLL_MS = 2000
export const REQUEST_TIMEOUT_MS = 5000

// SENT_STUCK_MS가 지나도록 NAS가 응답하면 조작이 처리되지 않은 것으로 보고 대시보드로 돌아갈 수 있게 한다.
export const SENT_STUCK_MS = 2 * 60_000

// DOWN_STUCK_MS가 지나도록 다시 시작한 NAS가 돌아오지 않으면 확인할 곳을 안내한다.
export const DOWN_STUCK_MS = 5 * 60_000

// PowerScreenView는 전원 화면에 그릴 내용이다.
export interface PowerScreenView {
  titleKey: ParseKeys
  messageKey: ParseKeys
  // spinner면 진행 표시, 아니면 그 아이콘
  icon: 'spinner' | IconName
  // hintKey는 오래 걸릴 때의 안내다.
  hintKey?: ParseKeys
  // back이면 "대시보드로 돌아가기" 단추를 보인다(조작이 처리되지 않은 것 같을 때).
  back: boolean
}

// screenView는 감시 상태와 지금 시각으로 전원 화면의 내용을 정한다.
export function screenView(state: PowerWatchState, now: number): PowerScreenView {
  const elapsed = now - state.since
  const notDown = state.phase === 'sent' && elapsed > SENT_STUCK_MS
  const notDownHint = notDown ? { hintKey: 'power.screen.notDown' as const, back: true } : { back: false }
  switch (state.action) {
    case 'shutdown':
      return state.phase === 'down'
        ? {
            titleKey: 'power.screen.shutdownTitle',
            messageKey: 'power.screen.shutdownMessage',
            icon: 'system-shutdown-symbolic',
            back: false,
          }
        : {
            titleKey: 'power.screen.shuttingDownTitle',
            messageKey: 'power.screen.shutdownMessage',
            icon: 'spinner',
            ...notDownHint,
          }
    case 'standby':
      return state.phase === 'down'
        ? {
            titleKey: 'power.screen.standbyTitle',
            messageKey: 'power.screen.standbyMessage',
            icon: 'system-suspend-symbolic',
            back: false,
          }
        : {
            titleKey: 'power.screen.enteringStandbyTitle',
            messageKey: 'power.screen.standbyMessage',
            icon: 'spinner',
            ...notDownHint,
          }
    default: {
      const stuck = state.phase === 'down' && elapsed > DOWN_STUCK_MS
      return {
        titleKey: 'power.screen.rebootTitle',
        messageKey: 'power.screen.rebootMessage',
        icon: 'spinner',
        ...(stuck ? { hintKey: 'power.screen.rebootStuck', back: false } : notDownHint),
      }
    }
  }
}

export interface PowerWatchDeps {
  fetchHealth: (signal: AbortSignal) => Promise<void>
  // reload는 페이지를 새로 고친다.
  reload: () => void
  now?: () => number
  schedule?: (fn: () => void, ms: number) => () => void
}

export interface PowerWatchStore extends PowerWatchState {
  // start는 조작을 보낸 뒤 감시를 시작한다. 이미 감시 중이면 아무것도 하지 않는다.
  start: (action: PowerAction, modeKey?: ParseKeys) => void
  stop: () => void
}

export function createPowerWatch(deps: PowerWatchDeps) {
  const now = deps.now ?? Date.now
  const schedule =
    deps.schedule ??
    ((fn, ms) => {
      const id = setTimeout(fn, ms)
      return () => clearTimeout(id)
    })
  return create<PowerWatchStore>()((set, get) => {
    // generation은 start·stop마다 바뀌어 이전 감시의 늦은 응답을 버린다.
    let generation = 0
    let cancel: (() => void) | null = null
    let inflight: AbortController | null = null

    const apply = (gen: number, ev: PowerWatchEvent) => {
      if (gen !== generation) return
      const { state, effect } = step(get(), ev, now())
      set({ phase: state.phase, action: state.action, modeKey: state.modeKey, since: state.since })
      if (effect === 'reload') {
        generation++
        deps.reload()
      } else {
        cancel = schedule(() => void tick(gen), POLL_MS)
      }
    }

    const tick = async (gen: number) => {
      cancel = null
      const ctl = new AbortController()
      inflight = ctl
      const stopTimeout = schedule(() => ctl.abort(), REQUEST_TIMEOUT_MS)
      try {
        await deps.fetchHealth(ctl.signal)
        apply(gen, { type: 'healthy' })
      } catch {
        apply(gen, { type: 'unreachable' })
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
      ...idlePowerWatch,
      start: (action, modeKey) => {
        if (get().phase !== 'idle') return
        halt()
        const gen = generation
        set({ phase: 'sent', action, modeKey: modeKey ?? null, since: now() })
        cancel = schedule(() => void tick(gen), POLL_MS)
      },
      stop: () => {
        halt()
        set(idlePowerWatch)
      },
    }
  })
}
