import type { ParseKeys } from 'i18next'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { useContextMenuTrigger } from '../../shell/useContextMenuTrigger'
import { formatBytes, formatPercent } from '../common/format'
import { processMenuItems } from './menus'
import {
  DEFAULT_PROCESS_SORT,
  type ProcessSort,
  type ProcessSortKey,
  filterProcesses,
  nextSort,
  processKey,
  scaleCpu,
  sortProcesses,
  stateKey,
} from './processes'
import { useSysinfoPrefsStore } from './sysinfoPrefsStore'
import type { SystemStatsSample } from './useSystemStats'

interface Props {
  sample: SystemStatsSample | null
  stale: boolean
}

const COLUMNS: { key: ProcessSortKey; labelKey: ParseKeys; numeric?: boolean }[] = [
  { key: 'name', labelKey: 'sysinfo.processes.columns.name' },
  { key: 'pid', labelKey: 'sysinfo.processes.columns.pid', numeric: true },
  { key: 'user', labelKey: 'sysinfo.processes.columns.user' },
  { key: 'state', labelKey: 'sysinfo.processes.columns.state' },
  { key: 'cpu', labelKey: 'sysinfo.processes.columns.cpu', numeric: true },
  { key: 'memory', labelKey: 'sysinfo.processes.columns.memory', numeric: true },
]

// ProcessesPanel은 프로세스 탭이다(관리자만). 검색·커널 스레드 표시·열 정렬은 창을 닫으면 처음으로 돌아가고,
// CPU 사용률 기준(우클릭 메뉴)은 사용자 설정으로 저장한다.
export function ProcessesPanel({ sample, stale }: Props) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const [query, setQuery] = useState('')
  const [showKernel, setShowKernel] = useState(false)
  const [sort, setSort] = useState<ProcessSort>(DEFAULT_PROCESS_SORT)
  const cpuScale = useSysinfoPrefsStore((s) => s.cpuScale)
  const menu = useContextMenuTrigger({
    items: () => {
      const s = useSysinfoPrefsStore.getState()
      return processMenuItems(s.cpuScale, (scale) => s.set({ cpuScale: scale }), s.status !== 'ready')
    },
  })

  const res = sample?.response
  const list = res?.processes
  const cpu = sample?.processCpu
  const cores = res?.cpu.cores.length ?? 1
  const collator = useMemo(() => new Intl.Collator(lang, { sensitivity: 'base', numeric: true }), [lang])
  const rows = useMemo(
    () => (list && cpu ? sortProcesses(filterProcesses(list, query, showKernel), sort, cpu, collator.compare) : []),
    [list, cpu, query, showKernel, sort, collator],
  )
  const failed = res?.failedSources?.includes('proc-processes') === true

  let notice: string | null = null
  if (!res) notice = t('sysinfo.processes.loading')
  else if (!list) notice = failed ? t('sysinfo.processes.failed') : t('sysinfo.processes.loading')
  else if (rows.length === 0) notice = t('sysinfo.processes.empty')

  return (
    <div className="sysinfo-processes">
      <div className="sysinfo-toolbar">
        <input
          type="search"
          className="sysinfo-search"
          value={query}
          placeholder={t('sysinfo.processes.search')}
          aria-label={t('sysinfo.processes.search')}
          onChange={(e) => setQuery(e.target.value)}
        />
        <label className="sysinfo-check">
          <input type="checkbox" checked={showKernel} onChange={(e) => setShowKernel(e.target.checked)} />
          {t('sysinfo.processes.showKernel')}
        </label>
        {list && <span className="sysinfo-count">{t('sysinfo.processes.count', { count: rows.length })}</span>}
      </div>
      <div className={stale ? 'sysinfo-table-wrap stale' : 'sysinfo-table-wrap'} {...menu.handlers}>
        <table className="sysinfo-table">
          <thead>
            <tr>
              {COLUMNS.map((c) => {
                const active = sort.key === c.key
                return (
                  <th
                    key={c.key}
                    className={`col-${c.key}${c.numeric ? ' num' : ''}`}
                    aria-sort={active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : undefined}
                  >
                    <button type="button" onClick={() => setSort(nextSort(sort, c.key))}>
                      <span>{t(c.labelKey)}</span>
                      <span className="sysinfo-sort" aria-hidden="true">
                        {active ? (sort.dir === 'asc' ? '▲' : '▼') : ''}
                      </span>
                    </button>
                  </th>
                )
              })}
            </tr>
          </thead>
          <tbody>
            {rows.map((p) => {
              const key = processKey(p)
              const v = cpu?.get(key)
              const state = stateKey(p.state)
              return (
                <tr key={key}>
                  <td className="col-name">
                    <div className="sysinfo-proc-name">{p.name}</div>
                    {p.command && (
                      <div className="sysinfo-proc-cmd selectable" title={p.command}>
                        {p.command}
                      </div>
                    )}
                  </td>
                  <td className="col-pid num">{p.pid}</td>
                  <td className="col-user">{p.user}</td>
                  <td className="col-state">{state ? t(state) : p.state}</td>
                  <td className="col-cpu num">{v === undefined ? '—' : formatPercent(scaleCpu(v, cpuScale, cores), lang)}</td>
                  <td className="col-memory num">{p.kernel ? '—' : formatBytes(p.rssBytes, lang)}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {notice && <p className="sysinfo-table-notice">{notice}</p>}
      </div>
    </div>
  )
}
