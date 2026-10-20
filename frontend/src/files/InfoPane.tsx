// 정보 패널. 파일을 눌렀을 때만 목록 오른쪽에 연다(창이 좁으면 목록 위에 덮어 연다). 종류·크기·수정한 날짜와
// 권한(소유자, 그룹, 모드, ACL, 내 권한)을 보이고 내려받기 단추를 둔다. 관리자에게는 권한 편집 화면을 다시 만들지 않고
// 그 파일이 든 공유 폴더의 OMV ACL 화면 링크를 둔다.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, fetchFileInfo } from '../api/client'
import type { FileInfo } from '../api/types.gen'
import { formatBytes, formatDateTime } from '../addons/common/format'
import { useCapabilitiesStore } from '../addons/capabilities'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import { openWorkbenchRoute } from '../shell/launchApp'
import { startDownload } from './download'
import { KIND_KEYS, kindIcon, kindOf, modeString } from './kinds'
import { type Root, locationLabel } from './paths'
import { errorKey } from './messages'

interface Props {
  path: string
  roots: readonly Root[]
  admin: boolean
  onClose: () => void
}

// 상태에는 그 결과를 받은 경로를 함께 둔다. 지금 경로와 다르면 받는 중으로 본다.
type State = { path: string } & ({ status: 'loading' } | { status: 'ready'; info: FileInfo } | { status: 'error'; err: unknown })

export function InfoPane({ path, roots, admin, onClose }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const workbench = useCapabilitiesStore((s) => Boolean(s.caps?.workbenchUrl))
  const [result, setState] = useState<State>({ path: '', status: 'loading' })
  const [downloadFailure, setDownloadError] = useState<{ path: string; err: unknown } | null>(null)

  useEffect(() => {
    const ctl = new AbortController()
    fetchFileInfo(path, ctl.signal).then(
      (info) => setState({ path, status: 'ready', info }),
      (err: unknown) => {
        if (!ctl.signal.aborted) setState({ path, status: 'error', err })
      },
    )
    return () => ctl.abort()
  }, [path])
  const state: State = result.path === path ? result : { path, status: 'loading' }
  const downloadError = downloadFailure?.path === path ? downloadFailure.err : null

  const name = path.slice(path.lastIndexOf('/') + 1)
  const info = state.status === 'ready' ? state.info : null
  const kind = kindOf({ name, type: info?.type ?? 'file', linkType: info?.linkType })
  const canDownload = info !== null && (info.type === 'file' || info.linkType === 'file') && info.access?.read !== false

  const accessLabel = (): string => {
    const a = info?.access
    if (!a) return t('files.info.unknown')
    if (a.read && a.write) return t('files.info.readWrite')
    if (a.read) return t('files.info.readOnly')
    if (a.write) return t('files.info.writeOnly')
    return t('files.info.noAccess')
  }
  const accessTone = info?.access?.write ? 'ok' : ''

  const download = () => {
    setDownloadError(null)
    startDownload(path, name).catch((err: unknown) => {
      if (!(err instanceof ApiError && err.status === 401)) setDownloadError({ path, err })
    })
  }

  return (
    <aside className="files-info" aria-label={t('files.info.label')}>
      <button type="button" className="files-icon-button files-info-close" aria-label={t('files.info.close')} title={t('files.info.close')} onClick={onClose}>
        <IconImage name="window-close-symbolic" size={16} />
      </button>
      <div className="files-info-head">
        <IconImage name={kindIcon(kind)} size={72} />
        <h2 className="selectable">{name}</h2>
        {info && (
          <p>
            {info.type === 'file'
              ? t('files.info.kindSize', { kind: t(KIND_KEYS[kind]), size: formatBytes(info.size, lang) })
              : t(KIND_KEYS[kind])}
          </p>
        )}
      </div>
      <div className="files-info-actions">
        <button type="button" className="addon-button primary" disabled={!canDownload} onClick={download}>
          <IconImage name="download-symbolic" size={16} />
          {t('files.info.download')}
        </button>
      </div>
      {downloadError !== null && (
        <p className="addon-banner files-info-error" role="alert">
          {t(errorKey(downloadError))}
        </p>
      )}
      {state.status === 'error' && (
        <p className="addon-banner files-info-error" role="alert">
          {state.err instanceof ApiError && state.err.code ? t(errorKey(state.err)) : t('files.info.loadFailed')}
        </p>
      )}
      {info && (
        <>
          <dl className="files-info-list">
            <dt>{t('files.info.modified')}</dt>
            <dd>{formatDateTime(new Date(info.modTime).toISOString(), lang)}</dd>
            <dt>{t('files.info.location')}</dt>
            <dd className="selectable">{locationLabel(roots, path)}</dd>
            {info.target !== undefined && (
              <>
                <dt>{t('files.info.target')}</dt>
                <dd className="selectable files-mono">{info.target}</dd>
              </>
            )}
          </dl>
          <div className="files-info-section">
            <h3>{t('files.info.permissions')}</h3>
            <span className={`files-access ${accessTone}`}>{t('files.info.myAccess', { access: accessLabel() })}</span>
          </div>
          <dl className="files-info-list">
            <dt>{t('files.info.owner')}</dt>
            <dd>{info.owner ?? info.uid}</dd>
            <dt>{t('files.info.group')}</dt>
            <dd>{info.group ?? info.gid}</dd>
            <dt>{t('files.info.mode')}</dt>
            <dd className="files-mono">{modeString(info.mode)}</dd>
            <dt>{t('files.info.acl')}</dt>
            <dd>{info.acl === undefined ? t('files.info.unknown') : info.acl ? t('files.info.aclYes') : t('files.info.aclNo')}</dd>
          </dl>
          <div className="files-info-note">
            <p>{t('files.info.note')}</p>
            {admin && workbench && info.shareUuid && (
              <button
                type="button"
                className="files-link"
                onClick={() => openWorkbenchRoute(`storage/shared-folders/acl/${encodeURIComponent(info.shareUuid ?? '')}`)}
              >
                {t('files.info.editAcl')}
                <IconImage name="external-link-symbolic" size={14} />
              </button>
            )}
          </div>
        </>
      )}
    </aside>
  )
}
