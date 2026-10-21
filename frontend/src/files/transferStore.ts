// 파일 작업(복사, 올리기) 목록. 파일 탐색기 창이 다른 디스크로 끌어 놓은 복사(startCopy)와 고르거나 끌어 놓은 올리기
// (startUpload)를 더하고, 파일 작업 창(TransfersWindow, 앱 transfers)이 그린다. 탐색기 창마다 상태를 따로 두는 것과
// 달리, 작업은 그 창을 닫아도 이어지고 작업 창 하나에 모이므로 모듈 저장소에 둔다.
//
// - 작업은 종류와 관계없이 하나씩 차례로 한다(도우미의 사용자당 동시 연산 4개 안에서 목록에 자리를 남긴다). 복사는
//   이름이 많으면 요청 여러 개로 나누고(copyJobs.ts의 copyChunks), 올리기는 파일마다 요청 하나다
// - 작업 창은 작업이 OPEN_DELAY_MS보다 오래 걸릴 때 연다(금방 끝나는 작업에 창이 깜빡이지 않게). 모두 문제없이 끝나면
//   스스로 닫고, 하지 못하거나 건너뛴 항목이 있거나 올리기를 취소해 다시 올릴 파일이 남았으면 (금방 끝났어도) 열어
//   남겨 둔다
// - 작업 창을 닫으면 남은 작업을 취소한다(복사하던 항목은 도우미가 지우고, 올리던 파일은 백엔드가 임시 파일을 지운다).
//   창 컴포넌트의 정리 함수는 개발 모드(StrictMode)에서 마운트 직후에도 불리므로 쓰지 않고, 창 저장소에서 창이
//   사라지는 것을 본다. 닫힌 창은 닫기 애니메이션 동안 이 목록으로 그려지므로 목록은 다음 작업을 더할 때 비운다
// - 작업하는 동안은 세션 활동으로 알리고(입력 없이도 로그아웃되지 않게) 페이지를 떠나려 하면 경고한다
// - 보안 확인 대화상자는 작업을 더한 탐색기 창에 띄우고, 그 창이 닫혔으면 작업 창, 그것도 없으면 바탕화면에 띄운다
import { create } from 'zustand'
import { ApiError, copyFiles, uploadFile } from '../api/client'
import { reportActivity } from '../auth/activity'
import { type DialogScope, scopeId, useDialogStore } from '../shell/dialogs'
import type { Size } from '../windows/geometry'
import { type WindowStore, useWindowStore } from '../windows/store'
import { type CopyJob, addAmount, copyChunks, newJob, nextRate, settled } from './copyJobs'
import { uploadError } from './messages'
import { notifyFilesChanged } from './ops'
import { type Root, joinPath } from './paths'
import { withStepUp } from './stepUp'
import {
  type NewUpload,
  type UploadJob,
  completed,
  fileFailed,
  fileFinished,
  fileProgressed,
  fileStarted,
  newUploadJob,
  nextFile,
  retried,
  stopped,
  unfinished,
  uploadAmount,
} from './uploadJobs'

// OPEN_DELAY_MS는 작업을 시작한 뒤 작업 창을 열기까지 기다리는 시간이다.
const OPEN_DELAY_MS = 400
// ACTIVITY_MS는 작업하는 동안 세션 활동을 알리는 간격이다(세션 유지기가 최대 60초에 한 번 연장한다).
const ACTIVITY_MS = 30_000
// CLEANUP_MS는 복사를 취소·실패한 뒤 대상 폴더를 한 번 더 알리기까지의 시간이다. 도우미는 연결이 끊긴 것을 안 뒤에
// 하던 항목의 임시 이름을 지우므로, 바로 다시 읽은 목록에는 그 임시 이름이 남아 있을 수 있다.
const CLEANUP_MS = 2000

export type TransferJob = CopyJob | UploadJob

// TransferEntry는 작업과, 작업 창이 위치를 그릴 때 쓰는 뿌리(작업을 더한 창의 사이드바)다.
export interface TransferEntry {
  job: TransferJob
  roots: readonly Root[]
}

interface TransferState {
  entries: TransferEntry[]
}

export const useTransferStore = create<TransferState>()(() => ({ entries: [] }))

