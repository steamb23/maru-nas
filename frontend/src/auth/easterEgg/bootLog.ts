// 가짜 systemd 부팅 로그 효과. `[  OK  ] Started ...` 줄이 콘솔처럼 위에서부터 쌓이고, 화면이 차면 위로 밀린다.
// 그럴듯한 부팅 로그로 시작해 갈수록 sysadmin 드립·해독 문구 효과의 소재로 선을 넘고,
// 마지막에는 로그인을 기다리는 start job이 끝없이 돈다. 그 job이 끝나면 화면을 비우고 처음부터 다시 부팅한다.
//
// 진짜 장애 로그로 오해하지 않도록 FAILED·TIME 같은 실패 상태는 쓰지 않고, start job은 모두 OK로 끝나거나
// 끝나지 않는 채로 반복된다. 문구는 영어로 고정한다(현지화 제외).
import type { EggEffect, Rng } from './effects'

export type LogLine =
  | { kind: 'ok' | 'info' | 'starting'; text: string }
  // 진행 중인 start job. spinner는 SPINNER_FRAMES의 칸, elapsed는 화면에 보일 경과 시간(초)이다.
  | { kind: 'job'; name: string; spinner: number; elapsed: number; limit: string }

type Step =
  | { kind: 'ok' | 'info' | 'starting'; text: string; delay: number }
  | {
      kind: 'job'
      name: string
      durationMs: number
      limit: string
      // shown은 job이 시작된 뒤 elapsedMs가 지났을 때 화면에 보일 경과 시간(초)이다. 실제 시간보다 빠르게 흐른다.
      shown: (elapsedMs: number) => number
      // 끝나면 이 줄(OK)로 바뀐다. 없으면 끝나는 순간 한 바퀴가 끝난다.
      done?: string
    }

// 다음 줄까지의 기본 간격. 그럴듯한 줄은 실제 부팅처럼 빠르게 몰아서, 드립 줄은 읽을 시간을 두고 나온다.
const FAST = 120
const READ = 1100

const ok = (text: string, delay = FAST): Step => ({ kind: 'ok', text, delay })
const info = (text: string, delay = READ): Step => ({ kind: 'info', text, delay })
const starting = (unit: string, delay = FAST): Step => ({ kind: 'starting', text: `Starting ${unit}...`, delay })

