// 파일 복사 작업 목록. 파일 탐색기 창이 다른 디스크로 끌어 놓은 항목을 더하고(startCopy), 복사 진행 창(CopyWindow,
// 앱 copy)이 그린다. 탐색기 창마다 상태를 따로 두는 것과 달리, 복사는 그 창을 닫아도 이어지고 진행 창 하나에 모이므로
// 모듈 저장소에 둔다.
//
// - 작업은 하나씩 차례로 한다(도우미의 사용자당 동시 연산 4개 안에서 목록에 자리를 남긴다). 이름이 많으면 요청 여러
//   개로 나눈다(copyJobs.ts의 copyChunks)
// - 진행 창은 복사가 OPEN_DELAY_MS보다 오래 걸릴 때 연다(금방 끝나는 복사에 창이 깜빡이지 않게). 모두 문제없이 끝나면
//   스스로 닫고, 하지 못하거나 건너뛴 항목이 있으면 (금방 끝났어도) 열어 남겨 둔다
// - 진행 창을 닫으면 남은 작업을 취소한다(하던 항목은 도우미가 지운다). 창 컴포넌트의 정리 함수는 개발 모드
//   (StrictMode)에서 마운트 직후에도 불리므로 쓰지 않고, 창 저장소에서 창이 사라지는 것을 본다. 닫힌 창은 닫기
//   애니메이션 동안 이 목록으로 그려지므로 목록은 다음 작업을 더할 때 비운다
// - 복사하는 동안은 세션 활동으로 알리고(입력 없이도 로그아웃되지 않게) 페이지를 떠나려 하면 경고한다
import { create } from 'zustand'
import { ApiError, copyFiles } from '../api/client'
import { reportActivity } from '../auth/activity'
import type { DialogScope } from '../shell/dialogs'
import type { Size } from '../windows/geometry'
import { type WindowStore, useWindowStore } from '../windows/store'
import { type CopyJob, addAmount, copyChunks, newJob, nextRate, settled } from './copyJobs'
import { notifyFilesChanged } from './ops'
import type { Root } from './paths'
import { withStepUp } from './stepUp'

// OPEN_DELAY_MS는 복사를 시작한 뒤 진행 창을 열기까지 기다리는 시간이다.
const OPEN_DELAY_MS = 400
// ACTIVITY_MS는 복사하는 동안 세션 활동을 알리는 간격이다(세션 유지기가 최대 60초에 한 번 연장한다).
const ACTIVITY_MS = 30_000
// CLEANUP_MS는 취소·실패 뒤 대상 폴더를 한 번 더 알리기까지의 시간이다. 도우미는 연결이 끊긴 것을 안 뒤에 하던
// 항목의 임시 이름을 지우므로, 바로 다시 읽은 목록에는 그 임시 이름이 남아 있을 수 있다.
const CLEANUP_MS = 2000

// CopyEntry는 작업과, 진행 창이 위치를 그릴 때 쓰는 뿌리(작업을 더한 창의 사이드바)다.
export interface CopyEntry {
  job: CopyJob
  roots: readonly Root[]
}

interface CopyState {
  entries: CopyEntry[]
}

export const useCopyStore = create<CopyState>()(() => ({ entries: [] }))

interface Runtime {
  // scope는 작업을 더한 탐색기 창이다(보안 확인 대화상자를 그 창에 띄운다).
  scope: DialogScope
  bounds: Size
  ctl: AbortController
}

const runtimes = new Map<number, Runtime>()
let nextId = 1
let pumping = false
// lastBounds는 마지막으로 더한 작업의 데스크톱 영역 크기다(끝난 뒤 진행 창을 열 때 쓴다).
let lastBounds: Size | null = null
// dismissed는 진행 창이 닫혀 지금 목록을 더 보이지 않는다는 뜻이다. 다음 작업을 더할 때 끝난 작업을 지운다.
let dismissed = false

const update = (id: number, fn: (job: CopyJob) => CopyJob) =>
  useCopyStore.setState((s) => ({ entries: s.entries.map((e) => (e.job.id === id ? { ...e, job: fn(e.job) } : e)) }))

const jobOf = (id: number) => useCopyStore.getState().entries.find((e) => e.job.id === id)?.job

export interface CopyRequest {
  dir: string
  names: readonly string[]
  target: string
  roots: readonly Root[]
  scope: DialogScope
  // bounds는 진행 창을 열 때 쓰는 데스크톱 영역 크기다.
  bounds: Size
}

// startCopy는 복사 작업을 더한다. 앞의 작업이 끝나면 시작한다.
export function startCopy(req: CopyRequest): void {
  const id = nextId++
  const job = newJob(id, req.dir, req.names, req.target, copyChunks(req.names).length)
  runtimes.set(id, { scope: req.scope, bounds: req.bounds, ctl: new AbortController() })
  lastBounds = req.bounds
  useCopyStore.setState((s) => ({
    entries: [...(dismissed ? s.entries.filter((e) => !settled(e.job)) : s.entries), { job, roots: req.roots }],
  }))
  dismissed = false
  void pump()
}

// cancelCopy는 작업을 취소한다. 하던 항목은 도우미가 지우고, 이미 복사한 항목은 남는다.
export function cancelCopy(id: number): void {
  const job = jobOf(id)
  if (!job || settled(job)) return
  runtimes.get(id)?.ctl.abort()
  if (job.status === 'queued') {
    runtimes.delete(id)
    update(id, (j) => ({ ...j, status: 'cancelled' }))
  }
}

