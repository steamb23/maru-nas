// 보안 강화 모드의 확인·관리 대화상자를 연다. 보안에 민감한 기능(웹 터미널, root 권한 상승)이 security_required를
// 받으면 requestStepUp으로 확인하고 다시 시도한다. 인증 수단을 바꾸기 전에는 requestManage로 관리 창을 연다.
// 내용은 StepUpBody.tsx, 수단 등록 대화상자는 securityDialogs.tsx.
import { createRef } from 'react'
import type { SecurityStatus } from '../api/types.gen'
import { type DialogScope, openDialog } from '../shell/dialogs'
import { SecurityDialogTitle, StepUpBody, type StepUpRequest } from './StepUpBody'

export type { StepUpRequest }

// requestStepUp은 이 세션의 보증 수준을 올린다. 확인하면 새 보안 상태, 취소하면 null로 끝난다.
// 이미 그 단계를 거친 세션이면 묻지 않고 바로 끝난다.
export function requestStepUp(scope: DialogScope, req: StepUpRequest = {}): Promise<SecurityStatus | null> {
  const inputRef = createRef<HTMLInputElement>()
  return openDialog<SecurityStatus>(scope, {
    title: <SecurityDialogTitle k="security.stepUp.title" />,
    initialFocus: inputRef,
    render: (ctl) => <StepUpBody mode="verify" req={req} inputRef={inputRef} onDone={ctl.close} onCancel={ctl.cancel} />,
  })
}

// requestManage는 비밀번호와 등록한 수단을 다시 확인해 관리 창을 연다. 열리면 새 보안 상태, 취소하면 null이다.
export function requestManage(scope: DialogScope): Promise<SecurityStatus | null> {
  const inputRef = createRef<HTMLInputElement>()
  return openDialog<SecurityStatus>(scope, {
    title: <SecurityDialogTitle k="security.stepUp.manageTitle" />,
    initialFocus: inputRef,
    render: (ctl) => <StepUpBody mode="manage" req={{}} inputRef={inputRef} onDone={ctl.close} onCancel={ctl.cancel} />,
  })
}
