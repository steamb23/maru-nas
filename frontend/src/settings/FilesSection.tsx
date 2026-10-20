// 설정 서버 페이지(관리자)의 파일 탐색기 절: 공유 폴더 스냅숏(개수, 받은 시각, 지금 받기)과 디스크 최상위 보기.
// 일반 사용자의 세션으로는 OMV 공유 폴더 목록을 받을 수 없어 관리자가 로그인할 때 받은 목록을 이름표로 둔다.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, fetchFilesSettings, refreshFilesShares, saveFilesSettings } from '../api/client'
import type { FilesSettings } from '../api/types.gen'
import { formatDateTime } from '../addons/common/format'
import { useLanguage } from '../i18n'
import { SettingsCard, SettingsNotice, SettingsRow, SettingsSection, Switch } from './controls'

type LoadState = { status: 'loading' } | { status: 'error' } | { status: 'ready'; data: FilesSettings }

export function FilesSection() {
  const { t } = useTranslation()
  const lang = useLanguage()
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [busy, setBusy] = useState<'refresh' | 'disks' | null>(null)
  const [failure, setFailure] = useState<'refresh' | 'save' | null>(null)

  useEffect(() => {
    const ctl = new AbortController()
    fetchFilesSettings(ctl.signal).then(
      (data) => setState({ status: 'ready', data }),
      () => {
        if (!ctl.signal.aborted) setState({ status: 'error' })
      },
    )
    return () => ctl.abort()
  }, [])

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
          {failure === 'refresh' ? t('settings.files.refreshFailed') : t('settings.files.saveFailed')}
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
    </SettingsSection>
  )
}