export const SCRIPT: readonly Step[] = [
  // 1. 그럴듯한 부팅
  ok('Created slice System Slice.'),
  ok('Reached target Slice Units.'),
  ok('Reached target Swaps.'),
  starting('Journal Service'),
  ok('Started Journal Service.'),
  starting('Load Kernel Modules'),
  ok('Finished Load Kernel Modules.'),
  ok('Finished Create Static Device Nodes in /dev.'),
  ok('Started Rule-based Manager for Device Events and Files.'),
  ok('Mounted Kernel Debug File System.'),
  ok('Finished Remount Root and Kernel File Systems.'),
  ok('Reached target Local File Systems (Pre).'),
  ok('Reached target Local File Systems.'),
  starting('Network Time Synchronization'),
  ok('Started Network Time Synchronization.'),
  ok('Reached target System Time Set.'),
  ok('Reached target Basic System.'),

  // 2. 홈랩 생활 드립이 슬슬 섞인다
  ok('Started Disk Spin-Up Ceremony.', READ),
  ok('Finished Counting Drive Bays (not enough).', READ),
  ok('Started Fan Controller (Jet Engine Mode).', READ),
  ok('Reached target Neighbors Complaining.', READ),
  ok('Started UPS Monitor (battery: vibes).', READ),
  ok('Started Cable Management (skipped).', READ),
  starting('Raise network interfaces'),
  ok('Finished Raise network interfaces.'),
  starting('Wait for Network to be Configured', 400),
  {
    kind: 'job',
    name: 'Wait for Network to be Configured',
    durationMs: 4000,
    limit: '2min',
    shown: (ms) => Math.floor((ms / 4000) * 119),
    done: 'Finished Wait for Network to be Configured (it was DNS).',
  },
  ok('Reached target Network.'),
  ok('Mounted /srv/media.'),
  ok('Mounted /srv/linux-isos (definitely just ISOs).', READ),
  ok('Mounted /srv/backup (last used: never).', READ),
  ok('Mounted /srv/old-nas-final-FINAL-v2.', READ),
  ok('Mounted /srv/photos (sorted: 0%).', READ),
  ok('Started Samba SMB Daemon.'),
  ok('Started NFS server and services.'),
  ok('Started Daily RAID-Is-Not-A-Backup Reminder.', READ),
  ok('Started Cron Jobs Nobody Remembers Writing.', READ),
  ok("Started 'Temporary' Fix From 2019.", READ),
  ok('Started Blinkenlights.', READ),
  ok('Reached target Too Many Containers.', READ),
  ok('Started Container #47 (purpose unknown).', READ),
  ok('Started Kubernetes for a Single Container.', READ),
  ok('Started Ansible Playbook for Changing One Line.', READ),
  ok('Started Homelab Budget Tracker (do not open).', READ),
  ok('Started chmod 777 Temptation Suppressor.', READ),
  starting('Sysadmin Motivation', 400),
  {
    kind: 'job',
    name: 'Sysadmin Motivation',
    durationMs: 5000,
    limit: '1min 30s',
    // 5초 동안 89초까지 흘러 제한 직전에 겨우 끝난다.
    shown: (ms) => Math.floor((ms / 5000) * 89),
    done: 'Started Sysadmin Motivation (just in time).',
  },
  ok('Reached target Multi-User System.'),

  // 3. 다른 작품·밈이 끼어든다
  info('Boot looks normal. Suspiciously normal.'),
  ok('Reached target Coffee.', READ),
  ok('Started Follow-the-White-LED Service.', READ),
  ok('Started Red Pill Daemon.', READ),
  info('blue-pill.service is masked.'),
  ok('Reached target The Matrix.', READ),
  starting('Chinese Lemon Chicken', 600),
  ok('Finished Slicing 1 Lemon.', 500),
  ok('Finished Adding 3 tbsp Soy Sauce (Dark).', 500),
  ok('Reached target Dinner.', READ),
  ok('Started HAL 9000.', READ),
  info("I'm sorry, Dave. I'm afraid I can't do that."),
  info('These are not the drives you are looking for.'),
  info('One does not simply restart production.'),
  ok("Started Towel Service. Don't Panic.", READ),
  info('Answer to life, the universe and everything: 42 containers.'),
  info('The cake is a lie. The coffee is not.'),
  info("It's dangerous to go alone. Take this: sudo."),
  ok('Started Doom (on the NAS, because we can).', READ),
  info('You must construct additional drive bays.'),
  ok("Started It's-Always-DNS Resolver.", READ),
  ok('Reached target Have You Tried Turning It Off And On Again.', READ),
  ok('Started Rubber Duck Debugging Service.', READ),
  ok('Started Works-On-My-NAS Certification.', READ),
  ok('Started 640K-Ought-To-Be-Enough Memory Manager.', READ),
  ok('Started I-Use-Arch-BTW Daemon (this is Debian).', READ),
  ok('Started vim.', 600),
  info('Still trying to exit vim. Hint: :q!'),
  ok('Started Y2K38 Countdown.', READ),
  ok('Reached target Year of the Linux Desktop.', READ),
  info('Target not actually reached. Maybe next year.'),

  // 4. systemd가 모든 것을 집어삼킨다
  ok('Started systemd-coffeed.', 400),
  ok('Started systemd-toasterd.', 400),
  ok('Started systemd-lemon-chickend.', 400),
  ok('Started systemd-emacsd.', 600),
  ok('Started systemd-kitchen-sinkd.', 600),
  ok('Started systemd-systemd.', READ),
  ok('Started systemd-systemd-systemd.', READ),
  ok('Started systemd-systemd-systemd-systemd.', READ),
  info('systemd has absorbed the toaster. Nobody is surprised.'),
  info('PID 1 is now also PID 2 through 32768.'),
  info('All your base are belong to systemd.'),
  ok('Started Rewriting systemd in Rust.', READ),
  ok('Reached target Peak systemd.', READ),
  ok('Reached target Everything.', READ),

  // 5. 드립이 바닥난다
  ok('Reached target Out of Jokes.', READ),
  ok('Started Joke Recycling Service.', READ),
  ok('Started systemd-coffeed. Again.', READ),
  info('This log line intentionally left blank.'),
  info('Achievement unlocked: watched the entire boot log.'),
  ok('Started Clippy.', 600),
  info("It looks like you're trying to log in."),
  info('Boot finished. It took longer than your coffee.'),
  ok('Reached target Login Screen (you are here).', READ),
  {
    kind: 'job',
    name: 'User To Log In',
    durationMs: 75_000,
    limit: 'no limit',
    // 처음엔 실제 시간처럼 가다가 점점 빨라져 끝날 즈음엔 몇 주를 기다린 셈이 된다.
    shown: (ms) => Math.floor(2 ** (ms / 3500) - 1),
  },
]

