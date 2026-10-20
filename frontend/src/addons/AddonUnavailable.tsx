import { useTranslation } from 'react-i18next'
import type { UnavailableReason } from './capabilities'

interface Props {
  reason: UnavailableReason
  // 플러그인 없음일 때 보여 줄 필요한 플러그인 패키지 이름
  plugins: readonly string[]
  // 위젯처럼 좁은 곳에 쓰는 짧은 안내
  compact?: boolean
}

// AddonUnavailable은 애드온·셸 앱을 쓸 수 없을 때 창·위젯 보기 대신 보여 주는 안내다.
export function AddonUnavailable({ reason, plugins, compact = false }: Props) {
  const { t } = useTranslation()
  let message: string
  if (reason === 'admin_required') message = compact ? t('widgets.adminRequired') : t('addons.adminRequired')
  else if (reason === 'helper_missing') message = t('addons.helperMissing')
  else message = compact ? t('widgets.pluginMissing') : t('addons.pluginMissing')
  return (
    <div className={compact ? 'placeholder compact' : 'placeholder'}>
      <div className="addon-unavailable" role="status">
        <p>{message}</p>
        {reason === 'plugin_missing' && plugins.length > 0 && (
          <p className="selectable">{t('addons.requiredPlugins', { plugins: plugins.join(', ') })}</p>
        )}
        {reason === 'helper_missing' && !compact && <p>{t('addons.helperMissingDetail')}</p>}
      </div>
    </div>
  )
}
