// 파일 탐색기의 경로 계산. 경로는 백엔드와 같은 절대 경로이고, 화면에는 보이는 폴더(홈, 공유 폴더, 디스크)의
// 이름부터 그린다. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { FilesRoots, TrashVolume } from '../api/types.gen'

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
// 홈 폴더 뿌리는 경로 표시줄처럼 계정 이름 대신 homeName(화면 문구)으로 보인다.
export function locationLabel(roots: readonly Root[], filePath: string, homeName: string): string {
  return folderLabel(roots, dirName(filePath), homeName)
}

// folderLabel은 폴더 dir 자신의 위치다(locationLabel과 같은 모양, 파일 작업 창의 원본·대상).
export function folderLabel(roots: readonly Root[], dir: string, homeName: string): string {
  const root = findRoot(roots, dir)
  if (!root) return dir
  return crumbs(root, dir)
    .map((c, i) => (i === 0 && root.kind === 'home' ? homeName : c.name))
    .join(' / ')
}

// UPLOAD_PREFIX는 도우미가 올리는 중인 임시 파일에 쓰는 이름 접두사다. 이 이름으로는 올리거나 만들거나 바꿀 수 없다.
const UPLOAD_PREFIX = '.onmaru-upload-'

// NameProblem은 새 이름(올리기, 폴더 만들기, 이름 바꾸기)으로 쓸 수 없는 이유다.
export type NameProblem = 'empty' | 'dots' | 'chars' | 'tooLong' | 'reserved'

// nameProblem은 새 이름으로 쓸 수 없는 이유다. 쓸 수 있으면 null이다(백엔드·도우미와 같은 검사).
export function nameProblem(name: string): NameProblem | null {
  if (name === '') return 'empty'
  if (name === '.' || name === '..') return 'dots'
  if (name.includes('/') || name.includes('\0')) return 'chars'
  if (new TextEncoder().encode(name).length > 255) return 'tooLong'
  if (name.startsWith(UPLOAD_PREFIX)) return 'reserved'
  return null
}

// uploadNameOk는 올릴 파일 이름으로 쓸 수 있는지다.
export function uploadNameOk(name: string): boolean {
  return nameProblem(name) === null
}

// stemLength는 이름 바꾸기에서 처음 고를 글자 수다. 파일은 마지막 확장자를 빼고(앞의 점으로 시작하는 이름은 전체),
// 폴더는 전체다.
export function stemLength(name: string, folder: boolean): number {
  const dot = name.lastIndexOf('.')
  return folder || dot <= 0 ? name.length : dot
}

// numberedName은 같은 이름이 있을 때 붙여 볼 n번째 이름이다: "새 폴더", "새 폴더 (2)", "새 폴더 (3)" …
export function numberedName(base: string, n: number): string {
  return n <= 1 ? base : `${base} (${n})`
}

// moveAllowed는 sources(옮길 경로)를 target 폴더로 옮길 수 있는지 경로 모양으로만 본다. 모두 이미 그 폴더에 있거나,
// 어느 하나가 target 자신이거나 그 위(폴더를 자기 안으로)면 옮길 수 없다. 다른 디스크인지는 서버가 정한다.
export function moveAllowed(sources: readonly string[], target: string): boolean {
  if (sources.length === 0) return false
  if (sources.every((p) => dirName(p) === target)) return false
  return !sources.some((p) => within(target, p))
}

// TRASH는 휴지통을 가리키는 표시다. 사이드바 휴지통 항목의 놓을 곳(data-drop-path)과 창 사이 변경 알림의 폴더로 쓴다.
// 절대 경로가 아니므로 실제 경로와 겹치지 않는다.
export const TRASH = 'trash:'

// volumeOf는 path가 든 휴지통 볼륨(홈 폴더나 디스크)이다. 겹치면 가장 긴 쪽이고, 없으면 null이다.
export function volumeOf(volumes: readonly TrashVolume[], path: string): TrashVolume | null {
  let best: TrashVolume | null = null
  for (const v of volumes) {
    if (within(path, v.top) && (!best || v.top.length > best.top.length)) best = v
  }
  return best
}
