// 디스크 용량 응답 검사와 표시 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { hasAddonSources } from '../../api/client'
import { isBool, isNum, isObj, isOptional, isStr } from '../../api/guards'
import type { Filesystem, StorageResponse } from '../../api/types.gen'

function isFilesystem(v: unknown): v is Filesystem {
  return (
    isObj(v) &&
    isStr(v.device) &&
    isOptional(v.label, isStr) &&
    isStr(v.mountPoint) &&
    isStr(v.type) &&
    isNum(v.totalBytes) &&
    isNum(v.usedBytes) &&
    isNum(v.availableBytes) &&
    isOptional(v.readOnly, isBool)
  )
}

export function isStorageResponse(v: unknown): v is StorageResponse {
  return isObj(v) && hasAddonSources(v) && Array.isArray(v.filesystems) && v.filesystems.every(isFilesystem)
}

// FsName은 파일시스템을 부르는 이름이다. root는 번역 키로 그린다.
export type FsName = { kind: 'root' } | { kind: 'text'; text: string }

// OMV가 파일시스템을 마운트하는 경로. 끝부분이 UUID라 이름으로 쓰기 어렵다.
const OMV_MOUNT_PREFIX = '/srv/dev-disk-by-'

// fsName은 라벨 → 루트(/) → 마운트 경로 마지막 부분 순으로 이름을 정한다.
// OMV 기본 마운트 경로(/srv/dev-disk-by-uuid-…)면 장치 이름(sda1)을 쓴다.
export function fsName(fs: Filesystem): FsName {
  if (fs.label) return { kind: 'text', text: fs.label }
  if (fs.mountPoint === '/') return { kind: 'root' }
  if (fs.mountPoint.startsWith(OMV_MOUNT_PREFIX)) {
    return { kind: 'text', text: fs.device.slice(fs.device.lastIndexOf('/') + 1) }
  }
  return { kind: 'text', text: fs.mountPoint.slice(fs.mountPoint.lastIndexOf('/') + 1) }
}

// usedPercent는 df와 같은 사용률이다: 사용 / (사용 + 일반 사용자 남은 공간).
export function usedPercent(fs: Filesystem): number {
  const denom = fs.usedBytes + fs.availableBytes
  return denom > 0 ? Math.min(100, (fs.usedBytes / denom) * 100) : 0
}

// widgetFilesystems는 위젯에 보여 줄 파일시스템이다. /boot 아래 작은 부트 파티션은 뺀다.
export function widgetFilesystems(list: readonly Filesystem[]): Filesystem[] {
  return list.filter((fs) => fs.mountPoint !== '/boot' && !fs.mountPoint.startsWith('/boot/'))
}
