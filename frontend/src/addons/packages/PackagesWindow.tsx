import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Job, PackageUpdate, PackagesResponse } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { IconImage } from '../../icons/IconImage'
import { refreshNotifications } from '../../notifications/useNotifications'
import { openPowerDialog } from '../../power/powerDialog'
import { Modal } from '../../shell/Modal'
import { openWorkbenchRoute } from '../../shell/launchApp'
import { useCapabilitiesStore } from '../capabilities'
import { type ActionErrorCode, isBusy } from '../common/actionState'
import { formatBytes, formatTime } from '../common/format'
import { useAddonActions } from '../common/useAddonActions'
import type { WindowViewProps } from '../types'
import { usePackagesNav } from './nav'
import {
  type OutputBuffer,
  UPDATE_SETTINGS_ROUTE,
  changelogRoute,
  elapsedSeconds,
  formatElapsed,
  jobHeading,
  jobReason,
  pickJob,
  startError,
  totalSize,
} from './packages'
import { useJobOutput, useJobs, useKeepAlive, usePackages } from './usePackages'

type Tab = 'list' | 'job'

// 진행 중 경과 시간을 다시 그리는 간격.
const TICK_MS = 1_000

// useNow는 on인 동안 1초마다 지금 시각을 다시 준다.
function useNow(on: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!on) return
    const timer = window.setInterval(() => setNow(Date.now()), TICK_MS)
    return () => window.clearInterval(timer)
  }, [on])
  return now
}

// PackagesWindow는 시스템 업데이트 앱이다(시안 C: 업데이트 목록과 작업 보기를 탭으로 바꾼다). 확인·설치는 서버
// 백그라운드 작업이라 창을 닫아도 이어지고, 작업 보기는 그 출력을 이어 받는다.
export function PackagesWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const { data, status, refresh } = usePackages(active)
  const jobsData = useJobs(active)
  const { jobs } = jobsData
  const current = jobs?.current ?? null
  const workbench = useCapabilitiesStore((s) => Boolean(s.caps?.workbenchUrl))

  // 알림의 출력 보기·백그라운드 작업의 열기는 작업 보기로 연다. 그 밖에는 진행 중인 작업이 있으면 작업 보기다.
  const nav = usePackagesNav()
  const [tab, setTab] = useState<Tab | null>(nav.seq > 0 ? 'job' : null)
  const [navSeq, setNavSeq] = useState(nav.seq)
  if (nav.seq !== navSeq) {
    setNavSeq(nav.seq)
    setTab('job')
  }
  const shownTab: Tab = tab ?? (current ? 'job' : 'list')
  const job = pickJob(jobs, nav.jobId)

  // 진행 중이던 작업이 끝나면 업데이트 목록을 다시 받는다(apt 후크가 목록을 이미 새로 만들었다). 진행 중이라 작업
  // 보기를 보이고 있었다면 결과를 보이도록 작업 보기에 머문다(사용자가 고른 탭은 그대로).
  const [lastCurrent, setLastCurrent] = useState<string | null>(null)
  if ((current?.id ?? null) !== lastCurrent) {
    if (lastCurrent && !current) {
      refresh()
      if (tab === null) setTab('job')
    }
    setLastCurrent(current?.id ?? null)
  }

  // 시작(성공·실패)하면 작업과 목록을 다시 받고, 알림도 바로 받아 알림 센터의 백그라운드 작업이 곧 보이게 한다.
  const ctl = useAddonActions('packages', () => {
    usePackagesNav.getState().forget()
    jobsData.refresh()
    refresh()
    refreshNotifications()
  })
  const { settle } = ctl
  useEffect(() => settle(), [jobs, settle])
  const startState = ctl.state.targets['']
  const starting = isBusy(ctl.state, '')
  // 시작하면 작업 보기로 간다.
  const [startedAt, setStartedAt] = useState<number | null>(null)
  if (startState?.kind === 'settling' && startState.since !== startedAt) {
    setStartedAt(startState.since)
    setTab('job')
  }
  const busy = current !== null || starting

  const output = useJobOutput(job, active, jobsData.refresh)
  // 창이 보일 때만 세션을 유지한다. 보이지 않으면 작업 목록도 받지 않아 끝난 작업을 진행 중으로 오해하기 때문이다.
  useKeepAlive(active && current !== null)

  const confirmOpen = ctl.state.confirm?.action === 'upgrade'

  return (
    <div className="pkg-window">
      <div className="pkg-tabs-row">
        <div className="pkg-tabs" role="tablist" aria-label={t('packages.tabs.label')}>
          <button
            type="button"
            role="tab"
            className="pkg-tab"
            aria-selected={shownTab === 'list'}
            onClick={() => setTab('list')}
          >
            {t('packages.tabs.list')}
            {data && data.packages.length > 0 && <span className="pkg-count">{data.packages.length}</span>}
          </button>
          <button
            type="button"
            role="tab"
            className="pkg-tab"
            aria-selected={shownTab === 'job'}
            onClick={() => setTab('job')}
          >
            {t('packages.tabs.job')}
            {current && <span className="pkg-spinner" role="img" aria-label={t('packages.job.inProgress')} />}
          </button>
        </div>
        <span className="pkg-spacer" />
        {workbench && (
          <button type="button" className="pkg-link" onClick={() => openWorkbenchRoute(UPDATE_SETTINGS_ROUTE)}>
            {t('packages.settings')}
            <IconImage name="external-link-symbolic" size={14} />
          </button>
        )}
      </div>

      {shownTab === 'list' ? (
        <ListView
          data={data}
          disconnected={status === 'disconnected'}
          current={current}
          busy={busy}
          starting={starting}
          startError={startState?.kind === 'error' ? startState.code : null}
          workbench={workbench}
          onCheck={() => ctl.request('', 'check')}
          onInstall={() => ctl.request('', 'upgrade')}
          onDismissError={() => ctl.dismiss('')}
          onShowJob={() => setTab('job')}
        />
      ) : (
        <JobView
          job={job}
          output={output.output}
          waiting={output.waiting}
          reboot={data?.reboot ?? null}
          onBack={() => setTab('list')}
          onCheck={busy ? null : () => ctl.request('', 'check')}
        />
      )}

      <InstallConfirm
        open={confirmOpen}
        data={data}
        onCancel={ctl.cancel}
        onConfirm={ctl.confirm}
      />
    </div>
  )
}

