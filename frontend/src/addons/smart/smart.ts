// SMART 응답 검사와 표시 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { hasAddonSources } from '../../api/client'
import { isArrayOf, isBool, isNum, isObj, isOneOf, isOptional, isStr } from '../../api/guards'
import type { SmartDisk, SmartResponse } from '../../api/types.gen'
import type { Level } from '../common/levels'

const STATUSES = ['good', 'warning', 'failing', 'unknown'] as const

function isSmartDisk(v: unknown): v is SmartDisk {
  return (
    isObj(v) &&
    isStr(v.device) &&
    isStr(v.model) &&
    isNum(v.sizeBytes) &&
    isOptional(v.celsius, isNum) &&
    isOneOf(v.status, STATUSES) &&
    isStr(v.overallStatus) &&
    isBool(v.monitored)
  )
}

export function isSmartResponse(v: unknown): v is SmartResponse {
  return (
    isObj(v) &&
    hasAddonSources(v) &&
    isStr(v.checkedAt) &&
    isNum(v.refreshInSeconds) &&
    isArrayOf(v.disks, isSmartDisk)
  )
}

// statusLevel은 상태 점·글자 색이다. 상태를 모르면 주의로 본다.
export function statusLevel(s: SmartDisk['status']): Level {
  if (s === 'good') return 'normal'
  if (s === 'failing') return 'crit'
  return 'warn'
}

const STATUS_ORDER: Record<SmartDisk['status'], number> = { failing: 0, warning: 1, unknown: 2, good: 3 }

// sortDisks는 문제 있는 디스크를 먼저, 같은 상태는 장치 이름 순서로 둔다.
export function sortDisks(list: readonly SmartDisk[]): SmartDisk[] {
  return [...list].sort(
    (a, b) => STATUS_ORDER[a.status] - STATUS_ORDER[b.status] || a.device.localeCompare(b.device),
  )
}

export interface SmartCounts {
  good: number
  warning: number
  failing: number
}

// countDisks는 상태별 디스크 수다. 상태를 모르는 디스크는 주의로 센다.
export function countDisks(list: readonly SmartDisk[]): SmartCounts {
  const out = { good: 0, warning: 0, failing: 0 }
  for (const d of list) {
    if (d.status === 'good') out.good++
    else if (d.status === 'failing') out.failing++
    else out.warning++
  }
  return out
}