// clearFinished는 끝난 작업을 목록에서 뺀다.
export function clearFinished(): void {
  useCopyStore.setState((s) => ({ entries: s.entries.filter((e) => !settled(e.job)) }))
}

async function pump() {
  if (pumping) return
  pumping = true
  try {
    for (;;) {
      const next = useCopyStore.getState().entries.find((e) => e.job.status === 'queued')
      if (!next) break
      await runJob(next.job.id)
    }
  } finally {
    pumping = false
  }
  finishQueue()
}

const copyOpen = (s: WindowStore) => s.windows.some((w) => w.appId === 'copy')

function onBeforeUnload(e: BeforeUnloadEvent) {
  e.preventDefault()
}

async function runJob(id: number) {
  const rt = runtimes.get(id)
  const job = jobOf(id)
  if (!job) return
  // 진행 창을 닫아 시작하기 전에 취소됐다.
  if (!rt || rt.ctl.signal.aborted) {
    runtimes.delete(id)
    update(id, (j) => ({ ...j, status: 'cancelled' }))
    return
  }
  update(id, (j) => ({ ...j, status: 'running', rate: { at: Date.now(), bytes: 0, rate: null } }))
  const openTimer = window.setTimeout(() => {
    if (jobOf(id)?.status === 'running') useWindowStore.getState().open('copy', rt.bounds)
  }, OPEN_DELAY_MS)
  const activity = window.setInterval(reportActivity, ACTIVITY_MS)
  window.addEventListener('beforeunload', onBeforeUnload)
  const chunks = copyChunks(job.names)
  try {
    for (const [i, names] of chunks.entries()) {
      if (rt.ctl.signal.aborted) break
      try {
        const res = await withStepUp(rt.scope, () =>
          copyFiles(
            { dir: job.dir, names: [...names], to: job.target },
            (p) =>
              update(id, (j) => {
                const amount = addAmount(j.base, p)
                return { ...j, amount, scanning: p.scanning === true, current: p.current ?? null, rate: nextRate(j.rate, Date.now(), amount.bytes) }
              }),
            rt.ctl.signal,
          ),
        )
        update(id, (j) => ({
          ...j,
          base: j.amount,
          chunk: i + 1,
          copied: j.copied + res.copied.length,
          failures: [...j.failures, ...res.failed.map((f) => ({ item: f.name, err: new ApiError(409, `/files/copy: ${f.code}`, f.code) }))],
          skipped: j.skipped + res.skipped,
        }))
      } catch (err) {
        window.setTimeout(() => notifyFilesChanged({ dirs: [job.target], gone: [] }), CLEANUP_MS)
        // 세션이 끝났거나(로그인 화면으로 간다) 보안 확인을 취소했으면 알리지 않고 취소로 둔다.
        const quiet = err instanceof ApiError && (err.status === 401 || err.code === 'security_required')
        if (rt.ctl.signal.aborted || quiet) {
          update(id, (j) => ({ ...j, status: 'cancelled', current: null }))
        } else {
          update(id, (j) => ({ ...j, status: 'failed', error: err, current: null }))
        }
        break
      } finally {
        // 요청이 끝나면(실패·취소 포함) 그 사이에 복사한 항목이 대상 폴더에 생겼을 수 있다.
        notifyFilesChanged({ dirs: [job.target], gone: [] })
      }
    }
    const status = rt.ctl.signal.aborted ? 'cancelled' : 'done'
    update(id, (j) => (settled(j) ? j : { ...j, status: j.failures.length > 0 ? 'failed' : status, current: null, scanning: false }))
  } finally {
    window.clearTimeout(openTimer)
    window.clearInterval(activity)
    window.removeEventListener('beforeunload', onBeforeUnload)
    runtimes.delete(id)
  }
}

// clean은 알릴 것 없이 끝난 작업인지다(다 복사했거나, 하지 못한 항목 없이 취소했다).
const clean = (j: CopyJob) => (j.status === 'done' || j.status === 'cancelled') && j.failures.length === 0 && j.skipped === 0 && !j.error

// finishQueue는 남은 작업이 없을 때 부른다. 모두 문제없이 끝났으면 진행 창을 닫고(창이 없으면 목록을 비운다), 알릴
// 것이 있으면 진행 창을 연다(이미 열려 있으면 그대로 둔다). 사용자가 진행 창을 닫았으면 다시 열지 않는다.
function finishQueue() {
  const { entries } = useCopyStore.getState()
  if (dismissed || entries.length === 0) return
  const wins = useWindowStore.getState().windows.filter((w) => w.appId === 'copy')
  if (entries.every((e) => clean(e.job))) {
    if (wins.length > 0) useWindowStore.getState().closeMany(wins.map((w) => w.id))
    else useCopyStore.setState({ entries: [] })
  } else if (wins.length === 0 && lastBounds) {
    useWindowStore.getState().open('copy', lastBounds)
  }
}

// 진행 창이 닫히면 남은 작업을 취소한다. 목록은 닫기 애니메이션이 끝날 때까지 그대로 둔다.
useWindowStore.subscribe((s, prev) => {
  if (!copyOpen(prev) || copyOpen(s)) return
  dismissed = true
  for (const rt of runtimes.values()) rt.ctl.abort()
})
