// 설정의 서버 페이지(관리자): 웹에서 바꾸는 가동 설정(워크벤치 주소, SMART 확인 시각)과 가동 설정 현황.
// 나머지 가동 설정은 /etc/default/onmaru에서 바꾸고 서비스를 다시 시작해야 하므로 값·출처와 방법만 보여 준다.
import type { ParseKeys } from 'i18next'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useCapabilitiesStore } from '../addons/capabilities'
import { ApiError, fetchCapabilities, fetchServerSettings, saveServerSettings } from '../api/client'
import type { ServerSettings } from '../api/types.gen'
import { IconImage } from '../icons/IconImage'
import { SettingsCard, SettingsNotice, SettingsRow, SettingsSection } from './controls'
import { OVERRIDE_FLAGS, type OverrideKey, SOURCE_LABELS, overridesWith, runtimeLabelKey } from './server'

const HOW_TO_COMMANDS = 'sudoedit /etc/default/onmaru\nsudo systemctl restart onmaru'

type LoadState = { status: 'loading' } | { status: 'ready'; data: ServerSettings } | { status: 'error' }

export function ServerPage() {
  const { t } = useTranslation()
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [saving, setSaving] = useState<OverrideKey | null>(null)
  const [errorKey, setErrorKey] = useState<ParseKeys | null>(null)

  useEffect(() => {
    const ctl = new AbortController()
    fetchServerSettings(ctl.signal).then(
      (data) => setState({ status: 'ready', data }),
      () => {
        if (!ctl.signal.aborted) setState({ status: 'error' })
      },
    )
    return () => ctl.abort()
  }, [])

  // reload는 저장한 뒤 서버 값을 다시 받는다.
  const reload = async () => {
    try {
      setState({ status: 'ready', data: await fetchServerSettings() })
    } catch {
      setState({ status: 'error' })
    }
  }

  // save는 key 하나를 value로 덮어쓰거나(null이면 서버 값으로 되돌림) 저장한 뒤 다시 불러온다.
  // 워크벤치 주소는 capability로 앱 목록·프로필 메뉴에 반영되므로 capability도 다시 받는다.
  const save = async (key: OverrideKey, value: string | null) => {
    if (state.status !== 'ready') return
    setSaving(key)
    setErrorKey(null)
    try {
      await saveServerSettings(overridesWith(state.data, key, value))
      fetchCapabilities().then(useCapabilitiesStore.getState().set, () => {})
    } catch (err) {
      setErrorKey(err instanceof ApiError && err.status === 400 ? 'settings.server.invalid' : 'settings.server.saveFailed')
    } finally {
      setSaving(null)
      await reload()
    }
  }

  const data = state.status === 'ready' ? state.data : null

  return (
    <>
      <SettingsNotice>{t('settings.server.intro')}</SettingsNotice>
      {state.status === 'loading' && <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>}
      {state.status === 'error' && <SettingsNotice tone="error">{t('settings.server.loadFailed')}</SettingsNotice>}
      {errorKey && <SettingsNotice tone="error">{t(errorKey)}</SettingsNotice>}

      {data && (
        <SettingsSection title={t('settings.server.editable')} description={t('settings.server.editableDesc')}>
          <SettingsCard>
            <OverrideRow
              key={`wb-${data.workbenchUrl.value}`}
              label={t('settings.server.flags.workbenchUrl')}
              description={t('settings.server.workbenchDesc')}
              setting={data.workbenchUrl}
              type="url"
              placeholder="https://nas.example.lan/"
              busy={saving === 'workbenchUrl'}
              onSave={(v) => void save('workbenchUrl', v)}
            />
            <OverrideRow
              key={`smart-${data.smartRefreshAt.value}`}
              label={t('settings.server.flags.smartRefreshAt')}
              description={t('settings.server.smartDesc')}
              setting={data.smartRefreshAt}
              type="time"
              busy={saving === 'smartRefreshAt'}
              onSave={(v) => void save('smartRefreshAt', v)}
            />
          </SettingsCard>
        </SettingsSection>
      )}

      {data && (
        <SettingsSection title={t('settings.server.runtime')} description={t('settings.server.runtimeDesc')}>
          <SettingsCard>
            {data.runtime.map((r) => {
              const labelKey = runtimeLabelKey(r.flag)
              const overridden = (Object.keys(OVERRIDE_FLAGS) as OverrideKey[]).some(
                (k) => OVERRIDE_FLAGS[k] === r.flag && data[k].overridden,
              )
              return (
                <SettingsRow
                  key={r.flag}
                  label={labelKey ? t(labelKey) : r.flag}
                  description={<code className="server-code selectable">{r.env}</code>}
                >
                  <code className="server-value selectable">{r.value === '' ? t('settings.server.empty') : r.value}</code>
                  <span className="settings-badge">{t(SOURCE_LABELS[r.source])}</span>
                  {overridden && <span className="settings-badge server-overridden">{t('settings.server.overriddenBadge')}</span>}
                </SettingsRow>
              )
            })}
          </SettingsCard>
        </SettingsSection>
      )}

      <SettingsSection title={t('settings.server.howTo')}>
        <p className="settings-section-desc">{t('settings.server.howToDesc')}</p>
        <div className="server-commands">
          <pre className="selectable">{HOW_TO_COMMANDS}</pre>
          <CopyButton text={HOW_TO_COMMANDS} />
        </div>
        <p className="settings-hint">{t('settings.server.howToInstall')}</p>
      </SettingsSection>
    </>
  )
}

