// 설정의 로그인 화면 페이지(관리자): 로그인 배경과 대기 연출. 모든 사용자의 로그인 화면에 적용된다.
import { useTranslation } from 'react-i18next'
import { deleteSiteImage, saveSiteLogin, siteImageUrl, uploadSiteImage } from '../api/client'
import type { SiteInfo, Wallpaper } from '../api/types.gen'
import { useSiteStore } from '../appearance/siteStore'
import { DEFAULT_PRESET } from '../appearance/wallpapers'
import { SettingsCard, SettingsNotice, SettingsRow, SettingsSection, Switch } from './controls'
import { WallpaperPicker } from './pickers'
import { useImageActions } from './useImageActions'
import { useSiteSave } from './siteSettings'

const DEFAULT_WALLPAPER: Wallpaper = { kind: 'preset', preset: DEFAULT_PRESET }

export function LoginScreenPage() {
  const { t } = useTranslation()
  const info = useSiteStore((s) => s.info)
  const images = useImageActions()
  const site = useSiteSave()
  const ready = info !== null

  const saveLogin = (patch: Partial<Pick<SiteInfo, 'loginBackground' | 'loginEasterEgg'>>) => {
    if (!info) return
    const next = { ...info, ...patch }
    void site.save(
      () => next,
      () => saveSiteLogin({ background: next.loginBackground, easterEgg: next.loginEasterEgg }),
    )
  }

  return (
    <>
      <SettingsNotice>{t('settings.loginScreen.intro')}</SettingsNotice>
      {!ready && <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>}
      {site.failed && <SettingsNotice tone="error">{t('settings.common.siteSaveFailed')}</SettingsNotice>}

      <SettingsSection title={t('settings.loginScreen.background')}>
        <WallpaperPicker
          label={t('settings.loginScreen.background')}
          value={info?.loginBackground ?? DEFAULT_WALLPAPER}
          imageUrl={info?.images.loginBackground ? siteImageUrl('login-background', info.images.loginBackground.rev) : null}
          disabled={!ready || site.saving}
          busy={images.busy}
          onSelect={(w) => saveLogin({ loginBackground: w })}
          onUpload={(file) =>
            void images.upload(file, (f) => uploadSiteImage('login-background', f), () => void site.reload())
          }
          onDeleteImage={() =>
            void images.remove(
              () => deleteSiteImage('login-background'),
              () => void site.reload(),
            )
          }
        />
        <p className={images.errorKey ? 'settings-hint error' : 'settings-hint'} role={images.errorKey ? 'alert' : undefined}>
          {images.errorKey ? t(images.errorKey) : t('settings.loginScreen.backgroundHint')}
        </p>
      </SettingsSection>

      <SettingsSection title={t('settings.loginScreen.idle')}>
        <SettingsCard>
          <SettingsRow label={t('settings.loginScreen.easterEgg')} description={t('settings.loginScreen.easterEggDesc')}>
            <Switch
              label={t('settings.loginScreen.easterEgg')}
              checked={info?.loginEasterEgg ?? true}
              disabled={!ready || site.saving}
              onChange={(on) => saveLogin({ loginEasterEgg: on })}
            />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    </>
  )
}
