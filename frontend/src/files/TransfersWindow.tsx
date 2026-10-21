// 파일 작업 창(앱 transfers). 탐색기에서 다른 디스크로 끌어 놓은 복사와 올리기를 작업마다 진행 막대와 함께 보인다. 상태는
// transferStore.ts에 있고, 이 창은 그리기와 취소·다시 올리기만 한다. 창을 닫으면 남은 작업을 취소한다(transferStore가
// 창 저장소를 본다).
import { useTranslation } from 'react-i18next'
import { formatBytes } from '../addons/common/format'
import type { WindowViewProps } from '../addons/types'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import type { IconName } from '../icons/resolver'
import { type CopyJob, durationOf, progressOf, remainingSeconds, settled } from './copyJobs'
import { FailuresBody } from './DeleteBody'
import { kindIcon, kindOf } from './kinds'
import { uploadErrorKey } from './messages'
import { groupFailures, opErrorKey } from './ops'
import { type Root, folderLabel } from './paths'
import { ProgressMeter, TaskCurrent } from './TaskProgress'
import { type TransferEntry, cancelTransfer, clearFinished, retryUpload, useTransferStore } from './transferStore'
import { type UploadError, type UploadJob, retryable, uploadAmount, uploadCurrent, uploadProgress, uploadRemaining } from './uploadJobs'

const COPY_STATUS_KEYS = {
  queued: 'files.transfers.status.queued',
  running: 'files.transfers.status.copying',
  done: 'files.transfers.status.copied',
  failed: 'files.transfers.status.copyFailed',
  cancelled: 'files.transfers.status.cancelled',
} as const

const UPLOAD_STATUS_KEYS = {
  queued: 'files.transfers.status.queued',
  running: 'files.transfers.status.uploading',
  done: 'files.transfers.status.uploaded',
  failed: 'files.transfers.status.uploadFailed',
  cancelled: 'files.transfers.status.cancelled',
} as const

export function TransfersWindow(_props: WindowViewProps) {
  const { t } = useTranslation()
  const entries = useTransferStore((s) => s.entries)
  const active = entries.some((e) => !settled(e.job))
  const finished = entries.some((e) => settled(e.job))
  return (
    <div className="transfer-window">
      {entries.length === 0 ? (
        <p className="transfer-empty">{t('files.transfers.empty')}</p>
      ) : (
        <ul className="transfer-jobs">
          {entries.map((e) => (
            <TransferRow key={e.job.id} entry={e} />
          ))}
        </ul>
      )}
      <footer className="transfer-footer">
        <span>{active ? t('files.transfers.closeHint') : ''}</span>
        <button type="button" className="addon-button" disabled={!finished} onClick={clearFinished}>
          {t('files.transfers.clear')}
        </button>
      </footer>
    </div>
  )
}

function TransferRow({ entry }: { entry: TransferEntry }) {
  return entry.job.kind === 'copy' ? <CopyRow job={entry.job} roots={entry.roots} /> : <UploadRow job={entry.job} roots={entry.roots} />
}

// Head는 작업의 아이콘, 이름, 위치와 취소 단추다.
function Head({ job, title, where, whereTitle, icon }: { job: CopyJob | UploadJob; title: string; where: string; whereTitle: string; icon: IconName }) {
  const { t } = useTranslation()
  const failed = job.status === 'failed'
  return (
    <div className="transfer-job-head">
      <IconImage name={failed ? 'dialog-warning-symbolic' : icon} size={failed ? 20 : 32} />
      <div className="transfer-job-title">
        <strong title={title}>{title}</strong>
        <span title={whereTitle}>{where}</span>
      </div>
      {!settled(job) && (
        <button type="button" className="addon-button" onClick={() => cancelTransfer(job.id)}>
          {t('dialog.cancel')}
        </button>
      )}
    </div>
  )
}

function CopyRow({ job, roots }: { job: CopyJob; roots: readonly Root[] }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const fmt = (n: number) => new Intl.NumberFormat(lang).format(n)
  const home = t('files.nav.home')
  const from = folderLabel(roots, job.dir, home)
  const to = folderLabel(roots, job.target, home)
  const title = job.names.length === 1 ? job.names[0] : t('files.drag.items', { count: job.names.length, total: fmt(job.names.length) })
  const status = job.status === 'running' && job.scanning ? t('files.transfers.status.scanning') : t(COPY_STATUS_KEYS[job.status])
  return (
    <li className={`transfer-job ${job.status}`}>
      <Head job={job} title={title} where={`${from} → ${to}`} whereTitle={`${job.dir} → ${job.target}`} icon="folder-download" />
      {!settled(job) && <ProgressMeter fraction={progressOf(job)} label={title} />}
      <p className="transfer-job-status">
        <span>{status}</span>
        <CopyDetail job={job} />
      </p>
      {job.current && job.status === 'running' && <TaskCurrent name={job.current} />}
      {job.error !== null && <p className="transfer-job-error">{t(opErrorKey(job.error))}</p>}
      {job.failures.length > 0 && <FailuresBody groups={groupFailures(job.failures, (n) => n)} />}
      {job.skipped > 0 && <p className="transfer-job-note">{t('files.transfers.skipped', { count: job.skipped, total: fmt(job.skipped) })}</p>}
    </li>
  )
}

