// 명령형 대화상자(dialogs.ts)를 범위 하나에 그린다. Desktop과 Window가 범위 컨텍스트 안에 하나씩 둔다.
// scope는 바뀌지 않는 값으로 넘긴다(Window는 useMemo, Desktop은 상수).
import { type ReactNode, memo, useLayoutEffect, useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { Modal } from './Modal'
import {
  type DialogControl,
  type DialogEntry,
  type DialogScope,
  attachOutlet,
  detachOutlet,
  removeDialog,
  scopeId,
  settleDialog,
  useDialogStore,
} from './dialogs'

export function DialogOutlet({ scope }: { scope: DialogScope }) {
  const id = scopeId(scope)
  const entries = useDialogStore(useShallow((s) => s.entries.filter((e) => e.scope === id)))
  // 그리는 동안만 이 범위로 열 수 있다. 떼면 남은 대화상자는 null로 끝난다.
  useLayoutEffect(() => {
    attachOutlet(scope)
    return () => detachOutlet(scope)
  }, [scope])
  return entries.map((e) => <DialogView key={e.key} entry={e} scope={scope.kind} />)
}

const DialogView = memo(function DialogView({ entry, scope }: { entry: DialogEntry; scope: DialogScope['kind'] }) {
  const { key } = entry
  const ctl = useMemo<DialogControl<unknown>>(
    () => ({ close: (value) => settleDialog(key, value), cancel: () => settleDialog(key, null) }),
    [key],
  )
  const { render, footer, onClosed, ...options } = entry.spec
  return (
    <Modal
      {...options}
      scope={scope}
      open={entry.open}
      onCancel={ctl.cancel}
      footer={typeof footer === 'function' ? <Slot render={footer} ctl={ctl} /> : footer}
      onClosed={() => {
        onClosed?.()
        removeDialog(key)
      }}
    >
      <Slot render={render} ctl={ctl} />
    </Modal>
  )
})

// Slot은 render 함수를 컴포넌트처럼 그린다. 안에서 쓴 훅은 이 컴포넌트의 것이 된다.
function Slot({ render, ctl }: { render: (ctl: DialogControl<unknown>) => ReactNode; ctl: DialogControl<unknown> }) {
  return render(ctl)
}
