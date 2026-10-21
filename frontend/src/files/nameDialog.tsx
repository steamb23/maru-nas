// 폴더 만들기·이름 바꾸기 창 범위 대화상자. 만들거나 바꾼 항목, 취소하면 null로 끝난다. 내용은 NameForm.tsx.
import { createRef } from 'react'
import type { FileItem } from '../api/types.gen'
import { type DialogScope, openDialog } from '../shell/dialogs'
import { NameForm, NameTitle } from './NameForm'
import { stemLength } from './paths'

export interface NameDialogOptions {
  mode: 'mkdir' | 'rename'
  initial: string
  // folder면 이름 전체를 고른다(이름 바꾸기).
  folder: boolean
  submit: (name: string) => Promise<FileItem>
}

export function askName(scope: DialogScope, opts: NameDialogOptions): Promise<FileItem | null> {
  const inputRef = createRef<HTMLInputElement>()
  const selectEnd = opts.mode === 'mkdir' ? opts.initial.length : stemLength(opts.initial, opts.folder)
  return openDialog<FileItem>(scope, {
    title: <NameTitle mode={opts.mode} />,
    initialFocus: inputRef,
    render: (ctl) => (
      <NameForm
        mode={opts.mode}
        initial={opts.initial}
        selectEnd={selectEnd}
        inputRef={inputRef}
        submit={opts.submit}
        onDone={ctl.close}
        onCancel={ctl.cancel}
      />
    ),
  })
}
