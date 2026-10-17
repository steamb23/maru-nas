// 권한 상승(관리자 비밀번호 재확인) 대화상자를 연다. 비밀번호는 백엔드를 거쳐 권한 도우미가 직접 확인하고,
// 이 세션에 root 권한을 몇 분 동안 붙인다. root 권한이 필요한 기능이 elevation_required를 받으면
// requestElevation으로 묻고 다시 시도한다. 내용은 ElevationForm.tsx.
import { createRef } from 'react'
import type { HelperStatus } from '../api/types.gen'
import { ElevationForm, ElevationTitle } from './ElevationForm'
import { type DialogScope, openDialog } from './dialogs'

// requestElevation은 비밀번호를 묻고 상승한다. 상승하면 새 도우미 상태, 취소하면 null로 끝난다.
export function requestElevation(scope: DialogScope): Promise<HelperStatus | null> {
  const inputRef = createRef<HTMLInputElement>()
  return openDialog<HelperStatus>(scope, {
    title: <ElevationTitle />,
    initialFocus: inputRef,
    render: (ctl) => <ElevationForm scope={scope} inputRef={inputRef} onDone={ctl.close} onCancel={ctl.cancel} />,
  })
}
