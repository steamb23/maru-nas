// 프로필 메뉴의 "전원"이 여는 데스크톱 대화상자. 조작을 보내면 대화상자를 닫고 전원 감시를 시작해
// App이 데스크톱 대신 전원 화면(PowerScreen.tsx)을 그린다. 다시 시작을 예약하면 감시 없이 닫는다.
// 내용은 PowerOptions.tsx.
import { openDialog } from '../shell/dialogs'
import { PowerBody, PowerTitle } from './PowerOptions'
import { type PowerWhen, createPowerDialogStore } from './dialogStore'
import type { PowerAction } from './power'
import { usePowerWatch } from './watchStore'

// 타일 세 개가 한 줄에 들어가는 너비(시안 A)
const DIALOG_WIDTH = 468

export interface PowerDialogOptions {
  // step·when을 주면 그 확인 단계로 바로 연다(예약 알림·시스템 업데이트 창은 다시 시작의 "시간 정해서").
  step?: PowerAction
  when?: PowerWhen
}

export function openPowerDialog(opts: PowerDialogOptions = {}): void {
  const store = createPowerDialogStore(opts)
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
          onScheduled={() => ctl.close(true)}
        />
      ),
    },
  )
}
