// 설정 페이지가 함께 쓰는 틀과 컨트롤: 절(제목·설명), 묶음 카드, 이름-값 행, 켜기·끄기 스위치.
import type { ReactNode } from 'react'

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
