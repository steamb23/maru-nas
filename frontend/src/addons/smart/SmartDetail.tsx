import { type KeyboardEvent, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { SmartAttribute, SmartDetail, SmartDisk, SmartField, SmartRaw, SmartSelfTest } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { IconImage } from '../../icons/IconImage'
import { CopyButton } from '../../shell/CopyButton'
import { openWorkbenchRoute } from '../../shell/launchApp'
import { formatBytes, formatCelsius, formatCount, formatDateTime, formatPercent, formatUnit } from '../common/format'
import type { Level } from '../common/levels'
import {
  type DetailProblem,
  type DetailTab,
  type Tile,
  SMART_TASKS_ROUTE,
  attributeLevel,
  detailProblem,
  detailTabs,
  healthGroups,
  healthLevel,
  isAttrName,
  isHealthKey,
  isInfoKey,
  mediaBadges,
  overviewTiles,
  problemCount,
  selfAssessmentKey,
  smartDeviceRoute,
  smartctlName,
  statusBanner,
  testLevel,
} from './detail'
import { statusLevel } from './smart'
import { type Load, useSmartRaw } from './useSmartDetail'

interface Props {
  disk: SmartDisk
  state: Load<SmartDetail>
  onReload: () => void
  // onBack이 있으면(좁은 창) 디스크 목록으로 돌아가는 단추를 보인다.
  onBack?: () => void
  // onRefreshList는 목록을 다시 받는다(디스크가 사라졌을 때). 지금 받을 수 없으면 없다.
  onRefreshList?: () => void
  workbench: boolean
}

// 점 색 클래스(status-dot). Level의 normal은 정상 초록이다.
const dot = (level: Level | 'idle') => `status-dot ${level}`
// 글자 색 클래스. 정상은 기본 색이다(목록의 디스크 카드와 같다).
const tone = (level: Level) => (level === 'normal' ? '' : ` level-${level}`)

// SmartDetailView는 고른 디스크 하나의 SMART 상세다(머리글, 탭, 탭 내용). 디스크마다 다시 마운트한다(key).
export function SmartDetailView({ disk, state, onReload, onBack, onRefreshList, workbench }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const id = useId()
  const detail = state.status === 'ready' ? state.data : null
  const tabs = detailTabs(detail?.kind)
  const [tab, setTab] = useState<DetailTab>('overview')
  const current = tabs.includes(tab) ? tab : 'overview'
  const raw = useSmartRaw(disk.device, current === 'raw', detail)
  const tabRefs = useRef(new Map<DetailTab, HTMLButtonElement>())
  const level = statusLevel(disk.status)

  const pick = (next: DetailTab) => {
    setTab(next)
    tabRefs.current.get(next)?.focus()
  }
  const onTabKey = (e: KeyboardEvent) => {
    const i = tabs.indexOf(current)
    let next: DetailTab | undefined
    if (e.key === 'ArrowRight') next = tabs[(i + 1) % tabs.length]
    else if (e.key === 'ArrowLeft') next = tabs[(i - 1 + tabs.length) % tabs.length]
    else if (e.key === 'Home') next = tabs[0]
    else if (e.key === 'End') next = tabs[tabs.length - 1]
    if (!next) return
    e.preventDefault()
    pick(next)
  }

  return (
    <>
      <header className="smart-detail-head">
        {onBack && (
          <button type="button" className="addon-button smart-back" onClick={onBack}>
            <IconImage name="go-previous-symbolic" size={14} />
            {t('smart.detail.back')}
          </button>
        )}
        <div className="smart-detail-title">
          <span className={dot(level)} aria-hidden="true" />
          <h2 className="selectable">{disk.device}</h2>
          <span className={`badge${tone(level)}`}>{t(`smart.status.${disk.status}`)}</span>
          {detail && mediaBadges(detail).map((b) => (
            <span key={b} className="badge" translate="no">
              {b}
            </span>
          ))}
          <span className="smart-spacer" />
          {workbench && detail && (
            <button type="button" className="smart-link" onClick={() => openWorkbenchRoute(smartDeviceRoute(detail.deviceFile))}>
              {t('smart.detail.workbench')}
              <IconImage name="external-link-symbolic" size={14} />
            </button>
          )}
          <button type="button" className="addon-button smart-reload" onClick={onReload} disabled={state.status === 'loading'}>
            <IconImage name="view-refresh-symbolic" size={14} />
            {t('smart.detail.reload')}
          </button>
        </div>
        <div className="smart-detail-sub">
          {disk.model}
          {disk.sizeBytes > 0 && ` · ${formatBytes(disk.sizeBytes, lang)}`}
          {detail && ` · ${t('smart.detail.readAt', { time: formatDateTime(detail.readAt, lang) })}`}
        </div>
      </header>
      {detail && (
        <div className="smart-tabs" role="tablist" aria-label={t('smart.detail.tabs.label')} onKeyDown={onTabKey}>
          {tabs.map((tb) => (
            <button
              key={tb}
              ref={(el) => {
                if (el) tabRefs.current.set(tb, el)
                else tabRefs.current.delete(tb)
              }}
              type="button"
              role="tab"
              id={`${id}-tab-${tb}`}
              className="smart-tab"
              aria-selected={tb === current}
              aria-controls={`${id}-panel`}
              tabIndex={tb === current ? 0 : -1}
              onClick={() => setTab(tb)}
            >
              {t(`smart.detail.tabs.${tb}`)}
            </button>
          ))}
        </div>
      )}
      <div
        className="smart-panel"
        role={detail ? 'tabpanel' : undefined}
        id={`${id}-panel`}
        aria-labelledby={detail ? `${id}-tab-${current}` : undefined}
      >
        {state.status === 'loading' && <LoadingView />}
        {state.status === 'error' && (
          <ProblemView problem={detailProblem(state.err)} onRetry={onReload} onRefreshList={onRefreshList} />
        )}
        {detail && current === 'overview' && <Overview disk={disk} detail={detail} />}
        {detail && current === 'attributes' && <Attributes attrs={detail.attributes} />}
        {detail && current === 'health' && <Health detail={detail} />}
        {detail && current === 'selfTests' && <SelfTests detail={detail} workbench={workbench} onShowRaw={() => pick('raw')} />}
        {detail && current === 'raw' && <Raw state={raw.state} onRetry={raw.retry} />}
      </div>
    </>
  )
}

function LoadingView({ raw = false }: { raw?: boolean }) {
  const { t } = useTranslation()
  return (
    <div className="smart-state" role="status">
      <span className="smart-spinner" aria-hidden="true" />
      <p className="smart-state-title">{raw ? t('smart.detail.raw.loading') : t('smart.detail.loading')}</p>
      {!raw && <p className="smart-state-text">{t('smart.detail.loadingHint')}</p>}
    </div>
  )
}

function ProblemView({ problem, onRetry, onRefreshList }: { problem: DetailProblem; onRetry: () => void; onRefreshList?: () => void }) {
  const { t } = useTranslation()
  if (problem === 'unsupported') {
    return (
      <div className="smart-state">
        <p className="smart-state-title">{t('smart.detail.unsupported')}</p>
        <p className="smart-state-text">{t('smart.detail.unsupportedHint')}</p>
      </div>
    )
  }
  if (problem === 'gone') {
    return (
      <div className="smart-state">
        <p className="smart-state-title">{t('smart.detail.gone')}</p>
        <p className="smart-state-text">{t('smart.detail.goneHint')}</p>
        {onRefreshList && (
          <button type="button" className="addon-button" onClick={onRefreshList}>
            {t('smart.refresh')}
          </button>
        )}
      </div>
    )
  }
  return (
    <div className="smart-state" role="alert">
      <p className="smart-state-title">{problem === 'timeout' ? t('smart.detail.timeout') : t('smart.detail.failed')}</p>
      <button type="button" className="addon-button" onClick={onRetry}>
        {t('smart.detail.retry')}
      </button>
    </div>
  )
}

// fieldText는 장치 정보·건강 정보 값 하나의 표시 문구다. 읽은 숫자가 있으면 단위에 맞게, 없으면 smartctl 값 그대로다.
function fieldText(f: SmartField, lang: string): string {
  if (f.num === undefined) return f.value
  switch (f.unit) {
    case 'percent':
      return formatPercent(f.num, lang, 0)
    case 'celsius':
      return formatCelsius(f.num, lang)
    case 'bytes':
      return formatBytes(f.num, lang)
    case 'hours':
      return formatUnit(f.num, 'hour', lang)
    case 'minutes':
      return formatUnit(f.num, 'minute', lang)
    case 'count':
      return formatCount(f.num, lang)
  }
  return f.value
}

function tileText(tile: Tile, lang: string): string {
  if (tile.value === undefined) return '–'
  switch (tile.unit) {
    case 'celsius':
      return formatCelsius(tile.value, lang)
    case 'hours':
      return formatUnit(tile.value, 'hour', lang)
    case 'percent':
      return formatPercent(tile.value, lang, 0)
  }
  return formatCount(tile.value, lang)
}

function Overview({ disk, detail }: { disk: SmartDisk; detail: SmartDetail }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const banner = statusBanner(disk)
  const tiles = overviewTiles(detail)
  const sub = (tile: Tile): string | null => {
    const s = tile.sub
    if (!s) return null
    switch (s.kind) {
      case 'minMax':
        return t('smart.detail.tileSub.minMax', { min: formatCelsius(s.min, lang), max: formatCelsius(s.max, lang) })
      case 'attribute':
        return t('smart.detail.tileSub.attribute', { id: s.id })
      case 'unsafeShutdowns':
        return t('smart.detail.tileSub.unsafeShutdowns', { value: formatCount(s.value, lang) })
      case 'threshold':
        return t('smart.detail.tileSub.threshold', { value: formatPercent(s.value, lang, 0) })
      case 'errorLog':
        return t('smart.detail.tileSub.errorLog', { value: formatCount(s.value, lang) })
      case 'sensorMax':
        return t('smart.detail.tileSub.sensorMax', { value: formatCelsius(s.value, lang) })
    }
  }
  return (
    <div className="smart-overview">
      {banner && (
        <p className={banner.level === 'crit' ? 'addon-banner' : 'addon-banner notice'}>{t(`smart.detail.banner.${banner.key}`)}</p>
      )}
      <ul className="smart-tiles">
        {tiles.map((tile) => (
          <li key={tile.id} className="smart-tile">
            <span className="smart-tile-label">{t(`smart.detail.tiles.${tile.id}`)}</span>
            <span className={`smart-tile-value${tone(tile.level)}`}>
              {tileText(tile, lang)}
            </span>
            {tile.meter !== undefined && (
              <span className="meter" aria-hidden="true">
                <span className={`meter-fill${tile.level === 'normal' ? '' : ` ${tile.level}`}`} style={{ width: `${tile.meter}%` }} />
              </span>
            )}
            {sub(tile) && <span className="smart-tile-sub">{sub(tile)}</span>}
          </li>
        ))}
      </ul>
      <h3 className="addon-section-title">{t('smart.detail.info')}</h3>
      <dl className="smart-kv">
        {detail.info.map((f) => (
          <div key={f.key} className="smart-kv-row">
            <dt>{isInfoKey(f.key) ? t(`smart.info.${f.key}`) : f.key}</dt>
            <dd>
              <InfoValue field={f} />
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function InfoValue({ field }: { field: SmartField }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  if (field.key === 'serial') {
    return (
      <>
        <span className="smart-mono selectable" translate="no">
          {field.value}
        </span>
        <CopyButton text={field.value} label={t('smart.detail.copySerial')} />
      </>
    )
  }
  if (field.key === 'selfAssessment') {
    const k = selfAssessmentKey(field.value)
    if (k) return <span className={k === 'passed' ? 'level-ok' : 'level-crit'}>{t(`smart.detail.selfAssessment.${k}`)}</span>
  }
  if (field.key === 'capacity' && field.num !== undefined) {
    return (
      <span>
        {formatBytes(field.num, lang)} <span className="smart-muted">({t('smart.detail.bytes', { value: formatCount(field.num, lang) })})</span>
      </span>
    )
  }
  const text = fieldText(field, lang)
  return (
    <span className={field.num === undefined ? 'selectable' : undefined} translate={field.num === undefined ? 'no' : undefined}>
      {text}
    </span>
  )
}

function Attributes({ attrs }: { attrs: SmartAttribute[] }) {
  const { t } = useTranslation()
  const [onlyProblems, setOnlyProblems] = useState(false)
  const problems = problemCount(attrs)
  const rows = onlyProblems ? attrs.filter((a) => attributeLevel(a) !== 'normal') : attrs
  return (
    <div className="smart-attrs">
      <div className="smart-toolbar">
        <span className="addon-card-sub">
          {t('smart.detail.attributes.count', { count: attrs.length })}
          {problems > 0 && ` · ${t('smart.detail.attributes.problems', { count: problems })}`}
        </span>
        <label className="smart-check">
          <input type="checkbox" checked={onlyProblems} onChange={(e) => setOnlyProblems(e.target.checked)} />
          {t('smart.detail.attributes.onlyProblems')}
        </label>
      </div>
      <p className="smart-muted smart-legend">{t('smart.detail.attributes.legend')}</p>
      {rows.length === 0 ? (
        <p className="addon-empty">{t('smart.detail.attributes.noProblems')}</p>
      ) : (
        <div className="smart-table-wrap">
          <table className="smart-table">
            <thead>
              <tr>
                <th className="num">{t('smart.detail.attributes.id')}</th>
                <th>{t('smart.detail.attributes.name')}</th>
                <th className="num">{t('smart.detail.attributes.value')}</th>
                <th className="num">{t('smart.detail.attributes.worst')}</th>
                <th className="num">{t('smart.detail.attributes.threshold')}</th>
                <th className="num">{t('smart.detail.attributes.raw')}</th>
                <th>{t('smart.detail.attributes.status')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((a) => (
                <AttributeRow key={a.id} attr={a} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function AttributeRow({ attr }: { attr: SmartAttribute }) {
  const { t } = useTranslation()
  const level = attributeLevel(attr)
  const status =
    attr.status === undefined && level === 'normal' ? null : level === 'normal' ? t('smart.status.good') : level === 'crit' ? t('smart.status.failing') : t('smart.status.warning')
  return (
    <tr className={level === 'normal' ? undefined : level}>
      <td className="num">{attr.id}</td>
      <td>
        <span className="smart-attr-name">
          {isAttrName(attr.name) ? t(`smart.attr.${attr.name}`) : attr.name}
          {attr.prefailure && <span className="smart-prefail">{t('smart.detail.attributes.prefailure')}</span>}
        </span>
        {isAttrName(attr.name) && (
          <span className="smart-en smart-mono" translate="no">
            {attr.name}
          </span>
        )}
      </td>
      <td className="num">{attr.value}</td>
      <td className="num">{attr.worst}</td>
      <td className="num">{attr.threshold}</td>
      <td className="num smart-mono selectable" translate="no">
        {attr.raw}
      </td>
      <td>
        {status === null ? (
          <span className="smart-muted" title={t('smart.detail.attributes.notAssessed')} aria-label={t('smart.detail.attributes.notAssessed')}>
            –
          </span>
        ) : (
          <span className={`smart-status${tone(level)}`}>
            <span className={dot(level)} aria-hidden="true" />
            {status}
          </span>
        )}
      </td>
    </tr>
  )
}

function Health({ detail }: { detail: SmartDetail }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  return (
    <div className="smart-health">
      <p className="smart-muted smart-legend">{t('smart.detail.health.intro')}</p>
      <div className="smart-table-wrap">
        <table className="smart-table smart-health-table">
          <thead>
            <tr>
              <th>{t('smart.detail.health.item')}</th>
              <th>{t('smart.detail.health.value')}</th>
            </tr>
          </thead>
          {healthGroups(detail.health).map((g) => (
            <tbody key={g.id}>
              <tr className="smart-group">
                <th scope="colgroup" colSpan={2}>
                  {t(`smart.detail.health.groups.${g.id}`)}
                </th>
              </tr>
              {g.fields.map((f) => {
                const level = healthLevel(f, detail)
                const text =
                  f.key === 'criticalWarning' && f.num === 0 ? t('smart.detail.health.noWarning') : fieldText(f, lang)
                return (
                  <tr key={`${f.key}${f.n ?? ''}`}>
                    <td>
                      {isHealthKey(f.key) ? t(`smart.health.${f.key}`, { n: f.n }) : f.key}
                      <span className="smart-en smart-mono" translate="no">
                        {smartctlName(f)}
                      </span>
                    </td>
                    <td>
                      <span className="smart-status wrap">
                        {level && <span className={dot(level)} aria-hidden="true" />}
                        {text}
                        {(f.unit === 'hex' || (f.unit === 'bytes' && f.key !== 'capacity')) && (
                          <span className="smart-muted smart-mono" translate="no">
                            {f.value}
                          </span>
                        )}
                      </span>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          ))}
        </table>
      </div>
    </div>
  )
}

function SelfTests({ detail, workbench, onShowRaw }: { detail: SmartDetail; workbench: boolean; onShowRaw: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="smart-selftests">
      {detail.selfTestStatus && (
        <p className="addon-banner notice">{t('smart.detail.selfTests.running', { status: detail.selfTestStatus })}</p>
      )}
      {detail.selfTestsUnreadable ? (
        <div className="smart-info">
          <p>{t('smart.detail.selfTests.unreadable')}</p>
          <button type="button" className="addon-button" onClick={onShowRaw}>
            {t('smart.detail.selfTests.showRaw')}
          </button>
        </div>
      ) : detail.selfTests.length === 0 ? (
        <p className="addon-empty">{t('smart.detail.selfTests.empty')}</p>
      ) : (
        <div className="smart-table-wrap">
          <table className="smart-table">
            <thead>
              <tr>
                <th className="num">{t('smart.detail.selfTests.num')}</th>
                <th>{t('smart.detail.selfTests.type')}</th>
                <th>{t('smart.detail.selfTests.result')}</th>
                {detail.kind === 'ata' && <th className="num">{t('smart.detail.selfTests.remaining')}</th>}
                <th className="num">{t('smart.detail.selfTests.powerOnHours')}</th>
                <th className="num">{t('smart.detail.selfTests.failingLba')}</th>
              </tr>
            </thead>
            <tbody>
              {detail.selfTests.map((s) => (
                <SelfTestRow key={s.num} test={s} ata={detail.kind === 'ata'} />
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="smart-foot">
        <span className="smart-muted">{t('smart.detail.selfTests.scheduleHint')}</span>
        {workbench && (
          <button type="button" className="smart-link" onClick={() => openWorkbenchRoute(SMART_TASKS_ROUTE)}>
            {t('smart.detail.selfTests.schedule')}
            <IconImage name="external-link-symbolic" size={14} />
          </button>
        )}
      </div>
    </div>
  )
}

function SelfTestRow({ test, ata }: { test: SmartSelfTest; ata: boolean }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const level = testLevel(test.result)
  return (
    <tr className={level === 'crit' ? 'crit' : undefined}>
      <td className="num">{test.num}</td>
      <td>
        {test.type === 'other' ? test.typeText : t(`smart.detail.selfTests.types.${test.type}`)}
        <span className="smart-en smart-mono" translate="no">
          {test.typeText}
        </span>
      </td>
      <td>
        <span className={`smart-status${level === 'crit' ? ' level-crit' : ''}`}>
          <span className={dot(level)} aria-hidden="true" />
          {test.result === 'other' ? test.statusText : t(`smart.detail.selfTests.results.${test.result}`)}
        </span>
        {test.result !== 'ok' && test.result !== 'other' && (
          <span className="smart-en smart-mono" translate="no">
            {test.statusText}
          </span>
        )}
      </td>
      {ata && <td className="num">{test.remaining === undefined ? '–' : formatPercent(test.remaining, lang, 0)}</td>}
      <td className="num">{formatUnit(test.powerOnHours, 'hour', lang)}</td>
      <td className="num smart-mono selectable">{test.failingLba ?? '–'}</td>
    </tr>
  )
}

function Raw({ state, onRetry }: { state: Load<SmartRaw>; onRetry: () => void }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  if (state.status === 'loading') return <LoadingView raw />
  if (state.status === 'error') return <ProblemView problem={detailProblem(state.err)} onRetry={onRetry} />
  const raw = state.data
  return (
    <div className="smart-raw">
      <div className="smart-toolbar">
        <span className="smart-muted">
          {t('smart.detail.raw.note')} {t('smart.detail.readAt', { time: formatDateTime(raw.readAt, lang) })}
        </span>
        <CopyButton text={raw.text} label={t('smart.detail.raw.copy')} />
      </div>
      {raw.truncated && <p className="addon-banner notice">{t('smart.detail.raw.truncated')}</p>}
      <pre className="smart-raw-text selectable" lang="en" translate="no" tabIndex={0}>
        {raw.text}
      </pre>
    </div>
  )
}
