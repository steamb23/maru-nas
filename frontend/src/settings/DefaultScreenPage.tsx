// 설정의 기본 화면 페이지(관리자): 화면 설정에서 테마나 바탕화면을 정하지 않은 사용자에게 적용하는 기본값.
import { useTranslation } from 'react-i18next'
import { deleteSiteImage, saveSiteDefaults, siteImageUrl, uploadSiteImage } from '../api/client'
import type { SiteInfo, Wallpaper } from '../api/types.gen'
import { useSiteStore } from '../appearance/siteStore'
import { type ThemeMode, toThemeMode } from '../appearance/theme'
import { DEFAULT_PRESET } from '../appearance/wallpapers'
import { SettingsNotice, SettingsSection } from './controls'
import { ThemePicker, WallpaperPicker } from './pickers'
import { useImageActions } from './useImageActions'
import { useSiteSave } from './siteSettings'

const DEFAULT_WALLPAPER: Wallpaper = { kind: 'preset', preset: DEFAULT_PRESET }

export function DefaultScreenPage() {
  const { t } = useTranslation()
  const info = useSiteStore((s) => s.info)
  const images = useImageActions()
  const site = useSiteSave()
  const ready = info !== null

  const saveDefaults = (patch: { theme?: ThemeMode; wallpaper?: Wallpaper }) => {
    if (!info) return
    const next: SiteInfo = {
      ...info,
      defaultTheme: patch.theme ?? info.defaultTheme,
      defaultWallpaper: patch.wallpaper ?? info.defaultWallpaper,
    }
    void site.save(
      () => next,
      () => saveSiteDefaults({ theme: next.defaultTheme, wallpaper: next.defaultWallpaper }),
    )
  }

  return (
    <>
      <SettingsNotice>{t('settings.defaultScreen.intro')}</SettingsNotice>
      {!ready && <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>}
      {site.failed && <SettingsNotice tone="error">{t('settings.common.siteSaveFailed')}</SettingsNotice>}

      <SettingsSection title={t('settings.screen.theme')}>
        <ThemePicker
          label={t('settings.screen.theme')}
          value={toThemeMode(info?.defaultTheme) ?? 'auto'}
          disabled={!ready || site.saving}
          onChange={(mode) => saveDefaults({ theme: mode })}
        />
      </SettingsSection>

      <SettingsSection title={t('settings.screen.wallpaper')}>
        <WallpaperPicker
          label={t('settings.screen.wallpaper')}
          value={info?.defaultWallpaper ?? DEFAULT_WALLPAPER}
          imageUrl={
            info?.images.defaultWallpaper ? siteImageUrl('default-wallpaper', info.images.defaultWallpaper.rev) : null
          }
          disabled={!ready || site.saving}
          busy={images.busy}
          onSelect={(w) => saveDefaults({ wallpaper: w })}
          onUpload={(file) =>
            void images.upload(file, (f) => uploadSiteImage('default-wallpaper', f), () => void site.reload())
          }
          onDeleteImage={() =>
            void images.remove(
              () => deleteSiteImage('default-wallpaper'),
              () => void site.reload(),
            )
          }
        />
        <p className={images.errorKey ? 'settings-hint error' : 'settings-hint'} role={images.errorKey ? 'alert' : undefined}>
          {images.errorKey ? t(images.errorKey) : t('settings.image.hint')}
        </p>
      </SettingsSection>
    </>
  )
}
