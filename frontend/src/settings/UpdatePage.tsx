// 설정의 업데이트 페이지(관리자): 대시보드에서 업그레이드(.claude/docs/upgrade.md). 업데이트 채널을 고르고, 확인을 요청하고,
// 확인 결과(새 버전, 커밋, 새 설정 질문, 수동 재설치 안내)를 보이고, root 권한 상승 뒤 업그레이드를 요청한다. 계산은
// update.ts, 업그레이드를 요청한 뒤 다시 시작하기를 기다리는 일은 upgrade/watch.ts(앱 전체 감시)가 한다.
//
// 설치기 문구(사유, 질문, 할 일, 오류)는 업그레이드 서비스가 상태 파일에 쓴 영어·한국어 두 가지뿐이다(pickText).
import type { ParseKeys, TFunction } from 'i18next'
import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createPoller } from '../addons/common/poller'
import { formatDateTime } from '../addons/common/format'
import { ApiError, applyUpgrade, fetchHelperStatus, fetchUpgrade, requestUpgradeCheck, setUpgradeChannel } from '../api/client'
import type { HelperStatus, UpgradeInfo } from '../api/types.gen'
import type { UpgradeCheck, UpgradeError, UpgradeQuestion } from '../api/upgrade.gen'
import type { WindowViewProps } from '../addons/types'
import { useLanguage } from '../i18n'
import { useDialogs } from '../shell/dialogPresets'
import { requestElevation } from '../shell/elevationDialog'
import { useUpgradeWatch } from '../upgrade/watchStore'
import { shortRevision } from './about/about'
import { CommandBlock, SettingsCard, SettingsNotice, SettingsRow, SettingsSection, Switch } from './controls'
import {
  APPLY_STAGES,
  type Answers,
  answerError,
  applyConfirm,
  applyStep,
  type Channel,
  type ChannelCard,
  channelCards,
  channelValue,
  describe,
  helpLines,
  initialAnswers,
  installedVersion,
  isDevTarget,
  manualCommands,
  pickText,
  repoBranch,
  settingsFor,
  type StageName,
  stageName,
  versionLabel,
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
  // savingChannel은 저장 중인 업데이트 채널이다. 저장하는 동안 고른 카드를 먼저 보인다.
  const [savingChannel, setSavingChannel] = useState<Channel | null>(null)
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

  const onChannel = async (next: Channel) => {
    if (!info || next === info.channel) return
    // 개발 채널은 릴리스 전 커밋을 받으므로 고르기 전에 한 번 묻는다.
    if (
      next === 'main' &&
      !(await dialogs.confirm({
        title: t('settings.update.channel.confirmMainTitle'),
        message: t('settings.update.channel.confirmMainMessage'),
        confirmLabel: t('settings.update.channel.confirmMain'),
      }))
    ) {
      return
    }
    setSavingChannel(next)
    setActionError(null)
    try {
      setLoad({ status: 'ready', info: await setUpgradeChannel(next) })
    } catch (err) {
      setActionError(actionErrorOf(err))
      await reload()
    } finally {
      setSavingChannel(null)
    }
  }

  const onApply = async (check: UpgradeCheck, settings: Record<string, string>) => {
    const { overwrite, dropped, switchTo } = applyConfirm(check)
    // 이전 버전으로 덮어쓰면 새 버전·커밋 목록 대신 빠지는 커밋을 알린다. 브랜치를 옮기면 두 경우 모두 알린다.
    const message = [
      overwrite ? t('settings.update.confirmOverwriteMessage', { count: dropped }) : t('settings.update.confirmMessage'),
      switchTo ? t('settings.update.confirmSwitch', { branch: switchTo }) : '',
    ]
      .filter((s) => s !== '')
      .join(' ')
    const ok = await dialogs.confirm({
      title: t(overwrite ? 'settings.update.confirmOverwriteTitle' : 'settings.update.confirmTitle'),
      message,
      confirmLabel: applyLabel(t, check),
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
              <ChannelSection
                info={info}
                channel={savingChannel ?? info.channel}
                busy={busy || state.busy || watching || savingChannel !== null}
                onChange={(c) => void onChannel(c)}
              />
              <CheckSection
                info={info}
                channel={savingChannel ?? info.channel}
                busy={busy || state.busy || watching || savingChannel !== null}
                onCheck={(m) => void onCheck(m)}
              />
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

// applyLabel은 업그레이드 단추의 문구다. 채널을 바꾸면 그 브랜치를, 이전 버전을 덮어쓰면 그렇다고 적는다.
function applyLabel(t: TFunction, check: UpgradeCheck): string {
  const { overwrite, switchTo } = applyConfirm(check)
  if (switchTo) return t(`settings.update.${overwrite ? 'overwriteTo' : 'applyTo'}.${switchTo}`)
  return t('settings.update.apply')
}

// actionErrorOf는 확인·업그레이드·채널 저장 요청의 오류 문구다.
function actionErrorOf(err: unknown): ActionError {
  if (!(err instanceof ApiError)) return { key: 'settings.update.errors.network' }
  switch (err.code) {
    case 'upgrade_busy':
      return { key: 'settings.update.errors.busy' }
    case 'upgrade_disabled':
      return { key: 'settings.update.errors.disabled' }
    case 'upgrade_outdated':
      return { key: 'settings.update.errors.outdated' }
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
  const { version, revision } = installedVersion(info)
  const branch = repoBranch(info)
  return (
    <SettingsSection title={t('settings.update.current')}>
      <SettingsCard>
        <SettingsRow label={t('settings.update.version')}>
          <span className="update-value-line">
            <code className="update-value selectable">{versionLabel(version, revision)}</code>
            {branch.kind === 'branch' && branch.name === 'main' && (
              <span className="settings-badge update-dev">{t('settings.update.devBuild')}</span>
            )}
          </span>
        </SettingsRow>
        {branch.kind !== 'unknown' && (
          <SettingsRow label={t('settings.update.branch')}>
            {branch.kind === 'branch' ? (
              <code className="update-value selectable">{branch.name}</code>
            ) : (
              <span className="update-value">{t('settings.update.branchDetached')}</span>
            )}
          </SettingsRow>
        )}
      </SettingsCard>
    </SettingsSection>
  )
}

// ChannelSection은 업데이트 채널 카드다. 고르면 바로 저장하고, 다음 원격 확인부터 그 채널을 본다.
function ChannelSection({
  info,
  channel,
  busy,
  onChange,
}: {
  info: UpgradeInfo
  channel: Channel
  busy: boolean
  onChange: (channel: Channel) => void
}) {
  const { t } = useTranslation()
  const name = useId()
  const branch = repoBranch(info)
  const { cards, selected } = channelCards(channel, branch)
  return (
    <SettingsSection title={t('settings.update.channel.title')} description={t('settings.update.channel.desc')}>
      <div className="update-channels" role="radiogroup" aria-label={t('settings.update.channel.title')}>
        {cards.map((card) => (
          <label key={card} className="update-channel">
            <input
              type="radio"
              className="picker-input"
              name={name}
              value={card}
              checked={selected === card}
              disabled={busy}
              onChange={() => onChange(channelValue(card))}
            />
            <ChannelCardText card={card} branch={branch} />
          </label>
        ))}
      </div>
      {selected === 'main' ? (
        <p className="update-warn">
          {t(
            // 이미 main 브랜치면 업그레이드해도 브랜치가 바뀌지 않는다.
            branch.kind === 'branch' && branch.name === 'main'
              ? 'settings.update.channel.mainWarningOnMain'
              : 'settings.update.channel.mainWarning',
          )}
        </p>
      ) : (
        <p className="settings-hint">{t('settings.update.channel.hint')}</p>
      )}
    </SettingsSection>
  )
}

function ChannelCardText({ card, branch }: { card: ChannelCard; branch: ReturnType<typeof repoBranch> }) {
  const { t } = useTranslation()
  let title: string
  let desc: string
  let code: string | null = card
  switch (card) {
    case 'stable':
      title = t('settings.update.channel.stable')
      desc = t('settings.update.channel.stableDesc')
      break
    case 'main':
      title = t('settings.update.channel.main')
      desc = t('settings.update.channel.mainDesc')
      break
    case 'current':
      if (branch.kind === 'detached') {
        title = t('settings.update.channel.detached')
        desc = t('settings.update.channel.detachedDesc')
        code = null
      } else {
        title = t('settings.update.channel.current')
        desc = t(branch.kind === 'branch' ? 'settings.update.channel.currentDesc' : 'settings.update.channel.currentUnknownDesc')
        code = branch.kind === 'branch' ? branch.name : null
      }
      break
  }
  return (
    <>
      <span className="update-channel-head">
        <span className="update-channel-dot" aria-hidden="true" />
        <span className="update-channel-name">{title}</span>
        {code && <code className="update-channel-branch">{code}</code>}
        {card === 'stable' && <span className="settings-badge update-recommended">{t('settings.update.channel.recommended')}</span>}
      </span>
      <span className="update-channel-desc">{desc}</span>
    </>
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
  channel,
  busy,
  onCheck,
}: {
  info: UpgradeInfo
  channel: Channel
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
          {channel ? t('settings.update.checkChannel', { channel }) : t('settings.update.checkRemote')}
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
    case 'channel':
      return <SettingsNotice>{t('settings.update.channel.changed')}</SettingsNotice>
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
          <span className="update-value-line">
            <code className="update-value selectable">
              {versionLabel(plan?.version || t('settings.update.unknown'), check.target)}
            </code>
            {isDevTarget(check) && <span className="settings-badge update-dev">{t('settings.update.devBuild')}</span>}
          </span>
        </SettingsRow>
        <SettingsRow label={t('settings.update.source')}>
          <span className="update-value">
            {check.mode === 'local'
              ? t('settings.update.sourceLocal')
              : t('settings.update.sourceRemote', { upstream: check.upstream ?? '' })}
          </span>
        </SettingsRow>
        {check.switch && check.channel && (
          <SettingsRow label={t('settings.update.branch')} description={t('settings.update.switchBranch')}>
            <span className="update-value-line">
              {check.branch ? (
                <code className="update-value selectable">{check.branch}</code>
              ) : (
                <span className="update-value">{t('settings.update.branchDetached')}</span>
              )}
              <span>→</span>
              <code className="update-value selectable">{check.channel}</code>
            </span>
          </SettingsRow>
        )}
      </SettingsCard>
      {(check.dropped ?? 0) > 0 && (
        <p className="update-warn">{t('settings.update.dropped', { count: check.dropped ?? 0 })}</p>
      )}
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
                  <code className="selectable">{shortRevision(c.hash)}</code> · {c.author} ·{' '}
                  {formatDateTime(c.time, lang)}
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
      {check.command ? (
        <>
          <p className="settings-section-desc update-follow">
            {info.user ? t('settings.update.manualHowUser', { user: info.user }) : t('settings.update.manualHow')}
          </p>
          <CommandBlock text={manualCommands(info.repo, check.command)} />
          <p className="settings-hint">{t('settings.update.manualPull')}</p>
        </>
      ) : (
        // 명령이 없으면 SSH로 저장소를 정리한 뒤 다시 확인하면 되는 사유뿐이다(로컬 채널 브랜치 정리 등).
        <p className="settings-section-desc update-follow">
          {info.user ? t('settings.update.manualRecheckUser', { user: info.user }) : t('settings.update.manualRecheck')}
        </p>
      )}
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
              {applyLabel(t, check)}
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
      <SettingsSection
        title={t('settings.update.applied', {
          version: versionLabel(st.apply?.version || t('settings.update.unknown'), st.apply?.target),
        })}
      >
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
