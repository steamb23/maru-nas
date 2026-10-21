// 미리보기 창. 파일 탐색기가 고른 파일(viewerStore.ts)을 종류대로 보인다: 이미지(<img>), 동영상·오디오(<video>,
// <audio>), PDF(브라우저 내장 뷰어의 <iframe>), 텍스트(앞 1MiB). 내용은 백엔드 미리보기(/api/files/preview)가 그 형식으로
// 보낸다. 미디어 요소는 오류 본문을 보여 주지 않으므로 먼저 정보를 받아 세션·보안 확인·읽기 권한을 확인한다.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, type RequiredLevel, fetchFileInfo, fetchPreviewBytes, filePreviewUrl } from '../api/client'
import type { FileInfo } from '../api/types.gen'
import { formatBytes } from '../addons/common/format'
import { reportActivity } from '../auth/activity'
import { startDownload } from '../files/download'
import { KIND_KEYS, kindIcon, kindOf } from '../files/kinds'
import { errorKey } from '../files/messages'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import { requestStepUp } from '../security/stepUpDialog'
import { useDialogs } from '../shell/dialogPresets'
import { type DecodedText, TEXT_LIMIT, decodeText } from './text'
import { type ViewerTarget, useViewerStore } from './viewerStore'

// ACTIVITY_MS는 동영상·오디오를 재생하는 동안 세션 활동을 알리는 간격이다(입력 없이 보는 동안 세션이 끝나지 않게).
const ACTIVITY_MS = 30_000

// 상태에는 그 결과를 받은 요청(seq와 다시 시도 횟수)을 함께 둔다. 지금 요청과 다르면 받는 중으로 본다.
type Load = { key: string } & (
  | { status: 'loading' }
  | { status: 'ready'; info: FileInfo; text?: DecodedText }
  | { status: 'error'; err: unknown }
  | { status: 'security'; level?: RequiredLevel }
)

export function ViewerWindow() {
  const { t } = useTranslation()
  const target = useViewerStore((s) => s.target)
  if (!target) {
    return (
      <div className="viewer-center">
        <p>{t('viewer.empty')}</p>
      </div>
    )
  }
  // 파일이 바뀌면 상태(확대, 재생 오류)를 새로 둔다.
  return <ViewerBody key={target.seq} target={target} />
}

