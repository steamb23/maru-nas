// SDK CLI(sdk/js/cli, maru-addon)의 검사·묶기. 같은 시험 벡터와 고정 파일을 Go 테스트도 쓴다
// (backend/internal/addonpkg/format/vectors_test.go, backend/internal/addonpkg/fixture_test.go).
import { execFileSync } from 'node:child_process'
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { delimiter, join } from 'node:path'
import { gunzipSync } from 'node:zlib'
import { afterAll, describe, expect, it } from 'vitest'
import {
  checkPackage,
  checkSchema,
  goModuleDeps,
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
  // 백엔드 예제의 실행 파일은 make sdk-examples가 만들고 커밋하지 않는다. 새로 받은 체크아웃에도 개발 디렉터리로는
  // 통과해야 하므로 "실행 파일 없음" 알림만 받는다.
  const NO_EXECUTABLE = /^no backend executable/

  it('sdk/examples의 예제와 시험 패키지(sdk/testdata/probe)는 검사를 통과한다', () => {
    const dir = join(SDK, 'examples')
    const examples = readdirSync(dir)
      .filter((d) => !d.startsWith('.'))
      .map((d) => join(dir, d))
    expect(examples.length).toBeGreaterThan(0)
    for (const ex of [...examples, join(SDK, 'testdata/probe')]) {
      const r = validate(ex)
      expect(r.problems, ex).toEqual([])
      // make sdk-examples가 실패하면 빈 backend/만 남을 수 있으므로 실행 파일이 있는지로 본다.
      const built = existsSync(join(ex, 'backend')) && readdirSync(join(ex, 'backend')).some((f) => f.startsWith('linux-'))
      if (r.manifest.backend && !built) expect(r.notes, ex).toEqual([expect.stringMatching(NO_EXECUTABLE)])
      else expect(r.notes, ex).toEqual([])
    }
  })

  // 백엔드 예제의 Go 소스는 sdk/go/examples/<id>에 둔다(make sdk-examples, scripts/dev.sh의 규칙).
  it('백엔드가 있는 예제는 sdk/go/examples에 같은 id의 Go 패키지가 있다', () => {
    const goExamples = readdirSync(join(SDK, 'go/examples'))
    for (const ex of goExamples) {
      const ui = [join(SDK, 'examples', ex), join(SDK, 'testdata', ex)].find((d) => existsSync(join(d, 'manifest.json')))
      expect(ui, ex).toBeDefined()
      const m = JSON.parse(readFileSync(join(ui as string, 'manifest.json'), 'utf8'))
      expect(m.id, ex).toBe(ex)
      expect(m.backend, ex).toBeDefined()
    }
    for (const ex of readdirSync(join(SDK, 'examples'))) {
      const m = JSON.parse(readFileSync(join(SDK, 'examples', ex, 'manifest.json'), 'utf8'))
      if (m.backend) expect(goExamples, ex).toContain(ex)
    }
  })
})

