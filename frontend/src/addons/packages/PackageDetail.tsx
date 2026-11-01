import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { PackageChangelog, PackageDetail as Detail, PackageUpdate } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { IconImage } from '../../icons/IconImage'
import { Modal } from '../../shell/Modal'
import { openWorkbenchRoute } from '../../shell/launchApp'
import { formatBytes } from '../common/format'
import { type ChangelogProblem, changelogProblem, parseDescription } from './detail'
import { changelogRoute, formatElapsed } from './packages'
import { type Load, usePackageChangelog, usePackageDescription } from './usePackages'

// 경과 시간을 다시 그리는 간격과, 기다린 시간을 보이기 시작하는 때.
const TICK_MS = 1_000
const SHOW_WAITED_AFTER_S = 5

interface Props {
  pkg: PackageUpdate
  open: boolean
  // 업데이트 작업(확인·설치)이 진행 중이거나 시작하는 중이다. 그동안 변경 기록을 묻지 않는다.
  busy: boolean
  workbench: boolean
  onClose: () => void
  onClosed: () => void
  // 패키지가 업데이트 목록에서 사라졌다. 창이 목록을 다시 받는다.
  onGone: () => void
}

// PackageDetail은 업데이트할 패키지 하나의 설명(영어 원문)과 변경 기록을 보이는 창 범위 모달이다. 크기를 고정해
// 변경 기록이 도착해도 상자가 흔들리지 않고, 본문 하나만 스크롤한다. 열면 닫기에 포커스한다.
export function PackageDetail({ pkg, open, busy, workbench, onClose, onClosed, onGone }: Props) {
  const { t } = useTranslation()
  const closeRef = useRef<HTMLButtonElement>(null)
  const desc = usePackageDescription(pkg.name, open)
  const changelog = usePackageChangelog(pkg.name, open, busy)
  const descGone = desc.state.status === 'ready' && desc.state.data === null
  const problem = changelog.state.status === 'error' ? changelogProblem(changelog.state.err) : null
  const gone = descGone || problem?.kind === 'gone'
  useEffect(() => {
    if (gone) onGone()
  }, [gone, onGone])

  return (
    <Modal
      open={open}
      scope="window"
      title={
        <span className="selectable" translate="no">
          {pkg.name}
        </span>
      }
      initialFocus={closeRef}
      onCancel={onClose}
      onClosed={onClosed}
      className="pkg-detail"
      size={{ width: 656, height: 480 }}
      footer={
        <>
          {workbench && pkg.filename && (
            <button type="button" className="pkg-link" onClick={() => openWorkbenchRoute(changelogRoute(pkg.filename ?? ''))}>
              {t('packages.detail.workbench')}
              <IconImage name="external-link-symbolic" size={14} />
            </button>
          )}
          <button type="button" className="addon-button" ref={closeRef} onClick={onClose}>
            {t('dialog.close')}
          </button>
        </>
      }
    >
      <div className="pkg-detail-body">
        <DetailMeta pkg={pkg} />
        {descGone ? (
          <GoneNotice />
        ) : (
          <>
            <DescriptionSection pkg={pkg} state={desc.state} onRetry={desc.retry} />
            <ChangelogSection pkg={pkg} state={changelog.state} busy={busy} onRetry={changelog.retry} />
          </>
        )}
      </div>
    </Modal>
  )
}

// DetailMeta는 버전 줄(지금 → 새 버전, 새로 설치)과 저장소·내려받을 크기다.
function DetailMeta({ pkg }: { pkg: PackageUpdate }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const version = (
    <span className="pkg-version" translate="no">
      {t('packages.detail.newVersion', { version: pkg.version })}
    </span>
  )
  return (
    <div className="pkg-detail-meta">
      <div className="pkg-detail-versions selectable">
        {pkg.oldVersion ? (
          <>
            <span className="pkg-old" translate="no">
              {t('packages.current', { version: pkg.oldVersion })}
            </span>
            <span className="pkg-detail-arrow" aria-hidden="true">
              →
            </span>
            {version}
          </>
        ) : (
          <>
            <span className="pkg-old new">{t('packages.newInstall')}</span>
            <span className="pkg-detail-arrow" aria-hidden="true">
              ·
            </span>
            {version}
          </>
        )}
      </div>
      <div className="pkg-detail-sub">
        {pkg.repository && (
          <span className="badge" translate="no">
            {pkg.repository}
          </span>
        )}
        <span className="pkg-muted pkg-nums">{t('packages.detail.size', { size: formatBytes(pkg.size, lang) })}</span>
      </div>
    </div>
  )
}

