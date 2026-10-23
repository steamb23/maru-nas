// 설정의 업데이트 페이지(UpdatePage.tsx)가 쓰는 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 상태는 업그레이드 서비스가 쓴 상태 파일 그대로다(GET /api/upgrade의 status, 형식은 backend/config). 백엔드는
// 꺼짐·요청 대기·중단 여부만 더해 준다. 설치기 문구(사유, 질문, 할 일, 오류)는 영어·한국어 두 가지뿐이다.
import type { UpgradeInfo } from '../api/types.gen'
import type { LocalizedText, UpgradeCheck, UpgradeQuestion } from '../api/upgrade.gen'
import { shortRevision } from './about/about'

// pickText는 설치기 문구를 화면 언어로 고른다. 한국어 화면이면 ko, 그 밖의 언어는 en이다(비었으면 다른 쪽).
export function pickText(text: LocalizedText | null | undefined, lang: string): string {
  if (!text) return ''
  const ko = lang === 'ko' || lang.startsWith('ko-')
  return (ko ? text.ko || text.en : text.en || text.ko) ?? ''
}

// Activity는 지금 진행 중인 일이다.
// pending: 확인 요청을 남겼고 업그레이드 서비스가 아직 가져가지 않음. stalled: 1분 넘게 가져가지 않음.
// checking·applying: 서비스가 확인·업그레이드 중. interrupted: 진행 중인 채로 서비스가 멈춤(다시 확인할 수 있다).
export type Activity = 'none' | 'pending' | 'stalled' | 'checking' | 'applying' | 'interrupted'

// CheckOutcome은 보여 줄 확인 결과다. 확인 결과는 상태가 checked일 때만 쓸 수 있다(업그레이드한 뒤나 실패한 뒤에는
// 업그레이드 서비스가 그 확인으로 진행하지 않으므로 다시 확인해야 한다).
// failed: 확인 실패. latest: 최신. web: 대시보드에서 업그레이드할 수 있음. manual: 수동 재설치 필요.
// outdated: 확인한 뒤 설치가 바뀌었음(실행 중인 커밋이 확인의 설치 기록과 다름).
// channel: 확인한 뒤 업데이트 채널을 바꿨음(원격 확인이 지금 고른 채널로 한 것이 아님).
export type CheckOutcome = 'none' | 'failed' | 'latest' | 'web' | 'manual' | 'outdated' | 'channel'

// ApplyOutcome은 마지막 업그레이드의 결과다(상태가 applied·apply-failed일 때).
export type ApplyOutcome = 'none' | 'applied' | 'failed'

export interface UpdateState {
  activity: Activity
  // stage는 진행 중이거나(checking·applying·interrupted) 실패한 단계다.
  stage: string | null
  check: CheckOutcome
  apply: ApplyOutcome
  // busy면 확인·업그레이드 단추를 막는다.
  busy: boolean
}

export function describe(info: UpgradeInfo): UpdateState {
  const st = info.status ?? null
  let activity: Activity = 'none'
  if (info.pending) activity = info.pendingStalled ? 'stalled' : 'pending'
  else if (st && (st.state === 'checking' || st.state === 'applying')) {
    activity = info.interrupted ? 'interrupted' : st.state
  }
  let check: CheckOutcome = 'none'
  if (st?.state === 'check-failed') check = 'failed'
  else if (st?.state === 'checked' && st.check) check = checkOutcome(st.check, info)
  let apply: ApplyOutcome = 'none'
  if (st?.state === 'applied') apply = 'applied'
  else if (st?.state === 'apply-failed') apply = 'failed'
  const busy = activity === 'pending' || activity === 'checking' || activity === 'applying'
  return { activity, stage: st?.stage || null, check, apply, busy }
}

function checkOutcome(check: UpgradeCheck, info: UpgradeInfo): CheckOutcome {
  if (check.mode === 'remote' && (check.channel ?? '') !== info.channel) return 'channel'
  if (check.upToDate) return 'latest'
  const installed = check.installed?.revision
  const running = info.revision
  if (installed && running && installed !== running) return 'outdated'
  return check.web ? 'web' : 'manual'
}

