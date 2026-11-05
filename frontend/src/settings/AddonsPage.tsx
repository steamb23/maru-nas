// 설정의 애드온 페이지(관리자): 애드온 패키지(.maru-addon)를 올려 권한을 검토하고 설치한다. 설치한 패키지를 켜고 끄고
// 지운다. 계산(정렬, 문구 키, 권한 차이)은 addons.ts, 설계는 .claude/docs/addon-packages.md의 "권한 요약과 승인".
//
// 패키지가 준 문구(이름, 설명, 작성자)는 평문으로만 그리고 패키지 배지를 붙인다. 아이콘은 <img>로만 그린다.
// 설치·켜기는 관리 창(manage_required)과, 쓰기·백엔드가 있으면 단계 확인(security_required)을 거친 뒤 다시 보낸다.
// 백엔드 섹션은 패키지 백엔드가 켜졌는지와 root 동기화의 상태를 보이고 다시 동기화를 요청한다(설계는
// .claude/docs/addon-packages.md의 "root 동기화"). 요청이 처리되기를 기다리는 동안 목록을 다시 받는다.
import type { ParseKeys } from 'i18next'
import { createRef, type ReactNode, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { formatDateTime } from '../addons/common/format'
import type { WindowViewProps } from '../addons/types'
import { refreshCapabilities } from '../addons/useCapabilities'
import { localizedText } from '../addonpkg/catalog'
import {
  discardAddonUpload,
  fetchAddonPackages,
  installAddonUpload,
  requestAddonSync,
  setAddonPackageEnabled,
  uninstallAddonPackage,
  uploadAddonPackage,
} from '../api/client'
import type {
  AddonBackendSync,
  AddonPackageStatus,
  AddonPackagesResponse,
  AddonPendingInstall,
  AddonPermissionsDiff,
} from '../api/types.gen'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import { requestManage, requestStepUp } from '../security/stepUpDialog'
import { type Dialogs, useDialogs } from '../shell/dialogPresets'
import { MessageButtons } from '../shell/MessageDialog'
import {
  ADDON_BACKENDS_COMMAND,
  type AddonError,
  MAX_AUTH_ROUNDS,
  type PermissionItem,
  SYNC_POLL_MS,
  addonError,
  authStep,
  backendBadgeKey,
  canRequestSync,
  failedBackends,
  mib,
  packageTitle,
  reasonKey,
  riskKeys,
  shouldPollSync,
  sortPackages,
  stateBadgeKey,
  syncNotices,
  warningKeys,
  withChanges,
} from './addons'
import { CommandBlock, SettingsCard, SettingsNotice, SettingsRow, SettingsSection, Switch } from './controls'

type LoadState = { status: 'loading' } | { status: 'error' } | { status: 'ready'; data: AddonPackagesResponse }

interface Done {
  key: ParseKeys
  name: string
}

// 백엔드 기본 한도(addonpkg.DefaultLimits.Compressed). 목록을 받기 전에만 쓴다.
const DEFAULT_MAX_UPLOAD = 64 * 1024 * 1024

export function AddonsPage({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const dialogs = useDialogs()
  const [load, setLoad] = useState<LoadState>({ status: 'loading' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<AddonError | null>(null)
  const [done, setDone] = useState<Done | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const data = load.status === 'ready' ? load.data : null
  const maxBytes = data?.maxUploadBytes ?? DEFAULT_MAX_UPLOAD
  const maxMiB = mib(maxBytes)

  // 창이 보일 때마다 목록을 다시 받는다.
  useEffect(() => {
    if (!active) return
    const ctl = new AbortController()
    fetchAddonPackages(ctl.signal).then(
      (d) => setLoad({ status: 'ready', data: d }),
      () => {
        if (!ctl.signal.aborted) setLoad((prev) => (prev.status === 'ready' ? prev : { status: 'error' }))
      },
    )
    return () => ctl.abort()
  }, [active])

  const reload = async () => {
    try {
      setLoad({ status: 'ready', data: await fetchAddonPackages() })
    } catch {
      // 다음에 페이지가 보일 때 다시 받는다.
    }
  }

  // 동기화 결과를 기다리는 동안 목록을 다시 받는다. 받기가 끝날 때마다(실패해도) pollTick이 바뀌어 다음 차례를 건다.
  const [pollTick, setPollTick] = useState(0)
  const polling = active && shouldPollSync(data?.backends)
  useEffect(() => {
    if (!polling) return
    const timer = window.setTimeout(() => {
      void reload().finally(() => setPollTick((n) => n + 1))
    }, SYNC_POLL_MS)
    return () => window.clearTimeout(timer)
  }, [polling, pollTick])

  // withAuth는 요청을 보내고, 관리 창·단계 확인이 모자라다는 거절이면 확인한 뒤 다시 보낸다. 취소하면 null이다.
  const withAuth = async <T,>(task: () => Promise<T>): Promise<T | null> => {
    for (let round = 0; ; round++) {
      try {
        return await task()
      } catch (err) {
        const step = round < MAX_AUTH_ROUNDS ? authStep(err) : null
        if (!step) throw err
        const ok =
          step.kind === 'manage'
            ? await requestManage(dialogs.scope, 'addons')
            : await requestStepUp(dialogs.scope, { level: step.level })
        if (!ok) return null
      }
    }
  }

  // act는 패키지를 바꾸는 작업 하나를 한다. 끝나면 목록과 데스크톱의 앱 목록(capabilities)을 다시 받는다.
  const act = async (task: () => Promise<Done | null>) => {
    if (busy) return
    setBusy(true)
    setError(null)
    setDone(null)
    try {
      setDone(await task())
    } catch (err) {
      setError(addonError(err, maxMiB))
    } finally {
      setBusy(false)
      await reload()
      void refreshCapabilities()
    }
  }

  const onFile = (file: File) => {
    if (file.size > maxBytes) {
      setDone(null)
      setError({ key: 'settings.addons.errors.tooLarge', params: { size: maxMiB } })
      return
    }
    void act(async () => {
      const pending = await uploadAddonPackage(file)
      let installed = false
      try {
        if (!(await reviewInstall(dialogs, pending))) return null
        const st = await withAuth(() => installAddonUpload(pending.pendingId))
        if (!st) return null
        installed = true
        return {
          key: pending.installed ? 'settings.addons.done.updated' : 'settings.addons.done.installed',
          name: packageTitle(st, lang),
        }
      } finally {
        // 설치하지 않았으면 검토 대기를 버린다(세션마다 2개까지라 남겨 두면 다음 올리기가 밀어낸다).
        if (!installed) void discardAddonUpload(pending.pendingId).catch(() => {})
      }
    })
  }

  const onToggle = (p: AddonPackageStatus, enabled: boolean) =>
    void act(async () => {
      const st = await withAuth(() => setAddonPackageEnabled(p.id, enabled))
      if (!st) return null
      return { key: enabled ? 'settings.addons.done.enabled' : 'settings.addons.done.disabled', name: packageTitle(p, lang) }
    })

  const onUninstall = async (p: AddonPackageStatus) => {
    if (busy) return
    const choice = await confirmUninstall(dialogs, p, packageTitle(p, lang))
    if (!choice) return
    void act(async () => {
      const ok = await withAuth(async () => {
        await uninstallAddonPackage(p.id, choice.purge)
        return true
      })
      if (!ok) return null
      return { key: 'settings.addons.done.uninstalled', name: packageTitle(p, lang) }
    })
  }

  // onSync는 root 동기화를 다시 요청한다(실패한 백엔드를 다시 켜 보거나 멈춘 요청을 다시 쓴다).
  const onSync = async () => {
    if (busy) return
    setBusy(true)
    setError(null)
    setDone(null)
    try {
      const backends = await requestAddonSync()
      setLoad((prev) => (prev.status === 'ready' ? { status: 'ready', data: { ...prev.data, backends } } : prev))
      setDone({ key: 'settings.addons.backend.requested', name: '' })
    } catch (err) {
      setError(addonError(err, maxMiB))
    } finally {
      setBusy(false)
    }
  }

  const onDetails = (p: AddonPackageStatus) =>
    void dialogs.open<true>({
      title: t('settings.addons.details.title'),
      size: { width: 520 },
      render: () => <PackageSummary pkg={p} mode="details" />,
      footer: (ctl) => (
        <MessageButtons buttons={[{ id: 'ok', labelKey: 'dialog.ok', tone: 'primary' }]} onPress={() => ctl.close(true)} />
      ),
    })

  const packages = data ? sortPackages(data.packages, lang) : []

  return (
    <>
      <SettingsNotice>{t('settings.addons.intro')}</SettingsNotice>
      {load.status === 'loading' && <SettingsNotice>{t('settings.common.loading')}</SettingsNotice>}
      {load.status === 'error' && <SettingsNotice tone="error">{t('settings.common.loadFailed')}</SettingsNotice>}
      {data?.indexError && <SettingsNotice tone="error">{t('settings.addons.indexError')}</SettingsNotice>}
      {error && (
        <SettingsNotice tone="error">
          {t(error.key, error.params)}
          {error.detail && <span className="addons-pkg-detail selectable">{error.detail}</span>}
        </SettingsNotice>
      )}
      {done && <SettingsNotice>{t(done.key, { name: done.name })}</SettingsNotice>}

      {data && (
        <>
          <SettingsSection title={t('settings.addons.install')} description={t('settings.addons.installDesc', { size: maxMiB })}>
            {!data.canInstall && <p className="settings-hint">{t('settings.addons.cannotInstall')}</p>}
            <div className="settings-actions">
              <button
                type="button"
                className="addon-button primary"
                disabled={busy || !data.canInstall}
                onClick={() => fileRef.current?.click()}
              >
                {t('settings.addons.upload')}
              </button>
              <input
                ref={fileRef}
                type="file"
                accept=".maru-addon"
                hidden
                onChange={(e) => {
                  const file = e.currentTarget.files?.[0]
                  // 같은 파일을 다시 고를 수 있게 비운다.
                  e.currentTarget.value = ''
                  if (file) onFile(file)
                }}
              />
            </div>
          </SettingsSection>

          <SettingsSection title={t('settings.addons.installed')} description={t('settings.addons.installedDesc')}>
            {packages.length === 0 ? (
              <p className="settings-hint">{t('settings.addons.empty')}</p>
            ) : (
              <SettingsCard>
                {packages.map((p) => (
                  <PackageRow
                    key={`${p.dev ? 'dev' : 'pkg'}:${p.id}`}
                    pkg={p}
                    busy={busy}
                    onToggle={(on) => onToggle(p, on)}
                    onDetails={() => onDetails(p)}
                    onUninstall={() => void onUninstall(p)}
                  />
                ))}
              </SettingsCard>
            )}
          </SettingsSection>

          <BackendSection sync={data.backends} failed={failedBackends(data.packages)} busy={busy} onSync={() => void onSync()} />
        </>
      )}
    </>
  )
}

// BackendSection은 패키지 백엔드 상태다: 꺼진 설치는 켜는 명령, 설치본은 마지막 동기화 시각과 상태·오류, 다시 동기화.
function BackendSection({
  sync,
  failed,
  busy,
  onSync,
}: {
  sync: AddonBackendSync
  failed: number
  busy: boolean
  onSync: () => void
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const notices = syncNotices(sync, failed)
  return (
    <SettingsSection title={t('settings.addons.backend.title')} description={t('settings.addons.backend.desc')}>
      {sync.managed && (
        <p className="settings-hint">
          {sync.syncedAt
            ? t('settings.addons.backend.syncedAt', { time: formatDateTime(sync.syncedAt, lang) })
            : t('settings.addons.backend.neverSynced')}
        </p>
      )}
      {notices.map((n) => (
        <SettingsNotice key={n.key} tone={n.tone}>
          {t(n.key, n.params)}
          {n.detail && <span className="addons-pkg-detail selectable">{n.detail}</span>}
        </SettingsNotice>
      ))}
      {notices.some((n) => n.command) && <CommandBlock text={ADDON_BACKENDS_COMMAND} />}
      {sync.managed && (
        <div className="settings-actions">
          <button type="button" className="addon-button" disabled={busy || !canRequestSync(sync)} onClick={onSync}>
            {t('settings.addons.backend.syncAgain')}
          </button>
        </div>
      )}
    </SettingsSection>
  )
}

function PackageRow({
  pkg,
  busy,
  onToggle,
  onDetails,
  onUninstall,
}: {
  pkg: AddonPackageStatus
  busy: boolean
  onToggle: (enabled: boolean) => void
  onDetails: () => void
  onUninstall: () => void
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const name = packageTitle(pkg, lang)
  const badge = stateBadgeKey(pkg)
  const backendBadge = backendBadgeKey(pkg)
  const problem = pkg.state === 'rejected' || pkg.state === 'invalid'
  let description: ReactNode = problem ? t(reasonKey(pkg.reason)) : localizedText(pkg.description, lang)
  if (!problem && pkg.backend?.state === 'failed') {
    description = (
      <>
        {t('settings.addons.backend.failed')}
        {pkg.backend.error && <span className="addons-pkg-detail selectable">{pkg.backend.error}</span>}
      </>
    )
  }
  return (
    <SettingsRow
      label={
        <span className="addons-pkg-title">
          <IconImage name={pkg.icon ? { url: pkg.icon } : 'application-x-addon'} size={32} />
          <span className="addons-pkg-name">{name}</span>
          {pkg.version && <span className="addons-pkg-version">{pkg.version}</span>}
          {pkg.dev && <span className="settings-badge">{t('settings.addons.dev')}</span>}
          {badge && <span className="settings-badge addons-pkg-problem">{t(badge)}</span>}
          {backendBadge && (
            <span className={pkg.backend?.state === 'failed' ? 'settings-badge addons-pkg-problem' : 'settings-badge'}>
              {t(backendBadge)}
            </span>
          )}
        </span>
      }
      description={description || undefined}
    >
      <button type="button" className="addon-button" onClick={onDetails}>
        {t('settings.addons.detailsButton')}
      </button>
      {!pkg.dev && (
        <>
          <button type="button" className="addon-button" disabled={busy} onClick={onUninstall}>
            {t('settings.addons.uninstall')}
          </button>
          <Switch
            checked={pkg.enabled}
            disabled={busy}
            label={t('settings.addons.enableLabel', { name })}
            onChange={onToggle}
          />
        </>
      )}
    </SettingsRow>
  )
}

// reviewInstall은 올린 패키지의 권한을 보이고 설치할지 묻는다. 설치를 누르면 true다.
async function reviewInstall(dialogs: Dialogs, pending: AddonPendingInstall): Promise<boolean> {
  const cancelRef = createRef<HTMLButtonElement>()
  const update = pending.installed !== undefined
  // 권한이 넓어지거나 OMV 쓰기가 있으면 위험 색으로 그리고, 언제나 취소에 먼저 포커스한다.
  const risky = (pending.diff?.widens ?? false) || (pending.package.permissions?.risks.length ?? 0) > 0
  const ok = await dialogs.open<true>({
    title: <ReviewTitle update={update} />,
    size: { width: 520 },
    initialFocus: cancelRef,
    render: () => (
      <PackageSummary
        pkg={pending.package}
        installed={pending.installed}
        diff={pending.diff}
        warnings={pending.warnings}
        mode="review"
      />
    ),
    footer: (ctl) => (
      <MessageButtons
        buttons={[
          { id: 'cancel', labelKey: 'dialog.cancel' },
          {
            id: 'install',
            labelKey: update ? 'settings.addons.review.update' : 'settings.addons.review.install',
            tone: risky ? 'danger' : 'primary',
          },
        ]}
        focusId="cancel"
        focusRef={cancelRef}
        onPress={(id) => (id === 'install' ? ctl.close(true) : ctl.cancel())}
      />
    ),
  })
  return ok === true
}

function ReviewTitle({ update }: { update: boolean }) {
  const { t } = useTranslation()
  return t(update ? 'settings.addons.review.updateTitle' : 'settings.addons.review.title')
}

// confirmUninstall은 지울지 묻는다. 백엔드가 있는 패키지는 백엔드 데이터도 지울지 고른다.
async function confirmUninstall(dialogs: Dialogs, pkg: AddonPackageStatus, name: string): Promise<{ purge: boolean } | null> {
  const cancelRef = createRef<HTMLButtonElement>()
  const choice = { purge: false }
  return dialogs.open<{ purge: boolean }>({
    role: 'alertdialog',
    title: <UninstallTitle />,
    initialFocus: cancelRef,
    render: () => (
      <UninstallBody
        name={name}
        backend={pkg.permissions?.backend ?? false}
        onPurgeChange={(purge) => {
          choice.purge = purge
        }}
      />
    ),
    footer: (ctl) => (
      <MessageButtons
        buttons={[
          { id: 'cancel', labelKey: 'dialog.cancel' },
          { id: 'uninstall', labelKey: 'settings.addons.uninstall', tone: 'danger' },
        ]}
        focusId="cancel"
        focusRef={cancelRef}
        onPress={(id) => (id === 'uninstall' ? ctl.close({ purge: choice.purge }) : ctl.cancel())}
      />
    ),
  })
}

function UninstallTitle() {
  const { t } = useTranslation()
  return t('settings.addons.uninstallTitle')
}

function UninstallBody({
  name,
  backend,
  onPurgeChange,
}: {
  name: string
  backend: boolean
  onPurgeChange: (purge: boolean) => void
}) {
  const { t } = useTranslation()
  return (
    <>
      <div className="modal-message">{t('settings.addons.uninstallMessage', { name })}</div>
      {backend && (
        <>
          <label className="addons-pkg-check">
            <input type="checkbox" onChange={(e) => onPurgeChange(e.currentTarget.checked)} />
            <span>{t('settings.addons.purge')}</span>
          </label>
          <p className="settings-hint">{t('settings.addons.purgeHint')}</p>
        </>
      )}
    </>
  )
}

// PackageSummary는 패키지 하나의 정보와 권한이다. review는 올린 패키지(설치 전, 업데이트면 차이 포함), details는
// 설치한 패키지다.
function PackageSummary({
  pkg,
  installed,
  diff,
  warnings = [],
  mode,
}: {
  pkg: AddonPackageStatus
  installed?: AddonPackageStatus
  diff?: AddonPermissionsDiff
  warnings?: readonly string[]
  mode: 'review' | 'details'
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const perms = pkg.permissions
  const description = localizedText(pkg.description, lang)
  // 검토 중인 패키지는 아직 아이콘 주소가 없다. 업데이트면 설치된 패키지의 아이콘을 쓰지 않는다(새 패키지의 것이 아니다).
  const icon = mode === 'details' && pkg.icon ? { url: pkg.icon } : ('application-x-addon' as const)
  const network = (v: string) => t(v === 'lan' ? 'settings.addons.network.lan' : v === 'loopback' ? 'settings.addons.network.loopback' : 'settings.addons.network.other', { value: v })

  return (
    <div className="addons-summary">
      <div className="addons-summary-head">
        <IconImage name={icon} size={48} />
        <div className="addons-summary-text">
          <div className="addons-summary-name">
            <span>{packageTitle(pkg, lang)}</span>
            <span className="settings-badge">{t('addonPackages.badge')}</span>
          </div>
          <div className="addons-summary-meta">
            {installed?.version && installed.version !== pkg.version
              ? t('settings.addons.review.versionChange', { from: installed.version, to: pkg.version ?? '' })
              : pkg.version && t('settings.addons.review.version', { version: pkg.version })}
          </div>
          {pkg.author && <div className="addons-summary-meta">{t('settings.addons.review.author', { author: pkg.author })}</div>}
          {pkg.authorUrl && <div className="addons-summary-meta selectable">{pkg.authorUrl}</div>}
        </div>
      </div>
      {description && <p className="addons-summary-desc">{description}</p>}
      {mode === 'review' && <p className="settings-hint">{t('settings.addons.review.authorNote')}</p>}

      {warningKeys(warnings).map((k) => (
        <p key={k} className="addons-summary-warning">
          {t(k)}
        </p>
      ))}

      {mode === 'details' && (pkg.state === 'rejected' || pkg.state === 'invalid') && (
        <p className="addons-summary-warning">
          {t(reasonKey(pkg.reason))}
          {pkg.detail && <span className="addons-pkg-detail selectable">{pkg.detail}</span>}
        </p>
      )}

      {perms && (
        <>
          <h4 className="addons-summary-heading">{t('settings.addons.review.permissions')}</h4>
          {diff && (
            <p className="settings-hint">
              {t(diff.widens ? 'settings.addons.review.widens' : 'settings.addons.review.notWidens')}
            </p>
          )}
          <dl className="addons-perms">
            <PermRow label={t('settings.addons.perm.users')}>
              {t(perms.admin ? 'settings.addons.perm.adminOnly' : 'settings.addons.perm.everyone')}
              {diff?.adminAdded && <Change kind="added" />}
            </PermRow>
            {perms.plugins.length > 0 && (
              <PermRow label={t('settings.addons.perm.plugins')}>
                <PermList items={withChanges(perms.plugins, diff?.addedPlugins)} />
              </PermRow>
            )}
            <PermRow label={t('settings.addons.perm.read')}>
              <PermList items={withChanges(perms.read, diff?.addedRead, diff?.removedRead)} />
            </PermRow>
            {(perms.write.length > 0 || (diff?.removedWrite.length ?? 0) > 0) && (
              <PermRow label={t('settings.addons.perm.write')}>
                <PermList items={withChanges(perms.write, diff?.addedWrite, diff?.removedWrite)} />
              </PermRow>
            )}
            {perms.actions.length > 0 && (
              <PermRow label={t('settings.addons.perm.actions')}>
                <PermList items={withChanges(perms.actions, diff?.addedActions)} />
              </PermRow>
            )}
            <PermRow label={t('settings.addons.perm.backend')}>
              {t(perms.backend ? 'settings.addons.perm.backendYes' : 'settings.addons.perm.backendNo')}
              {diff?.backendAdded && <Change kind="added" />}
            </PermRow>
            {(perms.backend || (diff?.removedNetwork.length ?? 0) > 0) && (
              <PermRow label={t('settings.addons.perm.network')}>
                {perms.network.length === 0 && (diff?.removedNetwork.length ?? 0) === 0 ? (
                  t('settings.addons.network.none')
                ) : (
                  <PermList items={withChanges(perms.network, diff?.addedNetwork, diff?.removedNetwork)} label={network} />
                )}
              </PermRow>
            )}
          </dl>

          {perms.risks.length > 0 && (
            <ul className="addons-risks">
              {riskKeys(perms.risks).map((k) => (
                <li key={k}>{t(k)}</li>
              ))}
            </ul>
          )}
          {perms.write.length > 0 && <p className="settings-hint">{t('settings.addons.review.writeSteps')}</p>}
        </>
      )}

      <dl className="addons-perms">
        {pkg.sha256 && (
          <PermRow label="SHA-256">
            <code className="addons-sha selectable">{pkg.sha256}</code>
          </PermRow>
        )}
        {mode === 'details' && pkg.installedBy && (
          <PermRow label={t('settings.addons.details.installedBy')}>{pkg.installedBy}</PermRow>
        )}
        {mode === 'details' && pkg.installedAt && (
          <PermRow label={t('settings.addons.details.installedAt')}>{formatDateTime(pkg.installedAt, lang)}</PermRow>
        )}
        {mode === 'details' && pkg.updatedAt && pkg.updatedAt !== pkg.installedAt && (
          <PermRow label={t('settings.addons.details.updatedAt')}>{formatDateTime(pkg.updatedAt, lang)}</PermRow>
        )}
        {mode === 'details' && pkg.dev && <PermRow label={t('settings.addons.dev')}>{t('settings.addons.details.devNote')}</PermRow>}
        {mode === 'details' && pkg.backend && (
          <PermRow label={t('settings.addons.backend.state')}>
            {t(
              pkg.backend.state === 'running'
                ? 'settings.addons.backend.running'
                : pkg.backend.state === 'failed'
                  ? 'settings.addons.backend.failed'
                  : 'settings.addons.backend.pendingBadge',
            )}
            {pkg.backend.error && <span className="addons-pkg-detail selectable">{pkg.backend.error}</span>}
          </PermRow>
        )}
      </dl>
    </div>
  )
}

function PermRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt>{label}</dt>
      <dd>{children}</dd>
    </>
  )
}

function PermList({ items, label }: { items: readonly PermissionItem[]; label?: (v: string) => string }) {
  const { t } = useTranslation()
  if (items.length === 0) return t('settings.addons.perm.nothing')
  return (
    <ul className="addons-perm-list">
      {items.map((i) => (
        <li key={i.value} className={i.change}>
          {label ? label(i.value) : <code className="selectable">{i.value}</code>}
          {i.change && <Change kind={i.change} />}
        </li>
      ))}
    </ul>
  )
}

function Change({ kind }: { kind: 'added' | 'removed' }) {
  const { t } = useTranslation()
  return (
    <span className={`addons-perm-change ${kind}`}>
      {t(kind === 'added' ? 'settings.addons.review.added' : 'settings.addons.review.removed')}
    </span>
  )
}
