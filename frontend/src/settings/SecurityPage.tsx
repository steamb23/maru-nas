// 설정의 보안 페이지: 이 세션의 확인 수준과 "지금 확인", 내 인증 수단(패스키, 인증 앱, 복구 코드) 관리와 새 기기에
// 줄 기기 추가 코드, 패스키로 로그인하는 브라우저. 수단을 바꾸기 전에는 비밀번호와 등록한 수단을 다시 확인해 관리 창(5분)을 연다
// (security/stepUpDialog.tsx). 패스키로 로그인의 켜기·해제는 관리 창 없이 한다.
import type { ParseKeys } from 'i18next'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { formatUnit } from '../addons/common/format'
import {
  ApiError,
  createDeviceCode,
  deletePasskey,
  deletePasskeyLogin,
  deleteTOTP,
  fetchSecurity,
  regenerateRecoveryCodes,
} from '../api/client'
import { PasskeyLoginDaysDefault, type SecurityChangeResponse, type SecurityStatus } from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { useLanguage } from '../i18n'
import { passkeyLoginFor, syncPasskeyLogins } from '../security/passkeyLoginStorage'
import {
  addPasskeyDialog,
  deviceCodeDialog,
  enablePasskeyLoginDialog,
  setupTOTPDialog,
  showRecoveryCodes,
} from '../security/securityDialogs'
import { ASSURANCE_KEYS, LEVEL_KEYS, type ManagePurpose, type SecurityTarget, securityError } from '../security/messages'
import { requestManage, requestStepUp } from '../security/stepUpDialog'
import { passkeysSupported } from '../security/webauthn'
import { useDialogs } from '../shell/dialogPresets'
import { SettingsCard, SettingsNotice, SettingsRow, SettingsSection } from './controls'

type LoadState = { status: 'loading' } | { status: 'error'; key: ParseKeys } | { status: 'ready'; data: SecurityStatus }

const TARGET_KEYS: Record<SecurityTarget, ParseKeys> = {
  terminal: 'security.targets.terminal',
  elevation: 'security.targets.elevation',
  files: 'security.targets.files',
}

// 관리 창이 이만큼 남지 않았으면 다시 확인한다(대화상자를 여는 사이에 닫히지 않게).
const MANAGE_MARGIN_MS = 15_000

const now = () => Date.now()

