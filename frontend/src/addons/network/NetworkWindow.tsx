import { useTranslation } from 'react-i18next'
import type { NetInterface } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { formatBytes, formatLinkSpeed, formatRate } from '../common/format'
import type { WindowViewProps } from '../types'
import { type Rate, isUp } from './network'
import { useNetwork } from './useNetwork'

function InterfaceCard({ iface, rate }: { iface: NetInterface; rate: Rate | undefined }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const up = isUp(iface)
  const errors = iface.rxErrors + iface.txErrors
  const dropped = iface.rxDropped + iface.txDropped
  return (
    <li className="addon-card">
      <div className="addon-card-head">
        <span className="addon-card-title">{iface.name}</span>
        <span className={up ? 'badge' : 'badge level-warn'}>{up ? t('network.up') : t('network.down')}</span>
        {iface.speedMbps && <span className="badge">{formatLinkSpeed(iface.speedMbps, lang)}</span>}
      </div>
      {up && (
        <div className="net-rates net-rates-large">
          <span>
            {t('network.rx')} <strong>{rate ? formatRate(rate.rx, lang) : '—'}</strong>
          </span>
          <span>
            {t('network.tx')} <strong>{rate ? formatRate(rate.tx, lang) : '—'}</strong>
          </span>
        </div>
      )}
      <div className="addon-card-sub">
        {t('network.totals', { rx: formatBytes(iface.rxBytes, lang), tx: formatBytes(iface.txBytes, lang) })}
      </div>
      {(errors > 0 || dropped > 0) && (
        <div className="addon-card-sub">{t('network.errors', { errors, dropped })}</div>
      )}
    </li>
  )
}

// NetworkWindow는 물리 인터페이스의 상태·속도·누적 전송량과, 접어 둔 가상 인터페이스 목록이다.
export function NetworkWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const { data, status } = useNetwork(active)
  const list = data?.response.interfaces ?? []
  const physical = list.filter((i) => !i.virtual)
  const virtual = list.filter((i) => i.virtual)
  const listClass = status === 'disconnected' ? 'addon-list stale' : 'addon-list'

  return (
    <div className="addon-window">
      {status === 'disconnected' && (
        <p className="addon-banner" role="status">
          {t('addonWindow.disconnected')}
        </p>
      )}
      {data && physical.length === 0 && <p className="addon-empty">{t('network.empty')}</p>}
      <ul className={listClass}>
        {physical.map((i) => (
          <InterfaceCard key={i.name} iface={i} rate={data?.rates.get(i.name)} />
        ))}
      </ul>
      {virtual.length > 0 && (
        <details className="addon-details">
          <summary>{t('network.virtual', { count: virtual.length })}</summary>
          <ul className={listClass}>
            {virtual.map((i) => (
              <InterfaceCard key={i.name} iface={i} rate={data?.rates.get(i.name)} />
            ))}
          </ul>
        </details>
      )}
    </div>
  )
}
