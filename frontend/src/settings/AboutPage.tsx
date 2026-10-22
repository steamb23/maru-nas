// 설정의 정보 페이지: 묶음 로고와 태그라인, 버전·빌드 정보, 라이선스, 제3자 고지.
// 대기 이스터에그(ClawdParade)도 여기서 그린다.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { WindowViewProps } from '../addons/types'
import { fetchAbout } from '../api/client'
import type { AboutResponse } from '../api/types.gen'
import lockupUrl from '../assets/logo/maru-lockup.svg?url'
import { useAuthStore } from '../auth/store'
import { BRAND_COPYRIGHT, BRAND_NAME, BRAND_ROMANIZED, BRAND_TAGLINE } from '../brand'
import { useLanguage } from '../i18n'
import { formatOMVVersion, formatRevisionTime, shortRevision } from './about/about'
import { NOTICES } from './about/notices'
import { ClawdParade } from './ClawdParade'
import { SettingsCard, SettingsRow, SettingsSection } from './controls'

type AboutState = { status: 'loading' } | { status: 'ready'; about: AboutResponse } | { status: 'error' }

export function AboutPage({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  // OMV 버전은 백엔드가 관리자에게만 준다(OMV System.getInformation과 같은 기준).
  const admin = useAuthStore((s) => s.user)?.admin === true
  const [state, setState] = useState<AboutState>({ status: 'loading' })

  useEffect(() => {
    const ctl = new AbortController()
    fetchAbout(ctl.signal).then(
      (about) => setState({ status: 'ready', about }),
      () => {
        if (!ctl.signal.aborted) setState({ status: 'error' })
      },
    )
    return () => ctl.abort()
  }, [])

  const about = state.status === 'ready' ? state.about : null
  const notRecorded = t('settings.about.notRecorded')
  const revisionTime = about ? formatRevisionTime(about, lang) : null
  const omvVersion = about ? formatOMVVersion(about) : null

  return (
    <>
      <div className="about-brand">
        <img className="about-lockup" src={lockupUrl} alt={`${BRAND_NAME} (${BRAND_ROMANIZED})`} draggable={false} />
        <p className="about-tagline">{BRAND_TAGLINE}</p>
      </div>

      <SettingsSection title={t('settings.about.build')}>
        {state.status === 'error' ? (
          <p className="about-error">{t('settings.about.loadFailed')}</p>
        ) : (
          <SettingsCard>
            <SettingsRow label={t('settings.about.version')}>
              <span className="about-value selectable">{about?.version ?? '…'}</span>
            </SettingsRow>
            <SettingsRow label={t('settings.about.commit')}>
              <span className="about-value selectable" title={about?.revision}>
                {about ? (about.revision ? shortRevision(about.revision) : notRecorded) : '…'}
              </span>
              {about?.modified && <span className="settings-badge">{t('settings.about.modified')}</span>}
            </SettingsRow>
            <SettingsRow label={t('settings.about.commitTime')}>
              <span className="about-value">{about ? (revisionTime ?? notRecorded) : '…'}</span>
            </SettingsRow>
            <SettingsRow label={t('settings.about.goVersion')}>
              <span className="about-value selectable">{about?.goVersion ?? '…'}</span>
            </SettingsRow>
            {admin && (
              <SettingsRow label={t('settings.about.omvVersion')}>
                <span className="about-value selectable">
                  {about ? (omvVersion ?? t('settings.about.unknown')) : '…'}
                </span>
              </SettingsRow>
            )}
          </SettingsCard>
        )}
      </SettingsSection>

      <SettingsSection title={t('settings.about.license')}>
        <p className="about-copyright selectable">{BRAND_COPYRIGHT}</p>
        <p className="about-text">{t('settings.about.licenseText')}</p>
      </SettingsSection>

      <SettingsSection title={t('settings.about.notices')} description={t('settings.about.noticesIntro')}>
        <SettingsCard>
          {NOTICES.map((n) => (
            <SettingsRow
              key={n.name}
              label={
                <a className="about-link" href={n.url} target="_blank" rel="noopener noreferrer">
                  {n.name}
                </a>
              }
              description={
                <>
                  <span className="selectable">© {n.copyright}</span>
                  {n.noteKey && <span className="about-note">{t(n.noteKey)}</span>}
                </>
              }
            >
              <span className="settings-badge">{n.license}</span>
            </SettingsRow>
          ))}
        </SettingsCard>
      </SettingsSection>

      <ClawdParade active={active} />
    </>
  )
}