interface ListProps {
  data: PackagesResponse | null
  disconnected: boolean
  current: Job | null
  busy: boolean
  starting: boolean
  startError: ActionErrorCode | null
  workbench: boolean
  onCheck: () => void
  onInstall: () => void
  onDismissError: () => void
  onShowJob: () => void
}

function ListView(p: ListProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const { data, current } = p
  const now = useNow(current !== null)
  const count = data?.packages.length ?? 0
  const err = p.startError ? startError(p.startError) : null

  return (
    <div className="pkg-panel" role="tabpanel" aria-label={t('packages.tabs.list')}>
      {p.disconnected && (
        <p className="addon-banner" role="status">
          {t('addonWindow.disconnected')}
        </p>
      )}
      {current && (
        <div className="pkg-during" role="status">
          <span className="pkg-spinner" aria-hidden="true" />
          <span className="pkg-during-title">
            {current.mine
              ? t(current.kind === 'apt-update' ? 'packages.during.update' : 'packages.during.upgrade')
              : t('packages.during.other', { owner: current.owner })}
          </span>
          <span className="pkg-muted pkg-nums">
            {current.owner} · {formatElapsed(elapsedSeconds(current, now), lang)}
          </span>
          <span className="pkg-spacer" />
          <button type="button" className="addon-button pkg-small" onClick={p.onShowJob}>
            {t('packages.during.view')}
          </button>
        </div>
      )}
      <div className="pkg-toolbar">
        <button type="button" className="addon-button pkg-icon-button" disabled={p.busy} onClick={p.onCheck}>
          <IconImage name="view-refresh-symbolic" size={16} />
          {t('packages.check')}
        </button>
        <button
          type="button"
          className="addon-button primary pkg-icon-button"
          disabled={p.busy || count === 0}
          onClick={p.onInstall}
        >
          {t('packages.install')}
          {count > 0 && <span className="pkg-count on-accent">{count}</span>}
        </button>
        {data?.indexed && (
          <span className="pkg-meta">
            {count > 0 && t('packages.summary', { count, size: formatBytes(totalSize(data.packages), lang) })}
            {count > 0 && data.indexedAt && ' · '}
            {data.indexedAt && t('packages.lastChecked', { time: formatTime(data.indexedAt, lang) })}
          </span>
        )}
      </div>
      {err && (
        <div className="pkg-notice error" role="alert">
          <IconImage name="dialog-error-symbolic" size={16} />
          <div className="pkg-notice-text">
            <strong>{t(err.title)}</strong>
            <span>{t(err.body)}</span>
          </div>
          {p.startError === 'config_dirty' && p.workbench && (
            <button type="button" className="addon-button pkg-small" onClick={() => openWorkbenchRoute('')}>
              {t('packages.start.dirty.action')}
            </button>
          )}
          <button
            type="button"
            className="pkg-close"
            aria-label={t('actions.dismiss')}
            title={t('actions.dismiss')}
            onClick={p.onDismissError}
          >
            <IconImage name="window-close-symbolic" size={12} />
          </button>
        </div>
      )}
      {data?.reboot.required && !current && <RebootNotice packages={data.reboot.packages} />}
      {data && !data.indexed && (
        <div className="pkg-empty">
          <IconImage name="software-update-available-symbolic" size={32} />
          <p className="pkg-empty-title">{t('packages.noIndex.title')}</p>
          <p className="pkg-empty-body">{t('packages.noIndex.body')}</p>
          <button type="button" className="addon-button primary" disabled={p.busy} onClick={p.onCheck}>
            {t('packages.check')}
          </button>
        </div>
      )}
      {data?.indexed && count === 0 && (
        <div className="pkg-empty">
          <IconImage name="process-completed-symbolic" size={32} className="pkg-ok" />
          <p className="pkg-empty-title">{t('packages.upToDate.title')}</p>
          <p className="pkg-empty-body">{t('packages.upToDate.body')}</p>
        </div>
      )}
      {count > 0 && data && (
        <div className={`pkg-list-wrap${current || p.disconnected ? ' dim' : ''}`}>
          <div className="pkg-row pkg-head" aria-hidden="true">
            <span>{t('packages.columns.package')}</span>
            <span className="pkg-right">{t('packages.columns.version')}</span>
            <span className="pkg-right">{t('packages.columns.size')}</span>
            <span />
          </div>
          <ul className="pkg-list selectable" aria-label={t('packages.listLabel')}>
            {data.packages.map((pkg) => (
              <PackageRow key={pkg.name} pkg={pkg} workbench={p.workbench} />
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

function PackageRow({ pkg, workbench }: { pkg: PackageUpdate; workbench: boolean }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  return (
    <li className="pkg-row">
      <div className="pkg-name-col">
        <span className="pkg-name">{pkg.name}</span>
        <span className="pkg-sub">
          {pkg.repository && <span className="badge">{pkg.repository}</span>}
          {pkg.summary && <span className="pkg-summary">{pkg.summary}</span>}
        </span>
      </div>
      <div className="pkg-version-col">
        <span className="pkg-version" title={pkg.version}>
          {pkg.version}
        </span>
        <span className={pkg.oldVersion ? 'pkg-old' : 'pkg-old new'} title={pkg.oldVersion}>
          {pkg.oldVersion ? t('packages.current', { version: pkg.oldVersion }) : t('packages.newInstall')}
        </span>
      </div>
      <span className="pkg-size pkg-nums">{formatBytes(pkg.size, lang)}</span>
      {workbench && pkg.filename ? (
        <button
          type="button"
          className="row-action-button"
          title={t('packages.changelog')}
          aria-label={t('packages.changelogLabel', { name: pkg.name })}
          onClick={() => openWorkbenchRoute(changelogRoute(pkg.filename ?? ''))}
        >
          <IconImage name="external-link-symbolic" size={14} />
        </button>
      ) : (
        <span />
      )}
    </li>
  )
}

function RebootNotice({ packages }: { packages?: readonly string[] }) {
  const { t } = useTranslation()
  return (
    <div className="pkg-notice warn" role="status">
      <IconImage name="system-reboot-symbolic" size={16} />
      <div className="pkg-notice-text">
        <strong>{t('packages.reboot.title')}</strong>
        {packages && packages.length > 0 && (
          <span className="pkg-ellipsis">
            {t('packages.reboot.packages')} <code>{packages.join(', ')}</code>
          </span>
        )}
      </div>
      <button type="button" className="addon-button pkg-small" onClick={() => openPowerDialog()}>
        {t('packages.reboot.action')}
      </button>
    </div>
  )
}

interface JobProps {
  job: Job | null
  output: OutputBuffer | null
  waiting: boolean
  reboot: PackagesResponse['reboot'] | null
  onBack: () => void
  // 결과를 모를 때 다시 확인하는 단추. 작업이 진행 중이면 null이다.
  onCheck: (() => void) | null
}

function JobView({ job, output, waiting, reboot, onBack, onCheck }: JobProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const running = job?.state === 'running'
  const now = useNow(running)
  if (!job) {
    return (
      <div className="pkg-panel" role="tabpanel" aria-label={t('packages.tabs.job')}>
        <div className="pkg-empty">
          <p className="pkg-empty-body">{t('packages.job.none')}</p>
        </div>
      </div>
    )
  }
  const elapsed = formatElapsed(elapsedSeconds(job, now), lang)
  const sub = job.endedAt
    ? t('packages.job.ended', {
        owner: job.owner,
        start: formatTime(job.startedAt, lang),
        end: formatTime(job.endedAt, lang),
        elapsed,
      })
    : t('packages.job.started', { owner: job.owner, start: formatTime(job.startedAt, lang), elapsed })
  const heading =
    job.state === 'succeeded' && job.count !== undefined && job.count > 0
      ? t(job.kind === 'apt-update' ? 'packages.job.found' : 'packages.job.upgraded', { count: job.count })
      : t(jobHeading(job))
  const reason = jobReason(job)
  const tone = running ? (waiting ? 'idle' : 'run') : job.state === 'succeeded' ? 'ok' : job.state === 'failed' ? 'crit' : 'warn'
  const icon =
    tone === 'ok'
      ? 'process-completed-symbolic'
      : tone === 'crit'
        ? 'dialog-error-symbolic'
        : tone === 'warn'
          ? 'dialog-warning-symbolic'
          : 'software-update-available-symbolic'

  return (
    <div className="pkg-panel pkg-job" role="tabpanel" aria-label={t('packages.tabs.job')}>
      <div className="pkg-job-head" role="status">
        <span className={`pkg-job-icon ${tone}`}>
          <IconImage name={icon} size={20} />
        </span>
        <div className="pkg-job-title">
          <span className="pkg-job-heading">
            {heading}
            {waiting && <span className="badge pkg-waiting-tag">{t('packages.job.waitingTag')}</span>}
          </span>
          <span className="pkg-muted pkg-nums">{sub}</span>
        </div>
        {!running && (
          <button type="button" className="addon-button pkg-icon-button" onClick={onBack}>
            <IconImage name="go-previous-symbolic" size={16} />
            {t('packages.job.back')}
          </button>
        )}
      </div>
      {running && !waiting && (
        <div className="meter pkg-busy" role="progressbar" aria-label={heading}>
          <div className="meter-fill" />
        </div>
      )}
      {running && waiting && <p className="pkg-waiting">{t('packages.job.waiting')}</p>}
      {running && !job.mine && <p className="pkg-hint">{t('packages.job.other')}</p>}
      {!running && reason && (
        <div className={`pkg-notice ${job.state === 'failed' ? 'error' : 'warn'}`} role="status">
          <IconImage name={job.state === 'failed' ? 'dialog-error-symbolic' : 'dialog-warning-symbolic'} size={16} />
          <span className="pkg-notice-text">{t(reason)}</span>
          {job.state === 'unknown' && onCheck && (
            <button type="button" className="addon-button pkg-small" onClick={onCheck}>
              {t('packages.check')}
            </button>
          )}
        </div>
      )}
      {job.state === 'succeeded' && job.kind === 'apt-upgrade' && reboot?.required && (
        <RebootNotice packages={reboot.packages} />
      )}
      <OutputView output={output} running={running} incomplete={job.outputIncomplete === true} />
      {running && <p className="pkg-hint">{t('packages.job.background')}</p>}
    </div>
  )
}

// OutputView는 작업 출력이다. 맨 아래에 있으면 새 출력을 따라가고, 위로 올려 보는 동안은 따라가지 않는다.
function OutputView({ output, running, incomplete }: { output: OutputBuffer | null; running: boolean; incomplete: boolean }) {
  const { t } = useTranslation()
  const ref = useRef<HTMLPreElement>(null)
  const [follow, setFollow] = useState(true)
  const text = output?.text ?? ''
  useLayoutEffect(() => {
    const el = ref.current
    if (el && follow) el.scrollTop = el.scrollHeight
  }, [text, follow])
  const onScroll = () => {
    const el = ref.current
    if (el) setFollow(el.scrollHeight - el.scrollTop - el.clientHeight < 24)
  }
  const meta: string[] = []
  if (output) meta.push(t('packages.job.lines', { count: output.lines }))
  if (output?.truncated || incomplete) meta.push(t('packages.job.truncated'))
  if (running && follow) meta.push(t('packages.job.following'))
  return (
    <>
      <div className="pkg-output-head">
        <span className="pkg-output-title">{t('packages.job.output')}</span>
        <span className="pkg-muted">{meta.join(' · ')}</span>
      </div>
      <pre ref={ref} className="pkg-output selectable" tabIndex={0} onScroll={onScroll}>
        {text || (output && !running ? t('packages.job.noOutput') : '')}
      </pre>
    </>
  )
}

interface ConfirmProps {
  open: boolean
  data: PackagesResponse | null
  onCancel: () => void
  onConfirm: () => void
}

// InstallConfirm은 모두 설치 확인이다(시안 M). 창 범위 모달이라 그 창만 막고, 취소에 포커스한다.
function InstallConfirm({ open, data, onCancel, onConfirm }: ConfirmProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const cancelRef = useRef<HTMLButtonElement>(null)
  const pkgs = data?.packages ?? []
  const kernel = pkgs.some((p) => p.name.startsWith('linux-image'))
  return (
    <Modal
      open={open}
      scope="window"
      role="alertdialog"
      title={t('packages.confirm.title', { count: pkgs.length })}
      initialFocus={cancelRef}
      onCancel={onCancel}
      size={{ width: 452 }}
      footer={
        <>
          <button type="button" className="addon-button" ref={cancelRef} onClick={onCancel}>
            {t('dialog.cancel')}
          </button>
          <button type="button" className="addon-button danger" onClick={onConfirm}>
            {t('packages.confirm.action')}
          </button>
        </>
      }
    >
      <p className="modal-message">{t('packages.confirm.intro')}</p>
      <ul className="pkg-confirm-list">
        <li>{t('packages.confirm.size', { size: formatBytes(totalSize(pkgs), lang) })}</li>
        <li>{t('packages.confirm.services')}</li>
        {kernel && <li>{t('packages.confirm.kernel')}</li>}
        <li>{t('packages.confirm.background')}</li>
      </ul>
    </Modal>
  )
}