describe('개발 디렉터리의 dev/', () => {
  // dev/(가짜 OMV 응답 dev/fake-rpc.json 등)는 개발용이라 검사하지 않고 묶지 않는다. 대시보드의 ReadDir도 같다.
  it('validate와 pack은 dev/를 건너뛴다', () => {
    const dir = join(tmp, 'with-dev')
    cpSync(DEV_MIN, dir, { recursive: true })
    mkdirSync(join(dir, 'dev', 'nested'), { recursive: true })
    writeFileSync(join(dir, 'dev', 'fake-rpc.json'), '{"Example.get": {"result": 1}}')
    writeFileSync(join(dir, 'dev', 'nested', 'notes.txt'), 'not part of the package')
    expect(validate(dir).problems).toEqual([])
    const out = join(tmp, 'with-dev.maru-addon')
    expect(pack(dir, { out }).report.problems).toEqual([])
    const names = [...readArchive(readFileSync(out)).files.keys()]
    expect(names.filter((n) => n.startsWith('dev'))).toEqual([])
    expect(names.sort()).toEqual([...readPackageDir(DEV_MIN).files.keys()].sort())
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
    expect(() => init(join(tmp, 'my-addon'), { backend: true })).toThrow(/not empty/)
  })

  it('--backend는 addon/ 패키지와 backend/ Go 모듈을 만든다', () => {
    const r = init(join(tmp, 'be-addon'), { backend: true })
    expect(r.packageDir).toBe(join(r.dir, 'addon'))
    expect(r.files).toEqual([
      '.gitignore',
      'README.md',
      'addon/i18n/en.json',
      'addon/icon.svg',
      'addon/manifest.json',
      'addon/ui/window.html',
      'addon/ui/window.js',
      'backend/go.mod',
      'backend/main.go',
    ])
    const report = validate(r.packageDir)
    expect(report.problems).toEqual([])
    expect(report.notes).toEqual([expect.stringMatching(/^no backend executable/)])
    expect(report.manifest.backend).toEqual({ network: [] })
    expect(report.manifest.actions.map((a: { name: string }) => a.name)).toEqual(['reset'])
    // 실행 파일이 없으면 묶지 않는다.
    expect(pack(r.packageDir, { out: join(tmp, 'never-be.maru-addon') }).report.problems).toEqual([
      expect.stringMatching(/declares a backend but there is no executable/),
    ])
    // 자리표시자가 남지 않는다.
    for (const f of r.files) expect(readFileSync(join(r.dir, f), 'utf8'), f).not.toMatch(/@@[A-Z_]+@@/)
    // 저장소 밖에 만들었으므로 Go SDK를 절대 경로로 가리킨다.
    const goMod = readFileSync(join(r.dir, 'backend/go.mod'), 'utf8')
    expect(goMod).toContain('module be-addon/backend\n\ngo 1.23\n')
    expect(goMod).toContain(`replace github.com/steamb23/maru-nas/sdk/go => ${join(SDK, 'go')}\n`)
  })

  it('--backend --ts는 TypeScript 프로젝트에 backend/를 더한다', () => {
    const r = init(join(tmp, 'ts-be'), { backend: true, ts: true })
    expect(r.files).toEqual(
      expect.arrayContaining(['package.json', 'vite.config.ts', 'src/window.ts', 'addon/manifest.json', 'backend/go.mod', 'backend/main.go']),
    )
    expect(readFileSync(join(r.dir, '.gitignore'), 'utf8')).toContain('addon/backend/')
    expect(readFileSync(join(r.dir, 'src/window.ts'), 'utf8')).toContain("app.action('reset')")
    // ui/는 빌드 결과라 아직 없다.
    expect(validate(r.packageDir).problems).toEqual(['ui/window.html is missing'])
    expect(validate(r.packageDir).manifest.backend).toEqual({ network: [] })
  })

  it('Go SDK replace 경로는 저장소 안이면 상대 경로, 밖이면 절대 경로다', () => {
    const repo = join(SDK, '..')
    expect(goModuleDeps(join(repo, 'work', 'my-addon'))).toContain('replace github.com/steamb23/maru-nas/sdk/go => ../../../sdk/go\n')
    expect(goModuleDeps(join(tmp, 'x'))).toContain(`=> ${join(SDK, 'go')}\n`)
  })

  // 만든 Go 백엔드가 빌드되는지 본다(템플릿이 깨지기 가장 쉬운 곳이다). go가 없으면 건너뛴다.
  const goBin = (process.env.PATH ?? '').split(delimiter).some((d) => d && existsSync(join(d, 'go')))
  it.skipIf(!goBin)('--backend가 만든 Go 모듈은 go vet을 통과한다', { timeout: 120_000 }, () => {
    const r = init(join(tmp, 'vet-be'), { backend: true })
    execFileSync('go', ['vet', './...'], {
      cwd: join(r.dir, 'backend'),
      env: { ...process.env, GOWORK: 'off', GOFLAGS: '', GOTOOLCHAIN: 'local' },
      stdio: 'pipe',
    })
  })
})
