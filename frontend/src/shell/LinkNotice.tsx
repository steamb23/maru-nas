// 링크 앱 열기 안내 대화상자의 내용(제목, 본문, 단추). 여는 흐름은 linkNoticeDialog.tsx다.
import type { ParseKeys } from 'i18next'
import type { Ref } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageText } from './MessageDialog'

export function LinkNoticeTitle() {
  const { t } = useTranslation()
  return t('linkNotice.title')
}

export function LinkNoticeBody({
  messageKey,
  onSkipChange,
}: {
  messageKey: ParseKeys
  onSkipChange: (skip: boolean) => void
}) {
  const { t } = useTranslation()
  return (
    <>
      <MessageText>{t(messageKey)}</MessageText>
      <label className="link-notice-skip">
        <input type="checkbox" onChange={(e) => onSkipChange(e.currentTarget.checked)} />
        <span>{t('linkNotice.dontShowAgain')}</span>
      </label>
    </>
  )
}

export function LinkNoticeButtons({
  openRef,
  onCancel,
  onOpen,
}: {
  openRef: Ref<HTMLButtonElement>
  onCancel: () => void
  onOpen: () => void
}) {
  const { t } = useTranslation()
  return (
    <>
      <button type="button" className="addon-button" onClick={onCancel}>
        {t('dialog.cancel')}
      </button>
      <button type="button" className="addon-button primary" ref={openRef} onClick={onOpen}>
        {t('linkNotice.open')}
      </button>
    </>
  )
}
