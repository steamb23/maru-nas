import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useLanguage } from '../../i18n'
import { useContextMenuTrigger } from '../../shell/useContextMenuTrigger'
import { ActionConfirmDialog, RowActionButtons, RowActionPanel } from '../common/RowActions'
import { actionMenuItems } from '../common/actionSpec'
import { useAddonActions } from '../common/useAddonActions'
import type { WindowViewProps } from '../types'
import { countServices, serviceActions, serviceState, serviceTone, sortByAttention } from './services'
import { useServices } from './useServices'

// ServicesWindow는 OMV 서비스 전체 목록(사용 설정, 실행 상태)이다. 켜고 끌 수 있는 서비스에는
// 행 버튼과 우클릭 메뉴를 둔다(애드온이 관리자 전용이므로 따로 역할을 보지 않는다).
export function ServicesWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const language = useLanguage()
  const { data, status, refresh } = useServices(active)
  const ctl = useAddonActions('services', refresh)
  const list = data ? sortByAttention(data.services, language) : []
  const counts = countServices(list)

  // 새 목록을 받으면 성공한 조작의 잠금을 푼다(그 전까지는 옛 상태가 보인다).
  const { settle } = ctl
  useEffect(() => settle(), [data, settle])

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
