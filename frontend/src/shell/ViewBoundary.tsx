import { Component, type ReactNode, Suspense } from 'react'
import { useTranslation } from 'react-i18next'

interface Props {
  children: ReactNode
  // 위젯처럼 좁은 곳에 쓰는 짧은 안내
  compact?: boolean
}

interface State {
  failed: boolean
}

// ViewBoundary는 창·위젯 보기 하나를 감싼다. 보기 코드는 처음 그릴 때 내려받으므로(addons/registry.ts의
// lazyView) 받는 동안은 내용을 비워 두고, 받지 못하거나 그리다 오류가 나면 그 보기 자리에만 안내를 보인다.
// 경계가 없으면 오류 하나가 React 루트 전체를 내려 바탕화면이 사라진다.
// 업그레이드 뒤 열려 있던 탭은 없어진 옛 청크를 요청해 실패한다. lazy는 실패한 결과를 기억하므로 창을 다시
// 열어도 같은 오류가 나며, 페이지를 새로 고쳐야 새 청크를 받는다.
export class ViewBoundary extends Component<Props, State> {
  state: State = { failed: false }

  static getDerivedStateFromError(): State {
    return { failed: true }
  }

  render() {
    if (this.state.failed) return <ViewFailed compact={this.props.compact ?? false} />
    return <Suspense fallback={null}>{this.props.children}</Suspense>
  }
}

// ViewFailed는 보기를 불러오지 못했을 때의 안내다. 위젯은 틀 전체가 포인터를 잡아(끌기) 안의 단추가 눌리지
// 않고 자리도 좁으므로 짧은 안내만 보인다. 위젯을 누르면 열리는 창에서 새로 고칠 수 있다.
function ViewFailed({ compact }: { compact: boolean }) {
  const { t } = useTranslation()
  return (
    <div className={compact ? 'placeholder compact' : 'placeholder'}>
      <div className="addon-unavailable" role="status">
        <p>{compact ? t('view.loadFailedShort') : t('view.loadFailed')}</p>
        {!compact && (
          <button type="button" className="addon-button" onClick={() => window.location.reload()}>
            {t('view.reload')}
          </button>
        )}
      </div>
    </div>
  )
}
