// 파일 복사 작업의 상태와 순수 계산(transferStore.ts가 쓴다). 이름을 요청마다 나누고, 요청 여러 개의 진행을 합치고, 속도를
// 잰다. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { FilesCopyProgress, TrashVolume } from '../api/types.gen'
import type { OpFailure } from './ops'
import { volumeOf } from './paths'

// MAX_NAMES는 복사 요청 하나의 이름 수(백엔드 proto.MaxCopy), MAX_NAMES_JSON은 그 이름들을 JSON으로 쓴 길이의 상한이다.
// 백엔드는 경로까지 담은 인자를 56KiB(proto.MaxCopyParams)까지 받으므로 경로 몫을 남긴다.
export const MAX_NAMES = 512
export const MAX_NAMES_JSON = 40 * 1024

// goJSONLength는 Go의 encoding/json이 문자열 s를 쓴 바이트 수의 상한이다(따옴표 포함). 제어 문자와 <, >, &,
// U+2028·U+2029는 \u 여섯 글자, "와 \는 두 글자다.
export function goJSONLength(s: string): number {
  let n = 2
  for (const ch of s) {
    const c = ch.codePointAt(0) ?? 0
    if (c < 0x20 || ch === '<' || ch === '>' || ch === '&' || c === 0x2028 || c === 0x2029) n += 6
    else if (ch === '"' || ch === '\\') n += 2
    else n += c < 0x80 ? 1 : c < 0x800 ? 2 : c < 0x10000 ? 3 : 4
  }
  return n
}

// copyChunks는 이름을 복사 요청 하나에 들어가게 순서대로 나눈다.
export function copyChunks(names: readonly string[]): string[][] {
  const out: string[][] = []
  let cur: string[] = []
  let size = 2
  for (const name of names) {
    const len = goJSONLength(name) + 1
    if (cur.length > 0 && (cur.length >= MAX_NAMES || size + len > MAX_NAMES_JSON)) {
      out.push(cur)
      cur = []
      size = 2
    }
    cur.push(name)
    size += len
  }
  if (cur.length > 0) out.push(cur)
  return out
}

// DropMode는 끌어 놓은 항목을 어떻게 하는지다. 다른 디스크(도우미의 기준 디렉터리, 휴지통 볼륨과 같다)면 복사한다.
export type DropMode = 'move' | 'copy' | 'trash'

// dropMode는 dir의 항목을 target에 놓을 때 할 일이다. 볼륨을 모르면 옮겨 본다(다른 디스크면 그때 복사로 바꾼다).
// 옮기기와 휴지통은 dir에 쓸 수 있어야 하므로, 쓸 수 없으면 복사만 된다(같은 디스크면 null).
export function dropMode(volumes: readonly TrashVolume[] | null, dir: string, target: string, trash: string, writable: boolean): DropMode | null {
  if (target === trash) return writable ? 'trash' : null
  const cross = volumes !== null && volumeOf(volumes, dir)?.top !== volumeOf(volumes, target)?.top
  if (cross) return 'copy'
  return writable ? 'move' : null
}

export interface CopyAmount {
  files: number
  bytes: number
  totalFiles: number
  totalBytes: number
}

export const NO_AMOUNT: CopyAmount = { files: 0, bytes: 0, totalFiles: 0, totalBytes: 0 }

// addAmount는 끝난 요청의 양(base)에 지금 요청의 진행을 더한다.
export function addAmount(base: CopyAmount, p: FilesCopyProgress): CopyAmount {
  return {
    files: base.files + p.files,
    bytes: base.bytes + p.bytes,
    totalFiles: base.totalFiles + p.totalFiles,
    totalBytes: base.totalBytes + p.totalBytes,
  }
}

// RateSample은 속도를 잰 마지막 표본이다. rate는 초당 바이트이고 아직 모르면 null이다.
export interface RateSample {
  at: number
  bytes: number
  rate: number | null
}

// RATE_MIN_MS는 속도를 다시 재는 최소 간격, RATE_WEIGHT는 새 표본의 비중(지수 평활)이다.
const RATE_MIN_MS = 500
const RATE_WEIGHT = 0.3

