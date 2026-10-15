// 링크 앱을 브라우저 새 탭으로 열기 전에 알린다. "다음에 알리지 않음"을 고르면 쿠키에 남기고
// 다음부터는 묻지 않고 바로 연다(쿠키 계산은 linkNotice.ts, 내용은 LinkNotice.tsx).
import type { ParseKeys } from 'i18next'
import { createRef } from 'react'
import type { LinkAppId } from '../windows/apps'
import { LinkNoticeBody, LinkNoticeButtons, LinkNoticeTitle } from './LinkNotice'
import { openDialog } from './dialogs'
import { noticeSkipped, skipNoticeCookie } from './linkNotice'

// 앱마다 보여 줄 안내 문구
const MESSAGE_KEYS: Record<LinkAppId, ParseKeys> = {
  workbench: 'linkNotice.workbench',
}

// confirmLinkOpen은 안내를 끄지 않았으면 데스크톱 대화상자로 묻고, "열기"를 누르면 open을 부른다. 껐으면 바로 부른다.
// open은 "열기"를 누른 클릭 처리 안에서 바로 부른다. 대화상자 결과를 기다린 뒤에 열면 팝업 차단에 걸릴 수 있다.
export function confirmLinkOpen(id: LinkAppId, open: () => void): void {
  if (noticeSkipped(document.cookie, id)) {
    open()
    return
  }
  // 체크박스는 본문에, 단추는 단추 줄에 있으므로 고른 값을 이 대화상자 하나의 객체로 함께 쓴다.
  const choice = { skip: false }
  const openRef = createRef<HTMLButtonElement>()
  void openDialog<true>(
    { kind: 'desktop' },
    {
      role: 'alertdialog',
      title: <LinkNoticeTitle />,
      initialFocus: openRef,
      render: () => <LinkNoticeBody messageKey={MESSAGE_KEYS[id]} onSkipChange={(skip) => (choice.skip = skip)} />,
      footer: (ctl) => (
        <LinkNoticeButtons
          openRef={openRef}
          onCancel={ctl.cancel}
          onOpen={() => {
            if (choice.skip) document.cookie = skipNoticeCookie(id, location.protocol === 'https:')
            open()
            ctl.close(true)
          }}
        />
      ),
    },
  )
}