function ViewerBody({ target }: { target: ViewerTarget }) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const dialogs = useDialogs()
  const [attempt, setAttempt] = useState(0)
  const [result, setResult] = useState<Load>({ key: '', status: 'loading' })
  const [mediaError, setMediaError] = useState(false)
  const [actual, setActual] = useState(false)
  const [playing, setPlaying] = useState(false)
  const [downloadError, setDownloadError] = useState<unknown>(null)
  const key = `${target.seq}:${attempt}`

  useEffect(() => {
    const ctl = new AbortController()
    const run = async () => {
      const info = await fetchFileInfo(target.path, ctl.signal)
      if (info.access && !info.access.read) throw new ApiError(403, `${target.path}: not readable`, 'permission_denied')
      if (target.kind !== 'text') return { info }
      const bytes = await fetchPreviewBytes(target.path, info.size, TEXT_LIMIT, ctl.signal)
      return { info, text: decodeText(bytes, info.size) }
    }
    run().then(
      (r) => setResult({ key, status: 'ready', ...r }),
      (err: unknown) => {
        if (ctl.signal.aborted) return
        if (err instanceof ApiError && err.code === 'security_required') setResult({ key, status: 'security', level: err.required })
        else setResult({ key, status: 'error', err })
      },
    )
    return () => ctl.abort()
  }, [key, target])
  const state: Load = result.key === key ? result : { key, status: 'loading' }

  // 재생하는 동안 세션 활동을 알린다.
  useEffect(() => {
    if (!playing) return
    const timer = window.setInterval(reportActivity, ACTIVITY_MS)
    return () => window.clearInterval(timer)
  }, [playing])

  const verify = async () => {
    const level = state.status === 'security' ? state.level : undefined
    if (await requestStepUp(dialogs.scope, { level, target: 'files' })) setAttempt((n) => n + 1)
  }

  const download = () => {
    setDownloadError(null)
    startDownload(target.path, target.name).catch((err: unknown) => {
      if (!(err instanceof ApiError && err.status === 401)) setDownloadError(err)
    })
  }

  const info = state.status === 'ready' ? state.info : null
  const kind = kindOf({ name: target.name, type: 'file' })
  const url = filePreviewUrl(target.path)

  let body
  if (state.status === 'loading') {
    body = <div className="viewer-center" aria-busy="true" />
  } else if (state.status === 'security') {
    body = (
      <div className="viewer-center">
        <IconImage name="security-high-symbolic" size={40} />
        <p>{t('files.security.message')}</p>
        <button type="button" className="addon-button primary" onClick={verify}>
          {t('files.security.verify')}
        </button>
      </div>
    )
  } else if (state.status === 'error') {
    body = (
      <div className="viewer-center" role="alert">
        <p>{t(errorKey(state.err))}</p>
        <button type="button" className="addon-button" onClick={() => setAttempt((n) => n + 1)}>
          {t('files.errors.retry')}
        </button>
      </div>
    )
  } else if (mediaError) {
    body = (
      <div className="viewer-center" role="alert">
        <IconImage name={kindIcon(kind)} size={64} />
        <p>{t(target.kind === 'image' ? 'viewer.imageFailed' : target.kind === 'pdf' ? 'viewer.pdfFailed' : 'viewer.mediaFailed')}</p>
      </div>
    )
  } else if (target.kind === 'image') {
    body = (
      <div className={actual ? 'viewer-image actual' : 'viewer-image'}>
        <img
          src={url}
          alt={target.name}
          draggable={false}
          onError={() => setMediaError(true)}
          onClick={() => setActual((a) => !a)}
          title={actual ? t('viewer.fit') : t('viewer.actualSize')}
        />
      </div>
    )
  } else if (target.kind === 'video') {
    body = (
      <div className="viewer-media">
        <video
          src={url}
          controls
          preload="metadata"
          onError={() => setMediaError(true)}
          onPlay={() => setPlaying(true)}
          onPause={() => setPlaying(false)}
          onEnded={() => setPlaying(false)}
        />
      </div>
    )
  } else if (target.kind === 'audio') {
    body = (
      <div className="viewer-media audio">
        <IconImage name={kindIcon(kind)} size={96} />
        <audio
          src={url}
          controls
          preload="metadata"
          onError={() => setMediaError(true)}
          onPlay={() => setPlaying(true)}
          onPause={() => setPlaying(false)}
          onEnded={() => setPlaying(false)}
        />
      </div>
    )
  } else if (target.kind === 'pdf') {
    body = <iframe className="viewer-pdf" src={url} title={target.name} onError={() => setMediaError(true)} />
  } else {
    const text = state.text
    body = text?.binary ? (
      <div className="viewer-center" role="alert">
        <IconImage name={kindIcon(kind)} size={64} />
        <p>{t('viewer.binary')}</p>
      </div>
    ) : (
      <div className="viewer-text-wrap">
        {text?.truncated && (
          <p className="addon-banner notice viewer-banner" role="status">
            <IconImage name="dialog-information-symbolic" size={16} />
            {t('viewer.truncated', { size: formatBytes(TEXT_LIMIT, lang) })}
          </p>
        )}
        <pre className="viewer-text selectable" tabIndex={0}>
          {text?.text}
        </pre>
      </div>
    )
  }

  return (
    <div className="viewer">
      <div className="viewer-toolbar">
        <IconImage name={kindIcon(kind)} size={20} />
        <div className="viewer-title">
          <strong title={target.path}>{target.name}</strong>
          <span>
            {t(KIND_KEYS[kind])}
            {info ? ` · ${formatBytes(info.size, lang)}` : ''}
          </span>
        </div>
        {target.kind === 'image' && state.status === 'ready' && !mediaError && (
          <button type="button" className="addon-button" aria-pressed={actual} onClick={() => setActual((a) => !a)}>
            {actual ? t('viewer.fit') : t('viewer.actualSize')}
          </button>
        )}
        <button type="button" className="addon-button" disabled={!info} onClick={download}>
          <IconImage name="download-symbolic" size={16} />
          {t('files.info.download')}
        </button>
      </div>
      {downloadError !== null && (
        <p className="addon-banner viewer-banner" role="alert">
          {t(errorKey(downloadError))}
        </p>
      )}
      <div className="viewer-body">{body}</div>
    </div>
  )
}
