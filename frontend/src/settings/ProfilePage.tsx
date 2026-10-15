// 설정의 프로필 페이지: 프로필 사진(자르기 편집기로 바꾸기, 지우기), OMV 계정 정보(전자 우편, 태그, 그룹),
// 전자 우편·태그 편집, OMV 사용자 편집 화면 열기(관리자). 계정 정보는 GET /api/profile, 저장은 PUT /api/profile이다.
// 태그는 OMV 사용자 편집 화면의 "Tags"와 같은 값이다.
import type { ParseKeys } from 'i18next'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useCapabilitiesStore } from '../addons/capabilities'
import { ApiError, deleteAvatarImage, fetchProfile, saveProfile } from '../api/client'
import type { ProfileRequest, ProfileResponse } from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { IconImage } from '../icons/IconImage'
import { useAvatarStore } from '../profile/store'
import { addTags, sameTags, validEmail, validTags } from '../profile/tags'
import { useDialogs } from '../shell/dialogPresets'
import { openWorkbenchRoute } from '../shell/launchApp'
import { ProfileAvatar } from '../shell/ProfileMenu'
import { userEditRoute } from '../windows/links'
import { SettingsCard, SettingsNotice, SettingsRow, SettingsSection } from './controls'
import { editAvatar } from './editAvatar'
import { ACCEPTED_IMAGE_TYPES } from './imageUpload'
import { TagInput } from './TagInput'
import { useImageActions } from './useImageActions'

type LoadState = { status: 'loading' } | { status: 'ready'; profile: ProfileResponse } | { status: 'error' }

interface Notice {
  tone: 'info' | 'error'
  key: ParseKeys
}

// saveErrorKey는 계정 정보 저장 실패를 문구 키로 바꾼다.
function saveErrorKey(err: unknown): ParseKeys {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'config_dirty':
        return 'settings.profile.saveConfigDirty'
      case 'action_failed':
        return 'settings.profile.saveApplyFailed'
      case 'profile_readonly':
        return 'settings.profile.readOnly.locked'
      case 'action_busy':
        return 'settings.profile.saveBusy'
      case 'bad_request':
        return 'settings.profile.invalid'
    }
  }
  return 'settings.profile.saveFailed'
}

