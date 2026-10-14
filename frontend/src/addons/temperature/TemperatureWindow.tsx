import { useTranslation } from 'react-i18next'
import type { SensorGroup } from '../../api/types.gen'
import { useLanguage } from '../../i18n'
import { formatCelsius, formatDateTime } from '../common/format'
import type { WindowViewProps } from '../types'
import { tempLevel } from './temperature'
import { useTemperature } from './useTemperature'

function GroupCard({ group }: { group: SensorGroup }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const title = group.kind === 'cpu' ? t('temperature.cpu') : group.device || group.name
  return (
    <li className="addon-card">
      <div className="addon-card-head">
        <span className="addon-card-title">{title}</span>
        {group.name !== title && <span className="badge">{group.name}</span>}
      </div>
      {group.checkedAt && (
        <div className="addon-card-sub">
          {t('temperature.checkedAt', { time: formatDateTime(group.checkedAt, lang) })}
        </div>
      )}
      <dl className="temp-sensors">
        {group.sensors.map((s, i) => (
          <div key={i} className="temp-sensor">
            <dt>{s.label || t('temperature.sensor', { n: i + 1 })}</dt>
            <dd className={`level-${tempLevel(s, group.kind)}`}>{formatCelsius(s.celsius, lang)}</dd>
          </div>
        ))}
      </dl>
    </li>
  )
}

// TemperatureWindow는 칩·장치별 온도 센서 전체다.
export function TemperatureWindow({ active }: WindowViewProps) {
  const { t } = useTranslation()
  const { data, status } = useTemperature(active)

  return (
    <div className="addon-window">
      {status === 'disconnected' && (
        <p className="addon-banner" role="status">
          {t('addonWindow.disconnected')}
        </p>
      )}
      {status !== 'disconnected' && data?.failedSources?.length ? (
        <p className="addon-banner notice" role="status">
          {t('addonWindow.partial')}
        </p>
      ) : null}
      {data && data.groups.length === 0 && <p className="addon-empty">{t('temperature.empty')}</p>}
      <ul className={status === 'disconnected' ? 'addon-list stale' : 'addon-list'}>
        {data?.groups.map((g) => <GroupCard key={g.id} group={g} />)}
      </ul>
    </div>
  )
}
