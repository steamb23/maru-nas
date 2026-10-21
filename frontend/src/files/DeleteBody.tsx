// 삭제 확인 대화상자의 제목과 내용, 여러 항목 작업의 실패 안내. 명령형으로 여는 대화상자라 번역 훅을 쓰는 컴포넌트로 그린다.
import { useTranslation } from 'react-i18next'
import type { FileItem } from '../api/types.gen'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import { kindIcon, kindOf } from './kinds'
import type { FailureGroup } from './ops'

// DELETE_LIST는 확인 대화상자에 이름을 보이는 최대 항목 수다.
export const DELETE_LIST = 5

export function DeleteTitle({ items }: { items: readonly FileItem[] }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  if (items.length === 1) return <>{t('files.delete.titleOne', { name: items[0].name })}</>
  return <>{t('files.delete.titleMany', { count: items.length, total: new Intl.NumberFormat(lang).format(items.length) })}</>
}

export function DeleteBody({ items }: { items: readonly FileItem[] }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const folders = items.some((i) => i.type === 'dir')
  const more = items.length - DELETE_LIST
  return (
    <div className="files-delete">
      {items.length > 1 && (
        <ul className="files-delete-list">
          {items.slice(0, DELETE_LIST).map((it) => (
            <li key={it.name}>
              <IconImage name={kindIcon(kindOf(it))} size={16} />
              <span>{it.name}</span>
            </li>
          ))}
          {more > 0 && (
            <li className="files-delete-more">{t('files.delete.more', { count: more, total: new Intl.NumberFormat(lang).format(more) })}</li>
          )}
        </ul>
      )}
      <p>{t('files.delete.permanent')}</p>
      {folders && <p>{t('files.delete.folders')}</p>}
    </div>
  )
}

// FailuresBody는 여러 항목 작업에서 하지 못한 항목을 이유별로 보인다.
export function FailuresBody({ groups }: { groups: readonly (FailureGroup & { more: number })[] }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  return (
    <div className="files-delete">
      {groups.map((g) => (
        <div key={g.key} className="files-failure">
          <p>{t(g.key)}</p>
          <ul className="files-delete-list">
            {g.names.map((n) => (
              <li key={n}>
                <span>{n}</span>
              </li>
            ))}
            {g.more > 0 && (
              <li className="files-delete-more">{t('files.delete.more', { count: g.more, total: new Intl.NumberFormat(lang).format(g.more) })}</li>
            )}
          </ul>
        </div>
      ))}
    </div>
  )
}
