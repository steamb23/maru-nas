// 인증 수단을 등록하는 대화상자(패스키, 인증 앱), 새 복구 코드를 보여 주는 대화상자, 패스키로 로그인을 켜는 대화상자를
// 연다. 설정의 보안 페이지가 쓰며, 등록 대화상자는 관리 창(stepUpDialog.tsx의 requestManage)이 열린 뒤에 연다.
// 내용은 PasskeyAdd.tsx, TOTPSetup.tsx, RecoveryCodes.tsx, PasskeyLoginSetup.tsx.
import { createRef } from 'react'
import type { SecurityChangeResponse, SecurityStatus } from '../api/types.gen'
import { type DialogScope, openDialog } from '../shell/dialogs'
import { PasskeyAdd } from './PasskeyAdd'
import { PasskeyLoginSetup } from './PasskeyLoginSetup'
import { RecoveryCodes } from './RecoveryCodes'
import { SecurityDialogTitle } from './StepUpBody'
import { TOTPSetup } from './TOTPSetup'

// addPasskeyDialog는 새 패스키를 등록한다. 등록하면 바뀐 상태(첫 수단이면 복구 코드 포함), 취소하면 null이다.
export function addPasskeyDialog(scope: DialogScope, defaultName: string): Promise<SecurityChangeResponse | null> {
  const inputRef = createRef<HTMLInputElement>()
  return openDialog<SecurityChangeResponse>(scope, {
    title: <SecurityDialogTitle k="security.passkeys.addTitle" />,
    initialFocus: inputRef,
    render: (ctl) => <PasskeyAdd defaultName={defaultName} inputRef={inputRef} onDone={ctl.close} onCancel={ctl.cancel} />,
  })
}

// setupTOTPDialog는 인증 앱을 등록한다. 등록하면 바뀐 상태(첫 수단이면 복구 코드 포함), 취소하면 null이다.
export function setupTOTPDialog(scope: DialogScope): Promise<SecurityChangeResponse | null> {
  const inputRef = createRef<HTMLInputElement>()
  return openDialog<SecurityChangeResponse>(scope, {
    title: <SecurityDialogTitle k="security.totp.title" />,
    initialFocus: inputRef,
    render: (ctl) => <TOTPSetup inputRef={inputRef} onDone={ctl.close} onCancel={ctl.cancel} />,
  })
}

// showRecoveryCodes는 새로 만든 복구 코드를 한 번만 보여 주고 닫히면 끝난다.
export async function showRecoveryCodes(scope: DialogScope, codes: readonly string[]): Promise<void> {
  await openDialog<true>(scope, {
    title: <SecurityDialogTitle k="security.recovery.title" />,
    render: (ctl) => <RecoveryCodes codes={codes} onClose={() => ctl.close(true)} />,
  })
}

// enablePasskeyLoginDialog는 이 브라우저에서 패스키로 로그인을 켠다(비밀번호와 패스키를 함께 확인하므로 관리 창은
// 필요 없다). 켜면 바뀐 상태, 취소하면 null이다. days는 지금 켜면 쓸 수 있는 기간(일)이다.
export function enablePasskeyLoginDialog(scope: DialogScope, username: string, days: number): Promise<SecurityStatus | null> {
  const inputRef = createRef<HTMLInputElement>()
  return openDialog<SecurityStatus>(scope, {
    title: <SecurityDialogTitle k="security.passkeyLogin.title" />,
    initialFocus: inputRef,
    render: (ctl) => (
      <PasskeyLoginSetup username={username} days={days} inputRef={inputRef} onDone={ctl.close} onCancel={ctl.cancel} />
    ),
  })
}
