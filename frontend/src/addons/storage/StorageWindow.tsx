import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { formatBytes, formatPercent } from '../common/format'
import { UsageBar } from '../common/UsageBar'
import type { WindowViewProps } from '../types'
import { fsName, usedPercent } from './storage'
import { useStorage } from './useStorage'

// StorageWindow는 로컬 파일시스템마다 이름·형식·마운트 경로·사용량을 보여 준다.
export function StorageWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data, status } = useStorage(active)

  return (
    <div className="addon-window">
      {status === 'disconnected' && (
        <p className="addon-banner" role="status">
          {t('addonWindow.disconnected')}
        </p>
      )}
      {data && data.filesystems.length === 0 && <p className="addon-empty">{t('storage.empty')}</p>}
      <ul className={status === 'disconnected' ? 'addon-list stale' : 'addon-list'}>
        {data?.filesystems.map((fs) => {
          const name = fsName(fs)
          const pct = usedPercent(fs)
          return (
            <li key={fs.mountPoint} className="addon-card">
              <div className="addon-card-head">
                <span className="addon-card-title">{name.kind === 'root' ? t('storage.root') : name.text}</span>
                <span className="badge">{fs.type}</span>
                {fs.readOnly && <span className="badge">{t('storage.readOnly')}</span>}
                <span className="addon-card-value">{formatPercent(pct, lang)}</span>
              </div>
              <div className="addon-card-sub selectable">{fs.mountPoint}</div>
              <UsageBar percent={pct} />
              <div className="addon-card-sub">
                {t('storage.usage', {
                  used: formatBytes(fs.usedBytes, lang),
                  total: formatBytes(fs.totalBytes, lang),
                  free: formatBytes(fs.availableBytes, lang),
                })}
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