function UploadRow({ job, roots }: { job: UploadJob; roots: readonly Root[] }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const fmt = (n: number) => new Intl.NumberFormat(lang).format(n)
  const to = folderLabel(roots, job.target, t('files.nav.home'))
  const n = job.files.length
  const title = n === 1 ? job.files[0].name : t('files.drag.items', { count: n, total: fmt(n) })
  const current = job.status === 'running' ? uploadCurrent(job) : null
  const failures = job.files.filter((f) => f.status === 'failed').map((f) => ({ item: f.name, err: f.error }))
  const again = retryable(job, false)
  const replace = retryable(job, true)
  return (
    <li className={`transfer-job ${job.status}`}>
      <Head job={job} title={title} where={t('files.transfers.uploadTo', { folder: to })} whereTitle={job.target} icon={kindIcon(n === 1 ? kindOf({ name: job.files[0].name, type: 'file' }) : 'file')} />
      {!settled(job) && <ProgressMeter fraction={job.status === 'queued' ? 0 : uploadProgress(job)} label={title} />}
      <p className="transfer-job-status">
        <span>{t(UPLOAD_STATUS_KEYS[job.status])}</span>
        <UploadDetail job={job} />
      </p>
      {current && n > 1 && <TaskCurrent name={current} />}
      {job.error !== null && <p className="transfer-job-error">{t(opErrorKey(job.error))}</p>}
      {failures.length > 0 && <FailuresBody groups={groupFailures(failures, (name) => name, 5, (err) => uploadErrorKey(err as UploadError))} />}
      {settled(job) && (again > 0 || replace > 0) && (
        <div className="transfer-job-actions">
          {replace > 0 && (
            <button type="button" className="addon-button" onClick={() => retryUpload(job.id, true)}>
              {t('files.transfers.overwrite', { count: replace, total: fmt(replace) })}
            </button>
          )}
          {again > 0 && (
            <button type="button" className="addon-button" onClick={() => retryUpload(job.id, false)}>
              {t('files.transfers.retry', { count: again, total: fmt(again) })}
            </button>
          )}
        </div>
      )}
    </li>
  )
}

// useRemaining은 남은 시간(초)을 문구로 바꾸는 함수다.
function useRemaining() {
  const { t } = useTranslation()
  const lang = useLanguage()
  return (seconds: number) => {
    const d = durationOf(seconds)
    return t('files.transfers.remaining', { time: new Intl.NumberFormat(lang, { style: 'unit', unit: d.unit, unitDisplay: 'long' }).format(d.value) })
  }
}

// CopyDetail은 복사한 양·파일 수·속도·남은 시간이다.
function CopyDetail({ job }: { job: CopyJob }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const remaining = useRemaining()
  // 끝난 작업은 복사한 것이 있을 때만 그 양을 보인다.
  if (job.status === 'queued' || (settled(job) && job.amount.files === 0)) return null
  const fmt = (n: number) => new Intl.NumberFormat(lang).format(n)
  const a = job.amount
  const parts = [
    t('files.transfers.size', { done: formatBytes(a.bytes, lang), total: formatBytes(a.totalBytes, lang) }),
    t('files.transfers.files', { count: a.totalFiles, done: fmt(a.files), total: fmt(a.totalFiles) }),
  ]
  if (job.status === 'running' && job.rate.rate !== null) parts.push(t('files.transfers.rate', { size: formatBytes(job.rate.rate, lang) }))
  const left = remainingSeconds(job)
  if (left !== null) parts.push(remaining(left))
  return <span className="transfer-job-detail">{parts.join(' · ')}</span>
}

// UploadDetail은 올린 양·파일 수·속도·남은 시간이다.
function UploadDetail({ job }: { job: UploadJob }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const remaining = useRemaining()
  const a = uploadAmount(job)
  if (job.status === 'queued' || (settled(job) && a.files === 0)) return null
  const fmt = (n: number) => new Intl.NumberFormat(lang).format(n)
  const parts = [t('files.transfers.size', { done: formatBytes(a.bytes, lang), total: formatBytes(a.totalBytes, lang) })]
  if (a.totalFiles > 1) parts.push(t('files.transfers.files', { count: a.totalFiles, done: fmt(a.files), total: fmt(a.totalFiles) }))
  if (job.status === 'running' && job.rate.rate !== null) parts.push(t('files.transfers.rate', { size: formatBytes(job.rate.rate, lang) }))
  const left = uploadRemaining(job)
  if (left !== null) parts.push(remaining(left))
  return <span className="transfer-job-detail">{parts.join(' · ')}</span>
}
