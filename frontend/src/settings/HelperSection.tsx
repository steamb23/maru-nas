// 설정 서버 페이지(관리자)의 권한 도우미 절: 도우미 상태, 도우미가 실제로 실행하는 권한(uid·그룹),
// root 권한 상승·해제. 상승은 비밀번호 확인 대화상자(shell/elevationDialog.tsx)로 한다. 계산은 helper.ts.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { dropElevation, fetchHelperIdentity, fetchHelperStatus } from '../api/client'
import type { HelperIdentity, HelperIdentityResponse, HelperStatus } from '../api/types.gen'
import { useDialogs } from '../shell/dialogPresets'
import { formatClock } from '../shell/elevation'
import { requestElevation } from '../shell/elevationDialog'
import { SettingsCard, SettingsNotice, SettingsRow, SettingsSection } from './controls'
import { HELPER_STATE_DESC_KEYS, HELPER_STATE_KEYS, elevationEndsAt, groupNames, secondsLeft } from './helper'

interface Loaded {
  status: 'ready'
  helper: HelperStatus
  identity: HelperIdentityResponse | null
  // 상승이 끝나는 시각(ms). 상승하지 않았으면 null
  endsAt: number | null
}

type LoadState = { status: 'loading' } | { status: 'error' } | Loaded

// loadHelper는 상태를 받고, 쓸 수 있으면 실제 실행 권한도 받는다(도우미가 worker를 한 번 실행한다).
async function loadHelper(signal?: AbortSignal): Promise<Loaded> {
  const helper = await fetchHelperStatus(signal)
  const identity = helper.state === 'active' ? await fetchHelperIdentity(signal).catch(() => null) : null
  return { status: 'ready', helper, identity, endsAt: elevationEndsAt(helper, Date.now()) }
}

export function HelperSection() {
  const { t } = useTranslation()
  const dialogs = useDialogs()
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [busy, setBusy] = useState(false)
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const ctl = new AbortController()
    loadHelper(ctl.signal).then(setState, () => {
      if (!ctl.signal.aborted) setState({ status: 'error' })
    })
    return () => ctl.abort()
  }, [])

  const reload = async () => {
    try {
      const loaded = await loadHelper()
      setNow(Date.now())
      setState(loaded)
    } catch {
      setState({ status: 'error' })
    }
  }

  // 상승 중이면 1초마다 남은 시간을 다시 그리고, 끝나면 상태를 다시 받는다.
  const endsAt = state.status === 'ready' ? state.endsAt : null
  useEffect(() => {
    if (endsAt === null) return
    const id = window.setInterval(() => {
      const at = Date.now()
      setNow(at)
      if (at >= endsAt) {
        window.clearInterval(id)
        loadHelper().then(setState, () => setState({ status: 'error' }))
      }
    }, 1000)
    return () => window.clearInterval(id)
  }, [endsAt])

  const onElevate = async () => {
    if (await requestElevation(dialogs.scope)) await reload()
  }

  const onDrop = async () => {
    setBusy(true)
    try {
      await dropElevation()
    } catch {
      // 해제에 실패해도 상태를 다시 받아 실제 상태를 보여 준다.
    } finally {
      setBusy(false)
      await reload()
    }
  }

  return (
    <SettingsSection title={t('settings.helper.title')} description={t('settings.helper.desc')}>
      {state.status === 'loading' && <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>}
      {state.status === 'error' && <SettingsNotice tone="error">{t('settings.helper.loadFailed')}</SettingsNotice>}
      {state.status === 'ready' && (
        <HelperCard
          helper={state.helper}
          identity={state.identity}
          left={secondsLeft(state.endsAt, now)}
          busy={busy}
          onElevate={() => void onElevate()}
          onDrop={() => void onDrop()}
        />
      )}
    </SettingsSection>
  )
}

function HelperCard({
  helper,
  identity,
  left,
  busy,
  onElevate,
  onDrop,
}: {
  helper: HelperStatus
  identity: HelperIdentityResponse | null
  left: number
  busy: boolean
  onElevate: () => void
  onDrop: () => void
}) {
  const { t } = useTranslation()
  return (
    <SettingsCard>
      <SettingsRow label={t('settings.helper.status')} description={t(HELPER_STATE_DESC_KEYS[helper.state])}>
        <span className={`settings-badge helper-state ${helper.state}`}>{t(HELPER_STATE_KEYS[helper.state])}</span>
      </SettingsRow>
      {identity && <IdentityRow label={t('settings.helper.identity')} id={identity.user} />}
      {helper.state === 'active' && (
        <SettingsRow
          label={t('settings.helper.root')}
          description={helper.canElevate ? t('settings.helper.rootDesc') : t('settings.helper.notAllowed')}
        >
          {helper.canElevate && left > 0 && (
            <>
              <span className="settings-badge helper-elevated">
                {t('settings.helper.elevated', { time: formatClock(left) })}
              </span>
              <button type="button" className="addon-button" disabled={busy} onClick={onDrop}>
                {t('settings.helper.drop')}
              </button>
            </>
          )}
          {helper.canElevate && left === 0 && (
            <button type="button" className="addon-button primary" disabled={busy} onClick={onElevate}>
              {t('settings.helper.elevate')}
            </button>
          )}
        </SettingsRow>
      )}
      {identity?.root && left > 0 && <IdentityRow label={t('settings.helper.rootIdentity')} id={identity.root} />}
    </SettingsCard>
  )
}

function IdentityRow({ label, id }: { label: string; id: HelperIdentity }) {
  const { t } = useTranslation()
  return (
    <SettingsRow label={label} description={t('settings.helper.groups', { groups: groupNames(id) })}>
      <code className="helper-value selectable">{t('settings.helper.identityValue', { user: id.user, uid: id.uid })}</code>
    </SettingsRow>
  )
}
