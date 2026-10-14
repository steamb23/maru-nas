// 문구와 단추로 된 기본 대화상자 내용. 명령형 기본 형태(dialogPresets.tsx)가 쓰고,
// 선언형 Modal 안에서도 같은 모양으로 쓸 수 있다.
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { ParseKeys } from 'i18next'

export type ButtonTone = 'default' | 'primary' | 'danger'

export interface MessageButton {
  id: string
  // 단추 이름. 없으면 labelKey를 번역한다.
  label?: ReactNode
  labelKey?: ParseKeys
  tone?: ButtonTone
}

export function MessageText({ children }: { children: ReactNode }) {
  return <div className="modal-message">{children}</div>
}

interface ButtonsProps {
  buttons: readonly MessageButton[]
  // 열 때 포커스할 단추
  focusId?: string
  onPress: (id: string) => void
}

export function MessageButtons({ buttons, focusId, onPress }: ButtonsProps) {
  const { t } = useTranslation()
  return buttons.map((b) => (
    <button
      key={b.id}
      type="button"
      className={b.tone && b.tone !== 'default' ? `addon-button ${b.tone}` : 'addon-button'}
      // 열 때 한 번만 포커스한다(Modal은 이미 상자 안에 포커스가 있으면 옮기지 않는다).
      autoFocus={b.id === focusId}
      onClick={() => onPress(b.id)}
    >
      {b.label ?? (b.labelKey ? t(b.labelKey) : b.id)}
    </button>
  ))
}