export function SecurityPage() {
  const { t } = useTranslation()
  const lang = useLanguage()
  const dialogs = useDialogs()
  const username = useAuthStore((s) => s.user?.username)
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [busy, setBusy] = useState(false)
  const [errorKey, setErrorKey] = useState<ParseKeys | null>(null)
  // 관리 창이 닫히는 시각(이 브라우저의 Date.now() 기준 ms)
  const manageUntil = useRef(0)

  // apply는 새 보안 상태를 보인다. 그리기 전에 이 브라우저의 패스키로 로그인 항목을 서버 목록에 맞춘다. 목록에 없으면
  // (해제, 만료, 패스키 삭제, 관리자 초기화) 지우고, 있으면 만료 시각을 서버 값으로 맞춘다(관리자가 기간을 줄였을 때).
  // 처음 불러올 때도 쓰므로 사용자 이름은 저장소에서 바로 읽는다.
  const apply = (st: SecurityStatus) => {
    const current = useAuthStore.getState().user?.username
    if (current) syncPasskeyLogins(current, st.passkeyLogins)
    manageUntil.current = now() + st.manageSeconds * 1000
    setState({ status: 'ready', data: st })
  }

  useEffect(() => {
    const ctl = new AbortController()
    fetchSecurity(ctl.signal).then(apply, (err: unknown) => {
      if (!ctl.signal.aborted) setState({ status: 'error', key: securityError(err).key })
    })
    return () => ctl.abort()
  }, [])

  const data = state.status === 'ready' ? state.data : null

  // ensureManage는 관리 창이 열려 있지 않으면 확인 대화상자로 연다. 열려 있거나 열었으면 true다. purpose는 확인
  // 대화상자의 수단 순서를 정한다(패스키 등록이면 기기 추가 코드도 받는다).
  const ensureManage = async (purpose?: ManagePurpose) => {
    if (now() < manageUntil.current - MANAGE_MARGIN_MS) return true
    const st = await requestManage(dialogs.scope, purpose)
    if (st) apply(st)
    return st !== null
  }

  // run은 수단을 바꾸는 작업 하나를 한다. 관리 창이 필요하면 먼저 열고, 새 복구 코드가 오면 한 번 보여 준다.
  // 상태를 바꾸지 않는 작업(기기 추가 코드)은 null을 돌려준다.
  const run = async (task: () => Promise<SecurityChangeResponse | null>, purpose?: ManagePurpose) => {
    if (busy) return
    setErrorKey(null)
    setBusy(true)
    try {
      if (!(await ensureManage(purpose))) return
      const res = await task()
      if (!res) return
      apply(res.status)
      if (res.recoveryCodes?.length) await showRecoveryCodes(dialogs.scope, res.recoveryCodes)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'manage_required') manageUntil.current = 0
      setErrorKey(securityError(err).key)
    } finally {
      setBusy(false)
    }
  }

  // runDirect는 관리 창이 필요 없는 작업(패스키로 로그인 켜기·해제) 하나를 한다. 취소하면 task가 null을 돌려준다.
  const runDirect = async (task: () => Promise<SecurityStatus | null>) => {
    if (busy) return
    setErrorKey(null)
    setBusy(true)
    try {
      const st = await task()
      if (st) apply(st)
    } catch (err) {
      setErrorKey(securityError(err).key)
    } finally {
      setBusy(false)
    }
  }

  const confirm = (title: ParseKeys, message: ParseKeys, confirmKey: ParseKeys) =>
    dialogs.confirm({ title: t(title), message: t(message), confirmLabel: t(confirmKey), danger: true })

  const verifyNow = async () => {
    const st = await requestStepUp(dialogs.scope)
    if (st) apply(st)
  }

  // disablePasskeyLogin은 등록 하나를 해제한다. 이미 없으면(다른 탭에서 해제, 만료) 목록만 다시 받는다.
  const disablePasskeyLogin = (id: string) =>
    void runDirect(async () => {
      if (!(await confirm('settings.security.disablePasskeyLoginTitle', 'settings.security.disablePasskeyLoginMessage', 'settings.security.disable')))
        return null
      try {
        return await deletePasskeyLogin(id)
      } catch (err) {
        if (!(err instanceof ApiError) || err.status !== 404) throw err
        return fetchSecurity()
      }
    })

  const date = (iso: string) => new Date(iso).toLocaleDateString(lang, { dateStyle: 'medium' })
  const supported = passkeysSupported()
  // 이 브라우저의 등록 ID. 읽을 때마다 저장소를 보므로 켜기·해제 뒤 다시 그리면 바로 바뀐다.
  const mine = username ? passkeyLoginFor(username)?.enrollment : undefined
  const loginDays = data?.policy.passkeyLoginDays ?? PasskeyLoginDaysDefault

  return (
    <>
      <SettingsNotice>{t('settings.security.intro')}</SettingsNotice>
      {state.status === 'loading' && <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>}
      {state.status === 'error' && <SettingsNotice tone="error">{t(state.key)}</SettingsNotice>}
      {errorKey && <SettingsNotice tone="error">{t(errorKey)}</SettingsNotice>}

      {data && (
        <>
          <SettingsSection title={t('settings.security.session')} description={t('settings.security.sessionDesc')}>
            <SettingsCard>
              <SettingsRow label={t('settings.security.assurance')} description={t(ASSURANCE_DESC_KEYS[data.assurance])}>
                <span className={`settings-badge security-assurance ${data.assurance}`}>{t(ASSURANCE_KEYS[data.assurance])}</span>
                {/* 수단이 없으면 비밀번호 재확인까지만 할 수 있다 */}
                {data.assurance !== 'twoFactor' && (data.methods.length > 0 || data.assurance === 'login') && (
                  <button type="button" className="addon-button" disabled={busy} onClick={() => void verifyNow()}>
                    {t('settings.security.verifyNow')}
                  </button>
                )}
              </SettingsRow>
              {(Object.keys(TARGET_KEYS) as SecurityTarget[]).map((target) => {
                const level = data.policy[target] ?? 'off'
                return (
                  <SettingsRow key={target} label={t(TARGET_KEYS[target])}>
                    <span className="settings-badge">{t(LEVEL_KEYS[level])}</span>
                  </SettingsRow>
                )
              })}
            </SettingsCard>
          </SettingsSection>

          <SettingsSection title={t('settings.security.passkeys')} description={t('settings.security.passkeysDesc')}>
            {!data.passkeysAvailable && <p className="settings-hint">{t('settings.security.passkeysOff')}</p>}
            {data.passkeysAvailable && !supported && <p className="settings-hint">{t('settings.security.passkeysUnsupported')}</p>}
            <SettingsCard>
              {data.passkeys.map((p) => (
                <SettingsRow
                  key={p.id}
                  label={<span className="selectable">{p.name}</span>}
                  description={[
                    p.lastUsed
                      ? t('settings.security.passkeyUsed', { created: date(p.created), used: date(p.lastUsed) })
                      : t('settings.security.passkeyCreated', { created: date(p.created) }),
                    // 동기화 저장소(iCloud 키체인, 비밀번호 관리자)의 패스키. 막지 않고 표시만 한다.
                    ...(p.backupEligible ? [t('settings.security.passkeySynced')] : []),
                  ].join(' · ')}
                >
                  <button
                    type="button"
                    className="addon-button"
                    disabled={busy}
                    onClick={() =>
                      void run(async () =>
                        (await confirm('settings.security.deletePasskeyTitle', 'settings.security.deletePasskeyMessage', 'settings.security.delete'))
                          ? deletePasskey(p.id)
                          : null,
                      )
                    }
                  >
                    {t('settings.security.delete')}
                  </button>
                </SettingsRow>
              ))}
              <SettingsRow
                label={t('settings.security.addPasskey')}
                description={data.passkeys.length === 0 ? t('settings.security.noPasskeys') : undefined}
              >
                <button
                  type="button"
                  className="addon-button primary"
                  disabled={busy || !data.passkeysAvailable || !supported}
                  onClick={() =>
                    void run(
                      () => addPasskeyDialog(dialogs.scope, t('settings.security.passkeyDefaultName', { n: data.passkeys.length + 1 })),
                      'addPasskey',
                    )
                  }
                >
                  {t('settings.security.add')}
                </button>
              </SettingsRow>
              {/* 이 기기의 패스키가 없는 새 기기에서 패스키를 등록할 때 쓴다. 이 브라우저의 패스키 지원과는 관계없다. */}
              <SettingsRow label={t('settings.security.deviceCode')} description={t('settings.security.deviceCodeDesc')}>
                <button
                  type="button"
                  className="addon-button"
                  disabled={busy || !data.passkeysAvailable || (data.passkeys.length === 0 && !data.totp)}
                  onClick={() =>
                    void run(async () => {
                      await deviceCodeDialog(dialogs.scope, await createDeviceCode())
                      return null
                    })
                  }
                >
                  {t('settings.security.deviceCodeCreate')}
                </button>
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>

          <SettingsSection
            title={t('settings.security.passkeyLogins')}
            description={t('settings.security.passkeyLoginsDesc', { days: formatUnit(loginDays, 'day', lang) })}
          >
            <SettingsCard>
              {data.passkeyLogins.map((l) => (
                <SettingsRow
                  key={l.id}
                  label={
                    <span className="selectable">
                      {data.passkeys.find((p) => p.id === l.passkey)?.name ?? t('settings.security.passkeyLoginUnknown')}
                    </span>
                  }
                  description={[
                    l.lastUsed
                      ? t('settings.security.passkeyLoginUsed', { created: date(l.created), used: date(l.lastUsed) })
                      : t('settings.security.passkeyLoginCreated', { created: date(l.created) }),
                    t('settings.security.passkeyLoginExpires', { expires: date(l.expiresAt) }),
                  ].join(' · ')}
                >
                  {l.id === mine && <span className="settings-badge">{t('settings.security.thisBrowser')}</span>}
                  <button type="button" className="addon-button" disabled={busy} onClick={() => disablePasskeyLogin(l.id)}>
                    {t('settings.security.disable')}
                  </button>
                </SettingsRow>
              ))}
              <SettingsRow
                label={t('settings.security.passkeyLoginHere')}
                description={
                  data.passkeys.length === 0
                    ? t('settings.security.passkeyLoginNeedsPasskey')
                    : mine
                      ? t('settings.security.passkeyLoginHereOn')
                      : t('settings.security.passkeyLoginHereOff')
                }
              >
                <button
                  type="button"
                  className={mine ? 'addon-button' : 'addon-button primary'}
                  disabled={busy || !username || !data.passkeysAvailable || !supported || data.passkeys.length === 0}
                  onClick={() => {
                    if (username) void runDirect(() => enablePasskeyLoginDialog(dialogs.scope, username, loginDays))
                  }}
                >
                  {mine ? t('settings.security.passkeyLoginReenable') : t('settings.security.passkeyLoginEnable')}
                </button>
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>

          <SettingsSection title={t('settings.security.totp')} description={t('settings.security.totpDesc')}>
            <SettingsCard>
              <SettingsRow
                label={t('settings.security.totpApp')}
                description={
                  data.totp ? t('settings.security.totpOn', { created: date(data.totp.created) }) : t('settings.security.totpOff')
                }
              >
                {data.totp && (
                  <button
                    type="button"
                    className="addon-button"
                    disabled={busy}
                    onClick={() =>
                      void run(async () =>
                        (await confirm('settings.security.deleteTotpTitle', 'settings.security.deleteTotpMessage', 'settings.security.delete'))
                          ? deleteTOTP()
                          : null,
                      )
                    }
                  >
                    {t('settings.security.delete')}
                  </button>
                )}
                <button
                  type="button"
                  className={data.totp ? 'addon-button' : 'addon-button primary'}
                  disabled={busy}
                  onClick={() => void run(() => setupTOTPDialog(dialogs.scope))}
                >
                  {data.totp ? t('settings.security.totpReplace') : t('settings.security.totpSetup')}
                </button>
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>

          <SettingsSection title={t('settings.security.recovery')} description={t('settings.security.recoveryDesc')}>
            <SettingsCard>
              <SettingsRow
                label={t('settings.security.recoveryLeft', { count: data.recoveryCodes })}
                description={data.passkeys.length === 0 && !data.totp ? t('settings.security.recoveryNeedsMethod') : undefined}
              >
                <button
                  type="button"
                  className="addon-button"
                  disabled={busy || (data.passkeys.length === 0 && !data.totp)}
                  onClick={() =>
                    void run(async () =>
                      (await confirm('settings.security.regenerateTitle', 'settings.security.regenerateMessage', 'settings.security.regenerate'))
                        ? regenerateRecoveryCodes()
                        : null,
                    )
                  }
                >
                  {t('settings.security.regenerate')}
                </button>
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
        </>
      )}
    </>
  )
}

const ASSURANCE_DESC_KEYS: Record<SecurityStatus['assurance'], ParseKeys> = {
  login: 'settings.security.assuranceLoginDesc',
  password: 'settings.security.assurancePasswordDesc',
  twoFactor: 'settings.security.assuranceTwoFactorDesc',
}
