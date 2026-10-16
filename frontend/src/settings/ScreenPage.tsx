// 설정의 화면 페이지(사용자): 테마, 바탕화면, 작업표시줄 정렬. 고르면 바로 적용하고 저장한다.
// 테마·바탕화면은 정하지 않으면 관리자가 정한 기본 화면을 따르며, 정한 값은 "기본값 사용"으로 비울 수 있다.
// 작업표시줄 정렬은 관리자 기본값이 없다(정하지 않으면 가운데).
import { useTranslation } from 'react-i18next'
import { useShallow } from 'zustand/react/shallow'
import { deleteWallpaperImage, uploadWallpaperImage, wallpaperImageUrl } from '../api/client'
import { useSiteStore } from '../appearance/siteStore'
import { useAppearanceStore } from '../appearance/store'
import { TASKBAR_ALIGNS, taskbarAlignOf } from '../appearance/taskbar'
import { effectiveTheme, toThemeMode } from '../appearance/theme'
import { SettingsCard, SettingsRow, SettingsSection, StoreNotice } from './controls'
import { ThemePicker, WallpaperPicker } from './pickers'
import { useImageActions } from './useImageActions'

export function ScreenPage() {
  const { t } = useTranslation()
  const { status, prefs, image, saveFailed } = useAppearanceStore(
    useShallow((s) => ({ status: s.status, prefs: s.prefs, image: s.image, saveFailed: s.saveFailed })),
  )
  const { update, imageUploaded, imageDeleted } = useAppearanceStore.getState()
  const site = useSiteStore((s) => s.info)
  const images = useImageActions()
  const ready = status === 'ready'

  const userTheme = toThemeMode(prefs.theme)
  const siteDefaultWallpaper = site?.defaultWallpaper
  // 기본값을 따르는 중이면 기본 바탕화면 칸을 고른 것으로 그린다. 관리자가 올린 이미지는 이 목록에 없다.
  const shownWallpaper =
    prefs.wallpaper ?? (siteDefaultWallpaper?.kind === 'preset' ? siteDefaultWallpaper : undefined)

  return (
    <>
      <StoreNotice status={status} saveFailed={saveFailed} />

      <SettingsSection title={t('settings.screen.theme')}>
        <ThemePicker
          label={t('settings.screen.theme')}
          value={userTheme ?? effectiveTheme(undefined, site?.defaultTheme)}
          disabled={!ready}
          onChange={(mode) => update({ theme: mode })}
        />
        <FollowDefault following={userTheme === null} disabled={!ready} onReset={() => update({ theme: undefined })} />
      </SettingsSection>

      <SettingsSection title={t('settings.screen.wallpaper')}>
        <WallpaperPicker
          label={t('settings.screen.wallpaper')}
          value={shownWallpaper}
          imageUrl={image ? wallpaperImageUrl(image.rev) : null}
          disabled={!ready}
          busy={images.busy}
          onSelect={(w) => update({ wallpaper: w })}
          onUpload={(file) => void images.upload(file, uploadWallpaperImage, imageUploaded)}
          onDeleteImage={() => void images.remove(deleteWallpaperImage, imageDeleted)}
        />
        <p className={images.errorKey ? 'settings-hint error' : 'settings-hint'} role={images.errorKey ? 'alert' : undefined}>
          {images.errorKey ? t(images.errorKey) : t('settings.image.hint')}
        </p>
        <FollowDefault
          following={prefs.wallpaper === undefined}
          disabled={!ready}
          onReset={() => update({ wallpaper: undefined })}
        />
      </SettingsSection>

      <SettingsSection title={t('settings.screen.taskbar')}>
        <SettingsCard>
          <SettingsRow label={t('settings.screen.taskbarAlign')} description={t('settings.screen.taskbarAlignDesc')}>
            <select
              className="settings-select"
              aria-label={t('settings.screen.taskbarAlign')}
              value={taskbarAlignOf(prefs.taskbarAlign)}
              disabled={!ready}
              onChange={(e) => update({ taskbarAlign: taskbarAlignOf(e.target.value) })}
            >
              {TASKBAR_ALIGNS.map((align) => (
                <option key={align} value={align}>
                  {t(`settings.screen.taskbarAligns.${align}`)}
                </option>
              ))}
            </select>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    </>
  )
}

// FollowDefault는 관리자 기본값을 따르는 중인지 알리거나, 정한 값을 비워 기본값을 따르게 하는 버튼을 둔다.
function FollowDefault({ following, disabled, onReset }: { following: boolean; disabled: boolean; onReset: () => void }) {
  const { t } = useTranslation()
  if (following) return <p className="settings-hint">{t('settings.screen.following')}</p>
  return (
    <div className="settings-actions">
      <button type="button" className="addon-button" disabled={disabled} onClick={onReset}>
        {t('settings.screen.useDefault')}
      </button>
    </div>
  )
}