interface Runtime {
  // scope는 작업을 더한 탐색기 창이다(보안 확인 대화상자를 그 창에 띄운다).
  scope: DialogScope
  bounds: Size
  ctl: AbortController
  // files는 올릴 파일의 내용이다(작업의 files와 같은 순서). 다시 올릴 수 있게 작업이 목록에 있는 동안 둔다.
  files?: readonly File[]
}

// runtimes는 목록에 있는 작업마다 하나다(목록에서 빠지면 지운다).
const runtimes = new Map<number, Runtime>()
let nextId = 1
let pumping = false
// lastBounds는 마지막으로 더한 작업의 데스크톱 영역 크기다(끝난 뒤 작업 창을 열 때 쓴다).
let lastBounds: Size | null = null
// dismissed는 작업 창이 닫혀 지금 목록을 더 보이지 않는다는 뜻이다. 다음 작업을 더할 때 끝난 작업을 지운다.
let dismissed = false

const update = <J extends TransferJob>(id: number, fn: (job: J) => J) =>
  useTransferStore.setState((s) => ({ entries: s.entries.map((e) => (e.job.id === id ? { ...e, job: fn(e.job as J) } : e)) }))

const jobOf = (id: number) => useTransferStore.getState().entries.find((e) => e.job.id === id)?.job

// setEntries는 목록을 바꾸고, 목록에서 빠진 작업의 런타임(올릴 파일 포함)을 지운다.
function setEntries(fn: (entries: TransferEntry[]) => TransferEntry[]) {
  useTransferStore.setState((s) => ({ entries: fn(s.entries) }))
  const ids = new Set(useTransferStore.getState().entries.map((e) => e.job.id))
  for (const id of runtimes.keys()) if (!ids.has(id)) runtimes.delete(id)
}

// TransferOrigin은 작업을 더한 탐색기 창이다.
export interface TransferOrigin {
  roots: readonly Root[]
  scope: DialogScope
  // bounds는 작업 창을 열 때 쓰는 데스크톱 영역 크기다.
  bounds: Size
}

function addJob(job: TransferJob, origin: TransferOrigin, files?: readonly File[]) {
  runtimes.set(job.id, { scope: origin.scope, bounds: origin.bounds, ctl: new AbortController(), files })
  lastBounds = origin.bounds
  setEntries((entries) => [...(dismissed ? entries.filter((e) => !settled(e.job)) : entries), { job, roots: origin.roots }])
  dismissed = false
  void pump()
}

export interface CopyRequest extends TransferOrigin {
  dir: string
  names: readonly string[]
  target: string
}

// startCopy는 복사 작업을 더한다. 앞의 작업이 끝나면 시작한다.
export function startCopy(req: CopyRequest): void {
  addJob(newJob(nextId++, req.dir, req.names, req.target, copyChunks(req.names).length), req)
}

export interface UploadRequest extends TransferOrigin {
  target: string
  // uploads는 올릴 파일과 덮어쓸지다(같은 이름 확인은 탐색기 창이 먼저 한다, uploads.ts의 checkUploads).
  uploads: readonly (NewUpload & { file: File })[]
}

// startUpload는 올리기 작업을 더한다. 앞의 작업이 끝나면 시작한다.
export function startUpload(req: UploadRequest): void {
  if (req.uploads.length === 0) return
  addJob(
    newUploadJob(nextId++, req.target, req.uploads),
    req,
    req.uploads.map((u) => u.file),
  )
}

// cancelTransfer는 작업을 취소한다. 하던 항목은 지우고, 이미 끝낸 항목은 남는다.
export function cancelTransfer(id: number): void {
  const job = jobOf(id)
  if (!job || settled(job)) return
  runtimes.get(id)?.ctl.abort()
  if (job.status === 'queued') update(id, (j) => (j.kind === 'upload' ? stopped(j) : { ...j, status: 'cancelled' }))
}

// retryUpload는 올리지 못했거나 취소한 파일을 다시 올린다. overwrite면 같은 이름이 생겨 올리지 못한 파일을 덮어쓴다.
export function retryUpload(id: number, overwrite: boolean): void {
  const job = jobOf(id)
  const rt = runtimes.get(id)
  if (!job || job.kind !== 'upload' || !settled(job) || !rt?.files) return
  rt.ctl = new AbortController()
  update<UploadJob>(id, (j) => retried(j, overwrite))
  dismissed = false
  void pump()
}

