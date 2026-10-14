// 문구와 단추로 된 기본 대화상자 내용. 명령형 기본 형태(dialogPresets.tsx)가 쓰고,
// 선언형 Modal 안에서도 같은 모양으로 쓸 수 있다.
import type { ReactNode, Ref } from 'react'
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
  // focusId 단추에 붙일 ref. Modal의 initialFocus로 넘긴다.
  focusId?: string
  focusRef?: Ref<HTMLButtonElement>
  onPress: (id: string) => void
}

export function MessageButtons({ buttons, focusId, focusRef, onPress }: ButtonsProps) {
  const { t } = useTranslation()
  return buttons.map((b) => (
    <button
      key={b.id}
      type="button"
      className={b.tone && b.tone !== 'default' ? `addon-button ${b.tone}` : 'addon-button'}
      // 첫 포커스는 Modal이 initialFocus로 옮긴다. autoFocus를 쓰면 Modal이 열기 전 포커스를 기억하기 전에
      // 포커스가 옮겨져, 닫은 뒤 돌려줄 곳을 잃는다.
      ref={b.id === focusId ? focusRef : undefined}
      onClick={() => onPress(b.id)}
    >
      {b.label ?? (b.labelKey ? t(b.labelKey) : b.id)}
    </button>
  ))
}
