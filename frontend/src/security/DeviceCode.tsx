// 기기 추가 코드를 보여 주는 대화상자의 내용. 새 기기의 본인 확인에 입력할 코드와 남은 시간을 보인다.
// 여는 함수는 securityDialogs.tsx의 deviceCodeDialog.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { DeviceCodeResponse } from '../api/types.gen'
import { formatClock } from '../shell/elevation'

// secondsLeft는 deadline(Date.now() 기준 ms)까지 남은 초다.
const secondsLeft = (deadline: number) => Math.max(0, Math.ceil((deadline - Date.now()) / 1000))

export function DeviceCode({ code, onClose }: { code: DeviceCodeResponse; onClose: () => void }) {
  const { t } = useTranslation()
  const [deadline] = useState(() => Date.now() + code.expiresInSeconds * 1000)
  const [left, setLeft] = useState(code.expiresInSeconds)

  useEffect(() => {
    const id = setInterval(() => setLeft(secondsLeft(deadline)), 1000)
    return () => clearInterval(id)
  }, [deadline])

  const expired = left <= 0
  return (
    <>
      <p className="modal-message">{t('security.deviceCode.intro')}</p>
      <p className={expired ? 'device-code selectable expired' : 'device-code selectable'}>
        <code>{code.code}</code>
      </p>
      {expired ? (
        <p className="modal-message" role="status">
          {t('security.deviceCode.expired')}
        </p>
      ) : (
        <p className="modal-message">{t('security.deviceCode.expires', { remaining: formatClock(left) })}</p>
      )}
      <div className="modal-buttons">
        <button type="button" className="addon-button primary" onClick={onClose}>
          {t('security.deviceCode.done')}
        </button>
      </div>
    </>
  )
}