// clearFinished는 끝난 작업을 목록에서 뺀다.
export function clearFinished(): void {
  setEntries((entries) => entries.filter((e) => !settled(e.job)))
}

async function pump() {
  if (pumping) return
  pumping = true
  try {
    for (;;) {
      const next = useTransferStore.getState().entries.find((e) => e.job.status === 'queued')
      if (!next) break
      await runJob(next.job)
    }
  } finally {
    pumping = false
  }
  finishQueue()
}

const transfersOpen = (s: WindowStore) => s.windows.some((w) => w.appId === 'transfers')

function onBeforeUnload(e: BeforeUnloadEvent) {
  e.preventDefault()
}

// liveScope는 보안 확인 대화상자를 띄울 곳이다. 작업을 더한 창이 닫혔으면 작업 창, 그것도 없으면 바탕화면이다.
function liveScope(scope: DialogScope): DialogScope {
  const { outlets } = useDialogStore.getState()
  if (outlets.includes(scopeId(scope))) return scope
  const win = useWindowStore.getState().windows.find((w) => w.appId === 'transfers')
  const own: DialogScope | null = win ? { kind: 'window', windowId: win.id } : null
  return own && outlets.includes(scopeId(own)) ? own : { kind: 'desktop' }
}

// quietStop은 알리지 않고 취소로 둘 오류다(세션이 끝나 로그인 화면으로 가거나, 보안 확인을 취소했다).
const quietStop = (err: unknown) => err instanceof ApiError && (err.status === 401 || err.code === 'security_required')

async function runJob(job: TransferJob) {
  const rt = runtimes.get(job.id)
  // 작업 창을 닫아 시작하기 전에 취소됐다.
  if (!rt || rt.ctl.signal.aborted) {
    update(job.id, (j) => (j.kind === 'upload' ? stopped(j) : { ...j, status: 'cancelled' }))
    return
  }
  update(job.id, (j) => ({ ...j, status: 'running', rate: { at: Date.now(), bytes: j.kind === 'upload' ? uploadAmount(j).bytes : 0, rate: null } }))
  const openTimer = window.setTimeout(() => {
    if (jobOf(job.id)?.status === 'running') useWindowStore.getState().open('transfers', rt.bounds)
  }, OPEN_DELAY_MS)
  const activity = window.setInterval(reportActivity, ACTIVITY_MS)
  window.addEventListener('beforeunload', onBeforeUnload)
  try {
    if (job.kind === 'copy') await runCopy(job, rt)
    else await runUpload(job, rt)
  } finally {
    window.clearTimeout(openTimer)
    window.clearInterval(activity)
    window.removeEventListener('beforeunload', onBeforeUnload)
  }
}

async function runCopy(job: CopyJob, rt: Runtime) {
  const id = job.id
  const chunks = copyChunks(job.names)
  for (const [i, names] of chunks.entries()) {
    if (rt.ctl.signal.aborted) break
    try {
      const res = await withStepUp(liveScope(rt.scope), () =>
        copyFiles(
          { dir: job.dir, names: [...names], to: job.target },
          (p) =>
            update<CopyJob>(id, (j) => {
              const amount = addAmount(j.base, p)
              return { ...j, amount, scanning: p.scanning === true, current: p.current ?? null, rate: nextRate(j.rate, Date.now(), amount.bytes) }
            }),
          rt.ctl.signal,
        ),
      )
      update<CopyJob>(id, (j) => ({
        ...j,
        base: j.amount,
        chunk: i + 1,
        copied: j.copied + res.copied.length,
        failures: [...j.failures, ...res.failed.map((f) => ({ item: f.name, err: new ApiError(409, `/files/copy: ${f.code}`, f.code) }))],
        skipped: j.skipped + res.skipped,
      }))
    } catch (err) {
      window.setTimeout(() => notifyFilesChanged({ dirs: [job.target], gone: [] }), CLEANUP_MS)
      if (rt.ctl.signal.aborted || quietStop(err)) {
        update<CopyJob>(id, (j) => ({ ...j, status: 'cancelled', current: null }))
      } else {
        update<CopyJob>(id, (j) => ({ ...j, status: 'failed', error: err, current: null }))
      }
      break
    } finally {
      // 요청이 끝나면(실패·취소 포함) 그 사이에 복사한 항목이 대상 폴더에 생겼을 수 있다.
      notifyFilesChanged({ dirs: [job.target], gone: [] })
    }
  }
  const status = rt.ctl.signal.aborted ? 'cancelled' : 'done'
  update<CopyJob>(id, (j) => (settled(j) ? j : { ...j, status: j.failures.length > 0 ? 'failed' : status, current: null, scanning: false }))
}