// OverrideRow는 웹에서 덮어쓰는 설정 한 줄이다. 입력한 값을 저장하거나 서버 값(플래그·환경 변수)으로 되돌린다.
// 값이 바뀌면 부모가 key를 바꿔 입력 상자를 새 값으로 다시 시작한다.
function OverrideRow({
  label,
  description,
  setting,
  type,
  placeholder,
  busy,
  onSave,
}: {
  label: string
  description: string
  setting: ServerSettings['workbenchUrl']
  type: 'url' | 'time'
  placeholder?: string
  busy: boolean
  onSave: (value: string | null) => void
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(setting.value)
  const base = setting.base === '' ? t('settings.server.empty') : setting.base
  const changed = draft !== setting.value
  return (
    <SettingsRow
      label={label}
      description={
        <>
          {description}
          <span className="server-base">
            {setting.overridden
              ? t('settings.server.overridden', { base })
              : t('settings.server.fromServer', { base })}
          </span>
        </>
      }
    >
      <form
        className="server-override"
        onSubmit={(e) => {
          e.preventDefault()
          if (changed || !setting.overridden) onSave(draft.trim())
        }}
      >
        <input
          className="settings-input"
          type={type === 'url' ? 'text' : 'time'}
          inputMode={type === 'url' ? 'url' : undefined}
          value={draft}
          placeholder={placeholder}
          aria-label={label}
          disabled={busy}
          required={type === 'time'}
          onChange={(e) => setDraft(e.target.value)}
        />
        <button type="submit" className="addon-button primary" disabled={busy || (!changed && setting.overridden)}>
          {t('settings.server.save')}
        </button>
        {setting.overridden && (
          <button type="button" className="addon-button" disabled={busy} onClick={() => onSave(null)}>
            {t('settings.server.reset')}
          </button>
        )}
      </form>
    </SettingsRow>
  )
}

// CopyButton은 클립보드에 복사한다. 클립보드를 쓸 수 없는 환경(평문 HTTP 등)이면 그리지 않는다(글자는 선택해 복사).
function CopyButton({ text }: { text: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const id = window.setTimeout(() => setCopied(false), 2000)
    return () => window.clearTimeout(id)
  }, [copied])
  if (!navigator.clipboard) return null
  const label = copied ? t('settings.server.copied') : t('settings.server.copy')
  return (
    <button
      type="button"
      className="server-copy"
      aria-label={label}
      title={label}
      onClick={() => navigator.clipboard.writeText(text).then(() => setCopied(true), () => {})}
    >
      <IconImage name={copied ? 'object-select-symbolic' : 'edit-copy-symbolic'} size={16} />
    </button>
  )
}