export function ProfilePage() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const admin = user?.admin === true
  const workbench = useCapabilitiesStore((s) => Boolean(s.caps?.workbenchUrl))
  const avatar = useAvatarStore((s) => s.image)
  const dialogs = useDialogs()
  const images = useImageActions()
  const fileRef = useRef<HTMLInputElement>(null)
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [photoError, setPhotoError] = useState<ParseKeys | null>(null)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState<Notice | null>(null)

  useEffect(() => {
    const ctl = new AbortController()
    fetchProfile(ctl.signal).then(
      (profile) => setState({ status: 'ready', profile }),
      () => {
        if (!ctl.signal.aborted) setState({ status: 'error' })
      },
    )
    return () => ctl.abort()
  }, [])

  if (!user) return null
  const profile = state.status === 'ready' ? state.profile : null

  const pickPhoto = async (file: File) => {
    setPhotoError(null)
    setPhotoError(await editAvatar(dialogs, file, t('settings.profile.editor.title')))
  }

  const removePhoto = () =>
    void images.remove(deleteAvatarImage, () => useAvatarStore.getState().deleted(), {
      title: 'settings.profile.removePhoto',
      message: 'settings.profile.removePhotoConfirm',
    })

  const save = async (change: ProfileRequest) => {
    setSaving(true)
    setNotice(null)
    try {
      const res = await saveProfile(change)
      setNotice({ tone: 'info', key: res.applied ? 'settings.profile.savedApplied' : 'settings.profile.savedPending' })
    } catch (err) {
      setNotice({ tone: 'error', key: saveErrorKey(err) })
    } finally {
      setSaving(false)
      try {
        setState({ status: 'ready', profile: await fetchProfile() })
      } catch {
        // 바로 전 값을 그대로 둔다.
      }
    }
  }

  const photoErrorKey = photoError ?? images.errorKey

  return (
    <>
      <div className="profile-head">
        <button
          type="button"
          className="profile-photo"
          aria-label={t('settings.profile.changePhoto')}
          title={t('settings.profile.changePhoto')}
          disabled={images.busy}
          onClick={() => fileRef.current?.click()}
        >
          <ProfileAvatar user={user} size={96} />
        </button>
        <div className="profile-head-text">
          <span className="profile-head-name">{user.username}</span>
          <span className="profile-head-role">{admin ? t('profile.roleAdmin') : t('profile.roleUser')}</span>
          <div className="settings-actions">
            <button
              type="button"
              className="addon-button"
              disabled={images.busy}
              onClick={() => fileRef.current?.click()}
            >
              {t('settings.profile.changePhoto')}
            </button>
            {avatar && (
              <button type="button" className="addon-button" disabled={images.busy} onClick={removePhoto}>
                {t('settings.profile.removePhoto')}
              </button>
            )}
          </div>
        </div>
        <input
          ref={fileRef}
          type="file"
          hidden
          accept={ACCEPTED_IMAGE_TYPES.join(',')}
          onChange={(e) => {
            const file = e.target.files?.[0]
            e.target.value = ''
            if (file) void pickPhoto(file)
          }}
        />
      </div>
      {photoErrorKey && (
        <p className="settings-hint error" role="alert">
          {t(photoErrorKey)}
        </p>
      )}

      <SettingsSection title={t('settings.profile.account')}>
        {state.status === 'loading' && <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>}
        {state.status === 'error' && <SettingsNotice tone="error">{t('settings.profile.loadFailed')}</SettingsNotice>}
        {notice && <SettingsNotice tone={notice.tone}>{t(notice.key)}</SettingsNotice>}
        {profile ? (
          <AccountForm
            key={`${profile.email ?? ''}\n${(profile.tags ?? []).join(',')}`}
            profile={profile}
            admin={admin}
            saving={saving}
            onSave={(change) => void save(change)}
          />
        ) : (
          <SettingsCard>
            <SettingsRow label={t('settings.profile.username')}>
              <span className="selectable">{user.username}</span>
            </SettingsRow>
            <SettingsRow label={t('settings.profile.role')}>
              <span>{admin ? t('profile.roleAdmin') : t('profile.roleUser')}</span>
            </SettingsRow>
          </SettingsCard>
        )}
      </SettingsSection>

      <SettingsSection title={t('settings.profile.omv')}>
        {admin && workbench && profile && !profile.systemAccount && (
          <div className="settings-actions">
            <button
              type="button"
              className="addon-button"
              onClick={() => openWorkbenchRoute(userEditRoute(user.username))}
            >
              <IconImage name="external-link-symbolic" size={14} className="profile-omv-icon" />
              {t('settings.profile.editInOmv')}
            </button>
          </div>
        )}
        <p className="settings-hint">
          {!admin
            ? t('settings.profile.omvHintUser')
            : profile?.systemAccount
              ? t('settings.profile.omvHintSystem')
              : workbench
                ? t('settings.profile.omvHintAdmin')
                : t('settings.profile.omvHintNoWorkbench')}
        </p>
      </SettingsSection>
    </>
  )
}

