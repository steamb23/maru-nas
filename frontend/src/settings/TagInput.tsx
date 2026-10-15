// 태그 입력: 태그를 칩으로 보여 주고, 입력 상자에서 쉼표나 Enter로 더하며, 칩의 ×나 빈 상자에서 Backspace로 지운다.
// 입력 중인 글자는 pending으로 부모가 들고 있어, 저장할 때 아직 칩이 되지 않은 글자도 함께 넣을 수 있다.
import { type KeyboardEvent, useId } from 'react'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'
import { addTags } from '../profile/tags'

export function TagInput({
  tags,
  pending,
  onChange,
  disabled = false,
  label,
}: {
  tags: readonly string[]
  pending: string
  onChange: (tags: string[], pending: string) => void
  disabled?: boolean
  label: string
}) {
  const { t } = useTranslation()
  const listId = useId()

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' && pending.trim()) {
      // 폼을 제출하지 않고 태그로 더한다. 빈 상자에서 Enter는 저장(폼 제출)이다.
      e.preventDefault()
      onChange(addTags(tags, pending), '')
    } else if (e.key === 'Backspace' && pending === '' && tags.length > 0) {
      e.preventDefault()
      onChange(tags.slice(0, -1), '')
    }
  }

  return (
    <div className="tag-input" aria-disabled={disabled || undefined}>
      <ul className="tag-list" id={listId} aria-label={label}>
        {tags.map((tag) => (
          <li key={tag} className="tag-chip">
            <span className="selectable">{tag}</span>
            <button
              type="button"
              className="tag-remove"
              aria-label={t('settings.profile.removeTag', { tag })}
              title={t('settings.profile.removeTag', { tag })}
              disabled={disabled}
              onClick={() =>
                onChange(
                  tags.filter((x) => x !== tag),
                  pending,
                )
              }
            >
              <IconImage name="window-close-symbolic" size={12} />
            </button>
          </li>
        ))}
      </ul>
      <input
        className="tag-input-text"
        type="text"
        value={pending}
        placeholder={t('settings.profile.addTag')}
        aria-label={t('settings.profile.addTag')}
        aria-describedby={listId}
        disabled={disabled}
        onChange={(e) => {
          // 쉼표를 치거나 쉼표가 든 글자를 붙여 넣으면 마지막 쉼표 앞까지 태그로 만든다.
          const v = e.target.value
          const cut = v.lastIndexOf(',')
          if (cut < 0) onChange([...tags], v)
          else onChange(addTags(tags, v.slice(0, cut)), v.slice(cut + 1))
        }}
        onKeyDown={onKeyDown}
        onBlur={() => {
          if (pending.trim()) onChange(addTags(tags, pending), '')
        }}
      />
    </div>
  )
}
