// 온도 응답 검사와 표시 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { hasAddonSources } from '../../api/client'
import { isArrayOf, isNum, isObj, isOneOf, isOptional, isStr } from '../../api/guards'
import type { Sensor, SensorGroup, TemperatureResponse } from '../../api/types.gen'
import type { Level } from '../common/levels'

function isSensor(v: unknown): v is Sensor {
  return (
    isObj(v) &&
    isOptional(v.label, isStr) &&
    isNum(v.celsius) &&
    isOptional(v.highCelsius, isNum) &&
    isOptional(v.critCelsius, isNum)
  )
}

function isSensorGroup(v: unknown): v is SensorGroup {
  return (
    isObj(v) &&
    isStr(v.id) &&
    isOneOf(v.kind, ['cpu', 'disk', 'other'] as const) &&
    isStr(v.name) &&
    isOptional(v.device, isStr) &&
    isStr(v.source) &&
    isArrayOf(v.sensors, isSensor) &&
    isOptional(v.checkedAt, isStr)
  )
}

export function isTemperatureResponse(v: unknown): v is TemperatureResponse {
  return isObj(v) && hasAddonSources(v) && isArrayOf(v.groups, isSensorGroup)
}

// 칩이 경고 온도를 알려 주지 않을 때 쓰는 기준(섭씨). 디스크는 CPU보다 낮은 온도에서 수명이 줄어든다.
const DEFAULT_LIMIT: Record<SensorGroup['kind'], number> = { cpu: 85, disk: 60, other: 80 }
// 기준보다 이만큼 낮은 온도부터 주의로 본다.
const WARN_MARGIN = 10

// tempLevel은 기준(칩의 max, 없으면 crit, 없으면 종류별 기본값) 이상이면 위험, 그보다 10도 낮은
// 온도부터 주의다.
export function tempLevel(s: Sensor, kind: SensorGroup['kind']): Level {
  const limit = s.highCelsius ?? s.critCelsius ?? DEFAULT_LIMIT[kind]
  if (s.celsius >= limit) return 'crit'
  if (s.celsius >= limit - WARN_MARGIN) return 'warn'
  return 'normal'
}

// 묶음의 대표 센서로 쓰는 이름. CPU 패키지 전체 온도(Intel Package, AMD Tctl/Tdie), NVMe 대표값(Composite).
const HEADLINE_LABEL = /^(Package id|Tctl|Tdie|Composite)/i

// headline은 묶음의 대표 센서다. 대표 이름의 센서가 없으면 가장 뜨거운 센서다.
export function headline(g: SensorGroup): Sensor | null {
  if (g.sensors.length === 0) return null
  return (
    g.sensors.find((s) => HEADLINE_LABEL.test(s.label ?? '')) ??
    g.sensors.reduce((a, b) => (b.celsius > a.celsius ? b : a))
  )
}

export interface SummaryRow {
  key: string
  // cpu면 번역 키로 그린다. 디스크는 장치 이름(없으면 묶음 이름).
  kind: SensorGroup['kind']
  name: string
  sensor: Sensor
}

// summaryRows는 위젯에 보여 줄 대표값이다. CPU와 디스크만 보여 주고, 둘 다 없을 때만 그 밖의 센서를 보여 준다.
export function summaryRows(groups: readonly SensorGroup[]): SummaryRow[] {
  const main = groups.filter((g) => g.kind !== 'other')
  const rows: SummaryRow[] = []
  for (const g of main.length > 0 ? main : groups) {
    const s = headline(g)
    if (s) rows.push({ key: g.id, kind: g.kind, name: g.device || g.name, sensor: s })
  }
  return rows
}
