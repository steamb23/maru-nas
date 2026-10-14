import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { ApiError, runAddonAction } from '../../api/client'
import type { AddonId } from '../manifests.gen'
import {
  type ActionErrorCode,
  type ActionState,
  INITIAL_ACTION_STATE,
  askConfirm,
  cancelConfirm,
  dismiss,
  finish,
  isBusy,
  settle,
  start,
} from './actionState'
import { needsConfirm } from './actionSpec'

export interface AddonActions {
  state: ActionState
  // request는 조작을 고른다. manifest가 확인을 요구하면 확인을 열고, 아니면 바로 실행한다.
  request: (target: string, action: string) => void
  // confirm은 열린 확인의 조작을 실행한다.
  confirm: () => void
  cancel: () => void
  dismiss: (target: string) => void
  // settle은 새 현황 응답을 받았을 때 부른다. 성공한 조작의 잠금을 푼다.
  settle: () => void
}

function errorCode(err: unknown): ActionErrorCode {
  return err instanceof ApiError && err.code ? err.code : 'unknown'
}

// useAddonActions는 애드온 하나의 조작 상태와 실행을 다룬다. 조작이 성공하면 onDone을 부른다
// (보통 현황을 바로 다시 가져오는 refresh). 창을 닫아도 백엔드의 조작은 끝까지 진행된다.
export function useAddonActions(id: AddonId, onDone: () => void): AddonActions {
  const [state, setState] = useState<ActionState>(INITIAL_ACTION_STATE)
  // 최신 상태를 이벤트 처리기에서 읽는다(중복 실행 방지). 상태는 update로만 바꾸므로 늘 같다.
  const stateRef = useRef(state)
  const onDoneRef = useRef(onDone)
  useLayoutEffect(() => {
    onDoneRef.current = onDone
  })
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const update = useCallback((fn: (s: ActionState) => ActionState) => {
    stateRef.current = fn(stateRef.current)
    setState(stateRef.current)
  }, [])

  const run = useCallback(
    (target: string, action: string) => {
      if (isBusy(stateRef.current, target)) return
      update((s) => start(s, target, action))
      runAddonAction(id, action, target).then(
        () => {
          if (!mounted.current) return
          update((s) => finish(s, target, null))
          onDoneRef.current()
        },
        (err: unknown) => {
          if (!mounted.current) return
          update((s) => finish(s, target, errorCode(err)))
          // 실패해도 상태가 바뀌었을 수 있다(일부 진행, 시간 초과).
          onDoneRef.current()
        },
      )
    },
    [id, update],
  )

  const request = useCallback(
    (target: string, action: string) => {
      if (needsConfirm(id, action)) update((s) => askConfirm(s, { target, action }))
      else run(target, action)
    },
    [id, run, update],
  )

  const confirm = useCallback(() => {
    const c = stateRef.current.confirm
    if (!c) return
    update(cancelConfirm)
    run(c.target, c.action)
  }, [run, update])

  return {
    state,
    request,
    confirm,
    cancel: useCallback(() => update(cancelConfirm), [update]),
    dismiss: useCallback((target: string) => update((s) => dismiss(s, target)), [update]),
    settle: useCallback(() => update(settle), [update]),
  }
}
