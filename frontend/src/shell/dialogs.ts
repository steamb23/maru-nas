// 명령형 대화상자 저장소. await로 결과를 받는 대화상자를 연다. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 범위마다 DialogOutlet 하나가 자기 범위의 대화상자를 Modal로 그린다(Desktop, Window). 그릴 곳(outlet)이 없는
// 범위로 열면 바로 null로 끝난다. 범위가 사라지면(창 닫힘, 로그아웃) 남은 대화상자도 null로 끝난다.
// 확인·알림 같은 기본 형태와 컴포넌트에서 쓰는 훅(useDialogs)은 dialogPresets.tsx에 있다.
import type { ReactNode } from 'react'
import { create } from 'zustand'
import type { ModalOptions } from './Modal'

export type DialogScope = { kind: 'desktop' } | { kind: 'window'; windowId: string }

export function scopeId(scope: DialogScope): string {
  return scope.kind === 'desktop' ? 'desktop' : `window:${scope.windowId}`
}

// DialogControl은 대화상자 내용이 받는 조작이다. close의 값이 openDialog의 결과가 되고, cancel은 null이다.
export interface DialogControl<T> {
  close: (value: T) => void
  cancel: () => void
}

// DialogSpec은 대화상자 하나다. render·footer는 컴포넌트처럼 그려지므로 안에서 훅을 써도 된다.
// 나머지는 Modal과 같은 설정이다(범위는 openDialog의 인자로 정한다).
export interface DialogSpec<T> extends Omit<ModalOptions, 'scope' | 'footer'> {
  render: (ctl: DialogControl<T>) => ReactNode
  footer?: ReactNode | ((ctl: DialogControl<T>) => ReactNode)
}

export interface DialogEntry {
  key: number
  // scopeId
  scope: string
  spec: DialogSpec<unknown>
  // 결과가 정해지면 false. 닫는 애니메이션이 끝나면(removeDialog) 목록에서 빠진다.
  open: boolean
}

interface DialogState {
  entries: readonly DialogEntry[]
  // 지금 그려지고 있는 범위(scopeId)
  outlets: readonly string[]
}

export const useDialogStore = create<DialogState>(() => ({ entries: [], outlets: [] }))

// 아직 결과를 받지 않은 대화상자의 resolve. 함수라 저장소 상태에 두지 않는다.
const pending = new Map<number, (value: unknown) => void>()
let nextKey = 1

// openDialog는 scope에 대화상자를 연다. close(값)이면 그 값, 취소·Esc·배경 누르기·범위 소멸이면 null로 끝난다.
export function openDialog<T>(scope: DialogScope, spec: DialogSpec<T>): Promise<T | null> {
  const id = scopeId(scope)
  if (!useDialogStore.getState().outlets.includes(id)) return Promise.resolve(null)
  const key = nextKey++
  return new Promise<T | null>((resolve) => {
    pending.set(key, resolve as (value: unknown) => void)
    useDialogStore.setState((s) => ({
      entries: [...s.entries, { key, scope: id, spec: spec as DialogSpec<unknown>, open: true }],
    }))
  })
}

// settleDialog는 결과를 정하고 닫기 시작한다. 이미 정해졌으면 무시한다.
export function settleDialog(key: number, value: unknown) {
  const resolve = pending.get(key)
  if (!resolve) return
  pending.delete(key)
  useDialogStore.setState((s) => ({ entries: s.entries.map((e) => (e.key === key ? { ...e, open: false } : e)) }))
  resolve(value)
}

// removeDialog는 닫는 애니메이션이 끝난 대화상자를 목록에서 뺀다.
export function removeDialog(key: number) {
  settleDialog(key, null)
  useDialogStore.setState((s) => ({ entries: s.entries.filter((e) => e.key !== key) }))
}

// dismissScope는 scope의 열린 대화상자를 모두 취소한다(닫는 애니메이션은 재생된다).
export function dismissScope(scope: DialogScope) {
  const id = scopeId(scope)
  for (const e of useDialogStore.getState().entries) if (e.scope === id) settleDialog(e.key, null)
}

// attachOutlet·detachOutlet은 DialogOutlet이 마운트·언마운트될 때 부른다. 떼면 그 범위의 대화상자를 모두
// null로 끝내고 지운다(그릴 곳이 없어 애니메이션도 없다).
export function attachOutlet(scope: DialogScope) {
  const id = scopeId(scope)
  useDialogStore.setState((s) => (s.outlets.includes(id) ? s : { outlets: [...s.outlets, id] }))
}

export function detachOutlet(scope: DialogScope) {
  const id = scopeId(scope)
  dismissScope(scope)
  useDialogStore.setState((s) => ({
    outlets: s.outlets.filter((o) => o !== id),
    entries: s.entries.filter((e) => e.scope !== id),
  }))
}
