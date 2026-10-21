// 파일 복사 진행 창(앱 copy). 탐색기에서 다른 디스크로 끌어 놓은 복사를 작업마다 진행 막대와 함께 보인다. 상태는
// copyStore.ts에 있고, 이 창은 그리기와 취소만 한다. 창을 닫으면 남은 복사를 취소한다(copyStore가 창 저장소를 본다).
import { useTranslation } from 'react-i18next'
import { formatBytes } from '../addons/common/format'
import type { WindowViewProps } from '../addons/types'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import { type CopyJob, durationOf, progressOf, remainingSeconds, settled } from './copyJobs'
import { type CopyEntry, cancelCopy, clearFinished, useCopyStore } from './copyStore'
import { FailuresBody } from './DeleteBody'
import { groupFailures, opErrorKey } from './ops'
import { folderLabel } from './paths'

const STATUS_KEYS = {
  queued: 'files.copy.status.queued',
  running: 'files.copy.status.running',
  done: 'files.copy.status.done',
  failed: 'files.copy.status.failed',
  cancelled: 'files.copy.status.cancelled',
} as const

export function CopyWindow(_props: WindowViewProps) {
  const { t } = useTranslation()
  const entries = useCopyStore((s) => s.entries)
  const active = entries.some((e) => !settled(e.job))
  const finished = entries.some((e) => settled(e.job))
  return (
    <div className="copy-window">
      {entries.length === 0 ? (
        <p className="copy-empty">{t('files.copy.empty')}</p>
      ) : (
        <ul className="copy-jobs">
          {entries.map((e) => (
            <CopyRow key={e.job.id} entry={e} />
          ))}
        </ul>
      )}
      <footer className="copy-footer">
        <span>{active ? t('files.copy.closeHint') : ''}</span>
        <button type="button" className="addon-button" disabled={!finished} onClick={clearFinished}>
          {t('files.copy.clear')}
        </button>
      </footer>
    </div>
  )
}

function CopyRow({ entry }: { entry: CopyEntry }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { job, roots } = entry
  const fmt = (n: number) => new Intl.NumberFormat(lang).format(n)
  const home = t('files.nav.home')
  const from = folderLabel(roots, job.dir, home)
  const to = folderLabel(roots, job.target, home)
  const title = job.names.length === 1 ? job.names[0] : t('files.drag.items', { count: job.names.length, total: fmt(job.names.length) })
  const fraction = progressOf(job)
  const status = job.status === 'running' && job.scanning ? t('files.copy.status.scanning') : t(STATUS_KEYS[job.status])
  return (
    <li className={`copy-job ${job.status}`}>
      <div className="copy-job-head">
        <IconImage name={job.status === 'failed' ? 'dialog-warning-symbolic' : 'folder-download'} size={job.status === 'failed' ? 20 : 32} />
        <div className="copy-job-title">
          <strong title={job.names.length === 1 ? job.names[0] : undefined}>{title}</strong>
          <span title={`${job.dir} → ${job.target}`}>
            {from} → {to}
          </span>
        </div>
        {!settled(job) && (
          <button type="button" className="addon-button" onClick={() => cancelCopy(job.id)}>
            {t('dialog.cancel')}
          </button>
        )}
      </div>
      {!settled(job) && (
        <div
          className={`meter copy-meter${fraction === null ? ' busy' : ''}`}
          role="progressbar"
          aria-label={title}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={fraction === null ? undefined : Math.round(fraction * 100)}
        >
          <div className="meter-fill" style={fraction === null ? undefined : { width: `${fraction * 100}%` }} />
        </div>
      )}
      <p className="copy-job-status">
        <span>{status}</span>
        <Detail job={job} />
      </p>
      {job.current && job.status === 'running' && (
        <p className="copy-job-current" title={job.current}>
          {job.current}
        </p>
      )}
      {job.error !== null && <p className="copy-job-error">{t(opErrorKey(job.error))}</p>}
      {job.failures.length > 0 && <FailuresBody groups={groupFailures(job.failures, (n) => n)} />}
      {job.skipped > 0 && <p className="copy-job-note">{t('files.copy.skipped', { count: job.skipped, total: fmt(job.skipped) })}</p>}
    </li>
  )
}

// Detail은 복사한 양·파일 수·속도·남은 시간이다.
function Detail({ job }: { job: CopyJob }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  // 끝난 작업은 복사한 것이 있을 때만 그 양을 보인다.
  if (job.status === 'queued' || (settled(job) && job.amount.files === 0)) return null
  const fmt = (n: number) => new Intl.NumberFormat(lang).format(n)
  const a = job.amount
  const parts = [
    t('files.copy.size', { done: formatBytes(a.bytes, lang), total: formatBytes(a.totalBytes, lang) }),
    t('files.copy.files', { count: a.totalFiles, done: fmt(a.files), total: fmt(a.totalFiles) }),
  ]
  if (job.status === 'running' && job.rate.rate !== null) parts.push(t('files.copy.rate', { size: formatBytes(job.rate.rate, lang) }))
  const left = remainingSeconds(job)
  if (left !== null) {
    const d = durationOf(left)
    parts.push(t('files.copy.remaining', { time: new Intl.NumberFormat(lang, { style: 'unit', unit: d.unit, unitDisplay: 'long' }).format(d.value) }))
  }
  return <span className="copy-job-detail">{parts.join(' · ')}</span>
}
