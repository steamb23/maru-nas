// SPDX-License-Identifier: MIT
// maru-addon CLI의 동작(init, validate, pack). 진입은 maru-addon.mjs다. Node 내장 모듈만 쓴다(의존성 없음).
//
// 규칙의 원본은 대시보드(backend/internal/addonpkg)다. 상수는 대시보드가 만든 스키마 파일(schema/manifest.schema.json)의
// x-maru에서 읽고 여기에 다시 적지 않는다. CLI는 manifest의 구조(스키마)와 파일 배치만 본다. 필드 사이의 조건,
// 문구의 제어 문자 같은 나머지는 대시보드가 설치할 때(개발 중에는 MARU_DEV_ADDONS로 불러올 때) 본다.
import { createHash } from 'node:crypto'
import { existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { basename, dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gunzipSync, gzipSync } from 'node:zlib'

const here = dirname(fileURLToPath(import.meta.url))

/** SDK 패키지 루트(sdk/js). */
export const SDK_ROOT = resolve(here, '..')

/** CLI가 알리는 사용자 오류(사용법, 잘못된 입력). 스택 없이 메시지만 보인다. */
export class CliError extends Error {}

/**
 * 파일 목록: 패키지 안 경로(슬래시) → 내용.
 * @typedef {Map<string, Buffer>} Files
 */

/**
 * 검사 결과. problems가 있으면 대시보드가 받지 않는다. notes는 알려 줄 점이다.
 * @typedef {{ manifest: any, problems: string[], notes: string[] }} Report
 */

/** @type {any} */
let schemaCache = null

/** 스키마 파일(x-maru 포함)을 읽는다. @returns {any} */
export function loadSchema() {
  if (!schemaCache) schemaCache = JSON.parse(readFileSync(join(SDK_ROOT, 'schema', 'manifest.schema.json'), 'utf8'))
  return schemaCache
}

/** @returns {{ version: string }} */
function sdkPackage() {
  return JSON.parse(readFileSync(join(SDK_ROOT, 'package.json'), 'utf8'))
}

// ---------------------------------------------------------------------------------------------------------------
// 스키마 검사. 대시보드가 만드는 스키마가 쓰는 키워드만 다룬다(type, const, enum, pattern, min/maxLength,
// minimum/maximum, items, maxItems, uniqueItems, properties, required, additionalProperties, propertyNames,
// minProperties).

/** @param {unknown} v @param {string} type */
function typeOk(v, type) {
  switch (type) {
    case 'object':
      return typeof v === 'object' && v !== null && !Array.isArray(v)
    case 'array':
      return Array.isArray(v)
    case 'integer':
      return Number.isInteger(v)
    case 'string':
    case 'boolean':
    case 'number':
      return typeof v === type
  }
  return true
}

/** @param {string} path @param {string} key */
function child(path, key) {
  return path ? `${path}.${key}` : key
}

/**
 * value를 schema로 검사해 문제를 out에 더한다. path는 JSON 경로(views.window.entry)다.
 * @param {unknown} value
 * @param {any} schema
 * @param {string} [path]
 * @param {string[]} [out]
 * @returns {string[]}
 */
export function checkSchema(value, schema, path = '', out = []) {
  const at = path || 'manifest'
  if ('const' in schema && value !== schema.const) {
    out.push(`${at}: must be ${JSON.stringify(schema.const)}`)
    return out
  }
  if (schema.enum && !schema.enum.includes(value)) {
    out.push(`${at}: must be one of ${schema.enum.map((/** @type {unknown} */ e) => JSON.stringify(e)).join(', ')}`)
    return out
  }
  if (schema.type && !typeOk(value, schema.type)) {
    out.push(`${at}: must be ${schema.type === 'integer' || schema.type === 'object' || schema.type === 'array' ? 'an' : 'a'} ${schema.type}`)
    return out
  }
  if (typeof value === 'string') {
    const n = [...value].length
    if (schema.minLength !== undefined && n < schema.minLength) out.push(`${at}: must not be empty`)
    else if (schema.maxLength !== undefined && n > schema.maxLength) out.push(`${at}: must be at most ${schema.maxLength} characters`)
    else if (schema.pattern && !new RegExp(schema.pattern, 'u').test(value)) {
      out.push(schema.pattern === '\\S' ? `${at}: must not be blank` : `${at}: must match ${schema.pattern}`)
    }
  }
  if (typeof value === 'number') {
    if (schema.minimum !== undefined && value < schema.minimum) out.push(`${at}: must be at least ${schema.minimum}`)
    if (schema.maximum !== undefined && value > schema.maximum) out.push(`${at}: must be at most ${schema.maximum}`)
  }
  if (Array.isArray(value)) {
    if (schema.maxItems !== undefined && value.length > schema.maxItems) out.push(`${at}: at most ${schema.maxItems} items`)
    if (schema.uniqueItems) {
      const seen = new Set()
      value.forEach((item, i) => {
        const key = JSON.stringify(item)
        if (seen.has(key)) out.push(`${at}[${i}]: ${key} listed twice`)
        seen.add(key)
      })
    }
    if (schema.items) value.forEach((item, i) => checkSchema(item, schema.items, `${at}[${i}]`, out))
  }
  if (typeOk(value, 'object')) {
    const obj = /** @type {Record<string, unknown>} */ (value)
    const keys = Object.keys(obj)
    if (schema.minProperties !== undefined && keys.length < schema.minProperties) {
      out.push(`${at}: needs at least ${schema.minProperties} ${schema.minProperties === 1 ? 'entry' : 'entries'}`)
    }
    for (const r of schema.required ?? []) if (!(r in obj)) out.push(`${child(path, r)}: is required`)
    for (const k of keys) {
      const p = child(path, k)
      if (schema.propertyNames) {
        const before = out.length
        checkSchema(k, schema.propertyNames, p, out)
        if (out.length > before) continue
      }
      if (schema.properties && k in schema.properties) checkSchema(obj[k], schema.properties[k], p, out)
      else if (schema.additionalProperties === false) out.push(`${p}: unknown field`)
      else if (typeof schema.additionalProperties === 'object') checkSchema(obj[k], schema.additionalProperties, p, out)
    }
  }
  return out
}

// ---------------------------------------------------------------------------------------------------------------
// 파일 배치. 대시보드의 Read(압축 파일)·ReadDir(개발 디렉터리)와 같은 규칙이다.

/**
 * 풀린 패키지 디렉터리를 읽는다. 심볼릭 링크와 일반 파일이 아닌 항목은 문제로 남긴다. 최상위 dev/는 개발용이라 뺀다.
 * @param {string} dir
 * @returns {{ files: Files, dirs: string[], problems: string[] }}
 */
export function readPackageDir(dir) {
  const x = loadSchema()['x-maru'].archive
  /** @type {Files} */
  const files = new Map()
  /** @type {string[]} */
  const dirs = []
  /** @type {string[]} */
  const problems = []
  /** @param {string} abs @param {string} rel */
  const walk = (abs, rel) => {
    for (const name of readdirSync(abs).sort()) {
      const r = rel ? `${rel}/${name}` : name
      if (r === x.devDir) continue
      const st = lstatSync(join(abs, name))
      if (st.isSymbolicLink()) problems.push(`${r}: symbolic links are not allowed`)
      else if (st.isDirectory()) {
        dirs.push(r)
        walk(join(abs, name), r)
      } else if (st.isFile()) files.set(r, readFileSync(join(abs, name)))
      else problems.push(`${r}: only regular files and directories are allowed`)
    }
  }
  if (!existsSync(dir) || !lstatSync(dir).isDirectory()) throw new CliError(`${dir}: not a directory`)
  walk(dir, '')
  return { files, dirs, problems }
}

/** @param {Buffer} h @param {number} off @param {number} len */
function field(h, off, len) {
  const end = h.indexOf(0, off)
  return h.toString('utf8', off, end < 0 || end > off + len ? off + len : end)
}

/** @param {Buffer} h @param {number} off @param {number} len */
function octal(h, off, len) {
  const s = field(h, off, len).trim()
  return s === '' ? 0 : parseInt(s, 8)
}

/**
 * .maru-addon(gzip tar)을 읽는다. 대시보드가 받지 않을 항목은 문제로 남긴다.
 * @param {Buffer} data
 * @returns {{ files: Files, dirs: string[], problems: string[], entries: number }}
 */
export function readArchive(data) {
  const lim = loadSchema()['x-maru'].archive.limits
  /** @type {string[]} */
  const problems = []
  if (data.length > lim.compressed) problems.push(`archive is larger than ${lim.compressed} bytes`)
  /** @type {Buffer} */
  let tar
  try {
    tar = gunzipSync(data, { maxOutputLength: lim.total + lim.entries * 1024 + 1024 * 1024 })
  } catch (err) {
    throw new CliError(`not a gzip file or too large when unpacked: ${/** @type {Error} */ (err).message}`)
  }
  /** @type {Files} */
  const files = new Map()
  /** @type {string[]} */
  const dirs = []
  let entries = 0
  /** @type {string | null} */
  let longName = null
  for (let off = 0; off + 512 <= tar.length; ) {
    const h = tar.subarray(off, off + 512)
    if (h.every((b) => b === 0)) break
    let sum = 0
    for (let i = 0; i < 512; i++) sum += i >= 148 && i < 156 ? 0x20 : h[i]
    if (sum !== octal(h, 148, 8)) throw new CliError(`tar header at ${off}: bad checksum`)
    const size = octal(h, 124, 12)
    const type = String.fromCharCode(h[156] || 0x30)
    const body = tar.subarray(off + 512, off + 512 + size)
    off += 512 + Math.ceil(size / 512) * 512
    const prefix = field(h, 345, 155)
    let name = longName ?? (prefix ? `${prefix}/${field(h, 0, 100)}` : field(h, 0, 100))
    longName = null
    if (type === 'x') {
      // pax 확장 머리: path만 다음 항목 이름으로 쓴다.
      const m = /(?:^|\n)\d+ path=([^\n]*)\n/.exec(body.toString('utf8'))
      if (m) longName = m[1]
      continue
    }
    if (type === 'L') {
      longName = body.toString('utf8').replace(/\0+$/, '')
      continue
    }
    if (type === 'g') continue
    entries++
    name = name.replace(/^\.\//, '').replace(/\/$/, '')
    if (type === '5') dirs.push(name)
    else if (type === '0') {
      if (files.has(name)) problems.push(`${name}: listed twice`)
      files.set(name, Buffer.from(body))
    } else problems.push(`${name}: only regular files and directories are allowed`)
  }
  return { files, dirs, problems, entries }
}

/**
 * 파일 이름이 받을 수 있는 자리인지와 크기 상한이다. 받을 수 없으면 문제 문장이다.
 * @param {string} name
 * @returns {number | string}
 */
function fileLimit(name) {
  const x = loadSchema()['x-maru']
  const a = x.archive
  if (name === 'manifest.json') return a.limits.manifest
  if (name === 'icon.svg') return a.limits.icon
  if (name === 'LICENSE' || name === 'README.md') return a.limits.text
  if (name.startsWith('ui/')) {
    if (name.split('/')[1] === a.reservedUI) return `${name}: ui/${a.reservedUI} is reserved`
    return a.limits.uiFile
  }
  if (name.startsWith('i18n/')) {
    const lang = name.slice(5).replace(/\.json$/, '')
    if (!name.endsWith('.json') || lang.includes('/') || !x.i18n.languages.includes(lang)) {
      return `${name}: must be i18n/<supported language>.json (${x.i18n.languages.join(', ')})`
    }
    return a.limits.i18n
  }
  if (name.startsWith('backend/')) {
    if (!a.backendFiles.includes(name.slice(8))) {
      return `${name}: must be ${a.backendFiles.map((/** @type {string} */ f) => `backend/${f}`).join(' or ')}`
    }
    return a.limits.backend
  }
  return `${name}: not allowed at the top level (allowed: ${[...a.topFiles, ...a.topDirs.map((/** @type {string} */ d) => `${d}/`)].join(', ')})`
}

/** 경로가 형식에 맞는지다. @param {string} name @returns {string | null} */
function pathProblem(name) {
  const a = loadSchema()['x-maru'].archive
  if (name.startsWith('/') || name.includes('\\') || name.includes('\0')) return `${JSON.stringify(name)}: invalid path`
  const parts = name.split('/')
  if (parts.length > a.maxDepth) return `${JSON.stringify(name)}: deeper than ${a.maxDepth} levels`
  const seg = new RegExp(a.segmentPattern)
  const bad = parts.find((p) => !seg.test(p))
  if (bad !== undefined) return `${JSON.stringify(name)}: invalid path segment ${JSON.stringify(bad)} (start with a letter or digit, then up to 99 of A-Z a-z 0-9 . _ -)`
  return null
}

/** 디렉터리가 받을 수 있는 자리인지다. @param {string} name @returns {string | null} */
function dirProblem(name) {
  const a = loadSchema()['x-maru'].archive
  const [top, ...rest] = name.split('/')
  if (top === 'ui') return rest[0] === a.reservedUI ? `${name}: ui/${a.reservedUI} is reserved` : null
  if (top === 'i18n' || top === 'backend') return rest.length > 0 ? `${name}: no subdirectories allowed` : null
  return `${name}: not allowed at the top level`
}

// ELF 기계 형식(e_machine)
const ELF_MACHINES = /** @type {Record<string, number>} */ ({ 'linux-amd64': 62, 'linux-arm64': 183 })

/** 백엔드 파일이 그 아키텍처의 64비트 리눅스 실행 파일인지다. @param {string} name @param {Buffer} b */
function elfProblem(name, b) {
  const file = name.slice('backend/'.length)
  if (b.length < 64 || b.readUInt32BE(0) !== 0x7f454c46) return `${name}: not an ELF executable`
  if (b[4] !== 2 || b[5] !== 1) return `${name}: not a 64-bit little-endian executable`
  if (b[7] !== 0 && b[7] !== 3) return `${name}: not a Linux executable`
  const type = b.readUInt16LE(16)
  if (type !== 2 && type !== 3) return `${name}: not an executable`
  if (b.readUInt16LE(18) !== ELF_MACHINES[file]) return `${name}: not a ${file} executable`
  return null
}

/**
 * 파일 목록을 대시보드의 규칙으로 검사한다. packed면 압축 파일(설치), 아니면 개발 디렉터리(MARU_DEV_ADDONS)다.
 * 개발 디렉터리의 백엔드 패키지는 실행 파일이 없어도 된다(개발 소켓으로 따로 띄운다).
 * @param {Files} files
 * @param {{ dirs?: string[], packed?: boolean, entries?: number }} [opts]
 * @returns {Report}
 */
export function checkPackage(files, opts = {}) {
  const schema = loadSchema()
  const x = schema['x-maru']
  const lim = x.archive.limits
  /** @type {string[]} */
  const problems = []
  /** @type {string[]} */
  const notes = []
  const dirs = opts.dirs ?? []

  const entries = opts.entries ?? files.size + dirs.length
  if (entries > lim.entries) problems.push(`more than ${lim.entries} entries`)
  for (const d of dirs) {
    const p = pathProblem(d) ?? dirProblem(d)
    if (p) problems.push(p)
  }
  let total = 0
  for (const [name, body] of files) {
    const p = pathProblem(name)
    if (p) {
      problems.push(p)
      continue
    }
    const max = fileLimit(name)
    if (typeof max === 'string') problems.push(max)
    else if (body.length > max) problems.push(`${name}: larger than ${max} bytes`)
    total += body.length
  }
  if (total > lim.total) problems.push(`unpacked size is larger than ${lim.total} bytes`)

  /** @type {any} */
  let manifest = null
  const raw = files.get('manifest.json')
  if (!raw) problems.push('manifest.json is missing')
  else {
    try {
      manifest = JSON.parse(raw.toString('utf8'))
    } catch (err) {
      problems.push(`manifest.json: not valid JSON: ${/** @type {Error} */ (err).message}`)
    }
  }
  if (manifest !== null) {
    checkSchema(manifest, schema, '', problems)
    if (typeOk(manifest, 'object')) {
      const need = ['icon.svg', `i18n/${x.i18n.fallback}.json`]
      for (const v of ['window', 'widget']) {
        const entry = manifest.views?.[v]?.entry
        if (typeof entry === 'string') need.push(`ui/${entry}`)
      }
      for (const f of need) if (!files.has(f)) problems.push(`${f} is missing`)
      if (Number.isInteger(manifest.sdk) && manifest.sdk > x.sdk) {
        notes.push(`sdk ${manifest.sdk} is newer than this SDK knows (${x.sdk}); older dashboards will not install it`)
      }
    }
  }

  const icon = files.get('icon.svg')
  if (icon && !icon.subarray(0, 1024).toString('utf8').toLowerCase().includes('<svg')) problems.push('icon.svg is not an SVG image')

  const keyPattern = new RegExp(x.i18n.keyPattern)
  for (const [name, body] of files) {
    if (!name.startsWith('i18n/') || typeof fileLimit(name) === 'string') continue
    /** @type {unknown} */
    let table
    try {
      table = JSON.parse(body.toString('utf8'))
    } catch {
      problems.push(`${name}: not valid JSON`)
      continue
    }
    if (!typeOk(table, 'object') || !Object.values(/** @type {object} */ (table)).every((v) => typeof v === 'string')) {
      problems.push(`${name}: must be a JSON object of strings`)
      continue
    }
    const entriesOf = Object.entries(/** @type {Record<string, string>} */ (table))
    if (entriesOf.length > x.i18n.maxKeys) problems.push(`${name}: more than ${x.i18n.maxKeys} keys`)
    for (const [k, v] of entriesOf) {
      if (!keyPattern.test(k)) problems.push(`${name}: invalid key ${JSON.stringify(k)}`)
      else if (Buffer.byteLength(v) > x.i18n.maxValueBytes) problems.push(`${name}: value of ${JSON.stringify(k)} is longer than ${x.i18n.maxValueBytes} bytes`)
    }
  }

  const backends = [...files.keys()].filter((n) => n.startsWith('backend/') && typeof fileLimit(n) !== 'string')
  for (const n of backends) {
    const p = elfProblem(n, /** @type {Buffer} */ (files.get(n)))
    if (p) problems.push(p)
  }
  if (manifest && typeOk(manifest, 'object')) {
    if (manifest.backend === undefined && backends.length > 0) {
      problems.push('backend files are present but the manifest declares no backend')
    }
    if (manifest.backend !== undefined && backends.length === 0) {
      const want = x.archive.backendFiles.map((/** @type {string} */ f) => `backend/${f}`).join(' or ')
      if (opts.packed) problems.push(`the manifest declares a backend but there is no executable (${want})`)
      else notes.push(`no backend executable (${want}); fine for MARU_DEV_ADDONS, required in a package file`)
    }
  }
  return { manifest, problems, notes }
}

/**
 * 디렉터리 또는 .maru-addon 파일을 검사한다.
 * @param {string} target
 * @returns {Report}
 */
export function validate(target) {
  if (!existsSync(target)) throw new CliError(`${target}: not found`)
  if (lstatSync(target).isDirectory()) {
    const { files, dirs, problems } = readPackageDir(target)
    const r = checkPackage(files, { dirs })
    return { ...r, problems: [...problems, ...r.problems] }
  }
  const { files, dirs, problems, entries } = readArchive(readFileSync(target))
  const r = checkPackage(files, { dirs, packed: true, entries })
  return { ...r, problems: [...problems, ...r.problems] }
}

// ---------------------------------------------------------------------------------------------------------------
// 묶기. 같은 파일이면 같은 tar가 나오게 한다: 경로 순서, 수정 시각 0, 소유자 0, 정한 권한(백엔드 0755, 나머지 0644).
// gzip은 zlib 버전에 따라 바이트가 다를 수 있으므로 sha256(설치 화면에 보이는 값)은 기기마다 다를 수 있다.

/** @param {Buffer} h @param {number} off @param {number} len @param {string} s */
function putString(h, off, len, s) {
  const b = Buffer.from(s, 'utf8')
  if (b.length > len) throw new CliError(`tar field too long: ${s}`)
  b.copy(h, off)
}

/** @param {Buffer} h @param {number} off @param {number} len @param {number} n */
function putOctal(h, off, len, n) {
  putString(h, off, len, `${n.toString(8).padStart(len - 1, '0')}\0`)
}

/** ustar 머리 하나. 100바이트가 넘는 경로는 prefix에 나눠 넣는다. @param {string} name @param {number} size @param {number} mode */
function tarHeader(name, size, mode) {
  const h = Buffer.alloc(512)
  let prefix = ''
  let base = name
  if (Buffer.byteLength(name) > 100) {
    // 이름 칸(100바이트)에 들어가는 가장 긴 뒷부분을 찾고, 앞부분은 prefix 칸(155바이트)에 둔다.
    let i = -1
    for (let j = name.indexOf('/'); j >= 0; j = name.indexOf('/', j + 1)) {
      if (Buffer.byteLength(name.slice(j + 1)) <= 100) {
        i = j
        break
      }
    }
    if (i < 0 || Buffer.byteLength(name.slice(0, i)) > 155) throw new CliError(`${name}: path is too long for a package`)
    prefix = name.slice(0, i)
    base = name.slice(i + 1)
  }
  putString(h, 0, 100, base)
  putOctal(h, 100, 8, mode)
  putOctal(h, 108, 8, 0)
  putOctal(h, 116, 8, 0)
  putOctal(h, 124, 12, size)
  putOctal(h, 136, 12, 0)
  h.fill(0x20, 148, 156)
  h[156] = 0x30
  putString(h, 257, 6, 'ustar\0')
  putString(h, 263, 2, '00')
  putOctal(h, 329, 8, 0)
  putOctal(h, 337, 8, 0)
  putString(h, 345, 155, prefix)
  let sum = 0
  for (const b of h) sum += b
  putString(h, 148, 8, `${sum.toString(8).padStart(6, '0')}\0 `)
  return h
}

/**
 * 파일 목록을 tar로 묶는다(경로 바이트 순).
 * @param {Files} files
 * @returns {Buffer}
 */
export function packTar(files) {
  /** @type {Buffer[]} */
  const parts = []
  const names = [...files.keys()].sort((a, b) => Buffer.compare(Buffer.from(a), Buffer.from(b)))
  for (const name of names) {
    const body = /** @type {Buffer} */ (files.get(name))
    parts.push(tarHeader(name, body.length, name.startsWith('backend/') ? 0o755 : 0o644), body)
    const pad = (512 - (body.length % 512)) % 512
    if (pad) parts.push(Buffer.alloc(pad))
  }
  parts.push(Buffer.alloc(1024))
  return Buffer.concat(parts)
}

/**
 * tar를 gzip으로 감싼다. 머리의 수정 시각은 0, 운영체제 바이트는 유닉스(3)로 둔다.
 * @param {Buffer} tar
 * @returns {Buffer}
 */
export function gzipTar(tar) {
  const gz = gzipSync(tar, { level: 9 })
  gz.writeUInt32LE(0, 4)
  gz[9] = 3
  return gz
}

/**
 * 패키지 디렉터리를 .maru-addon으로 묶는다. 묶기 전에 검사하고, 문제가 있으면 묶지 않는다.
 * @param {string} dir
 * @param {{ out?: string }} [opts]
 * @returns {{ out: string, sha256: string, rev: string, size: number, report: Report }}
 */
export function pack(dir, opts = {}) {
  const { files, dirs, problems } = readPackageDir(dir)
  const report = checkPackage(files, { dirs, packed: true })
  report.problems.unshift(...problems)
  if (report.problems.length > 0) return { out: '', sha256: '', rev: '', size: 0, report }
  const gz = gzipTar(packTar(files))
  const lim = loadSchema()['x-maru'].archive.limits
  if (gz.length > lim.compressed) {
    report.problems.push(`package is larger than ${lim.compressed} bytes`)
    return { out: '', sha256: '', rev: '', size: 0, report }
  }
  const out = resolve(opts.out ?? `${report.manifest.id}-${report.manifest.version}.maru-addon`)
  writeFileSync(out, gz)
  const sha256 = createHash('sha256').update(gz).digest('hex')
  return { out, sha256, rev: `${report.manifest.version}-${sha256.slice(0, 8)}`, size: gz.length, report }
}

// ---------------------------------------------------------------------------------------------------------------
// 새 패키지 만들기.

/** 저장소 체크아웃 안의 SDK인지다(npm으로 받은 것이 아님). */
function inCheckout() {
  return existsSync(join(SDK_ROOT, '..', '..', 'backend', 'go.mod'))
}

/** @param {string} id */
function titleOf(id) {
  return id
    .split('-')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ')
}

/**
 * 템플릿 디렉터리를 복사하며 자리표시자를 바꾼다. _gitignore는 .gitignore로 바꾼다(npm이 .gitignore를 빼고 배포한다).
 * @param {string} from
 * @param {string} to
 * @param {Record<string, string>} vars
 * @returns {string[]}
 */
function copyTemplate(from, to, vars) {
  /** @type {string[]} */
  const written = []
  /** @param {string} src @param {string} dst */
  const walk = (src, dst) => {
    mkdirSync(dst, { recursive: true })
    for (const name of readdirSync(src).sort()) {
      const s = join(src, name)
      const d = join(dst, name === '_gitignore' ? '.gitignore' : name)
      if (lstatSync(s).isDirectory()) walk(s, d)
      else {
        let text = readFileSync(s, 'utf8')
        for (const [k, v] of Object.entries(vars)) text = text.replaceAll(`@@${k}@@`, v)
        writeFileSync(d, text)
        written.push(relative(to, d))
      }
    }
  }
  walk(from, to)
  return written
}

/**
 * 새 패키지 디렉터리를 만든다. 기본은 빌드 없는 JS, ts면 TypeScript + Vite 프로젝트다.
 * @param {string} target
 * @param {{ id?: string, ts?: boolean, backend?: boolean }} [opts]
 * @returns {{ dir: string, id: string, files: string[], packageDir: string }}
 */
export function init(target, opts = {}) {
  if (opts.backend) throw new CliError('packages with a backend are not supported by this version of the SDK yet')
  const dir = resolve(target)
  if (existsSync(dir) && readdirSync(dir).length > 0) throw new CliError(`${target}: directory is not empty`)
  const schema = loadSchema()
  const id = opts.id ?? basename(dir).toLowerCase()
  const idProblems = checkSchema(id, schema.properties.id, 'id')
  if (idProblems.length > 0) {
    throw new CliError(`${idProblems[0]}; pass --id <id> (lowercase letters, digits and '-', at most ${schema.properties.id.maxLength} characters)`)
  }
  const kind = opts.ts ? 'ts' : 'plain'
  const packageDir = opts.ts ? join(dir, 'addon') : dir
  // 편집기가 쓰는 스키마 주소. TS 프로젝트는 설치한 SDK의 파일, 저장소 체크아웃 안이면 그 파일, 아니면 공개 주소다.
  const repo = resolve(SDK_ROOT, '..', '..')
  const insideRepo = inCheckout() && !relative(repo, packageDir).startsWith('..')
  const schemaPath = opts.ts
    ? '../node_modules/@maru-nas/addon-sdk/schema/manifest.schema.json'
    : insideRepo
      ? relative(packageDir, join(SDK_ROOT, 'schema', 'manifest.schema.json'))
      : schema.$id
  const vars = {
    ID: id,
    NAME: titleOf(id),
    SCHEMA: schemaPath.split('\\').join('/'),
    SDK_LEVEL: String(schema['x-maru'].sdk),
    // 공개 전에는 npm에 없으므로 저장소 체크아웃의 sdk/js를 가리킨다(저장소 안이면 상대 경로, 밖이면 절대 경로).
    SDK_DEP: !inCheckout()
      ? `^${sdkPackage().version}`
      : `file:${(relative(repo, dir).startsWith('..') ? SDK_ROOT : relative(dir, SDK_ROOT)).split('\\').join('/')}`,
  }
  const files = copyTemplate(join(here, 'templates', kind), dir, vars)
  return { dir, id, files, packageDir }
}
