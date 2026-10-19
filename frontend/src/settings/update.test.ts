import { describe as group, expect, it } from 'vitest'
import type { UpgradeInfo } from '../api/types.gen'
import type { UpgradeCheck, UpgradeQuestion, UpgradeStatus } from '../api/upgrade.gen'
import {
  answerError,
  applyStep,
  describe,
  initialAnswers,
  manualCommands,
  pickText,
  settingsFor,
  stageName,
  validSettingValue,
} from './update'

const REV_A = 'a'.repeat(40)
const REV_B = 'b'.repeat(40)

function info(status: Partial<UpgradeStatus> | null, extra: Partial<UpgradeInfo> = {}): UpgradeInfo {
  return {
    enabled: true,
    pending: false,
    pendingStalled: false,
    interrupted: false,
    version: '0.3.1',
    revision: REV_A,
    status: status ? ({ format: 1, state: 'checked', updatedAt: '', ...status } as UpgradeStatus) : undefined,
    ...extra,
  }
}

function check(extra: Partial<UpgradeCheck> = {}): UpgradeCheck {
  return {
    mode: 'remote',
    checkedAt: '',
    head: REV_A,
    target: REV_B,
    upToDate: false,
    installed: { format: 1, version: '0.3.1', revision: REV_A, revisionTime: '', modified: false, installedAt: '' },
    commits: [],
    commitsTruncated: false,
    web: true,
    manual: [],
    ...extra,
  }
}

group('pickText', () => {
  it('picks Korean only for Korean', () => {
    const t = { en: 'Hello', ko: '안녕' }
    expect(pickText(t, 'ko')).toBe('안녕')
    expect(pickText(t, 'en')).toBe('Hello')
    expect(pickText(t, 'ja')).toBe('Hello')
    expect(pickText({ en: '', ko: '안녕' }, 'de')).toBe('안녕')
    expect(pickText(undefined, 'ko')).toBe('')
  })
})

group('describe', () => {
  it('reads an idle page', () => {
    expect(describe(info(null))).toEqual({ activity: 'none', stage: null, check: 'none', apply: 'none', busy: false })
  })

  it('reads pending and stalled requests', () => {
    expect(describe(info(null, { pending: true }))).toMatchObject({ activity: 'pending', busy: true })
    // 멈춘 요청은 다시 요청할 수 있다.
    expect(describe(info(null, { pending: true, pendingStalled: true }))).toMatchObject({ activity: 'stalled', busy: false })
  })

  it('reads running and interrupted runs', () => {
    expect(describe(info({ state: 'checking', stage: 'fetch' }))).toMatchObject({ activity: 'checking', stage: 'fetch', busy: true })
    expect(describe(info({ state: 'applying', stage: 'build' }))).toMatchObject({ activity: 'applying', busy: true })
    expect(describe(info({ state: 'applying', stage: 'build' }, { interrupted: true }))).toMatchObject({
      activity: 'interrupted',
      busy: false,
    })
  })

  it('reads check outcomes', () => {
    expect(describe(info({ state: 'checked', check: check() })).check).toBe('web')
    expect(describe(info({ state: 'checked', check: check({ web: false }) })).check).toBe('manual')
    expect(describe(info({ state: 'checked', check: check({ upToDate: true, web: false }) })).check).toBe('latest')
    expect(describe(info({ state: 'check-failed' })).check).toBe('failed')
    // 확인한 뒤 SSH로 설치를 바꿨다.
    expect(describe(info({ state: 'checked', check: check() }, { revision: REV_B })).check).toBe('outdated')
    // 실행 중인 커밋을 모르면(go run) 비교하지 않는다.
    expect(describe(info({ state: 'checked', check: check() }, { revision: undefined })).check).toBe('web')
  })

  it('does not offer a check that was already used', () => {
    expect(describe(info({ state: 'applied', check: check() }))).toMatchObject({ check: 'none', apply: 'applied' })
    expect(describe(info({ state: 'apply-failed', stage: 'build', check: check() }))).toMatchObject({
      check: 'none',
      apply: 'failed',
      stage: 'build',
    })
  })
})

group('stages', () => {
  it('knows the stage names and the upgrade order', () => {
    expect(stageName('fetch')).toBe('fetch')
    expect(stageName('teleport')).toBeNull()
    expect(stageName(undefined)).toBeNull()
    expect(applyStep('inspect')).toBe(0)
    expect(applyStep('install')).toBe(4)
    expect(applyStep('fetch')).toBe(-1)
  })
})

const QUESTIONS: UpgradeQuestion[] = [
  {
    name: 'public-origin',
    kind: 'input',
    title: { en: '', ko: '' },
    help: { en: '', ko: '' },
    default: '',
    dash: true,
    emptyOK: true,
    reason: 'new',
  },
  {
    name: 'smart-refresh-at',
    kind: 'input',
    title: { en: '', ko: '' },
    help: { en: '', ko: '' },
    default: '00:00',
    dash: false,
    emptyOK: false,
    reason: 'invalid',
  },
  {
    name: 'example-toggle',
    kind: 'confirm',
    title: { en: '', ko: '' },
    help: { en: '', ko: '' },
    default: 'no',
    dash: false,
    emptyOK: false,
    reason: 'new',
  },
]

group('questions', () => {
  it('starts from the defaults', () => {
    expect(initialAnswers(QUESTIONS)).toEqual({ 'public-origin': '-', 'smart-refresh-at': '00:00', 'example-toggle': 'no' })
  })

  it('checks answers like the backend', () => {
    const [origin, smart, toggle] = QUESTIONS
    expect(answerError(origin, '-')).toBeNull()
    expect(answerError(origin, '')).toBeNull()
    expect(answerError(origin, ' https://nas.example.com/ ')).toBeNull()
    expect(answerError(origin, 'has space')).toBe('invalid')
    expect(answerError(smart, '')).toBe('required')
    expect(answerError(smart, '03:00')).toBeNull()
    expect(answerError(smart, '한글')).toBe('invalid')
    expect(answerError(toggle, 'yes')).toBeNull()
    expect(answerError(toggle, 'maybe')).toBe('required')
  })

  it('builds the settings to send', () => {
    expect(settingsFor(QUESTIONS, { 'public-origin': ' - ', 'smart-refresh-at': ' 03:00', 'example-toggle': 'yes' })).toEqual({
      'public-origin': '-',
      'smart-refresh-at': '03:00',
      'example-toggle': 'yes',
    })
    expect(settingsFor(QUESTIONS, { 'public-origin': '-', 'smart-refresh-at': '', 'example-toggle': 'yes' })).toBeNull()
  })

  it('accepts only printable ASCII without spaces', () => {
    expect(validSettingValue('')).toBe(true)
    expect(validSettingValue('~!a')).toBe(true)
    expect(validSettingValue('a b')).toBe(false)
    expect(validSettingValue('a\tb')).toBe(false)
    expect(validSettingValue('é')).toBe(false)
    expect(validSettingValue('x'.repeat(512))).toBe(true)
    expect(validSettingValue('x'.repeat(513))).toBe(false)
  })
})

group('manualCommands', () => {
  it('changes to the repository and runs the command', () => {
    expect(manualCommands('/home/alice/onmaru', './install.sh --reinstall')).toBe('cd /home/alice/onmaru\n./install.sh --reinstall')
    expect(manualCommands("/home/al ice/it's", './install.sh')).toBe("cd '/home/al ice/it'\\''s'\n./install.sh")
    expect(manualCommands(undefined, undefined)).toBe('./install.sh')
  })
})