// APPLY_STAGES는 업그레이드 단계(config.UpgradeStage*)의 순서다. 확인은 원격이면 fetch 뒤 inspect로 돌아오므로
// 순서를 그리지 않고 지금 단계만 보인다.
export const APPLY_STAGES = ['inspect', 'plan', 'update', 'build', 'install'] as const

export type StageName = 'inspect' | 'fetch' | 'plan' | 'update' | 'build' | 'install'

// stageName은 단계 이름이 아는 값이면 그 값, 아니면 null이다(다른 버전의 서비스가 쓴 단계).
export function stageName(stage: string | null | undefined): StageName | null {
  switch (stage) {
    case 'inspect':
    case 'fetch':
    case 'plan':
    case 'update':
    case 'build':
    case 'install':
      return stage
  }
  return null
}

// applyStep은 업그레이드 단계의 위치(0부터)다. 모르는 단계면 -1이다.
export function applyStep(stage: string | null | undefined): number {
  return APPLY_STAGES.indexOf(stage as (typeof APPLY_STAGES)[number])
}

// versionLabel은 버전과 커밋을 "0.4.0 (2222222)"처럼 쓴다(커밋은 늘 괄호 안, 앞 7글자). 커밋이 없으면 버전만,
// 버전이 없으면 괄호 안의 커밋만이다.
export function versionLabel(version: string | undefined, revision: string | undefined): string {
  const rev = revision ? `(${shortRevision(revision)})` : ''
  return [version ?? '', rev].filter((s) => s !== '').join(' ')
}

// installedVersion은 지금 설치된(실행 중인) 버전과 커밋이다. 실행 파일에 커밋이 없으면(go run 등) 같은 버전의 설치
// 기록에서 커밋을 가져온다.
export function installedVersion(info: UpgradeInfo): { version: string; revision: string | undefined } {
  const rec = info.installed
  const revision = info.revision || (rec && rec.version === info.version ? rec.revision : undefined)
  return { version: info.version, revision: revision || undefined }
}

// 설정 값의 상한(backend/config의 MaxUpgradeSettingValue).
const MAX_SETTING_VALUE = 512

// validSettingValue는 backend/config의 ValidUpgradeSetting과 같은 값 규칙이다: 공백·제어 문자 없는 출력 가능한 ASCII,
// 512바이트까지(빈 값 가능).
export function validSettingValue(value: string): boolean {
  if (value.length > MAX_SETTING_VALUE) return false
  for (let i = 0; i < value.length; i++) {
    const c = value.charCodeAt(i)
    if (c <= 0x20 || c > 0x7e) return false
  }
  return true
}

// Answers는 질문 이름별 답이다. confirm은 'yes'·'no', input은 입력한 글자(앞뒤 공백 포함)다.
export type Answers = Record<string, string>

// initialAnswers는 질문의 기본값이다. Dash 설정의 빈 기본값은 "-"(끔)로 보인다.
export function initialAnswers(questions: readonly UpgradeQuestion[]): Answers {
  const out: Answers = {}
  for (const q of questions) {
    if (q.kind === 'confirm') out[q.name] = q.default === 'no' ? 'no' : 'yes'
    else out[q.name] = q.dash && q.default === '' ? '-' : q.default
  }
  return out
}

export type AnswerError = 'required' | 'invalid'

// answerError는 입력 칸의 답이 보낼 수 없는 값인지다. 새 버전의 검사는 업그레이드 서비스가 빌드하기 전에 다시 한다.
export function answerError(q: UpgradeQuestion, raw: string | undefined): AnswerError | null {
  if (q.kind === 'confirm') return raw === 'yes' || raw === 'no' ? null : 'required'
  const value = (raw ?? '').trim()
  if (!validSettingValue(value)) return 'invalid'
  const empty = value === '' || (q.dash && value === '-')
  if (empty && !q.emptyOK) return 'required'
  return null
}

// settingsFor는 업그레이드 요청에 보낼 설정이다(입력 값은 앞뒤 공백을 지운다). Dash 설정의 "-"는 그대로 보낸다
// (업그레이드 서비스가 빈 값으로 바꾼다). 보낼 수 없는 답이 있으면 null이다.
export function settingsFor(questions: readonly UpgradeQuestion[], answers: Answers): Record<string, string> | null {
  const out: Record<string, string> = {}
  for (const q of questions) {
    if (answerError(q, answers[q.name])) return null
    out[q.name] = q.kind === 'confirm' ? (answers[q.name] as string) : (answers[q.name] ?? '').trim()
  }
  return out
}

