// 메모리 구성 막대의 조각 계산. DOM에 의존하지 않아 node 환경에서 테스트한다. 그리기는 MemoryComposition.tsx가 한다.
import type { MemoryDetail } from '../../api/types.gen'

export type MemoryPart = 'apps' | 'shared' | 'compressed' | 'buffers' | 'cache' | 'free'

// 막대 조각 색 자리(system-info.css의 --sysinfo-series-N). 여유는 색 없이 바탕으로 그린다.
export type MemorySlot = 1 | 2 | 3 | 4 | 5

// MEMORY_PARTS는 막대 순서다. 회수할 수 없는 앱 메모리에서 시작해 회수할 수 있는 캐시, 여유로 끝난다.
export const MEMORY_PARTS = [
  { part: 'apps', key: 'appsBytes', slot: 1 },
  { part: 'shared', key: 'sharedBytes', slot: 2 },
  { part: 'compressed', key: 'compressedBytes', slot: 3 },
  { part: 'buffers', key: 'buffersBytes', slot: 4 },
  { part: 'cache', key: 'cacheBytes', slot: 5 },
  { part: 'free', key: 'freeBytes', slot: null },
] as const satisfies readonly { part: MemoryPart; key: keyof MemoryDetail; slot: MemorySlot | null }[]

export interface MemorySegment {
  part: MemoryPart
  slot: MemorySlot | null
  bytes: number
  // 전체 대비 크기와 막대 안의 시작 위치(0~100)
  percent: number
  start: number
}

// memorySegments는 막대에 그릴 조각이다. 0인 항목은 뺀다. 합이 전체를 넘으면(응답이 어긋남) 100%에서 자른다.
export function memorySegments(detail: MemoryDetail, total: number): MemorySegment[] {
  if (total <= 0) return []
  const out: MemorySegment[] = []
  let start = 0
  for (const p of MEMORY_PARTS) {
    const bytes = detail[p.key]
    const percent = Math.min(100 - start, (bytes / total) * 100)
    if (bytes <= 0 || percent <= 0) continue
    out.push({ part: p.part, slot: p.slot, bytes, percent, start })
    start += percent
  }
  return out
}

// stepSegment는 키보드로 고른 조각을 옮긴 결과다. 고른 조각이 없거나 사라졌으면 오른쪽은 첫 조각, 왼쪽은 마지막 조각이고,
// 양 끝에서는 멈춘다. 조각이 없으면 null이다.
export function stepSegment(
  segments: readonly MemorySegment[],
  cur: MemoryPart | null,
  delta: -1 | 1,
): MemoryPart | null {
  if (segments.length === 0) return null
  const i = segments.findIndex((s) => s.part === cur)
  if (i < 0) return segments[delta > 0 ? 0 : segments.length - 1].part
  return segments[Math.min(segments.length - 1, Math.max(0, i + delta))].part
}