// DescriptionSection은 설명이다. 요약은 행 데이터라 바로 그리고, 긴 설명은 받는 대로 그 아래에 붙인다.
function DescriptionSection({ pkg, state, onRetry }: { pkg: PackageUpdate; state: Load<Detail | null>; onRetry: () => void }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const headingId = useId()
  const detail = state.status === 'ready' ? state.data : null
  const summary = pkg.summary || detail?.summary
  const blocks = detail?.description ? parseDescription(detail.description) : []
  return (
    <section className="pkg-detail-section" aria-labelledby={headingId}>
      <div className="pkg-detail-head">
        <h3 className="pkg-detail-heading" id={headingId}>
          {t('packages.detail.description')}
        </h3>
        {/* 다른 문구는 모두 번역되므로, 영어 문단이 번역이 빠진 것처럼 보이지 않게 알린다. */}
        {lang !== 'en' && <span className="badge">{t('packages.detail.original')}</span>}
      </div>
      <div className="pkg-detail-desc selectable" lang="en" tabIndex={0}>
        {summary && <p className="pkg-detail-summary">{summary}</p>}
        {blocks.map((b, i) =>
          b.kind === 'p' ? (
            <p key={i}>{b.text}</p>
          ) : b.kind === 'ul' ? (
            <ul key={i}>
              {b.items.map((item, j) => (
                <li key={j}>{item}</li>
              ))}
            </ul>
          ) : (
            <pre key={i}>{b.text}</pre>
          ),
        )}
      </div>
      {state.status === 'ready' && !detail?.description && <p className="pkg-detail-note">{t('packages.detail.noLong')}</p>}
      {detail?.descriptionFrom === 'installed' && (
        <p className="pkg-detail-note">
          <IconImage name="dialog-information-symbolic" size={14} />
          <span>{t('packages.detail.installedDesc')}</span>
        </p>
      )}
      {state.status === 'error' && (
        <div className="pkg-detail-note error" role="alert">
          <IconImage name="dialog-error-symbolic" size={14} />
          <span>{t('packages.detail.descFailed')}</span>
          <button type="button" className="addon-button pkg-small" onClick={onRetry}>
            {t('packages.detail.retry')}
          </button>
        </div>
      )}
    </section>
  )
}

interface ChangelogProps {
  pkg: PackageUpdate
  state: Load<PackageChangelog>
  busy: boolean
  onRetry: () => void
}

// ChangelogSection은 변경 기록이다. 설치된 버전보다 새 항목을 먼저 보이고, 설치된 버전과 그 이전 항목은 펼쳐 본다.
function ChangelogSection({ pkg, state, busy, onRetry }: ChangelogProps) {
  const { t } = useTranslation()
  const headingId = useId()
  const data = state.status === 'ready' ? state.data : null
  // 작업이 진행 중이면 묻지 않으므로, 받아 둔 것이 없으면 작업 안내를 보인다.
  const problem: ChangelogProblem | null =
    busy && !data ? { kind: 'busy' } : state.status === 'error' ? changelogProblem(state.err) : null
  const loading = !data && !problem
  return (
    <section className="pkg-detail-section" aria-labelledby={headingId} aria-busy={loading}>
      <div className="pkg-detail-head">
        <h3 className="pkg-detail-heading" id={headingId}>
          {t('packages.detail.changelog')}
        </h3>
        {data?.matched && <span className="pkg-detail-since">{t('packages.detail.since')}</span>}
      </div>
      {loading && <ChangelogLoading size={pkg.size} />}
      {problem && <ChangelogTrouble problem={problem} busy={busy} onRetry={onRetry} />}
      {data && <ChangelogText pkg={pkg} data={data} />}
    </section>
  )
}

