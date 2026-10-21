// 설정의 보안 정책 페이지(관리자): 기능별 보안 강화 단계, 패스키 공개 주소와 OMV 자동 로그아웃 안내, 패스키로
// 로그인의 만료 기간, 사용자 인증 수단 초기화. 정책을 바꾸거나 초기화하기 전에 확인을 요구하면(security_required)
// 확인한 뒤 다시 시도한다.
import type { ParseKeys } from 'i18next'
import { Fragment, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { formatUnit } from '../addons/common/format'
import {
  ApiError,
  fetchSecurityPolicy,
  fetchSecurityUsers,
  resetSecurityUser,
  saveSecurityPolicy,
} from '../api/client'
import {
  PasskeyLoginDaysDefault,
  type SecurityPolicy,
  type SecurityPolicyResponse,
  type SecurityUserInfo,
} from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { useLanguage } from '../i18n'
import { LEVEL_KEYS, type Level, type SecurityTarget, securityError } from '../security/messages'
import { PASSKEY_LOGIN_DAYS } from '../security/passkeyLogin'
import { requestStepUp } from '../security/stepUpDialog'
import { useDialogs } from '../shell/dialogPresets'
import { SettingsCard, SettingsNotice, SettingsRow, SettingsSection } from './controls'

interface Loaded {
  status: 'ready'
  policy: SecurityPolicyResponse
  users: SecurityUserInfo[]
}

type LoadState = { status: 'loading' } | { status: 'error' } | Loaded

const TARGETS: readonly { id: SecurityTarget; label: ParseKeys; desc: ParseKeys }[] = [
  { id: 'terminal', label: 'security.targets.terminal', desc: 'settings.securityPolicy.terminalDesc' },
  { id: 'elevation', label: 'security.targets.elevation', desc: 'settings.securityPolicy.elevationDesc' },
  { id: 'files', label: 'security.targets.files', desc: 'settings.securityPolicy.filesDesc' },
]

const LEVELS: readonly Level[] = ['off', 'password', 'twoFactor']

type FilesScope = NonNullable<SecurityPolicy['filesScope']>

type LoginDays = (typeof PASSKEY_LOGIN_DAYS)[number]

async function load(signal?: AbortSignal): Promise<Loaded> {
  const [policy, users] = await Promise.all([fetchSecurityPolicy(signal), fetchSecurityUsers(signal)])
  return { status: 'ready', policy, users }
}

export function SecurityPolicyPage() {
  const { t } = useTranslation()
  const lang = useLanguage()
  const dialogs = useDialogs()
  const me = useAuthStore((s) => s.user?.username)
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [busy, setBusy] = useState(false)
  const [errorKey, setErrorKey] = useState<ParseKeys | null>(null)

  useEffect(() => {
    const ctl = new AbortController()
    load(ctl.signal).then(setState, () => {
      if (!ctl.signal.aborted) setState({ status: 'error' })
    })
    return () => ctl.abort()
  }, [])

  const reload = async () => {
    try {
      setState(await load())
    } catch {
      setState({ status: 'error' })
    }
  }

  // withStepUp은 작업을 하고, 확인을 먼저 거치라는 답(security_required)이면 확인한 뒤 한 번 더 한다.
  // 취소하면 false다.
  const withStepUp = async (task: () => Promise<void>): Promise<boolean> => {
    try {
      await task()
      return true
    } catch (err) {
      if (!(err instanceof ApiError) || err.code !== 'security_required') throw err
      if (!(await requestStepUp(dialogs.scope, { level: err.required }))) return false
      await task()
      return true
    }
  }

  const act = async (task: () => Promise<boolean>) => {
    if (busy) return
    setBusy(true)
    setErrorKey(null)
    try {
      await task()
    } catch (err) {
      setErrorKey(err instanceof ApiError && err.status === 400 ? 'settings.securityPolicy.invalid' : securityError(err).key)
    } finally {
      setBusy(false)
      await reload()
    }
  }

  const setLevel = (current: SecurityPolicy, target: SecurityTarget, level: Level) =>
    void act(() => withStepUp(() => saveSecurityPolicy({ ...current, [target]: level })))

  // 파일 탐색기 확인을 어디에 적용할지 바꾼다(그 단계를 거친 세션이어야 한다).
  const setFilesScope = (current: SecurityPolicy, filesScope: FilesScope) =>
    void act(() => withStepUp(() => saveSecurityPolicy({ ...current, filesScope })))

  // 만료 기간만 바꾸고 나머지는 지금 정책 그대로 보낸다(비밀번호를 다시 확인한 세션이어야 한다).
  const setLoginDays = (current: SecurityPolicy, days: LoginDays) =>
    void act(() => withStepUp(() => saveSecurityPolicy({ ...current, passkeyLoginDays: days })))

  const reset = (username: string) =>
    void act(async () => {
      const ok = await dialogs.confirm({
        title: t('settings.securityPolicy.resetTitle'),
        message: t('settings.securityPolicy.resetMessage', { user: username }),
        confirmLabel: t('settings.securityPolicy.reset'),
        danger: true,
      })
      return ok && withStepUp(() => resetSecurityUser(username))
    })

  const data = state.status === 'ready' ? state : null
  const anyOn = data !== null && TARGETS.some((x) => (data.policy.policy[x.id] ?? 'off') !== 'off')

  return (
    <>
      <SettingsNotice>{t('settings.securityPolicy.intro')}</SettingsNotice>
      {state.status === 'loading' && <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>}
      {state.status === 'error' && <SettingsNotice tone="error">{t('settings.securityPolicy.loadFailed')}</SettingsNotice>}
      {errorKey && <SettingsNotice tone="error">{t(errorKey)}</SettingsNotice>}

      {data && (
        <>
          <SettingsSection title={t('settings.securityPolicy.levels')} description={t('settings.securityPolicy.levelsDesc')}>
            {anyOn && data.policy.autoLogoutMinutes === 0 && (
              <SettingsNotice tone="error">{t('settings.securityPolicy.noAutoLogout')}</SettingsNotice>
            )}
            <SettingsCard>
              {TARGETS.map((x) => (
                <Fragment key={x.id}>
                  <SettingsRow label={t(x.label)} description={t(x.desc)}>
                    <select
                      className="settings-select"
                      aria-label={t(x.label)}
                      value={data.policy.policy[x.id] ?? 'off'}
                      disabled={busy}
                      onChange={(e) => setLevel(data.policy.policy, x.id, e.target.value as Level)}
                    >
                      {LEVELS.map((l) => (
                        <option key={l} value={l}>
                          {t(LEVEL_KEYS[l])}
                        </option>
                      ))}
                    </select>
                  </SettingsRow>
                  {x.id === 'files' && (data.policy.policy.files ?? 'off') !== 'off' && (
                    <SettingsRow label={t('settings.securityPolicy.filesScope')} description={t('settings.securityPolicy.filesScopeDesc')}>
                      <select
                        className="settings-select"
                        aria-label={t('settings.securityPolicy.filesScope')}
                        value={data.policy.policy.filesScope ?? 'all'}
                        disabled={busy}
                        onChange={(e) => setFilesScope(data.policy.policy, e.target.value as FilesScope)}
                      >
                        <option value="all">{t('settings.securityPolicy.filesScopeAll')}</option>
                        <option value="writes">{t('settings.securityPolicy.filesScopeWrites')}</option>
                      </select>
                    </SettingsRow>
                  )}
                </Fragment>
              ))}
              <SettingsRow
                label={t('settings.securityPolicy.passkeys')}
                description={
                  data.policy.passkeyRpId
                    ? t('settings.securityPolicy.passkeysOn', { rpId: data.policy.passkeyRpId })
                    : t('settings.securityPolicy.passkeysOff')
                }
              >
                <span className="settings-badge">
                  {data.policy.passkeyRpId ? t('settings.securityPolicy.available') : t('settings.securityPolicy.unavailable')}
                </span>
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>

          <SettingsSection title={t('settings.securityPolicy.passkeyLogin')}>
            <SettingsCard>
              <SettingsRow label={t('settings.securityPolicy.passkeyLoginDays')} description={t('settings.securityPolicy.passkeyLoginDaysDesc')}>
                <select
                  className="settings-select"
                  aria-label={t('settings.securityPolicy.passkeyLoginDays')}
                  value={data.policy.policy.passkeyLoginDays ?? PasskeyLoginDaysDefault}
                  disabled={busy}
                  onChange={(e) => setLoginDays(data.policy.policy, Number(e.target.value) as LoginDays)}
                >
                  {PASSKEY_LOGIN_DAYS.map((d) => (
                    <option key={d} value={d}>
                      {formatUnit(d, 'day', lang)}
                    </option>
                  ))}
                </select>
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>

          <SettingsSection title={t('settings.securityPolicy.users')} description={t('settings.securityPolicy.usersDesc')}>
            {data.users.length === 0 ? (
              <p className="settings-hint">{t('settings.securityPolicy.noUsers')}</p>
            ) : (
              <SettingsCard>
                {data.users.map((u) => (
                  <SettingsRow
                    key={u.username}
                    label={<span className="selectable">{u.username}</span>}
                    description={
                      u.unreadable
                        ? t('settings.securityPolicy.unreadable')
                        : t('settings.securityPolicy.userMethods', {
                            passkeys: u.passkeys,
                            totp: u.totp ? t('settings.securityPolicy.yes') : t('settings.securityPolicy.no'),
                            recovery: u.recoveryCodes,
                          })
                    }
                  >
                    {u.username === me ? (
                      <span className="settings-badge">{t('settings.securityPolicy.you')}</span>
                    ) : (
                      <button type="button" className="addon-button" disabled={busy} onClick={() => reset(u.username)}>
                        {t('settings.securityPolicy.reset')}
                      </button>
                    )}
                  </SettingsRow>
                ))}
              </SettingsCard>
            )}
          </SettingsSection>
        </>
      )}
    </>
  )
}
