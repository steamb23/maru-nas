import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import type { Container } from '../../api/types.gen'
import { useAuthStore } from '../../auth/store'
import { useContextMenuTrigger } from '../../shell/useContextMenuTrigger'
import { ActionConfirmDialog, RowActionButtons, RowActionPanel } from '../common/RowActions'
import { actionMenuItems } from '../common/actionSpec'
import { useAddonActions } from '../common/useAddonActions'
import type { WindowViewProps } from '../types'
import { CONTAINER_ACTIONS, containerActions, containerTone, countContainers, sortByAttention } from './containers'
import { stateLabel } from './labels'
import { useContainers } from './useContainers'

const specsFor = (c: Container) => containerActions(c).map((a) => CONTAINER_ACTIONS[a])

// ContainersWindow는 런타임 상태와 컨테이너 전체 목록(이름, 이미지, 상태 원문)이다.
// 관리자에게는 행마다 시작·정지·재시작 버튼과 우클릭 메뉴를 보여 준다.
export function ContainersWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const { data, status, refresh } = useContainers(active)
  const admin = useAuthStore((s) => s.user?.admin === true)
  const ctl = useAddonActions('containers', refresh)
  const list = data ? sortByAttention(data.containers) : []
  const counts = countContainers(list)

  // 새 목록을 받으면 성공한 조작의 잠금을 푼다(그 전까지는 옛 상태가 보인다).
  const { settle } = ctl
  useEffect(() => settle(), [data, settle])

  const find = (name: string) => data?.containers.find((c) => c.name === name)
  const menu = useContextMenuTrigger({
    items: (el) => {
      const c = admin ? find(el.dataset.target ?? '') : undefined
      if (!c) return []
      return actionMenuItems(ctl.state, c.name, specsFor(c), {
        select: (spec) => ctl.request(c.name, spec.action),
      })
    },
  })

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
        {list.map((c) => {
          const specs = specsFor(c)
          return (
            <li key={c.name} className="ctr-row" data-target={c.name} {...(admin ? menu.handlers : {})}>
              <span className={`status-dot ${containerTone(c)}`} aria-hidden="true" />
              <div className="ctr-main">
                <div className="ctr-name">{c.name}</div>
                <div className="addon-card-sub selectable">{c.image}</div>
              </div>
              <div className="ctr-state">
                <div>{stateLabel(t, c)}</div>
                <div className="addon-card-sub">{c.status}</div>
              </div>
              {admin && <RowActionButtons ctl={ctl} target={c.name} specs={specs} label={c.name} />}
              {admin && <RowActionPanel ctl={ctl} target={c.name} specs={specs} label={c.name} />}
            </li>
          )
        })}
      </ul>
      {admin && (
        <ActionConfirmDialog
          ctl={ctl}
          lookup={(name) => {
            const c = find(name)
            return c ? { specs: specsFor(c), label: c.name } : null
          }}
        />
      )}
    </div>
  )
}