// ChangelogLoading은 받는 중이다. OMV가 패키지 파일을 먼저 내려받아야 하면 오래 걸리므로 기다린 시간을 보인다.
// 받기를 시작할 때 그려지므로 그린 시각을 시작 시각으로 쓴다.
function ChangelogLoading({ size }: { size: number }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const [since] = useState(() => Date.now())
  const [now, setNow] = useState(since)
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), TICK_MS)
    return () => window.clearInterval(timer)
  }, [])
  const waited = Math.max(0, Math.round((now - since) / 1000))
  return (
    <div className="pkg-waiting pkg-detail-placeholder">
      <div role="status">
        <p className="pkg-detail-placeholder-title">
          <span className="pkg-spinner" aria-hidden="true" />
          {t('packages.detail.loading')}
        </p>
        <p>{t('packages.detail.loadingHint', { size: formatBytes(size, lang) })}</p>
      </div>
      {waited >= SHOW_WAITED_AFTER_S && (
        <p className="pkg-nums">{t('packages.detail.waited', { elapsed: formatElapsed(waited, lang) })}</p>
      )}
    </div>
  )
}

function ChangelogTrouble({ problem, busy, onRetry }: { problem: ChangelogProblem; busy: boolean; onRetry: () => void }) {
  const { t } = useTranslation()
  switch (problem.kind) {
    case 'pending':
      return null
    case 'none':
      return (
        <div className="pkg-waiting pkg-detail-placeholder">
          <p>{t('packages.detail.none')}</p>
        </div>
      )
    case 'gone':
      return <GoneNotice />
    case 'busy':
      return (
        <div className="pkg-notice warn" role="status">
          <IconImage name="dialog-warning-symbolic" size={16} />
          <div className="pkg-notice-text">
            <strong>{t('packages.detail.busy.title')}</strong>
            <span>{t('packages.detail.busy.body')}</span>
          </div>
          {/* 창의 작업 목록이 늦게 갱신될 수 있어 다시 시도도 둔다. 작업이 진행 중인 줄 알면 막는다. */}
          <button type="button" className="addon-button pkg-small" disabled={busy} onClick={onRetry}>
            {t('packages.detail.retry')}
          </button>
        </div>
      )
    case 'failed':
      return (
        <div className="pkg-notice error" role="alert">
          <IconImage name="dialog-error-symbolic" size={16} />
          <div className="pkg-notice-text">
            <strong>{t('packages.detail.failed.title')}</strong>
            <span>{t(problem.body)}</span>
          </div>
          <button type="button" className="addon-button pkg-small" onClick={onRetry}>
            {t('packages.detail.retry')}
          </button>
        </div>
      )
  }
}

// ChangelogText는 받은 변경 기록이다. 나누지 못했으면(새로 설치, 설치된 버전의 항목이 없음) 전체를 보인다.
function ChangelogText({ pkg, data }: { pkg: PackageUpdate; data: PackageChangelog }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const earlierId = useId()
  const truncatedNote = data.truncated && (
    <p className="pkg-detail-note">
      <IconImage name="dialog-information-symbolic" size={14} />
      <span>{t('packages.detail.truncated')}</span>
    </p>
  )
  return (
    <>
      {!data.matched && pkg.oldVersion && (
        <p className="pkg-detail-note">
          <IconImage name="dialog-information-symbolic" size={14} />
          <span>{t('packages.detail.unmatched')}</span>
        </p>
      )}
      {data.recent ? (
        <pre className="pkg-output selectable" lang="en" translate="no" tabIndex={0}>
          {data.recent}
        </pre>
      ) : (
        <p className="pkg-detail-note">{t('packages.detail.noRecent')}</p>
      )}
      {data.earlier ? (
        <>
          <button
            type="button"
            className="pkg-detail-toggle"
            aria-expanded={expanded}
            aria-controls={earlierId}
            onClick={() => setExpanded((v) => !v)}
          >
            <IconImage name="pan-end-symbolic" size={16} />
            {t('packages.detail.earlier')}
          </button>
          <div className="pkg-detail-earlier" id={earlierId} hidden={!expanded}>
            <pre className="pkg-output selectable" lang="en" translate="no" tabIndex={0}>
              {data.earlier}
            </pre>
            {truncatedNote}
          </div>
        </>
      ) : (
        truncatedNote
      )}
    </>
  )
}

function GoneNotice() {
  const { t } = useTranslation()
  return (
    <div className="pkg-notice warn" role="status">
      <IconImage name="dialog-warning-symbolic" size={16} />
      <div className="pkg-notice-text">
        <strong>{t('packages.detail.gone.title')}</strong>
        <span>{t('packages.detail.gone.body')}</span>
      </div>
    </div>
  )
}
