// 설정 페이지가 함께 쓰는 틀과 컨트롤: 절(제목·설명), 묶음 카드, 이름-값 행, 켜기·끄기 스위치, 복사할 수 있는 명령.
import { type ReactNode, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'

export function SettingsSection({
  title,
  description,
  children,
}: {
  title: string
  description?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="settings-section">
      <h3 className="settings-section-title">{title}</h3>
      {description && <p className="settings-section-desc">{description}</p>}
      {children}
    </section>
  )
}

// SettingsCard는 행을 묶는 상자다. 행 사이에 구분선이 그려진다.
export function SettingsCard({ children }: { children: ReactNode }) {
  return <div className="settings-card">{children}</div>
}

// SettingsRow는 왼쪽에 이름(과 설명), 오른쪽에 값이나 컨트롤을 둔다. 좁으면 아래로 내려간다.
export function SettingsRow({
  label,
  description,
  children,
}: {
  label: ReactNode
  description?: ReactNode
  children?: ReactNode
}) {
  return (
    <div className="settings-row">
      <div className="settings-row-text">
        <div className="settings-row-label">{label}</div>
        {description && <div className="settings-row-desc">{description}</div>}
      </div>
      {children !== undefined && <div className="settings-row-control">{children}</div>}
    </div>
  )
}

// Switch는 켜기·끄기 스위치다. 이름은 label로 받아 화면 읽기 프로그램에 알린다.
export function Switch({
  checked,
  onChange,
  label,
  disabled = false,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      role="switch"
      className="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
    >
      <span className="switch-thumb" />
    </button>
  )
}

// SettingsNotice는 페이지 위쪽에 두는 짧은 안내다(불러오는 중, 저장 실패 등). error면 경고 색으로 그린다.
export function SettingsNotice({ tone = 'info', children }: { tone?: 'info' | 'error'; children: ReactNode }) {
  return (
    <p className={`settings-notice ${tone}`} role={tone === 'error' ? 'alert' : 'status'}>
      {children}
    </p>
  )
}

// StoreNotice는 설정 저장소 상태(불러오는 중, 실패, 저장 실패)를 알린다. 문제없으면 그리지 않는다.
export function StoreNotice({ status, saveFailed }: { status: 'idle' | 'loading' | 'ready' | 'error'; saveFailed: boolean }) {
  const { t } = useTranslation()
  if (status === 'error') return <SettingsNotice tone="error">{t('settings.common.loadFailed')}</SettingsNotice>
  if (status !== 'ready') return <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>
  if (saveFailed) return <SettingsNotice tone="error">{t('settings.common.saveFailed')}</SettingsNotice>
  return null
}

// CommandBlock은 사용자가 SSH로 실행할 명령이다. 글자를 선택할 수 있고 복사 단추가 있다.
export function CommandBlock({ text }: { text: string }) {
  return (
    <div className="settings-commands">
      <pre className="selectable">{text}</pre>
      <CopyButton text={text} />
    </div>
  )
}

// CopyButton은 클립보드에 복사한다. 클립보드를 쓸 수 없는 환경(평문 HTTP 등)이면 그리지 않는다(글자는 선택해 복사).
// 문구는 서버 페이지에서 처음 쓴 키(settings.server.copy)를 그대로 쓴다(다른 언어 번역이 그 키에 있다).
export function CopyButton({ text }: { text: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const id = window.setTimeout(() => setCopied(false), 2000)
    return () => window.clearTimeout(id)
  }, [copied])
  if (!navigator.clipboard) return null
  const label = copied ? t('settings.server.copied') : t('settings.server.copy')
  return (
    <button
      type="button"
      className="settings-copy"
      aria-label={label}
      title={label}
      onClick={() => navigator.clipboard.writeText(text).then(() => setCopied(true), () => {})}
    >
      <IconImage name={copied ? 'object-select-symbolic' : 'edit-copy-symbolic'} size={16} />
    </button>
  )
}
