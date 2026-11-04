// SDK CLI(sdk/js/cli, maru-addon)의 검사·묶기. 같은 시험 벡터와 고정 파일을 Go 테스트도 쓴다
// (backend/internal/addonpkg/format/vectors_test.go, backend/internal/addonpkg/fixture_test.go).
import { mkdtempSync, readFileSync, readdirSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { gunzipSync } from 'node:zlib'
import { afterAll, describe, expect, it } from 'vitest'
import {
  checkPackage,
  checkSchema,
  gzipTar,
  init,
  loadSchema,
  pack,
  packTar,
  readArchive,
  readPackageDir,
  validate,
} from '../../../sdk/js/cli/lib.mjs'

const SDK = new URL('../../../sdk/', import.meta.url).pathname
const VECTORS = join(SDK, 'testdata/manifests')
const DEV_MIN = join(SDK, 'testdata/dev-min')
const FIXTURE = join(SDK, 'testdata/packed/dev-min.maru-addon')

const tmp = mkdtempSync(join(tmpdir(), 'maru-addon-cli-'))
afterAll(() => rmSync(tmp, { recursive: true, force: true }))

function vectors(kind: string): [string, unknown][] {
  const dir = join(VECTORS, kind)
  return readdirSync(dir)
    .filter((f) => f.endsWith('.json'))
    .map((f) => [f, JSON.parse(readFileSync(join(dir, f), 'utf8'))])
}

// files는 개발 최소 패키지를 바탕으로 한 파일 목록이다. over로 바꾸거나(null이면 뺀다) 더한다.
function files(over: Record<string, string | Buffer | null> = {}): Map<string, Buffer> {
  const { files: base } = readPackageDir(DEV_MIN)
  const out = new Map<string, Buffer>(base)
  for (const [k, v] of Object.entries(over)) {
    if (v === null) out.delete(k)
    else out.set(k, Buffer.isBuffer(v) ? v : Buffer.from(v))
  }
  return out
}

function manifestWith(patch: Record<string, unknown>): string {
  const m = JSON.parse(readFileSync(join(DEV_MIN, 'manifest.json'), 'utf8'))
  return JSON.stringify({ ...m, ...patch })
}

describe('스키마 벡터', () => {
  const schema = loadSchema()

  it('valid는 통과한다', () => {
    for (const [name, m] of vectors('valid')) expect(checkSchema(m, schema), name).toEqual([])
  })

  it('invalid는 거절한다', () => {
    for (const [name, m] of vectors('invalid')) expect(checkSchema(m, schema).length, name).toBeGreaterThan(0)
  })

  // 대시보드만 거절하는 경우(필드 사이의 조건, 문구의 제어 문자 등). CLI가 이것까지 막게 되면 벡터를 invalid로 옮긴다.
  it('server-only는 스키마로는 통과한다', () => {
    for (const [name, m] of vectors('server-only')) expect(checkSchema(m, schema), name).toEqual([])
  })

  it('문제에 JSON 경로를 붙인다', () => {
    const m = JSON.parse(readFileSync(join(VECTORS, 'invalid/bad-entry.json'), 'utf8'))
    expect(checkSchema(m, schema)).toEqual([expect.stringMatching(/^views\.window\.entry: must match /)])
  })
})

describe('checkPackage', () => {
  it('개발 최소 패키지는 문제가 없다', () => {
    const { files: f, dirs, problems } = readPackageDir(DEV_MIN)
    expect(problems).toEqual([])
    expect(checkPackage(f, { dirs }).problems).toEqual([])
  })

  it('받지 않는 자리와 이름을 거절한다', () => {
    const cases: [string, RegExp][] = [
      // _maru는 경로 조각 규칙(글자나 숫자로 시작)에서 먼저 걸린다. 대시보드도 같은 순서다.
      ['ui/_maru/sdk.js', /invalid path segment "_maru"/],
      ['notes.txt', /not allowed at the top level/],
      ['ui/.hidden.js', /invalid path segment/],
      ['i18n/xx.json', /supported language/],
      ['i18n/sub/en.json', /supported language/],
      ['backend/linux-x86', /backend\/linux-amd64 or backend\/linux-arm64/],
    ]
    for (const [name, want] of cases) {
      expect(checkPackage(files({ [name]: 'x' })).problems, name).toEqual(expect.arrayContaining([expect.stringMatching(want)]))
    }
  })

  it('manifest가 가리키는 파일과 번역·아이콘 형식을 본다', () => {
    expect(checkPackage(files({ 'ui/window.html': null })).problems).toContain('ui/window.html is missing')
    expect(checkPackage(files({ 'i18n/en.json': null })).problems).toContain('i18n/en.json is missing')
    expect(checkPackage(files({ 'icon.svg': '<png>' })).problems).toContain('icon.svg is not an SVG image')
    expect(checkPackage(files({ 'i18n/ko.json': '{"a": 1}' })).problems).toContain('i18n/ko.json: must be a JSON object of strings')
    expect(checkPackage(files({ 'i18n/ko.json': '{"bad key": "x"}' })).problems).toEqual([
      'i18n/ko.json: invalid key "bad key"',
    ])
  })

  it('크기 상한을 넘으면 거절한다', () => {
    const big = Buffer.alloc(loadSchema()['x-maru'].archive.limits.icon + 1, 0x20)
    big.write('<svg')
    expect(checkPackage(files({ 'icon.svg': big })).problems).toEqual([expect.stringMatching(/^icon\.svg: larger than/)])
  })

  it('백엔드 선언과 실행 파일이 맞아야 한다', () => {
    const withBackend = files({ 'manifest.json': manifestWith({ backend: { network: [] } }) })
    // 개발 디렉터리는 실행 파일이 없어도 되고, 묶은 파일은 있어야 한다.
    expect(checkPackage(withBackend).problems).toEqual([])
    expect(checkPackage(withBackend).notes).toEqual([expect.stringMatching(/no backend executable/)])
    expect(checkPackage(withBackend, { packed: true }).problems).toEqual([expect.stringMatching(/no executable/)])
    expect(checkPackage(files({ 'backend/linux-amd64': '#!/bin/sh\n' })).problems).toEqual(
      expect.arrayContaining(['backend/linux-amd64: not an ELF executable', 'backend files are present but the manifest declares no backend']),
    )
  })
})

describe('SDK 예제', () => {
  it('sdk/examples의 예제는 검사를 통과한다', () => {
    const dir = join(SDK, 'examples')
    const examples = readdirSync(dir).filter((d) => !d.startsWith('.'))
    expect(examples.length).toBeGreaterThan(0)
    for (const ex of examples) {
      const r = validate(join(dir, ex))
      expect(r.problems, ex).toEqual([])
      expect(r.notes, ex).toEqual([])
    }
  })
})

describe('pack', () => {
  it('고정 파일과 같은 tar를 만든다(gzip 바이트는 zlib 버전에 따라 다를 수 있다)', () => {
    const { files: f } = readPackageDir(DEV_MIN)
    expect(packTar(f).equals(gunzipSync(readFileSync(FIXTURE)))).toBe(true)
  })

  it('같은 입력이면 같은 파일이다', () => {
    const { files: f } = readPackageDir(DEV_MIN)
    expect(gzipTar(packTar(f)).equals(gzipTar(packTar(f)))).toBe(true)
  })

  it('묶은 파일을 다시 읽으면 같은 파일 목록이다', () => {
    const out = join(tmp, 'dev-min.maru-addon')
    const r = pack(DEV_MIN, { out })
    expect(r.report.problems).toEqual([])
    expect(r.rev).toBe(`0.1.0-${r.sha256.slice(0, 8)}`)
    const back = readArchive(readFileSync(out))
    expect(back.problems).toEqual([])
    expect([...back.files.keys()].sort()).toEqual([...readPackageDir(DEV_MIN).files.keys()].sort())
    expect(validate(out).problems).toEqual([])
  })

  it('100바이트가 넘는 경로는 ustar prefix에 나눠 넣는다', () => {
    const long = `ui/${'a'.repeat(60)}/${'b'.repeat(60)}/c.js`
    const back = readArchive(gzipTar(packTar(files({ [long]: 'x' }))))
    expect(back.files.get(long)?.toString()).toBe('x')
  })

  it('문제가 있으면 묶지 않는다', () => {
    const r = pack(join(SDK, 'testdata'), { out: join(tmp, 'never.maru-addon') })
    expect(r.out).toBe('')
    expect(r.report.problems.length).toBeGreaterThan(0)
  })
})

describe('init', () => {
  it('빌드 없는 JS 템플릿은 바로 검사를 통과한다', () => {
    const r = init(join(tmp, 'my-addon'))
    expect(r.id).toBe('my-addon')
    const report = validate(r.packageDir)
    expect(report.problems).toEqual([])
    expect(report.manifest.name.en).toBe('My Addon')
  })

  it('TypeScript 템플릿은 addon/ 아래에 패키지를 둔다', () => {
    const r = init(join(tmp, 'ts-addon'), { ts: true })
    expect(r.files).toEqual(expect.arrayContaining(['package.json', 'vite.config.ts', '.gitignore', 'addon/manifest.json']))
    // ui/는 빌드 결과라 아직 없다.
    expect(validate(r.packageDir).problems).toEqual(['ui/window.html is missing'])
  })

  it('id 형식이 틀리거나 디렉터리가 비어 있지 않으면 만들지 않는다', () => {
    expect(() => init(join(tmp, 'Bad_Name'))).toThrow(/id: must match/)
    expect(() => init(join(tmp, 'my-addon'))).toThrow(/not empty/)
    expect(() => init(join(tmp, 'other'), { backend: true })).toThrow(/backend/)
  })
})