// 한 바퀴가 끝난 뒤 다시 부팅하기 전 빈 화면 시간
const REBOOT_GAP_MS = 1500
// start job이 OK로 바뀐 뒤 다음 줄까지의 간격
const JOB_DONE_PAUSE_MS = 800
// 드립이 아닌 줄의 간격은 실제 부팅처럼 들쑥날쑥하게 0.3~1.7배로 흔든다.
const JITTER_MIN = 0.3
const JITTER_RANGE = 1.4

// systemd가 start job을 기다릴 때 보여 주는, 6칸 안에서 오가는 별 세 개
export const SPINNER_FRAMES = ['***   ', ' ***  ', '  *** ', '   ***', '  *** ', ' ***  ']
const SPINNER_MS = 150

interface TimedStep {
  step: Step
  at: number
}

export interface BootPlan {
  steps: TimedStep[]
  // 한 바퀴 길이(마지막 job이 끝나고 빈 화면까지)
  cycle: number
}

// planBoot는 각 줄이 나올 시각을 정한다. FAST 간격만 rng로 흔든다.
export function planBoot(script: readonly Step[], rng: Rng): BootPlan {
  let at = 0
  const steps = script.map((step) => {
    const timed = { step, at }
    if (step.kind === 'job') at += step.durationMs + JOB_DONE_PAUSE_MS
    else at += step.delay === FAST ? FAST * (JITTER_MIN + rng() * JITTER_RANGE) : step.delay
    return timed
  })
  return { steps, cycle: at + REBOOT_GAP_MS }
}

// bootFrame은 연출 시작 후 timeMs일 때 화면에 쌓여 있는 줄이다(오래된 줄부터).
export function bootFrame(plan: BootPlan, timeMs: number): LogLine[] {
  const t = timeMs % plan.cycle
  const lines: LogLine[] = []
  for (const { step, at } of plan.steps) {
    if (at > t) break
    if (step.kind !== 'job') {
      lines.push({ kind: step.kind, text: step.text })
      continue
    }
    const elapsedMs = t - at
    if (elapsedMs < step.durationMs) {
      const spinner = Math.floor(elapsedMs / SPINNER_MS) % SPINNER_FRAMES.length
      lines.push({ kind: 'job', name: step.name, spinner, elapsed: step.shown(elapsedMs), limit: step.limit })
    } else if (step.done) {
      lines.push({ kind: 'ok', text: step.done })
    } else {
      // 끝나지 않는 job이 끝나면 다시 부팅할 때까지 빈 화면이다.
      return []
    }
  }
  return lines
}