// AccountForm은 계정 정보 카드다. 바꿀 수 있으면 전자 우편 입력과 태그 입력, 저장 버튼을 두고, 아니면 값과 이유를
// 보여 준다. 저장한 값이 바뀌면 부모가 key를 바꿔 입력을 새 값으로 다시 시작한다.
function AccountForm({
  profile,
  admin,
  saving,
  onSave,
}: {
  profile: ProfileResponse
  admin: boolean
  saving: boolean
  onSave: (change: ProfileRequest) => void
}) {
  const { t } = useTranslation()
  const none = t('settings.profile.none')
  const savedEmail = profile.email ?? ''
  const savedTags = profile.tags ?? []
  const [email, setEmail] = useState(savedEmail)
  const [tags, setTags] = useState<string[]>(savedTags)
  // 태그 입력 상자에서 아직 칩이 되지 않은 글자
  const [pending, setPending] = useState('')
  const [errorKey, setErrorKey] = useState<ParseKeys | null>(null)
  const editable = !profile.readOnlyReason

  const fixedRows = (
    <>
      <SettingsRow label={t('settings.profile.username')}>
        <span className="selectable">{profile.username}</span>
      </SettingsRow>
      <SettingsRow label={t('settings.profile.role')}>
        <span>{admin ? t('profile.roleAdmin') : t('profile.roleUser')}</span>
      </SettingsRow>
      {profile.groups && (
        <SettingsRow label={t('settings.profile.groups')}>
          <span className="selectable">{profile.groups.length > 0 ? profile.groups.join(', ') : none}</span>
        </SettingsRow>
      )}
    </>
  )

  if (!editable) {
    return (
      <>
        <SettingsCard>
          {fixedRows}
          <SettingsRow label={t('settings.profile.email')}>
            <span className="selectable">{savedEmail || none}</span>
          </SettingsRow>
          <SettingsRow label={t('settings.profile.tags')}>
            {savedTags.length > 0 ? (
              <ul className="tag-view">
                {savedTags.map((tag) => (
                  <li key={tag} className="tag-chip">
                    <span className="selectable">{tag}</span>
                  </li>
                ))}
              </ul>
            ) : (
              <span>{none}</span>
            )}
          </SettingsRow>
        </SettingsCard>
        <p className="settings-hint">{t(`settings.profile.readOnly.${profile.readOnlyReason!}`)}</p>
      </>
    )
  }

  const allTags = pending.trim() ? addTags(tags, pending) : tags
  const nextEmail = email.trim()
  const changed = nextEmail !== savedEmail || !sameTags(allTags, savedTags)

  const submit = () => {
    if (!validEmail(nextEmail)) {
      setErrorKey('settings.profile.emailInvalid')
      return
    }
    if (!validTags(allTags)) {
      setErrorKey('settings.profile.tagsInvalid')
      return
    }
    setErrorKey(null)
    const change: ProfileRequest = {}
    if (nextEmail !== savedEmail) change.email = nextEmail
    if (!sameTags(allTags, savedTags)) change.tags = allTags
    onSave(change)
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        if (changed) submit()
      }}
    >
      <SettingsCard>
        {fixedRows}
        <SettingsRow label={t('settings.profile.email')}>
          <input
            className="settings-input"
            type="email"
            value={email}
            placeholder={t('settings.profile.emailPlaceholder')}
            aria-label={t('settings.profile.email')}
            disabled={saving}
            onChange={(e) => setEmail(e.target.value)}
          />
        </SettingsRow>
        <SettingsRow label={t('settings.profile.tags')} description={t('settings.profile.tagsDesc')}>
          <TagInput
            label={t('settings.profile.tags')}
            tags={tags}
            pending={pending}
            disabled={saving}
            onChange={(next, text) => {
              setTags(next)
              setPending(text)
            }}
          />
        </SettingsRow>
      </SettingsCard>
      {errorKey && (
        <p className="settings-hint error" role="alert">
          {t(errorKey)}
        </p>
      )}
      <div className="settings-actions">
        <button type="submit" className="addon-button primary" disabled={saving || !changed}>
          {saving ? t('settings.profile.saving') : t('settings.server.save')}
        </button>
        <span className="settings-hint profile-save-hint">
          {admin ? t('settings.profile.saveDescAdmin') : t('settings.profile.saveDescUser')}
        </span>
      </div>
    </form>
  )
}
