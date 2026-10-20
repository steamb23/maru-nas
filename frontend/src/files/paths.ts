// 파일 탐색기의 경로 계산. 경로는 백엔드와 같은 절대 경로이고, 화면에는 보이는 폴더(홈, 공유 폴더, 디스크)의
// 이름부터 그린다. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { FilesRoots } from '../api/types.gen'

export type RootKind = 'home' | 'share' | 'disk'

export interface Root {
  kind: RootKind
  name: string
  path: string
  // uuid는 공유 폴더의 OMV uuid다(관리자의 ACL 화면 링크).
  uuid?: string
  // reachable이 false면 권한 도우미가 닿지 않는 곳이라 열 수 없다(루트 파일시스템의 공유 폴더).
  reachable: boolean
}

// rootsOf는 사이드바 순서(홈, 공유 폴더, 디스크)의 뿌리 목록이다.
export function rootsOf(r: FilesRoots): Root[] {
  const out: Root[] = []
  if (r.home) out.push({ kind: 'home', name: r.home.name, path: r.home.path, reachable: true })
  for (const s of r.shares) out.push({ kind: 'share', name: s.name, path: s.path, uuid: s.uuid, reachable: s.reachable })
  for (const d of r.disks) out.push({ kind: 'disk', name: d.name, path: d.path, reachable: true })
  return out
}

// within은 path가 root이거나 그 아래인지다.
export function within(path: string, root: string): boolean {
  return path === root || root === '/' || path.startsWith(`${root}/`)
}

// findRoot는 path가 속한 뿌리다. 겹치면(공유 폴더 안의 공유 폴더, 디스크 안의 공유 폴더) 가장 긴 쪽이다.
// 디스크와 공유 폴더가 같은 길이로 겹치면 공유 폴더를 고른다(이름표가 더 알기 쉽다).
export function findRoot(roots: readonly Root[], path: string): Root | null {
  let best: Root | null = null
  for (const r of roots) {
    if (!r.reachable || !within(path, r.path)) continue
    if (!best || r.path.length > best.path.length || (r.path.length === best.path.length && r.kind === 'share')) best = r
  }
  return best
}

export interface Crumb {
  name: string
  path: string
}

// crumbs는 경로 막대의 단계다. 첫 단계는 뿌리 이름이고, 그 뒤는 뿌리 아래 폴더 이름이다.
export function crumbs(root: Root, path: string): Crumb[] {
  const out: Crumb[] = [{ name: root.name, path: root.path }]
  if (path === root.path) return out
  const rest = path.slice(root.path === '/' ? 1 : root.path.length + 1).split('/')
  let cur = root.path
  for (const name of rest) {
    cur = cur === '/' ? `/${name}` : `${cur}/${name}`
    out.push({ name, path: cur })
  }
  return out
}

// parentOf는 상위 폴더다. 뿌리면 null이다(뿌리 밖으로는 올라가지 않는다).
export function parentOf(root: Root, path: string): string | null {
  if (path === root.path) return null
  const i = path.lastIndexOf('/')
  const parent = i <= 0 ? '/' : path.slice(0, i)
  return within(parent, root.path) ? parent : null
}

// joinPath는 폴더 안의 이름이다.
export function joinPath(dir: string, name: string): string {
  return dir === '/' ? `/${name}` : `${dir}/${name}`
}

// baseName은 경로의 마지막 이름이다.
export function baseName(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1)
}

// dirName은 경로가 든 폴더다.
export function dirName(path: string): string {
  const i = path.lastIndexOf('/')
  return i <= 0 ? '/' : path.slice(0, i)
}

// locationLabel은 정보 패널의 "위치"다: 뿌리 이름과 그 아래 폴더를 " / "로 잇는다.
export function locationLabel(roots: readonly Root[], filePath: string): string {
  const dir = dirName(filePath)
  const root = findRoot(roots, dir)
  if (!root) return dir
  return crumbs(root, dir)
    .map((c) => c.name)
    .join(' / ')
}

// UPLOAD_PREFIX는 도우미가 올리는 중인 임시 파일에 쓰는 이름 접두사다. 이 이름으로는 올릴 수 없다.
const UPLOAD_PREFIX = '.onmaru-upload-'

// uploadNameOk는 올릴 파일 이름으로 쓸 수 있는지다(백엔드·도우미와 같은 검사).
export function uploadNameOk(name: string): boolean {
  return (
    name !== '' &&
    name !== '.' &&
    name !== '..' &&
    !name.includes('/') &&
    !name.includes('\0') &&
    new TextEncoder().encode(name).length <= 255 &&
    !name.startsWith(UPLOAD_PREFIX)
  )
}
