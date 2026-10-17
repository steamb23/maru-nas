// 숨긴 탭에서도 늦춰지지 않는 타이머(poller.ts의 Scheduler). 시스템 정보 창이 탭을 숨겨도 기록을 이어 가는 데 쓴다.
//
// 브라우저는 숨긴 탭의 메인 스레드 타이머를 1초에 한 번, 크롬은 5분이 지나면 1분에 한 번으로 줄인다.
// Worker 안의 타이머는 줄이지 않으므로 예약을 Worker(timer.worker.ts)에 맡기고 메시지로 돌려받는다.
// Worker를 만들 수 없거나(node 테스트) 불러오지 못하면(업그레이드 뒤 옛 탭이 지워진 파일을 요청) setTimeout으로 대신한다.
// 사파리처럼 숨긴 페이지를 통째로 멈추는 브라우저에서는 이 방법으로도 이어 가지 못한다.
import { type Scheduler, timeoutScheduler } from './poller'

// WorkerLike는 이 모듈이 쓰는 Worker의 일부다(테스트에서 가짜를 넣는다).
export interface WorkerLike {
  postMessage: (message: { id: number; ms?: number }) => void
  terminate: () => void
  onmessage: ((e: MessageEvent<number>) => void) | null
  onerror: ((e: ErrorEvent) => void) | null
}

interface Entry {
  fn: () => void
  // 부를 시각(Date.now 기준). Worker가 실패해 setTimeout으로 옮길 때 남은 시간을 구한다.
  due: number
  // setTimeout으로 옮긴 뒤의 취소 함수
  fallback?: () => void
}

// createWorkerScheduler는 make로 Worker를 처음 예약할 때 하나 만들어 함께 쓰는 Scheduler를 만든다.
// make가 null을 주거나 던지면 setTimeout을 쓴다.
export function createWorkerScheduler(make: () => WorkerLike | null): Scheduler {
  // undefined: 아직 만들지 않음, null: 쓸 수 없음
  let worker: WorkerLike | null | undefined
  let nextId = 1
  const entries = new Map<number, Entry>()

  // Worker를 쓸 수 없게 되면 기다리던 예약을 남은 시간만큼 setTimeout으로 옮긴다.
  const fallBack = () => {
    worker?.terminate()
    worker = null
    for (const [id, e] of entries) {
      if (e.fallback) continue
      e.fallback = timeoutScheduler(
        () => {
          entries.delete(id)
          e.fn()
        },
        Math.max(0, e.due - Date.now()),
      )
    }
  }

  const get = (): WorkerLike | null => {
    if (worker !== undefined) return worker
    try {
      worker = make()
    } catch {
      worker = null
    }
    if (worker) {
      worker.onmessage = (e) => {
        const entry = entries.get(e.data)
        if (!entry || entry.fallback) return
        entries.delete(e.data)
        entry.fn()
      }
      worker.onerror = fallBack
    }
    return worker
  }

  return (fn, ms) => {
    const w = get()
    if (!w) return timeoutScheduler(fn, ms)
    const id = nextId++
    const entry: Entry = { fn, due: Date.now() + ms }
    entries.set(id, entry)
    w.postMessage({ id, ms })
    return () => {
      if (!entries.delete(id)) return
      if (entry.fallback) entry.fallback()
      else worker?.postMessage({ id })
    }
  }
}

// workerScheduler는 앱이 함께 쓰는 Worker 타이머다. Worker는 처음 예약할 때 만든다.
export const workerScheduler: Scheduler = createWorkerScheduler(() =>
  typeof Worker === 'undefined'
    ? null
    : new Worker(new URL('./timer.worker.ts', import.meta.url), { type: 'module' }),
)
