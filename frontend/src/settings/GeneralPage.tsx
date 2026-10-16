// 설정의 일반 페이지(사용자): 언어(이 브라우저)와 애니메이션.
import { useTranslation } from 'react-i18next'
import { useShallow } from 'zustand/react/shallow'
import { useAppearanceStore } from '../appearance/store'
import { setLanguage, useLanguage } from '../i18n'
import { SUPPORTED_LANGUAGES, isLanguage, languageName } from '../i18n/languages'
import { SettingsCard, SettingsRow, SettingsSection, StoreNotice, Switch } from './controls'

export function GeneralPage() {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { status, animations, saveFailed } = useAppearanceStore(
    useShallow((s) => ({ status: s.status, animations: s.prefs.animations, saveFailed: s.saveFailed })),
  )
  const { update } = useAppearanceStore.getState()
  // 시스템의 '동작 줄이기'가 켜져 있으면 설정과 관계없이 애니메이션이 꺼진다(animationsEnabled).
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches

  return (
    <>
      <StoreNotice status={status} saveFailed={saveFailed} />

      <SettingsSection title={t('settings.general.language')}>
        <SettingsCard>
          <SettingsRow label={t('settings.general.language')} description={t('settings.general.languageDesc')}>
            <select
              className="settings-select"
              aria-label={t('settings.general.language')}
              value={lang}
              onChange={(e) => {
                if (isLanguage(e.target.value)) void setLanguage(e.target.value)
              }}
            >
              {SUPPORTED_LANGUAGES.map((code) => (
                <option key={code} value={code} lang={code}>
                  {languageName(code)}
                </option>
              ))}
            </select>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title={t('settings.general.motion')}>
        <SettingsCard>
          <SettingsRow
            label={t('settings.general.animations')}
            description={reduced ? t('settings.general.reducedMotion') : t('settings.general.animationsDesc')}
          >
            <Switch
              label={t('settings.general.animations')}
              checked={animations !== false && !reduced}
              disabled={status !== 'ready' || reduced}
              onChange={(on) => update({ animations: on })}
            />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    </>
  )
}