// manualCommands는 수동 재설치 안내의 명령이다. 저장소 경로를 모르면 cd 줄을 뺀다.
export function manualCommands(repo: string | undefined, command: string | undefined): string {
  const lines: string[] = []
  if (repo) lines.push(`cd ${shellQuote(repo)}`)
  lines.push(command || './install.sh')
  return lines.join('\n')
}

// shellQuote는 경로에 셸 특수 문자가 있으면 작은따옴표로 감싼다.
function shellQuote(s: string): string {
  return /^[A-Za-z0-9_./@%+=:,-]+$/.test(s) ? s : `'${s.replaceAll("'", `'\\''`)}'`
}

// helpLines는 질문 설명을 줄로 나눈다(설치기 설명은 여러 줄일 수 있다).
export function helpLines(text: string): string[] {
  return text.split('\n').filter((l) => l.trim() !== '')
}

// 업데이트 채널(.claude/docs/upgrade.md의 "업데이트 채널"). 값은 site.json의 upgrade.channel이고 빈 값이면 지금
// 브랜치의 추적 브랜치를 따른다.
export type Channel = UpgradeInfo['channel']

// RepoBranch는 업그레이드 서비스가 마지막으로 본 저장소의 브랜치다. 확인한 적이 없거나 이전 서비스면 모른다.
export type RepoBranch = { kind: 'unknown' } | { kind: 'detached' } | { kind: 'branch'; name: string }

export function repoBranch(info: UpgradeInfo): RepoBranch {
  const st = info.status
  if (st?.detached) return { kind: 'detached' }
  if (st?.branch) return { kind: 'branch', name: st.branch }
  return { kind: 'unknown' }
}

// ChannelCard는 업데이트 채널 카드 하나다. current는 지금 브랜치를 따른다(채널 빈 값).
export type ChannelCard = 'stable' | 'main' | 'current'

// channelCards는 보일 카드와 고른 카드다. 저장소가 stable·main 브랜치에 있으면 채널을 고르지 않은 설치도 그
// 브랜치의 카드로 보이고 "지금 브랜치" 카드는 숨긴다(따라가는 브랜치가 같다). 다른 브랜치에 있거나, 브랜치가
// 아니거나, 아직 모르면 "지금 브랜치" 카드를 함께 보인다.
export function channelCards(channel: Channel, branch: RepoBranch): { cards: ChannelCard[]; selected: ChannelCard } {
  const on = branch.kind === 'branch' && (branch.name === 'stable' || branch.name === 'main') ? branch.name : null
  return {
    cards: on ? ['stable', 'main'] : ['stable', 'main', 'current'],
    selected: channel !== '' ? channel : (on ?? 'current'),
  }
}

// channelValue는 카드가 고르는 채널 값이다.
export function channelValue(card: ChannelCard): Channel {
  return card === 'current' ? '' : card
}

// isDevTarget은 확인의 대상이 개발 브랜치(main)에서 왔는지다. 채널을 고르지 않았으면 추적 브랜치로 판단한다.
export function isDevTarget(check: UpgradeCheck): boolean {
  if (check.mode !== 'remote') return false
  if (check.channel) return check.channel === 'main'
  // 추적 브랜치는 "원격/브랜치" 꼴이다(origin/main).
  const up = check.upstream ?? ''
  return up.slice(up.indexOf('/') + 1) === 'main' && up.includes('/')
}

// ApplyConfirm은 업그레이드 단추와 확인 대화상자의 문구를 고르는 값이다.
export interface ApplyConfirm {
  // overwrite는 이전 버전으로 덮어쓰는지다(지금 설치에만 있는 커밋 dropped개가 빠진다).
  overwrite: boolean
  dropped: number
  // switchTo는 업그레이드하면 저장소가 옮겨 가는 채널 브랜치다. 옮기지 않으면 null이다.
  switchTo: 'stable' | 'main' | null
}

export function applyConfirm(check: UpgradeCheck): ApplyConfirm {
  const dropped = check.dropped ?? 0
  const switchTo = check.switch && (check.channel === 'stable' || check.channel === 'main') ? check.channel : null
  return { overwrite: dropped > 0, dropped, switchTo }
}
