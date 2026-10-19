// 설정의 업데이트 페이지(UpdatePage.tsx)가 쓰는 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 상태는 업그레이드 서비스가 쓴 상태 파일 그대로다(GET /api/upgrade의 status, 형식은 backend/config). 백엔드는
// 꺼짐·요청 대기·중단 여부만 더해 준다. 설치기 문구(사유, 질문, 할 일, 오류)는 영어·한국어 두 가지뿐이다.
import type { UpgradeInfo } from '../api/types.gen'
import type { LocalizedText, UpgradeCheck, UpgradeQuestion } from '../api/upgrade.gen'

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
export type CheckOutcome = 'none' | 'failed' | 'latest' | 'web' | 'manual' | 'outdated'

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
  else if (st?.state === 'checked' && st.check) check = checkOutcome(st.check, info.revision)
  let apply: ApplyOutcome = 'none'
  if (st?.state === 'applied') apply = 'applied'
  else if (st?.state === 'apply-failed') apply = 'failed'
  const busy = activity === 'pending' || activity === 'checking' || activity === 'applying'
  return { activity, stage: st?.stage || null, check, apply, busy }
}

function checkOutcome(check: UpgradeCheck, running: string | undefined): CheckOutcome {
  if (check.upToDate) return 'latest'
  const installed = check.installed?.revision
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

// shortHash는 커밋 해시의 앞 7글자다.
export function shortHash(hash: string | undefined): string {
  return (hash ?? '').slice(0, 7)
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
