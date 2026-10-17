// 새로 만든 복구 코드를 한 번만 보여 주는 대화상자의 내용. 다시 볼 수 없으므로 복사하거나 적어 두게 한다.
// 여는 함수는 securityDialogs.tsx의 showRecoveryCodes.
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

export function RecoveryCodes({ codes, onClose }: { codes: readonly string[]; onClose: () => void }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const text = codes.join('\n')
  return (
    <>
      <p className="modal-message">{t('security.recovery.intro')}</p>
      <ol className="recovery-codes selectable">
        {codes.map((c) => (
          <li key={c}>
            <code>{c}</code>
          </li>
        ))}
      </ol>
      <p className="modal-message">{t('security.recovery.warning')}</p>
      <div className="modal-buttons">
        {navigator.clipboard && (
          <button
            type="button"
            className="addon-button"
            onClick={() => navigator.clipboard.writeText(text).then(() => setCopied(true), () => {})}
          >
            {copied ? t('security.recovery.copied') : t('security.recovery.copy')}
          </button>
        )}
        <button type="button" className="addon-button primary" onClick={onClose}>
          {t('security.recovery.done')}
        </button>
      </div>
    </>
  )
}