async function runUpload(job: UploadJob, rt: Runtime) {
  const id = job.id
  const files = rt.files ?? []
  for (;;) {
    const cur = jobOf(id)
    if (cur?.kind !== 'upload') return
    if (rt.ctl.signal.aborted) {
      update<UploadJob>(id, (j) => stopped(j))
      return
    }
    const i = nextFile(cur)
    if (i < 0) break
    const file = files[i]
    const f = cur.files[i]
    update<UploadJob>(id, (j) => fileStarted(j, i))
    try {
      // 보안 강화 정책이 쓰기에 확인을 요구하면 확인한 뒤 같은 파일을 다시 올린다.
      await withStepUp(liveScope(rt.scope), () =>
        uploadFile(joinPath(job.target, f.name), file, {
          overwrite: f.overwrite,
          signal: rt.ctl.signal,
          onProgress: (sent) =>
            update<UploadJob>(id, (j) => {
              const next = fileProgressed(j, i, sent)
              return { ...next, rate: nextRate(j.rate, Date.now(), uploadAmount(next).bytes) }
            }),
        }),
      )
      update<UploadJob>(id, (j) => fileFinished(j, i))
      notifyFilesChanged({ dirs: [job.target], gone: [] })
    } catch (err) {
      if (rt.ctl.signal.aborted || (err instanceof DOMException && err.name === 'AbortError')) {
        update<UploadJob>(id, (j) => stopped(j))
        return
      }
      // 세션이 끝났거나 보안 확인을 취소했으면 남은 파일도 올릴 수 없으므로 멈춘다.
      if (quietStop(err)) {
        update<UploadJob>(id, (j) => stopped(j))
        return
      }
      update<UploadJob>(id, (j) => fileFailed(j, i, uploadError(err)))
    }
  }
  update<UploadJob>(id, (j) => completed(j))
}

// clean은 알릴 것 없이 끝난 작업인지다. 복사는 다 했거나 하지 못한 항목 없이 취소한 것이고, 올리기는 모든 파일을 올린
// 것이다. 올리기를 취소했으면(작업 창의 취소, 보안 확인 취소, 세션 만료) 다시 올릴 파일이 남으므로 창에 남겨 둔다.
function clean(j: TransferJob): boolean {
  if (j.status !== 'done' && j.status !== 'cancelled') return false
  if (j.kind === 'upload') return j.error === null && !unfinished(j)
  return j.failures.length === 0 && j.skipped === 0 && !j.error
}

// finishQueue는 남은 작업이 없을 때 부른다. 모두 문제없이 끝났으면 작업 창을 닫고(창이 없으면 목록을 비운다), 알릴
// 것이 있으면 작업 창을 연다(이미 열려 있으면 그대로 둔다). 사용자가 작업 창을 닫았으면 다시 열지 않는다.
function finishQueue() {
  const { entries } = useTransferStore.getState()
  if (dismissed || entries.length === 0) return
  const wins = useWindowStore.getState().windows.filter((w) => w.appId === 'transfers')
  if (entries.every((e) => clean(e.job))) {
    if (wins.length > 0) useWindowStore.getState().closeMany(wins.map((w) => w.id))
    else setEntries(() => [])
  } else if (wins.length === 0 && lastBounds) {
    useWindowStore.getState().open('transfers', lastBounds)
  }
}

// 작업 창이 닫히면 남은 작업을 취소한다. 목록은 닫기 애니메이션이 끝날 때까지 그대로 둔다.
useWindowStore.subscribe((s, prev) => {
  if (!transfersOpen(prev) || transfersOpen(s)) return
  dismissed = true
  for (const e of useTransferStore.getState().entries) cancelTransfer(e.job.id)
})
