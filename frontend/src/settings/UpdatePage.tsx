// 설정의 업데이트 페이지(관리자): 대시보드에서 업그레이드(ROADMAP.md). 확인을 요청하고, 확인 결과(새 버전, 커밋,
// 새 설정 질문, 수동 재설치 안내)를 보이고, root 권한 상승 뒤 업그레이드를 요청한다. 계산은 update.ts, 업그레이드를
// 요청한 뒤 다시 시작하기를 기다리는 일은 upgrade/watch.ts(앱 전체 감시)가 한다.
//
// 설치기 문구(사유, 질문, 할 일, 오류)는 업그레이드 서비스가 상태 파일에 쓴 영어·한국어 두 가지뿐이다(pickText).
import type { ParseKeys, TFunction } from 'i18next'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createPoller } from '../addons/common/poller'
import { formatDateTime } from '../addons/common/format'
import { ApiError, applyUpgrade, fetchHelperStatus, fetchUpgrade, requestUpgradeCheck } from '../api/client'
import type { HelperStatus, UpgradeInfo } from '../api/types.gen'
import type { UpgradeCheck, UpgradeError, UpgradeQuestion } from '../api/upgrade.gen'
import type { WindowViewProps } from '../addons/types'
import { useLanguage } from '../i18n'
import { useDialogs } from '../shell/dialogPresets'
import { requestElevation } from '../shell/elevationDialog'
import { useUpgradeWatch } from '../upgrade/watchStore'
import { CommandBlock, SettingsCard, SettingsNotice, SettingsRow, SettingsSection, Switch } from './controls'
import {
  APPLY_STAGES,
  type Answers,
  answerError,
  applyStep,
  describe,
  helpLines,
  initialAnswers,
  manualCommands,
  pickText,
  settingsFor,
  shortHash,
  type StageName,
  stageName,
} from './update'

// 진행 중일 때 상태를 다시 받는 간격.
const POLL_MS = 2000

type LoadState = { status: 'loading' } | { status: 'error' } | { status: 'ready'; info: UpgradeInfo }

interface ActionError {
  key: ParseKeys
  params?: Record<string, string | number>
}

