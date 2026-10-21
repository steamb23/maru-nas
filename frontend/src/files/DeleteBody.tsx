// 삭제 확인 대화상자의 제목과 내용, 여러 항목 작업의 실패 안내. 명령형으로 여는 대화상자라 번역 훅을 쓰는 컴포넌트로 그린다.
import { useTranslation } from 'react-i18next'
import type { FileItem } from '../api/types.gen'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import { kindIcon, kindOf } from './kinds'
import type { FailureGroup } from './ops'

// DELETE_LIST는 확인 대화상자에 이름을 보이는 최대 항목 수다.
export const DELETE_LIST = 5

// DeleteMode는 대화상자의 뜻이다: 휴지통으로 옮기기, 영구 삭제, 휴지통이 없는 디스크의 영구 삭제, 휴지통에서 영구 삭제.
export type DeleteMode = 'trash' | 'permanent' | 'noTrash' | 'purge'

type Named = Pick<FileItem, 'name' | 'type'>

export function DeleteTitle({ items, mode }: { items: readonly Named[]; mode: DeleteMode }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const trash = mode === 'trash'
  if (items.length === 1) return <>{t(trash ? 'files.delete.trashTitleOne' : 'files.delete.titleOne', { name: items[0].name })}</>
  const total = new Intl.NumberFormat(lang).format(items.length)
  return <>{t(trash ? 'files.delete.trashTitleMany' : 'files.delete.titleMany', { count: items.length, total })}</>
}

const NOTE_KEYS = {
  trash: 'files.delete.trashNote',
  permanent: 'files.delete.permanent',
  noTrash: 'files.delete.noTrash',
  purge: 'files.delete.purge',
} as const

export function DeleteBody({ items, mode }: { items: readonly Named[]; mode: DeleteMode }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  // 휴지통에 넣는 폴더는 안의 항목과 함께 옮겨지므로 따로 알리지 않는다.
  const folders = mode !== 'trash' && items.some((i) => i.type === 'dir')
  const more = items.length - DELETE_LIST
  return (
    <div className="files-delete">
      {items.length > 1 && (
        <ul className="files-delete-list">
          {items.slice(0, DELETE_LIST).map((it, i) => (
            <li key={`${i}:${it.name}`}>
              <IconImage name={kindIcon(kindOf({ name: it.name, type: it.type }))} size={16} />
              <span>{it.name}</span>
            </li>
          ))}
          {more > 0 && (
            <li className="files-delete-more">{t('files.delete.more', { count: more, total: new Intl.NumberFormat(lang).format(more) })}</li>
          )}
        </ul>
      )}
      <p>{t(NOTE_KEYS[mode])}</p>
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
