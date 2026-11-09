// 전원 모달의 다시 시작 예약("시간 정해서")에 쓰는 계산(DOM 없음). 날짜·시각은 이 브라우저의 시간대로 고르고
// 보인다. 지금 시각은 서버 시계(응답의 now)로 보정한 값을 쓴다(브라우저 시계가 틀려도 남은 시간이 맞게).
// 화면은 PowerSchedule.tsx다.
import type { ParseKeys } from 'i18next'
import { formatUptime } from '../addons/common/format'
import { ApiError } from '../api/client'
import {
  type HelperStatus,
  type PowerSchedule,
  PowerScheduleMaxAheadSeconds,
  PowerScheduleMinAheadSeconds,
} from '../api/types.gen'
import { noticeTimeText } from '../notifications/messages'

const DAY_MS = 24 * 60 * 60 * 1000

// 예약할 수 있는 시각의 범위(백엔드 api.PowerSchedule*AheadSeconds와 같다).
export const MIN_AHEAD_MS = PowerScheduleMinAheadSeconds * 1000
export const MAX_AHEAD_MS = PowerScheduleMaxAheadSeconds * 1000

// LAST_OFFSET은 날짜 목록의 마지막 날(오늘부터 며칠 뒤)이다. 그날은 지금 시각까지만 고를 수 있다.
export const LAST_OFFSET = Math.ceil(MAX_AHEAD_MS / DAY_MS)

// DEFAULT_TIME은 처음 보이는 시각이다. 사람이 쓰지 않을 새벽 시간이다.
export const DEFAULT_TIME = '03:00'

// clockSkew는 서버 시계에서 이 브라우저 시계를 뺀 값(ms)이다. 서버 시각을 읽지 못하면 0이다.
export function clockSkew(serverNow: string, clientNow: number): number {
  const ms = Date.parse(serverNow)
  return Number.isNaN(ms) ? 0 : ms - clientNow
}

// serverNow는 서버 시계로 보정한 지금 시각(ms)이다.
export function serverNow(skew: number): number {
  return Date.now() + skew
}

// dayStart는 now가 속한 날에서 offset일 뒤 날의 0시(이 브라우저의 시간대)다.
export function dayStart(now: number, offset: number): number {
  const d = new Date(now)
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + offset).getTime()
}

// parseTimeValue는 <input type="time">의 값("HH:MM", 초가 붙을 수 있음)이다. 형식이 아니면 null이다.
export function parseTimeValue(v: string): { hour: number; minute: number } | null {
  const m = /^(\d{2}):(\d{2})(?::\d{2}(?:\.\d+)?)?$/.exec(v)
  if (!m) return null
  const hour = Number(m[1])
  const minute = Number(m[2])
  if (hour > 23 || minute > 59) return null
  return { hour, minute }
}

// targetTime은 now가 속한 날에서 offset일 뒤 날의 그 시각(이 브라우저의 시간대, 분 단위)이다. 시각이 없으면 null이다.
// 일광 절약 시간으로 없는 시각은 Date가 앞으로 민다.
export function targetTime(now: number, offset: number, time: string): number | null {
  const t = parseTimeValue(time)
  if (!t) return null
  const d = new Date(now)
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + offset, t.hour, t.minute).getTime()
}

