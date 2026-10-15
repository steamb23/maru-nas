// 테마·배경 고르기. 사용자 화면 페이지와 관리자의 로그인 화면·기본 화면 페이지가 함께 쓴다.
// 고르기 칸은 숨긴 기본 라디오 버튼이라 화살표 키 이동과 화면 읽기는 브라우저 기본 동작을 따른다.
import { useId, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { Wallpaper } from '../api/types.gen'
import { THEME_MODES, type ThemeMode } from '../appearance/theme'
import { DEFAULT_PRESET, WALLPAPER_PRESETS, findPreset } from '../appearance/wallpapers'
import { IconImage } from '../icons/IconImage'
import { ACCEPTED_IMAGE_TYPES } from './imageUpload'

export function ThemePicker({
  value,
  onChange,
  disabled = false,
  label,
}: {
  value: ThemeMode
  onChange: (mode: ThemeMode) => void
  disabled?: boolean
  label: string
}) {
  const { t } = useTranslation()
  const name = useId()
  return (
    <div className="theme-picker" role="radiogroup" aria-label={label}>
      {THEME_MODES.map((mode) => (
        <label key={mode} className="theme-option">
          <input
            type="radio"
            className="picker-input"
            name={name}
            checked={value === mode}
            disabled={disabled}
            onChange={() => onChange(mode)}
          />
          <span className={`theme-preview theme-preview-${mode}`} aria-hidden="true">
            <span className="theme-preview-window">
              <span className="theme-preview-bar" />
              <span className="theme-preview-line" />
              <span className="theme-preview-line short" />
            </span>
          </span>
          <span className="theme-option-name">{t(`settings.screen.modes.${mode}`)}</span>
        </label>
      ))}
    </div>
  )
}

// selectedTile은 배경 서술자에 맞는 칸이다. 모르는 프리셋은 기본 배경 칸, 없는 이미지는 아무 칸도 아니다.
function selectedTile(value: Wallpaper | undefined, hasImage: boolean): string | null {
  if (!value) return null
  if (value.kind === 'image') return hasImage ? 'image' : null
  return findPreset(value.preset)?.id ?? DEFAULT_PRESET
}

export function WallpaperPicker({
  value,
  imageUrl,
  onSelect,
  onUpload,
  onDeleteImage,
  busy = false,
  disabled = false,
  label,
}: {
  // 지금 고른 배경. 없으면 아무 칸도 고르지 않은 것으로 그린다(기본값을 따르는 중).
  value: Wallpaper | undefined
  // 올린 이미지의 주소. 없으면 null이고 이미지 칸을 그리지 않는다.
  imageUrl: string | null
  onSelect: (w: Wallpaper) => void
  onUpload: (file: File) => void
  onDeleteImage: () => void
  // 올리거나 지우는 중이면 true
  busy?: boolean
  disabled?: boolean
  label: string
}) {
  const { t } = useTranslation()
  const name = useId()
  const fileRef = useRef<HTMLInputElement>(null)
  const selected = selectedTile(value, imageUrl !== null)
  const off = disabled || busy

  return (
    <div className="wallpaper-picker" role="radiogroup" aria-label={label}>
      {WALLPAPER_PRESETS.map((p) => (
        <label key={p.id} className="wallpaper-option" title={t(p.nameKey)}>
          <input
            type="radio"
            className="picker-input"
            name={name}
            checked={selected === p.id}
            disabled={off}
            onChange={() => onSelect({ kind: 'preset', preset: p.id })}
          />
          <span className="wallpaper-swatch" style={{ background: p.background }} aria-hidden="true" />
          <span className="wallpaper-option-name">{t(p.nameKey)}</span>
        </label>
      ))}
      {imageUrl && (
        <div className="wallpaper-image-option">
          <label className="wallpaper-option">
            <input
              type="radio"
              className="picker-input"
              name={name}
              checked={selected === 'image'}
              disabled={off}
              onChange={() => onSelect({ kind: 'image' })}
            />
            <span
              className="wallpaper-swatch"
              style={{ background: `center / cover no-repeat url("${imageUrl}")` }}
              aria-hidden="true"
            />
            <span className="wallpaper-option-name">{t('settings.image.uploaded')}</span>
          </label>
          <button
            type="button"
            className="wallpaper-delete"
            aria-label={t('settings.image.delete')}
            title={t('settings.image.delete')}
            disabled={off}
            onClick={onDeleteImage}
          >
            <IconImage name="edit-delete-symbolic" size={16} />
          </button>
        </div>
      )}
      <button
        type="button"
        className="wallpaper-option wallpaper-upload"
        disabled={off}
        onClick={() => fileRef.current?.click()}
      >
        <span className="wallpaper-swatch" aria-hidden="true">
          <IconImage name={busy ? 'view-refresh-symbolic' : 'list-add-symbolic'} size={20} />
        </span>
        <span className="wallpaper-option-name">{busy ? t('settings.image.uploading') : t('settings.image.upload')}</span>
      </button>
      <input
        ref={fileRef}
        type="file"
        hidden
        accept={ACCEPTED_IMAGE_TYPES.join(',')}
        onChange={(e) => {
          const file = e.target.files?.[0]
          // 같은 파일을 다시 고를 수 있게 비운다.
          e.target.value = ''
          if (file) onUpload(file)
        }}
      />
    </div>
  )
}