export function UpdatePage({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const dialogs = useDialogs()
  const [load, setLoad] = useState<LoadState>({ status: 'loading' })
  const [helper, setHelper] = useState<HelperStatus | null>(null)
  const [actionError, setActionError] = useState<ActionError | null>(null)
  const [busy, setBusy] = useState(false)
  const watchPhase = useUpgradeWatch((s) => s.phase)
  const watchInfo = useUpgradeWatch((s) => s.info)
  const startWatch = useUpgradeWatch((s) => s.start)

  // 감시하는 동안은 감시가 받은 상태를 보인다(같은 요청을 둘이 하지 않게).
  const watching = watchPhase !== 'idle'
  const info = watching && watchInfo ? watchInfo : load.status === 'ready' ? load.info : null
  const state = info ? describe(info) : null

  // 창이 보일 때마다 상태와 도우미 상태를 다시 받는다.
  useEffect(() => {
    if (!active) return
    const ctl = new AbortController()
    fetchUpgrade(ctl.signal).then(
      (data) => setLoad({ status: 'ready', info: data }),
      () => {
        if (!ctl.signal.aborted) setLoad((prev) => (prev.status === 'ready' ? prev : { status: 'error' }))
      },
    )
    fetchHelperStatus(ctl.signal).then(setHelper, () => {})
    return () => ctl.abort()
  }, [active])

  // 확인·업그레이드가 진행 중이면 2초마다 다시 받는다(감시가 받고 있으면 그만둔다).
  const polling = active && !watching && !!state && (state.busy || state.activity === 'stalled')
  useEffect(() => {
    if (!polling) return
    const poller = createPoller({
      fetch: fetchUpgrade,
      intervalMs: POLL_MS,
      timeoutMs: 10_000,
      onData: (data) => setLoad({ status: 'ready', info: data }),
      onError: () => {},
    })
    poller.start()
    return poller.stop
  }, [polling])

  // 감시가 끝나면(업그레이드 실패, 다시 시작하지 않고 끝남) 마지막 상태를 이어받는다. 그사이 페이지가 내려가 있었으면
  // 다시 그릴 때 상태를 새로 받으므로 놓쳐도 된다.
  useEffect(
    () =>
      useUpgradeWatch.subscribe((s, prev) => {
        if (s.phase === 'idle' && prev.phase !== 'idle' && s.info) setLoad({ status: 'ready', info: s.info })
      }),
    [],
  )

  // 진행 중인 업그레이드를 보면(이 페이지를 새로 고쳤거나 다른 관리자가 요청함) 앱 전체 감시를 시작한다.
  const applyingTarget = state?.activity === 'applying' ? info?.status?.apply?.target : undefined
  useEffect(() => {
    if (applyingTarget && info) startWatch(applyingTarget, info)
  }, [applyingTarget, info, startWatch])

  const reload = async () => {
    try {
      setLoad({ status: 'ready', info: await fetchUpgrade() })
    } catch {
      // 다음 주기에 다시 받는다.
    }
  }

  const onCheck = async (mode: 'remote' | 'local') => {
    setBusy(true)
    setActionError(null)
    try {
      setLoad({ status: 'ready', info: await requestUpgradeCheck(mode) })
    } catch (err) {
      setActionError(actionErrorOf(err))
      await reload()
    } finally {
      setBusy(false)
    }
  }

  const onApply = async (check: UpgradeCheck, settings: Record<string, string>) => {
    const ok = await dialogs.confirm({
      title: t('settings.update.confirmTitle'),
      message: t('settings.update.confirmMessage'),
      confirmLabel: t('settings.update.apply'),
    })
    if (!ok || !check.target) return
    setBusy(true)
    setActionError(null)
    const target = check.target
    try {
      let result: UpgradeInfo
      try {
        result = await applyUpgrade(target, settings)
      } catch (err) {
        // root 권한이 필요하다. 비밀번호를 다시 확인해 상승한 뒤 한 번 더 요청한다.
        if (!(err instanceof ApiError && err.code === 'elevation_required')) throw err
        if (!(await requestElevation(dialogs.scope))) return
        result = await applyUpgrade(target, settings)
      }
      setLoad({ status: 'ready', info: result })
      startWatch(target, result)
    } catch (err) {
      setActionError(actionErrorOf(err))
      await reload()
    } finally {
      setBusy(false)
      fetchHelperStatus().then(setHelper, () => {})
    }
  }

  return (
    <>
      <SettingsNotice>{t('settings.update.intro')}</SettingsNotice>
      {load.status === 'loading' && <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>}
      {load.status === 'error' && <SettingsNotice tone="error">{t('settings.update.loadFailed')}</SettingsNotice>}
      {actionError && <SettingsNotice tone="error">{t(actionError.key, actionError.params)}</SettingsNotice>}
      {info && state && (
        <>
          <VersionSection info={info} />
          {!info.enabled ? (
            <DisabledSection info={info} />
          ) : (
            <>
              <CheckSection info={info} busy={busy || state.busy || watching} onCheck={(m) => void onCheck(m)} />
              {watching || state.activity === 'applying' || state.activity === 'interrupted' ? (
                <ApplyProgress info={info} waiting={watching && state.activity !== 'applying'} />
              ) : (
                <>
                  <ApplyResult info={info} />
                  <CheckResult
                    info={info}
                    helper={helper}
                    busy={busy || state.busy}
                    onApply={(check, settings) => void onApply(check, settings)}
                  />
                </>
              )}
            </>
          )}
        </>
      )}
    </>
  )
}

// actionErrorOf는 확인·업그레이드 요청의 오류 문구다.
function actionErrorOf(err: unknown): ActionError {
  if (!(err instanceof ApiError)) return { key: 'settings.update.errors.network' }
  switch (err.code) {
    case 'upgrade_busy':
      return { key: 'settings.update.errors.busy' }
    case 'upgrade_disabled':
      return { key: 'settings.update.errors.disabled' }
    case 'upgrade_outdated':
      return { key: 'settings.update.errors.outdated' }
    case 'rate_limited':
      return { key: 'settings.update.errors.rateLimited', params: { seconds: err.retryAfterSeconds ?? 30 } }
    case 'elevation_not_allowed':
      return { key: 'settings.update.errors.elevationNotAllowed' }
    case 'helper_reauth':
      return { key: 'settings.update.errors.helperReauth' }
    case 'helper_unavailable':
      return { key: 'settings.update.errors.helperUnavailable' }
    case 'bad_request':
      return { key: 'settings.update.errors.badRequest' }
  }
  return { key: 'settings.update.errors.failed', params: { status: err.status } }
}

function VersionSection({ info }: { info: UpgradeInfo }) {
  const { t } = useTranslation()
  return (
    <SettingsSection title={t('settings.update.current')}>
      <SettingsCard>
        <SettingsRow label={t('settings.update.version')}>
          <code className="update-value selectable">{info.version}</code>
          {info.revision && <code className="update-value selectable">{shortHash(info.revision)}</code>}
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  )
}

const DISABLED_KEYS = {
  'not-enabled': 'settings.update.disabled.notEnabled',
  'no-helper': 'settings.update.disabled.noHelper',
  'state-dir': 'settings.update.disabled.stateDir',
} as const satisfies Record<NonNullable<UpgradeInfo['disabledReason']>, ParseKeys>

function DisabledSection({ info }: { info: UpgradeInfo }) {
  const { t } = useTranslation()
  const reason = info.disabledReason ?? 'not-enabled'
  return (
    <SettingsSection title={t('settings.update.disabled.title')} description={t(DISABLED_KEYS[reason])}>
      {reason !== 'state-dir' && <CommandBlock text={manualCommands(info.repo, './install.sh --web-upgrade')} />}
      <p className="settings-hint">{t('settings.update.disabled.how')}</p>
    </SettingsSection>
  )
}

function CheckSection({
  info,
  busy,
  onCheck,
}: {
  info: UpgradeInfo
  busy: boolean
  onCheck: (mode: 'remote' | 'local') => void
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const state = describe(info)
  const checkedAt = info.status?.state === 'checked' ? info.status.check?.checkedAt : undefined
  return (
    <SettingsSection title={t('settings.update.check')} description={t('settings.update.checkDesc')}>
      <div className="settings-actions">
        <button type="button" className="addon-button primary" disabled={busy} onClick={() => onCheck('remote')}>
          {t('settings.update.checkRemote')}
        </button>
        <button type="button" className="addon-button" disabled={busy} onClick={() => onCheck('local')}>
          {t('settings.update.checkLocal')}
        </button>
      </div>
      {state.activity === 'pending' && <p className="settings-hint update-progress">{t('settings.update.pending')}</p>}
      {state.activity === 'stalled' && <p className="settings-hint error">{t('settings.update.stalled')}</p>}
      {state.activity === 'checking' && (
        <p className="settings-hint update-progress">
          {t('settings.update.checking', { stage: stageLabel(t, state.stage) })}
        </p>
      )}
      {state.activity === 'interrupted' && <p className="settings-hint error">{t('settings.update.interrupted')}</p>}
      {checkedAt && state.activity === 'none' && (
        <p className="settings-hint">{t('settings.update.checkedAt', { time: formatDateTime(checkedAt, lang) })}</p>
      )}
    </SettingsSection>
  )
}

const STAGE_KEYS = {
  inspect: 'settings.update.stages.inspect',
  fetch: 'settings.update.stages.fetch',
  plan: 'settings.update.stages.plan',
  update: 'settings.update.stages.update',
  build: 'settings.update.stages.build',
  install: 'settings.update.stages.install',
} as const satisfies Record<StageName, ParseKeys>

// stageLabel은 단계 이름의 문구다. 모르는 단계(다른 버전의 서비스)는 이름을 그대로 보인다.
function stageLabel(t: TFunction, stage: string | null | undefined): string {
  const name = stageName(stage)
  return name ? t(STAGE_KEYS[name]) : (stage ?? '')
}

function ErrorDetails({ error }: { error: UpgradeError | undefined | null }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  if (!error) return null
  return (
    <>
      <p className="settings-hint error selectable">{pickText(error.text, lang)}</p>
      {error.log && (
        <details className="update-log">
          <summary>{t('settings.update.log')}</summary>
          <pre className="selectable">{error.log}</pre>
        </details>
      )}
    </>
  )
}

function CheckResult({
  info,
  helper,
  busy,
  onApply,
}: {
  info: UpgradeInfo
  helper: HelperStatus | null
  busy: boolean
  onApply: (check: UpgradeCheck, settings: Record<string, string>) => void
}) {
  const { t } = useTranslation()
  const state = describe(info)
  const check = info.status?.check
  switch (state.check) {
    case 'none':
      return null
    case 'failed':
      return (
        <SettingsSection
          title={t('settings.update.checkFailed', { stage: stageLabel(t, state.stage) })}
          description={t('settings.update.checkFailedDesc')}
        >
          <ErrorDetails error={info.status?.error} />
        </SettingsSection>
      )
    case 'latest':
      return <SettingsNotice>{t('settings.update.latest')}</SettingsNotice>
    case 'outdated':
      return <SettingsNotice tone="error">{t('settings.update.outdated')}</SettingsNotice>
  }
  if (!check) return null
  return (
    <>
      <TargetSection check={check} />
      {state.check === 'manual' ? (
        <ManualSection info={info} check={check} />
      ) : (
        <UpgradeSection key={check.target} check={check} helper={helper} busy={busy} onApply={onApply} />
      )}
    </>
  )
}

function TargetSection({ check }: { check: UpgradeCheck }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const plan = check.plan
  return (
    <SettingsSection title={t('settings.update.newVersion')}>
      <SettingsCard>
        <SettingsRow label={t('settings.update.version')}>
          <code className="update-value selectable">{plan?.version || t('settings.update.unknown')}</code>
          <code className="update-value selectable">{shortHash(check.target)}</code>
        </SettingsRow>
        <SettingsRow label={t('settings.update.source')}>
          <span className="update-value">
            {check.mode === 'local'
              ? t('settings.update.sourceLocal')
              : t('settings.update.sourceRemote', { upstream: check.upstream ?? '' })}
          </span>
        </SettingsRow>
      </SettingsCard>
      {check.commits.length > 0 && (
        <details className="update-commits">
          <summary>
            {check.commitsTruncated
              ? t('settings.update.commitsTruncated', { count: check.commits.length })
              : t('settings.update.commits', { count: check.commits.length })}
          </summary>
          <ul>
            {check.commits.map((c) => (
              <li key={c.hash}>
                <span className="update-commit-subject selectable">{c.subject}</span>
                <span className="update-commit-meta">
                  <code className="selectable">{shortHash(c.hash)}</code> · {c.author} · {formatDateTime(c.time, lang)}
                </span>
              </li>
            ))}
          </ul>
        </details>
      )}
    </SettingsSection>
  )
}

function ManualSection({ info, check }: { info: UpgradeInfo; check: UpgradeCheck }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  return (
    <SettingsSection title={t('settings.update.manualTitle')} description={t('settings.update.manualDesc')}>
      <ul className="update-reasons">
        {check.manual.map((r) => (
          <li key={`${r.id}-${r.setting ?? ''}`} className="selectable">
            {pickText(r.text, lang)}
          </li>
        ))}
      </ul>
      <p className="settings-section-desc update-follow">
        {info.user ? t('settings.update.manualHowUser', { user: info.user }) : t('settings.update.manualHow')}
      </p>
      <CommandBlock text={manualCommands(info.repo, check.command)} />
      <p className="settings-hint">{t('settings.update.manualPull')}</p>
    </SettingsSection>
  )
}

function UpgradeSection({
  check,
  helper,
  busy,
  onApply,
}: {
  check: UpgradeCheck
  helper: HelperStatus | null
  busy: boolean
  onApply: (check: UpgradeCheck, settings: Record<string, string>) => void
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const questions = check.plan?.questions ?? []
  const notes = check.plan?.notes ?? []
  const [answers, setAnswers] = useState<Answers>(() => initialAnswers(questions))
  const settings = settingsFor(questions, answers)
  // 업그레이드는 root 권한 상승이 필요하다. 상승할 수 없는 세션이면 단추 대신 이유를 보인다.
  const blockedKey: ParseKeys | null =
    helper === null || helper.state === 'active'
      ? helper?.canElevate === false
        ? 'settings.update.cannotElevate'
        : null
      : helper.state === 'reauth'
        ? 'settings.update.errors.helperReauth'
        : 'settings.update.noHelperTicket'
  return (
    <>
      {questions.length > 0 && (
        <SettingsSection title={t('settings.update.questions')} description={t('settings.update.questionsDesc')}>
          <SettingsCard>
            {questions.map((q) => (
              <QuestionRow
                key={q.name}
                question={q}
                value={answers[q.name] ?? ''}
                onChange={(v) => setAnswers((a) => ({ ...a, [q.name]: v }))}
              />
            ))}
          </SettingsCard>
        </SettingsSection>
      )}
      {notes.length > 0 && (
        <SettingsSection title={t('settings.update.notes')}>
          <ul className="update-reasons">
            {notes.map((n, i) => (
              <li key={i} className="selectable">
                {pickText(n, lang)}
              </li>
            ))}
          </ul>
        </SettingsSection>
      )}
      <SettingsSection title={t('settings.update.applyTitle')} description={t('settings.update.applyDesc')}>
        {blockedKey ? (
          <p className="settings-hint error">{t(blockedKey)}</p>
        ) : (
          <div className="settings-actions">
            <button
              type="button"
              className="addon-button primary"
              disabled={busy || settings === null}
              onClick={() => settings && onApply(check, settings)}
            >
              {t('settings.update.apply')}
            </button>
            {settings === null && <span className="settings-hint">{t('settings.update.answerAll')}</span>}
          </div>
        )}
      </SettingsSection>
    </>
  )
}

function QuestionRow({
  question: q,
  value,
  onChange,
}: {
  question: UpgradeQuestion
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const title = pickText(q.title, lang) || q.name
  const err = answerError(q, value)
  const description = (
    <>
      {helpLines(pickText(q.help, lang)).map((line, i) => (
        <span key={i} className="update-help">
          {line}
        </span>
      ))}
      <span className="update-help">
        <code>--{q.name}</code> · {t(q.reason === 'invalid' ? 'settings.update.reasonInvalid' : 'settings.update.reasonNew')}
      </span>
      {q.error && <span className="update-help error">{pickText(q.error, lang)}</span>}
      {q.kind === 'input' && err && (
        <span className="update-help error">
          {t(err === 'required' ? 'settings.update.answerRequired' : 'settings.update.answerInvalid')}
        </span>
      )}
    </>
  )
  return (
    <SettingsRow label={title} description={description}>
      {q.kind === 'confirm' ? (
        <Switch checked={value === 'yes'} label={title} onChange={(on) => onChange(on ? 'yes' : 'no')} />
      ) : (
        <input
          className="settings-input"
          type="text"
          value={value}
          aria-label={title}
          aria-invalid={err !== null}
          spellCheck={false}
          autoCapitalize="off"
          autoComplete="off"
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </SettingsRow>
  )
}

function ApplyProgress({ info, waiting }: { info: UpgradeInfo; waiting: boolean }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const st = info.status
  const interrupted = describe(info).activity === 'interrupted'
  const current = waiting ? -1 : applyStep(st?.stage)
  return (
    <SettingsSection title={t('settings.update.applying')} description={t('settings.update.applyingDesc')}>
      {waiting && <p className="settings-hint update-progress">{t('settings.update.applyWaiting')}</p>}
      {interrupted && <p className="settings-hint error">{t('settings.update.interrupted')}</p>}
      <ol className="update-stages">
        {APPLY_STAGES.map((s, i) => (
          <li key={s} className={i < current ? 'done' : i === current ? (interrupted ? 'stopped' : 'current') : undefined}>
            {t(STAGE_KEYS[s])}
          </li>
        ))}
      </ol>
      {st?.apply && !waiting && (
        <p className="settings-hint">
          {t('settings.update.requestedBy', { user: st.apply.user, time: formatDateTime(st.apply.startedAt, lang) })}
        </p>
      )}
    </SettingsSection>
  )
}

function ApplyResult({ info }: { info: UpgradeInfo }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const state = describe(info)
  const st = info.status
  if (state.apply === 'none' || !st) return null
  if (state.apply === 'applied') {
    const notes = st.check?.plan?.notes ?? []
    return (
      <SettingsSection title={t('settings.update.applied', { version: st.apply?.version || t('settings.update.unknown') })}>
        {st.apply?.finishedAt && (
          <p className="settings-hint">
            {t('settings.update.appliedAt', { user: st.apply.user, time: formatDateTime(st.apply.finishedAt, lang) })}
          </p>
        )}
        {notes.length > 0 && (
          <>
            <p className="settings-section-desc update-follow">{t('settings.update.notesAfter')}</p>
            <ul className="update-reasons">
              {notes.map((n, i) => (
                <li key={i} className="selectable">
                  {pickText(n, lang)}
                </li>
              ))}
            </ul>
          </>
        )}
      </SettingsSection>
    )
  }
  return (
    <SettingsSection
      title={t('settings.update.applyFailed', { stage: stageLabel(t, st.stage) })}
      description={t(st.stage === 'install' ? 'settings.update.applyFailedInstall' : 'settings.update.applyFailedDesc')}
    >
      <ErrorDetails error={st.error} />
    </SettingsSection>
  )
}
