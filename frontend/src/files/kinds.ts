// 항목의 종류(목록의 "종류" 열, 정보 패널)와 아이콘. 파일은 확장자로만 정한다(내용을 읽지 않는다).
// DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ParseKeys } from 'i18next'
import type { FileItem } from '../api/types.gen'
import type { IconName } from '../icons/resolver'

export type Kind =
  | 'folder'
  | 'locked'
  | 'link'
  | 'other'
  | 'unknown'
  | 'file'
  | 'image'
  | 'video'
  | 'audio'
  | 'archive'
  | 'diskImage'
  | 'pdf'
  | 'document'
  | 'spreadsheet'
  | 'presentation'
  | 'text'
  | 'markdown'
  | 'web'
  | 'code'
  | 'executable'

export const KIND_KEYS: Record<Kind, ParseKeys> = {
  folder: 'files.kinds.folder',
  locked: 'files.kinds.locked',
  link: 'files.kinds.link',
  other: 'files.kinds.other',
  unknown: 'files.kinds.unknown',
  file: 'files.kinds.file',
  image: 'files.kinds.image',
  video: 'files.kinds.video',
  audio: 'files.kinds.audio',
  archive: 'files.kinds.archive',
  diskImage: 'files.kinds.diskImage',
  pdf: 'files.kinds.pdf',
  document: 'files.kinds.document',
  spreadsheet: 'files.kinds.spreadsheet',
  presentation: 'files.kinds.presentation',
  text: 'files.kinds.text',
  markdown: 'files.kinds.markdown',
  web: 'files.kinds.web',
  code: 'files.kinds.code',
  executable: 'files.kinds.executable',
}

const KIND_ICONS: Record<Kind, IconName> = {
  folder: 'folder',
  locked: 'folder',
  link: 'inode-symlink',
  other: 'unknown',
  unknown: 'unknown',
  file: 'text-x-generic',
  image: 'image-x-generic',
  video: 'video-x-generic',
  audio: 'audio-x-generic',
  archive: 'package-x-generic',
  diskImage: 'application-x-cd-image',
  pdf: 'application-pdf',
  document: 'x-office-document',
  spreadsheet: 'x-office-spreadsheet',
  presentation: 'x-office-presentation',
  text: 'text-x-generic',
  markdown: 'text-markdown',
  web: 'text-html',
  code: 'text-x-script',
  executable: 'application-x-executable',
}

const EXTENSIONS: Record<string, Kind> = {}
const add = (kind: Kind, exts: string) => {
  for (const e of exts.split(' ')) EXTENSIONS[e] = kind
}
add('image', 'jpg jpeg png gif webp bmp tif tiff heic heif avif svg raw cr2 nef arw dng ico')
add('video', 'mp4 mkv mov avi wmv webm m4v mpg mpeg ts m2ts flv 3gp')
add('audio', 'mp3 flac wav aac m4a ogg opus wma aiff alac')
add('archive', 'zip 7z rar tar gz tgz bz2 xz zst lz4 deb rpm')
add('diskImage', 'iso img qcow2 vmdk vdi vhd vhdx')
add('pdf', 'pdf')
add('document', 'doc docx odt rtf hwp hwpx pages')
add('spreadsheet', 'xls xlsx ods csv tsv numbers')
add('presentation', 'ppt pptx odp key')
add('text', 'txt log srt smi ass vtt ini conf cfg nfo')
add('markdown', 'md markdown')
add('web', 'html htm xml')
add('code', 'sh bash zsh py js mjs ts tsx jsx go rs c h cpp java kt rb php pl lua sql json yaml yml toml css')
add('executable', 'exe msi run appimage apk')

// extension은 이름의 확장자(소문자, 점 없음)다. 점으로 시작하는 숨김 파일 이름 자체는 확장자로 보지 않는다.
export function extension(name: string): string {
  const i = name.lastIndexOf('.')
  if (i <= 0 || i === name.length - 1) return ''
  return name.slice(i + 1).toLowerCase()
}

// kindOf는 목록 항목의 종류다.
export function kindOf(item: Pick<FileItem, 'name' | 'type' | 'locked' | 'linkType'>): Kind {
  switch (item.type) {
    case 'dir':
      return item.locked ? 'locked' : 'folder'
    case 'symlink':
      return item.linkType === 'dir' ? 'folder' : 'link'
    case 'other':
      return 'other'
    case 'unknown':
      return 'unknown'
  }
  return EXTENSIONS[extension(item.name)] ?? 'file'
}

export function kindIcon(kind: Kind): IconName {
  return KIND_ICONS[kind]
}

// isFolderLike는 들어갈 수 있는 항목(폴더, 폴더로 가는 링크)인지다.
export function isFolderLike(item: Pick<FileItem, 'type' | 'linkType'>): boolean {
  return item.type === 'dir' || (item.type === 'symlink' && item.linkType === 'dir')
}

// modeString은 권한 비트를 ls 형식("rw-r--r--")과 8진수로 보인다: "rw-r--r-- (644)".
export function modeString(mode: number): string {
  const bits = 'rwxrwxrwx'
  let s = ''
  for (let i = 0; i < 9; i++) s += mode & (1 << (8 - i)) ? bits[i] : '-'
  const chars = s.split('')
  const special = (pos: number, bit: number, set: string, unset: string) => {
    if (mode & bit) chars[pos] = chars[pos] === 'x' ? set : unset
  }
  special(2, 0o4000, 's', 'S')
  special(5, 0o2000, 's', 'S')
  special(8, 0o1000, 't', 'T')
  const octal = (mode & 0o7777).toString(8)
  return `${chars.join('')} (${mode & 0o7000 ? octal.padStart(4, '0') : octal.padStart(3, '0')})`
}
