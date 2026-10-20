// 올리기 목록 패널. 목록 오른쪽 아래에 떠서 진행률, 남은 시간, 실패 이유를 보이고 취소·다시 올리기를 둔다. 접을 수 있다.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { formatBytes, formatUnit, formatUptime } from '../addons/common/format'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import { kindIcon, kindOf } from './kinds'
import { uploadErrorKey } from './messages'
import { type UploadItem, type UploadState, active, remainingSeconds } from './uploadQueue'

interface Props {
  state: UploadState
  onCancel: (id: number) => void
  onRetry: (id: number, overwrite?: boolean) => void
  onClear: () => void
}

export function UploadPanel({ state, onCancel, onRetry, onClear }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const [collapsed, setCollapsed] = useState(false)
  // 남은 시간을 1초마다 다시 셈한다.
  const [now, setNow] = useState(() => Date.now())
  const left = active(state)
  useEffect(() => {
    if (left === 0) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [left])

  if (state.items.length === 0) return null
  const total = state.items.length
  const title = left > 0 ? t('files.upload.inProgress', { total, active: left }) : t('files.upload.allDone', { total })

  const status = (i: UploadItem) => {
    switch (i.status) {
      case 'uploading':
        return `${Math.floor((i.sent / Math.max(1, i.size)) * 100)}%`
      case 'queued':
        return t('files.upload.waiting')
      case 'done':
        return t('files.upload.done')
      case 'skipped':
        return t('files.upload.skipped')
      case 'canceled':
        return t('files.upload.canceled')
      default:
        return ''
    }
  }

  return (
    <section className={`files-uploads${collapsed ? ' collapsed' : ''}`} aria-label={t('files.upload.title')}>
      <header>
        <h2 role="status">{title}</h2>
        {left === 0 && (
          <button type="button" className="files-text-button" onClick={onClear}>
            {t('files.upload.clear')}
          </button>
        )}
        <button
          type="button"
          className="files-icon-button"
          aria-expanded={!collapsed}
          aria-label={collapsed ? t('files.upload.expand') : t('files.upload.collapse')}
          title={collapsed ? t('files.upload.expand') : t('files.upload.collapse')}
          onClick={() => setCollapsed((c) => !c)}
        >
          <span className={`files-chevron${collapsed ? ' up' : ''}`} aria-hidden="true" />
        </button>
      </header>
      {!collapsed && (
        <ul>
          {state.items.map((i) => {
            const remain = remainingSeconds(i, now)
            return (
              <li key={i.id} className={i.status}>
                <div className="files-upload-line">
                  <IconImage name={kindIcon(kindOf({ name: i.name, type: 'file' }))} size={20} />
                  <span className="files-upload-name" title={i.name}>
                    {i.name}
                  </span>
                  <span className="files-upload-status">
                    {i.status === 'done' && <IconImage name="object-select-symbolic" size={14} />}
                    {status(i)}
                  </span>
                  {(i.status === 'uploading' || i.status === 'queued') && (
                    <button
                      type="button"
                      className="files-icon-button small"
                      aria-label={t('files.upload.cancel')}
                      title={t('files.upload.cancel')}
                      onClick={() => onCancel(i.id)}
                    >
                      <IconImage name="window-close-symbolic" size={14} />
                    </button>
                  )}
                  {(i.status === 'failed' || i.status === 'canceled') && (
                    <button type="button" className="files-text-button" onClick={() => onRetry(i.id, i.error === 'file_exists' ? true : undefined)}>
                      {i.error === 'file_exists' ? t('files.upload.overwrite') : t('files.upload.retry')}
                    </button>
                  )}
                </div>
                {i.status === 'uploading' && (
                  <>
                    <div className="meter files-upload-meter" aria-hidden="true">
                      <div className="meter-fill" style={{ width: `${(i.sent / Math.max(1, i.size)) * 100}%` }} />
                    </div>
                    <span className="files-upload-detail">
                      {t('files.upload.progress', { sent: formatBytes(i.sent, lang), size: formatBytes(i.size, lang) })}
                      {remain !== null && ` · ${t('files.upload.remaining', { time: formatRemaining(remain, lang) })}`}
                    </span>
                  </>
                )}
                {i.status === 'failed' && (
                  <span className="files-upload-detail error" role="alert">
                    <IconImage name="dialog-warning-symbolic" size={12} />
                    {t(uploadErrorKey(i.error))}
                  </span>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

// formatRemaining은 남은 시간이다. 1분보다 짧으면 초, 그 밖에는 분 단위(시간이 넘으면 시간과 분)다.
function formatRemaining(seconds: number, lang: string): string {
  if (seconds < 60) return formatUnit(seconds, 'second', lang)
  return formatUptime(seconds + 59, lang)
}
