#!/usr/bin/env node
// SPDX-License-Identifier: MIT
// maru-addon: 마루 NAS 애드온 패키지를 만들고(init) 검사하고(validate) 묶는다(pack). 동작은 lib.mjs에 있다.
import { CliError, init, pack, validate } from './lib.mjs'

const USAGE = `Usage: maru-addon <command> [options]

Commands:
  init <dir> [--id <id>] [--ts] [--backend]
                                  Create a new addon package: plain JavaScript, or TypeScript + Vite with --ts.
                                  --backend adds a Go backend (<dir>/backend) and puts the package in <dir>/addon
  validate <dir | file>           Check a package directory or a .maru-addon file
  pack <dir> [-o <file>]          Check a package directory and pack it into a .maru-addon file

The dashboard checks more than this tool when it installs a package. During development, load the
directory with MARU_DEV_ADDONS to see the dashboard's own checks (see docs/en/addon-dev.md).`

/** @param {string[]} args */
function parse(args) {
  /** @type {string[]} */
  const rest = []
  /** @type {Record<string, string | boolean>} */
  const flags = {}
  for (let i = 0; i < args.length; i++) {
    const a = args[i]
    if (a === '--ts' || a === '--backend' || a === '--help' || a === '-h') flags[a.replace(/^-+/, '')] = true
    else if (a === '--id' || a === '-o' || a === '--out') {
      const v = args[++i]
      if (v === undefined) throw new CliError(`${a} needs a value`)
      flags[a === '--id' ? 'id' : 'out'] = v
    } else if (a.startsWith('-')) throw new CliError(`unknown option ${a}`)
    else rest.push(a)
  }
  return { rest, flags }
}

/** @param {{ problems: string[], notes: string[] }} r */
function printReport(r) {
  for (const n of r.notes) console.log(`note: ${n}`)
  for (const p of r.problems) console.error(`error: ${p}`)
}

function main() {
  const [command, ...args] = process.argv.slice(2)
  const { rest, flags } = parse(args)
  if (!command || flags.help || flags.h) {
    console.log(USAGE)
    return 0
  }
  switch (command) {
    case 'init': {
      if (rest.length !== 1) throw new CliError('init needs a directory')
      const r = init(rest[0], { id: /** @type {string | undefined} */ (flags.id), ts: !!flags.ts, backend: !!flags.backend })
      console.log(`Created ${r.id} in ${r.dir}`)
      for (const f of r.files) console.log(`  ${f}`)
      if (flags.backend) {
        console.log(`\nNext: build the backend into addon/backend (see ${rest[0]}/README.md), then ${flags.ts ? 'npm install, npm run build and npm run pack' : `maru-addon pack ${rest[0]}/addon`}`)
      } else console.log(flags.ts ? '\nNext: npm install, npm run build, then npm run pack' : `\nNext: maru-addon pack ${rest[0]}`)
      return 0
    }
    case 'validate': {
      if (rest.length !== 1) throw new CliError('validate needs a directory or a .maru-addon file')
      const r = validate(rest[0])
      printReport(r)
      if (r.problems.length > 0) return 1
      console.log(`${rest[0]}: OK (${r.manifest.id} ${r.manifest.version})`)
      return 0
    }
    case 'pack': {
      if (rest.length !== 1) throw new CliError('pack needs a package directory')
      const r = pack(rest[0], { out: /** @type {string | undefined} */ (flags.out) })
      printReport(r.report)
      if (r.report.problems.length > 0) return 1
      console.log(`Packed ${r.out} (${r.size} bytes)`)
      console.log(`  sha256 ${r.sha256}`)
      console.log(`  rev    ${r.rev}`)
      return 0
    }
  }
  throw new CliError(`unknown command ${command}\n\n${USAGE}`)
}

try {
  process.exitCode = main()
} catch (err) {
  if (!(err instanceof CliError)) throw err
  console.error(`maru-addon: ${err.message}`)
  process.exitCode = 2
}
