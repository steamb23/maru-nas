// 임시 확인용 페이지. 1단계에서 데스크톱 셸로 교체한다.
import { useCallback, useEffect, useState } from 'react'
import { fetchSystem } from './api/client'
import type { SystemResponse } from './api/types.gen'
import { getIcon } from './icons/resolver'

type State =
  | { status: 'loading' }
  | { status: 'ok'; data: SystemResponse }
  | { status: 'error'; message: string }

export default function App() {
  const [state, setState] = useState<State>({ status: 'loading' })

  const load = useCallback((signal?: AbortSignal) => {
    fetchSystem(signal).then(
      (data) => setState({ status: 'ok', data }),
      (err: unknown) => {
        if (signal?.aborted) return
        setState({ status: 'error', message: err instanceof Error ? err.message : String(err) })
      },
    )
  }, [])

  useEffect(() => {
    const ctrl = new AbortController()
    load(ctrl.signal)
    return () => ctrl.abort()
  }, [load])

  return (
    <main>
      <h1>OMV 데스크톱 대시보드 (뼈대)</h1>

      <h2>아이콘</h2>
      <img src={getIcon('folder').url} alt="folder" width={64} height={64} />

      <h2>
        GET /api/system <button onClick={() => load()}>새로고침</button>
      </h2>
      {state.status === 'loading' && <p>불러오는 중…</p>}
      {state.status === 'error' && <p role="alert">연결 끊김: {state.message}</p>}
      {state.status === 'ok' && <pre>{JSON.stringify(state.data, null, 2)}</pre>}
    </main>
  )
}