// localTimeValue는 ms를 <input type="time">의 값(이 브라우저의 시간대, "HH:MM")으로 쓴다.
export function localTimeValue(ms: number): string {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

export type TargetCheck = 'ok' | 'empty' | 'past' | 'tooSoon' | 'tooFar'

// checkTarget은 고른 시각을 예약할 수 있는지다. 백엔드가 다시 확인한다(power_time_invalid).
export function checkTarget(target: number | null, now: number): TargetCheck {
  if (target === null) return 'empty'
  if (target <= now) return 'past'
  if (target - now < MIN_AHEAD_MS) return 'tooSoon'
  if (target - now > MAX_AHEAD_MS) return 'tooFar'
  return 'ok'
}

export interface ScheduleForm {
  // offset은 날짜 목록의 순서(오늘부터 며칠 뒤)다.
  offset: number
  time: string
}

// initialForm은 처음 보이는 날짜·시각이다. 다시 시작이 이미 예약되어 있으면 그 시각, 아니면 다음 03:00이다
// (오늘 03:00이 지났거나 MIN_AHEAD_MS 안이면 내일).
export function initialForm(now: number, schedule?: PowerSchedule): ScheduleForm {
  if (schedule?.mode === 'reboot') {
    const at = Date.parse(schedule.at)
    if (!Number.isNaN(at)) {
      const offset = Math.round((dayStart(at, 0) - dayStart(now, 0)) / DAY_MS)
      if (offset >= 0 && offset <= LAST_OFFSET) return { offset, time: localTimeValue(at) }
    }
  }
  const today = targetTime(now, 0, DEFAULT_TIME) ?? 0
  return { offset: today - now >= MIN_AHEAD_MS ? 0 : 1, time: DEFAULT_TIME }
}

// dateText는 날짜 목록의 날짜 문구다(예: 11월 9일 (월)).
export function dateText(ms: number, lang: string): string {
  return new Intl.DateTimeFormat(lang, { weekday: 'short', month: 'long', day: 'numeric' }).format(ms)
}

// clockText는 시각만 쓴 문구다(예: 오후 8:48).
export function clockText(ms: number, lang: string): string {
  return new Intl.DateTimeFormat(lang, { hour: 'numeric', minute: '2-digit' }).format(ms)
}

// leftText는 남은 시간의 문구다. 분 단위로 올린다(6시간 11분 30초 → 6시간 12분).
export function leftText(ms: number, lang: string): string {
  return formatUptime(Math.ceil(Math.max(0, ms) / 60000) * 60, lang)
}

// utcOffsetMinutes는 RFC 3339 시각 끝의 UTC 오프셋(분)이다. 없으면 null이다.
export function utcOffsetMinutes(s: string): number | null {
  const m = /(?:([+-])(\d{2}):(\d{2})|Z)$/.exec(s)
  if (!m) return null
  if (!m[1]) return 0
  const v = Number(m[2]) * 60 + Number(m[3])
  return m[1] === '-' ? -v : v
}

// nasWallTime은 at을 NAS의 벽시계로 쓴 문구다. 이 브라우저에서 보이는 문구와 같으면(시간대가 같거나 시차가
// 없으면) null이다. NAS 시간대 이름이 없거나 이 브라우저가 모르는 이름이면 서버 now의 UTC 오프셋으로 계산한다
// (그 사이 일광 절약 시간이 바뀌면 한 시간 틀릴 수 있다).
export function nasWallTime(at: number, lang: string, timeZone: string | undefined, serverNow: string): string | null {
  let nas: string | null = null
  if (timeZone) {
    try {
      nas = noticeTimeText(at, lang, timeZone)
    } catch {
      nas = null
    }
  }
  if (nas === null) {
    const offset = utcOffsetMinutes(serverNow)
    if (offset === null) return null
    nas = noticeTimeText(at + offset * 60000, lang, 'UTC')
  }
  return nas === noticeTimeText(at, lang) ? null : nas
}

// scheduleBlock은 이 세션이 예약할 수 없는 이유다. 예약·취소는 root 연산이라 권한 도우미와 상승이 필요하다.
// 도우미 상태를 아직 받지 못했으면 막지 않는다(백엔드가 거절하면 scheduleError가 문구를 정한다).
export function scheduleBlock(helper: HelperStatus | null): ParseKeys | null {
  if (!helper) return null
  switch (helper.state) {
    case 'disabled':
    case 'unavailable':
      return 'power.schedule.blocked.helper'
    case 'none':
    case 'reauth':
      return 'power.schedule.blocked.relogin'
    case 'active':
      return helper.canElevate ? null : 'power.schedule.blocked.elevate'
  }
}

// ERROR_KEYS는 예약·취소가 거절된 이유의 문구다.
const ERROR_KEYS: Partial<Record<string, ParseKeys>> = {
  power_time_invalid: 'power.schedule.error.time',
  bad_request: 'power.schedule.error.time',
  elevation_not_allowed: 'power.schedule.blocked.elevate',
  helper_reauth: 'power.schedule.blocked.relogin',
  helper_unavailable: 'power.schedule.blocked.helper',
  admin_required: 'actions.error.admin_required',
}

// scheduleError는 예약·취소 실패의 문구다. 모르는 실패면 fallback이다.
export function scheduleError(err: unknown, fallback: ParseKeys): ParseKeys {
  if (err instanceof ApiError && err.code) return ERROR_KEYS[err.code] ?? fallback
  return fallback
}
