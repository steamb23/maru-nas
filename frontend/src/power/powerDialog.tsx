// 프로필 메뉴의 "전원"이 여는 데스크톱 대화상자. 조작을 보내면 대화상자를 닫고 전원 감시를 시작해
// App이 데스크톱 대신 전원 화면(PowerScreen.tsx)을 그린다. 내용은 PowerOptions.tsx.
import { openDialog } from '../shell/dialogs'
import { PowerBody, PowerTitle } from './PowerOptions'
import { createPowerDialogStore } from './dialogStore'
import { usePowerWatch } from './watchStore'

// 타일 세 개가 한 줄에 들어가는 너비(시안 A)
const DIALOG_WIDTH = 468

export function openPowerDialog(): void {
  const store = createPowerDialogStore()
  void openDialog<true>(
    { kind: 'desktop' },
    {
      title: <PowerTitle store={store} />,
      size: { width: DIALOG_WIDTH },
      className: 'power-modal',
      render: (ctl) => (
        <PowerBody
          store={store}
          onCancel={ctl.cancel}
          onSent={(action, modeKey) => {
            ctl.close(true)
            usePowerWatch.getState().start(action, modeKey)
          }}
        />
      ),
    },
  )
}