// nextRate는 지금 시각 at에 복사한 바이트 수 bytes로 속도를 갱신한다. 간격이 짧으면 그대로 둔다.
export function nextRate(prev: RateSample, at: number, bytes: number): RateSample {
  const dt = at - prev.at
  if (dt < RATE_MIN_MS) return prev
  const now = (Math.max(0, bytes - prev.bytes) * 1000) / dt
  return { at, bytes, rate: prev.rate === null ? now : prev.rate * (1 - RATE_WEIGHT) + now * RATE_WEIGHT }
}

// TransferStatus는 파일 작업(복사, 올리기)의 상태다. failed는 하지 못한 항목이 있거나 작업 전체가 멈춘 것이다.
export type TransferStatus = 'queued' | 'running' | 'done' | 'failed' | 'cancelled'

export interface CopyJob {
  kind: 'copy'
  id: number
  dir: string
  names: readonly string[]
  target: string
  status: TransferStatus
  // amount는 끝난 요청의 양(base)에 지금 요청의 진행을 더한 것이다. 요청이 여러 개면 뒤 요청의 양은 그 요청을 셀 때
  // 더해진다.
  amount: CopyAmount
  base: CopyAmount
  scanning: boolean
  // current는 지금 복사하는 항목의 dir 안 상대 경로다.
  current: string | null
  // chunk는 끝난 요청 수, chunks는 전체 요청 수다.
  chunk: number
  chunks: number
  // copied는 복사한 맨 위 이름 수, failures는 복사하지 못한 이름과 이유(ApiError)다.
  copied: number
  failures: OpFailure<string>[]
  // skipped는 폴더 안에서 복사하지 않은 특수 파일 수다.
  skipped: number
  // error는 작업 전체를 멈춘 오류(세션 만료, 도우미 없음, 보안 확인 취소 등)다.
  error: unknown
  rate: RateSample
}

export function newJob(id: number, dir: string, names: readonly string[], target: string, chunks: number): CopyJob {
  return {
    kind: 'copy',
    id,
    dir,
    names,
    target,
    status: 'queued',
    amount: NO_AMOUNT,
    base: NO_AMOUNT,
    scanning: true,
    current: null,
    chunk: 0,
    chunks,
    copied: 0,
    failures: [],
    skipped: 0,
    error: null,
    rate: { at: 0, bytes: 0, rate: null },
  }
}

// settled는 끝난(성공, 실패, 취소) 작업인지다.
export function settled(job: { status: TransferStatus }): boolean {
  return job.status === 'done' || job.status === 'failed' || job.status === 'cancelled'
}

// progressOf는 0~1의 진행률이다. 요청이 하나면 바이트(파일 내용이 없으면 파일 수)로, 여러 개면 끝난 요청 수에 지금
// 요청의 비율을 더해 잰다. 세는 중이면 null이다(막대를 움직이며 보인다).
export function progressOf(job: CopyJob): number | null {
  if (job.status === 'done') return 1
  if (job.scanning) return null
  const { amount: a, base: b } = job
  const bytes = a.totalBytes - b.totalBytes
  const files = a.totalFiles - b.totalFiles
  const inner = bytes > 0 ? (a.bytes - b.bytes) / bytes : files > 0 ? (a.files - b.files) / files : 1
  return Math.min(1, (job.chunk + Math.min(1, inner)) / Math.max(1, job.chunks))
}

// remainingSeconds는 남은 시간(초)이다. 요청이 하나이고 속도를 알 때만 셀 수 있다.
export function remainingSeconds(job: CopyJob): number | null {
  if (job.status !== 'running' || job.scanning || job.chunks !== 1 || !job.rate.rate || job.rate.rate <= 0) return null
  return Math.max(0, (job.amount.totalBytes - job.amount.bytes) / job.rate.rate)
}

// durationOf는 남은 시간을 보일 단위와 값이다(1분 안은 초, 1시간 안은 분, 그 위는 시간을 소수 한 자리로).
export function durationOf(seconds: number): { unit: 'second' | 'minute' | 'hour'; value: number } {
  if (seconds < 60) return { unit: 'second', value: Math.max(1, Math.ceil(seconds)) }
  if (seconds < 3600) return { unit: 'minute', value: Math.ceil(seconds / 60) }
  return { unit: 'hour', value: Math.round(seconds / 360) / 10 }
}
