import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'

interface Props {
  text: string
  // label은 복사하기 전의 접근성 이름이다(무엇을 복사하는지). 복사한 뒤 2초 동안은 "복사했습니다"다.
  label: string
  className?: string
}

// CopyButton은 text를 클립보드에 복사하는 아이콘 단추다. 클립보드를 쓸 수 없는 환경(평문 HTTP 등)이면 그리지
// 않는다(글자는 선택해 복사한다).
export function CopyButton({ text, label, className }: Props) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const id = window.setTimeout(() => setCopied(false), 2000)
    return () => window.clearTimeout(id)
  }, [copied])
  if (!navigator.clipboard) return null
  const name = copied ? t('clipboard.copied') : label
  return (
    <button
      type="button"
      className={className ? `copy-button ${className}` : 'copy-button'}
      aria-label={name}
      title={name}
      onClick={() => navigator.clipboard.writeText(text).then(() => setCopied(true), () => {})}
    >
      <IconImage name={copied ? 'object-select-symbolic' : 'edit-copy-symbolic'} size={16} />
    </button>
  )
}
