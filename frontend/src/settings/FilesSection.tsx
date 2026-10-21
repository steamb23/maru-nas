// 설정 서버 페이지(관리자)의 파일 탐색기 절: 공유 폴더 스냅숏(개수, 받은 시각, 지금 받기), 디스크 최상위 보기, 디스크
// 휴지통. 일반 사용자의 세션으로는 OMV 공유 폴더 목록을 받을 수 없어 관리자가 로그인할 때 받은 목록을 이름표로 둔다.
// OMV 데이터 디스크 맨 위는 보통 root 소유라 사용자가 휴지통 폴더를 만들 수 없으므로, 관리자가 권한 상승한 뒤 디스크마다
// 모든 사용자가 쓰는 .Trash(1777)를 만든다.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ApiError,
  fetchFilesSettings,
  fetchSiteTrash,
  prepareSiteTrash,
  refreshFilesShares,
  saveFilesSettings,
} from '../api/client'
import type { FilesSettings, SiteTrashDisks } from '../api/types.gen'
import { formatDateTime } from '../addons/common/format'
import { useLanguage } from '../i18n'
import { useDialogs } from '../shell/dialogPresets'
import { requestElevation } from '../shell/elevationDialog'
import { SettingsCard, SettingsNotice, SettingsRow, SettingsSection, Switch } from './controls'

type LoadState = { status: 'loading' } | { status: 'error' } | { status: 'ready'; data: FilesSettings }

export function FilesSection() {
  const { t } = useTranslation()
  const lang = useLanguage()
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const dialogs = useDialogs()
  const [busy, setBusy] = useState<'refresh' | 'disks' | 'trash' | null>(null)
  const [failure, setFailure] = useState<'refresh' | 'save' | 'trash' | null>(null)
  const [trash, setTrash] = useState<SiteTrashDisks | null>(null)

  useEffect(() => {
    const ctl = new AbortController()
    fetchFilesSettings(ctl.signal).then(
      (data) => setState({ status: 'ready', data }),
      () => {
        if (!ctl.signal.aborted) setState({ status: 'error' })
      },
    )
    // 디스크 휴지통 상태는 권한 도우미로 읽는다. 도우미가 없으면 그 부분을 보이지 않는다.
    fetchSiteTrash(ctl.signal).then(setTrash, () => {})
    return () => ctl.abort()
  }, [])

  // prepareTrash는 디스크 맨 위에 공유 휴지통을 만든다. root 권한이 필요하면 상승한 뒤 한 번 더 요청한다.
  const prepareTrash = async (disk: string) => {
    setBusy('trash')
    setFailure(null)
    try {
      try {
        setTrash(await prepareSiteTrash(disk))
      } catch (err) {
        if (!(err instanceof ApiError && err.code === 'elevation_required')) throw err
        if (!(await requestElevation(dialogs.scope))) return
        setTrash(await prepareSiteTrash(disk))
      }
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 401)) setFailure('trash')
    } finally {
      setBusy(null)
    }
  }

  const refresh = async () => {
    setBusy('refresh')
    setFailure(null)
    try {
      setState({ status: 'ready', data: await refreshFilesShares() })
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 401)) setFailure('refresh')
    } finally {
      setBusy(null)
    }
  }

  const setShowDisks = async (showDisks: boolean) => {
    if (state.status !== 'ready') return
    const prev = state.data
    setState({ status: 'ready', data: { ...prev, showDisks } })
    setBusy('disks')
    setFailure(null)
    try {
      setState({ status: 'ready', data: await saveFilesSettings(showDisks) })
    } catch (err) {
      setState({ status: 'ready', data: prev })
      if (!(err instanceof ApiError && err.status === 401)) setFailure('save')
    } finally {
      setBusy(null)
    }
  }

  const data = state.status === 'ready' ? state.data : null
  return (
    <SettingsSection title={t('settings.files.title')}>
      {state.status === 'error' && <SettingsNotice tone="error">{t('files.errors.generic')}</SettingsNotice>}
      {failure && (
        <SettingsNotice tone="error">
          {failure === 'refresh'
            ? t('settings.files.refreshFailed')
            : failure === 'trash'
              ? t('settings.files.trashFailed')
              : t('settings.files.saveFailed')}
        </SettingsNotice>
      )}
      <SettingsCard>
        <SettingsRow
          label={t('settings.files.shares')}
          description={
            <>
              {t('settings.files.sharesDesc')}
              <br />
              {data?.sharesUpdatedAt
                ? t('settings.files.sharesState', {
                    count: data.shares.length,
                    time: formatDateTime(data.sharesUpdatedAt, lang),
                  })
                : data && t('settings.files.sharesNone')}
            </>
          }
        >
          <button type="button" className="addon-button" disabled={!data || busy !== null} onClick={() => void refresh()}>
            {t('settings.files.refresh')}
          </button>
        </SettingsRow>
        <SettingsRow label={t('settings.files.showDisks')} description={t('settings.files.showDisksDesc')}>
          <Switch
            checked={data?.showDisks ?? false}
            disabled={!data || busy !== null}
            label={t('settings.files.showDisks')}
            onChange={(v) => void setShowDisks(v)}
          />
        </SettingsRow>
      </SettingsCard>
      {trash && trash.disks.length > 0 && (
        <>
          <p className="settings-hint">{t('settings.files.trashDesc')}</p>
          <SettingsCard>
            {trash.disks.map((d) => (
              <SettingsRow
                key={d.path}
                label={t('settings.files.trashDisk', { disk: d.name })}
                description={<span className="selectable">{d.path}</span>}
              >
                {d.shared ? (
                  <span className="settings-badge">{t('settings.files.trashOn')}</span>
                ) : (
                  <button type="button" className="addon-button" disabled={busy !== null} onClick={() => void prepareTrash(d.path)}>
                    {t('settings.files.trashCreate')}
                  </button>
                )}
              </SettingsRow>
            ))}
          </SettingsCard>
        </>
      )}
    </SettingsSection>
  )
}
