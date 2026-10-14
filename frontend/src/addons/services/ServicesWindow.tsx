import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Service } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { useContextMenuTrigger } from '../../shell/useContextMenuTrigger'
import { ActionConfirmDialog, RowActionButtons, RowActionPanel } from '../common/RowActions'
import { actionMenuItems } from '../common/actionSpec'
import { isBusy } from '../common/actionState'
import { useAddonActions } from '../common/useAddonActions'
import type { WindowViewProps } from '../types'
import {
  countServices,
  holdServices,
  serviceActions,
  serviceReached,
  serviceState,
  serviceTone,
  sortByAttention,
} from './services'
import { useServices } from './useServices'

// ServicesWindow는 OMV 서비스 전체 목록(사용 설정, 실행 상태)이다. 켜고 끌 수 있는 서비스에는
// 행 버튼과 우클릭 메뉴를 둔다(애드온이 관리자 전용이므로 따로 역할을 보지 않는다).
export function ServicesWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const language = useLanguage()
  const { data, status, refresh } = useServices(active)
  const ctl = useAddonActions('services', refresh)
  // 조작 중인 서비스는 조작 전 상태로 보여 준다. 진행 표시는 행 버튼 자리에 나온다.
  const [held, setHeld] = useState<Readonly<Record<string, Service>>>({})
  const nextHeld = holdServices(held, data?.services ?? [], (name) => isBusy(ctl.state, name))
  if (nextHeld !== held) setHeld(nextHeld)
  const list = data ? sortByAttention(data.services.map((s) => nextHeld[s.name] ?? s), language) : []
  const counts = countServices(list)

  // 새 목록에서 설정과 실제 실행이 모두 바뀐 서비스만 잠금을 푼다. 설정만 바뀐 중간 상태에서는 기다린다.
  const { settle } = ctl
  useEffect(() => {
    settle((name, action) => {
      const s = data?.services.find((x) => x.name === name)
      return !s || serviceReached(s, action)
    })
  }, [data, settle])

  const find = (name: string) => data?.services.find((s) => s.name === name)
  const menu = useContextMenuTrigger({
    items: (el) => {
      const s = find(el.dataset.target ?? '')
      if (!s) return []
      return actionMenuItems(ctl.state, s.name, serviceActions(s), {
        select: (spec) => ctl.request(s.name, spec.action),
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
      {data && (
        <div className="ctr-summary">
          <span>{t('services.running', { count: counts.running })}</span>
          {counts.attention > 0 && <span>{t('services.attention', { count: counts.attention })}</span>}
        </div>
      )}
      {data && list.length === 0 && <p className="addon-empty">{t('services.empty')}</p>}
      <ul className={status === 'disconnected' ? 'addon-list ctr-list stale' : 'addon-list ctr-list'}>
        {list.map((s) => {
          const specs = serviceActions(s)
          return (
            <li key={s.name} className="ctr-row svc-row" data-target={s.name} {...menu.handlers}>
              <span className={`status-dot ${serviceTone(s)}`} aria-hidden="true" />
              <div className="ctr-main">
                <div className="ctr-name">{s.title}</div>
              </div>
              <div className="ctr-state">{t(`services.state.${serviceState(s)}`)}</div>
              {s.toggleable ? (
                <RowActionButtons ctl={ctl} target={s.name} specs={specs} label={s.title} />
              ) : (
                <span className="row-actions" aria-hidden="true" />
              )}
              <RowActionPanel ctl={ctl} target={s.name} specs={specs} label={s.title} />
            </li>
          )
        })}
      </ul>
      {list.some((s) => !s.toggleable) && <p className="addon-hint svc-hint">{t('services.toggleHint')}</p>}
      <ActionConfirmDialog
        ctl={ctl}
        lookup={(name) => {
          const s = find(name)
          return s ? { specs: serviceActions(s), label: s.title } : null
        }}
      />
    </div>
  )
}
