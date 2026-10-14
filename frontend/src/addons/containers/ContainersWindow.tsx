import { useTranslation } from 'react-i18next'
import type { WindowViewProps } from '../types'
import { containerTone, countContainers, sortByAttention } from './containers'
import { stateLabel } from './labels'
import { useContainers } from './useContainers'

// ContainersWindow는 런타임 상태와 컨테이너 전체 목록(이름, 이미지, 상태 원문)이다.
export function ContainersWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const { data, status } = useContainers(active)
  const list = data ? sortByAttention(data.containers) : []
  const counts = countContainers(list)

  return (
    <div className="addon-window">
      {status === 'disconnected' && (
        <p className="addon-banner" role="status">
          {t('addonWindow.disconnected')}
        </p>
      )}
      {data?.runtime === 'podman' && (
        <p className="addon-banner notice" role="status">
          {t('containers.podmanUnverified')}
        </p>
      )}
      {data && (
        <div className="ctr-summary">
          <span className="badge">
            {data.runtime === 'podman' ? 'Podman' : data.runtime === 'docker' ? 'Docker' : t('containers.unknownRuntime')}
            {data.runtimeStatus && ` · ${data.runtimeStatus}`}
          </span>
          <span>{t('containers.running', { count: counts.running })}</span>
          <span className="widget-detail-line">{t('containers.stopped', { count: counts.stopped })}</span>
        </div>
      )}
      {data && list.length === 0 && <p className="addon-empty">{t('containers.empty')}</p>}
      <ul className={status === 'disconnected' ? 'addon-list ctr-list stale' : 'addon-list ctr-list'}>
        {list.map((c) => (
          <li key={c.name} className="ctr-row">
            <span className={`status-dot ${containerTone(c)}`} aria-hidden="true" />
            <div className="ctr-main">
              <div className="ctr-name">{c.name}</div>
              <div className="addon-card-sub selectable">{c.image}</div>
            </div>
            <div className="ctr-state">
              <div>{stateLabel(t, c)}</div>
              <div className="addon-card-sub">{c.status}</div>
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}
