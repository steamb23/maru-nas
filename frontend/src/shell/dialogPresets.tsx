// 명령형 대화상자의 기본 형태(문구 + 단추, 확인, 알림)와, 컴포넌트에서 범위에 묶어 쓰는 훅.
// 모두 dialogs.ts의 openDialog 위에 만든 것이라, 더 복잡한 내용은 openDialog로 직접 그린다.
import { type ReactNode, useContext, useMemo } from 'react'
import type { ModalOptions } from './Modal'
import { type MessageButton, MessageButtons, MessageText } from './MessageDialog'
import { type DialogScope, type DialogSpec, openDialog } from './dialogs'
import { type ModalScope, WindowModalContext } from './modalScope'

type PresetOptions = Omit<ModalOptions, 'scope' | 'footer' | 'title'>

export interface MessageDialogOptions extends PresetOptions {
  title: ReactNode
  message: ReactNode
  buttons: readonly MessageButton[]
  focusId?: string
}

// messageDialog는 누른 단추의 id로 끝난다. 취소면 null이다.
export function messageDialog(scope: DialogScope, opts: MessageDialogOptions): Promise<string | null> {
  const { message, buttons, focusId, ...options } = opts
  return openDialog<string>(scope, {
    role: 'alertdialog',
    ...options,
    render: () => <MessageText>{message}</MessageText>,
    footer: (ctl) => <MessageButtons buttons={buttons} focusId={focusId} onPress={ctl.close} />,
  })
}

export interface ConfirmDialogOptions extends PresetOptions {
  title: ReactNode
  message: ReactNode
  // 기본: 확인 / 취소
  confirmLabel?: ReactNode
  cancelLabel?: ReactNode
  // 확인 단추를 위험 색으로 그리고 취소에 포커스한다.
  danger?: boolean
}

// confirmDialog는 확인을 누르면 true, 그 밖에는 false로 끝난다.
export async function confirmDialog(scope: DialogScope, opts: ConfirmDialogOptions): Promise<boolean> {
  const { confirmLabel, cancelLabel, danger, ...rest } = opts
  const id = await messageDialog(scope, {
    ...rest,
    buttons: [
      { id: 'cancel', label: cancelLabel, labelKey: 'dialog.cancel' },
      { id: 'confirm', label: confirmLabel, labelKey: 'dialog.ok', tone: danger ? 'danger' : 'primary' },
    ],
    focusId: danger ? 'cancel' : 'confirm',
  })
  return id === 'confirm'
}

export interface AlertDialogOptions extends PresetOptions {
  title: ReactNode
  message: ReactNode
  okLabel?: ReactNode
}

// alertDialog는 닫히면 끝난다.
export async function alertDialog(scope: DialogScope, opts: AlertDialogOptions): Promise<void> {
  const { okLabel, ...rest } = opts
  await messageDialog(scope, {
    ...rest,
    buttons: [{ id: 'ok', label: okLabel, labelKey: 'dialog.ok', tone: 'primary' }],
    focusId: 'ok',
  })
}

export interface Dialogs {
  scope: DialogScope
  open: <T>(spec: DialogSpec<T>) => Promise<T | null>
  message: (opts: MessageDialogOptions) => Promise<string | null>
  confirm: (opts: ConfirmDialogOptions) => Promise<boolean>
  alert: (opts: AlertDialogOptions) => Promise<void>
}

// useDialogs는 범위에 묶인 명령형 대화상자 함수다. scope가 없으면 창 보기 안에서는 그 창, 밖에서는 데스크톱이다.
// 창 밖에서 'window'를 고르면 데스크톱으로 연다.
export function useDialogs(opts?: { scope?: ModalScope }): Dialogs {
  const windowId = useContext(WindowModalContext)?.windowId
  const kind = opts?.scope ?? (windowId ? 'window' : 'desktop')
  return useMemo(() => {
    const scope: DialogScope = kind === 'window' && windowId ? { kind: 'window', windowId } : { kind: 'desktop' }
    return {
      scope,
      open: (spec) => openDialog(scope, spec),
      message: (o) => messageDialog(scope, o),
      confirm: (o) => confirmDialog(scope, o),
      alert: (o) => alertDialog(scope, o),
    }
  }, [kind, windowId])
}