// formatTimespan은 systemd처럼 경과 시간을 '1d 2h 3min 4s' 꼴로 쓴다(0인 단위는 생략, 0초는 '0s').
export function formatTimespan(seconds: number): string {
  const units: [string, number][] = [
    ['d', 86400],
    ['h', 3600],
    ['min', 60],
    ['s', 1],
  ]
  let rest = Math.max(0, Math.floor(seconds))
  const parts: string[] = []
  for (const [unit, size] of units) {
    const n = Math.floor(rest / size)
    rest -= n * size
    if (n > 0) parts.push(`${n}${unit}`)
  }
  return parts.length > 0 ? parts.join(' ') : '0s'
}

interface Segment {
  text: string
  color: string
}

const OK_COLOR = '#4cd964'
const SPINNER_COLOR = '#ffcc33'
const BRACKET_COLOR = '#d8d8d8'
const TEXT_COLOR = '#c8c8c8'
const INFO_COLOR = '#ffffff'
const FONT_FAMILY = 'ui-monospace, "DejaVu Sans Mono", Menlo, Consolas, monospace'
// 상태 칸이 없는 줄(Starting ...)은 상태 칸 너비만큼 들여 쓴다.
const STATUS_INDENT = ' '.repeat(9)

// lineSegments는 한 줄을 색이 다른 조각으로 나눈다.
export function lineSegments(line: LogLine): Segment[] {
  switch (line.kind) {
    case 'ok':
      return [
        { text: '[  ', color: BRACKET_COLOR },
        { text: 'OK', color: OK_COLOR },
        { text: '  ] ', color: BRACKET_COLOR },
        { text: line.text, color: TEXT_COLOR },
      ]
    case 'info':
      return [
        { text: '[ ', color: BRACKET_COLOR },
        { text: 'INFO', color: INFO_COLOR },
        { text: ' ] ', color: BRACKET_COLOR },
        { text: line.text, color: TEXT_COLOR },
      ]
    case 'starting':
      return [{ text: STATUS_INDENT + line.text, color: TEXT_COLOR }]
    case 'job':
      return [
        { text: '[', color: BRACKET_COLOR },
        { text: SPINNER_FRAMES[line.spinner], color: SPINNER_COLOR },
        { text: '] ', color: BRACKET_COLOR },
        {
          text: `A start job is running for ${line.name} (${formatTimespan(line.elapsed)} / ${line.limit})`,
          color: TEXT_COLOR,
        },
      ]
  }
}

const lineKey = (line: LogLine | undefined) => (line ? lineSegments(line).map((s) => s.text).join('') : '')

export function createEffect(rng: Rng): EggEffect {
  const plan = planBoot(SCRIPT, rng)
  let width = 0
  let height = 0
  let fontSize = 14
  // 마지막으로 그린 내용. 줄이 늘거나 마지막 줄(진행 중인 job)이 바뀔 때만 다시 그린다.
  let drawnKey: string | null = null

  return {
    resize(w, h) {
      width = w
      height = h
      fontSize = Math.max(11, Math.min(16, w / 80))
      drawnKey = null
    },
    draw(ctx, timeMs) {
      const lines = bootFrame(plan, timeMs)
      const key = `${lines.length}:${lineKey(lines.at(-1))}`
      if (key === drawnKey) return
      drawnKey = key

      ctx.clearRect(0, 0, width, height)
      ctx.font = `${fontSize}px ${FONT_FAMILY}`
      ctx.textBaseline = 'top'
      const charWidth = ctx.measureText('M').width
      const lineHeight = Math.round(fontSize * 1.35)
      const pad = Math.round(fontSize * 0.75)
      // 화면이 차면 오래된 줄부터 위로 밀려난다.
      const rows = Math.max(1, Math.floor((height - pad * 2) / lineHeight))
      lines.slice(-rows).forEach((line, row) => {
        let col = 0
        for (const seg of lineSegments(line)) {
          ctx.fillStyle = seg.color
          ctx.fillText(seg.text, pad + col * charWidth, pad + row * lineHeight)
          col += seg.text.length
        }
      })
    },
  }
}
