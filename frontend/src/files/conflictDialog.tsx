// 올리기 이름 충돌을 묻는 창 범위 대화상자. 덮어쓰기·건너뛰기와, 남은 파일에도 같게 적용할지를 돌려준다.
// 닫으면(취소, Esc) null이고 남은 파일을 올리지 않는다.
import { createRef } from 'react'
import { MessageButtons } from '../shell/MessageDialog'
import { type DialogScope, openDialog } from '../shell/dialogs'
import { ConflictBody, ConflictTitle } from './ConflictBody'

export interface ConflictChoice {
  action: 'overwrite' | 'skip'
  all: boolean
}

export function askConflict(scope: DialogScope, name: string, many: boolean): Promise<ConflictChoice | null> {
  let all = false
  const focusRef = createRef<HTMLButtonElement>()
  return openDialog<ConflictChoice>(scope, {
    role: 'alertdialog',
    title: <ConflictTitle />,
    initialFocus: focusRef,
    render: () => <ConflictBody name={name} many={many} onApplyAll={(v) => (all = v)} />,
    footer: (ctl) => (
      <MessageButtons
        buttons={[
          { id: 'skip', labelKey: 'files.upload.skip' },
          { id: 'overwrite', labelKey: 'files.upload.overwrite', tone: 'primary' },
        ]}
        focusId="skip"
        focusRef={focusRef}
        onPress={(id) => ctl.close({ action: id === 'overwrite' ? 'overwrite' : 'skip', all })}
      />
    ),
  })
}
