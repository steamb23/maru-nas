// 디스크 하나의 SMART 상세 계산(응답 검사, 개요 타일, 표 묶음, 문구 키, 오류 분류). DOM에 의존하지 않아 node
// 환경에서 테스트한다. 형식은 백엔드 smart 애드온의 detail·raw 조작(api.SmartDetail, api.SmartRaw)이다.
import { ApiError } from '../../api/client'
import { isArrayOf, isBool, isNum, isObj, isOneOf, isOptional, isStr } from '../../api/guards'
import type {
  SmartAttribute,
  SmartDetail,
  SmartDisk,
  SmartField,
  SmartRaw,
  SmartSelfTest,
} from '../../api/types.gen'
import type { Level } from '../common/levels'

const KINDS = ['ata', 'nvme', 'other'] as const
const UNITS = ['percent', 'celsius', 'bytes', 'hours', 'minutes', 'count', 'hex'] as const
const STATUSES = ['good', 'warning', 'failing', 'unknown'] as const
const TEST_TYPES = ['short', 'extended', 'vendor', 'other'] as const
const TEST_RESULTS = ['ok', 'failed', 'aborted', 'running', 'other'] as const

const isUnit = (u: unknown): u is (typeof UNITS)[number] => isOneOf(u, UNITS)
const isStatus = (s: unknown): s is (typeof STATUSES)[number] => isOneOf(s, STATUSES)

function isField(v: unknown): v is SmartField {
  return (
    isObj(v) &&
    isStr(v.key) &&
    isOptional(v.n, isNum) &&
    isStr(v.value) &&
    isOptional(v.num, isNum) &&
    isOptional(v.unit, isUnit)
  )
}

function isAttribute(v: unknown): v is SmartAttribute {
  return (
    isObj(v) &&
    isNum(v.id) &&
    isStr(v.name) &&
    isStr(v.flags) &&
    isBool(v.prefailure) &&
    isNum(v.value) &&
    isNum(v.worst) &&
    isNum(v.threshold) &&
    isStr(v.whenFailed) &&
    isStr(v.raw) &&
    isOptional(v.rawNum, isNum) &&
    isOptional(v.status, isStatus)
  )
}

function isSelfTest(v: unknown): v is SmartSelfTest {
  return (
    isObj(v) &&
    isNum(v.num) &&
    isOneOf(v.type, TEST_TYPES) &&
    isStr(v.typeText) &&
    isOneOf(v.result, TEST_RESULTS) &&
    isStr(v.statusText) &&
    isOptional(v.remaining, isNum) &&
    isNum(v.powerOnHours) &&
    isOptional(v.failingLba, isStr)
  )
}

export function isSmartDetail(v: unknown): v is SmartDetail {
  return (
    isObj(v) &&
    isStr(v.device) &&
    isStr(v.deviceFile) &&
    isOneOf(v.kind, KINDS) &&
    isStr(v.readAt) &&
    isOptional(v.celsius, isNum) &&
    isOptional(v.powerOnHours, isNum) &&
    isOptional(v.powerCycles, isNum) &&
    isArrayOf(v.info, isField) &&
    isArrayOf(v.health, isField) &&
    isArrayOf(v.attributes, isAttribute) &&
    isArrayOf(v.selfTests, isSelfTest) &&
    isOptional(v.selfTestStatus, isStr) &&
    isOptional(v.selfTestsUnreadable, isBool) &&
    isOptional(v.raw, isStr) &&
    isOptional(v.rawTruncated, isBool)
  )
}

export function isSmartRaw(v: unknown): v is SmartRaw {
  return isObj(v) && isStr(v.device) && isStr(v.readAt) && isStr(v.text) && isOptional(v.truncated, isBool)
}

// OMV 워크벤치에서 디스크 하나의 SMART 상세 화면(8.0부터 같다. 대시보드 위젯이 같은 라우트를 연다).
export function smartDeviceRoute(deviceFile: string): string {
  return `storage/smart/devices/details/${encodeURIComponent(deviceFile)}`
}

// 자가 진단 예약 화면.
export const SMART_TASKS_ROUTE = 'storage/smart/tasks'

// 상세 탭. 속성은 ATA, 건강 정보는 NVMe만 있다. 그 밖(SAS 등)은 정보와 원문만 있다.
export type DetailTab = 'overview' | 'attributes' | 'health' | 'selfTests' | 'raw'

export function detailTabs(kind: SmartDetail['kind'] | undefined): DetailTab[] {
  if (kind === 'ata') return ['overview', 'attributes', 'selfTests', 'raw']
  if (kind === 'nvme') return ['overview', 'health', 'selfTests', 'raw']
  return ['overview', 'raw']
}

// DetailProblem은 상세·원문 조회 실패를 화면 상태로 나눈 것이다. pending은 같은 디스크를 다른 요청이 읽는
// 중이라(409 action_busy) 조금 뒤 다시 묻는다.
export type DetailProblem = 'pending' | 'unsupported' | 'gone' | 'timeout' | 'failed'

export function detailProblem(err: unknown): DetailProblem {
  const code = err instanceof ApiError ? err.code : undefined
  switch (code) {
    case 'action_busy':
      return 'pending'
    case 'smart_unsupported':
      return 'unsupported'
    case 'not_found':
      return 'gone'
    case 'action_timeout':
      return 'timeout'
  }
  return 'failed'
}

// mediaBadges는 머리글의 종류 배지(SATA·NVMe, SSD·HDD)다. 기술 용어라 번역하지 않는다.
export function mediaBadges(detail: SmartDetail): string[] {
  if (detail.kind === 'nvme') return ['NVMe', 'SSD']
  const out: string[] = []
  if (detail.kind === 'ata') out.push(infoValue(detail, 'sataVersion') ? 'SATA' : 'ATA')
  const rotation = infoValue(detail, 'rotationRate')
  if (rotation) out.push(/rpm/i.test(rotation) ? 'HDD' : /solid state/i.test(rotation) ? 'SSD' : '')
  return out.filter(Boolean)
}

function infoValue(detail: SmartDetail, key: string): string | undefined {
  return detail.info.find((f) => f.key === key)?.value
}

// BannerKey는 개요 맨 위 안내 띠의 문구다(목록의 OMV 종합 평가). 정상이면 없다.
export type BannerKey = 'attributeNow' | 'attributeInPast' | 'badSector' | 'badSectorMany' | 'unknown'

export function statusBanner(disk: SmartDisk): { key: BannerKey; level: Level } | null {
  switch (disk.overallStatus) {
    case 'GOOD':
      return null
    case 'BAD_ATTRIBUTE_NOW':
      return { key: 'attributeNow', level: 'crit' }
    case 'BAD_ATTRIBUTE_IN_THE_PAST':
      return { key: 'attributeInPast', level: 'crit' }
    case 'BAD_SECTOR':
      return { key: 'badSector', level: 'warn' }
    case 'BAD_SECTOR_MANY':
      return { key: 'badSectorMany', level: 'crit' }
  }
  return disk.status === 'good' ? null : { key: 'unknown', level: 'warn' }
}

// Tile은 개요의 요약 타일 하나다. 값을 모르면 value가 없다(화면은 "–").
export type TileId =
  | 'temperature'
  | 'powerOnHours'
  | 'powerCycles'
  | 'reallocated'
  | 'pending'
  | 'uncorrectable'
  | 'percentageUsed'
  | 'availableSpare'
  | 'mediaErrors'

export type TileSub =
  | { kind: 'minMax'; min: number; max: number }
  | { kind: 'attribute'; id: number }
  | { kind: 'unsafeShutdowns'; value: number }
  | { kind: 'threshold'; value: number }
  | { kind: 'errorLog'; value: number }
  | { kind: 'sensorMax'; value: number }

export interface Tile {
  id: TileId
  value?: number
  unit: 'celsius' | 'hours' | 'count' | 'percent'
  level: Level
  // meter는 막대로 보이는 비율(0~100)이다(수명 사용률).
  meter?: number
  sub?: TileSub
}

const MIN_MAX = /\(Min\/Max\s+(-?\d+)\/(-?\d+)\)/

function attribute(detail: SmartDetail, id: number): SmartAttribute | undefined {
  return detail.attributes.find((a) => a.id === id)
}

function healthNum(detail: SmartDetail, key: string, n = 0): number | undefined {
  return detail.health.find((f) => f.key === key && (f.n ?? 0) === n)?.num
}

// attributeLevel은 속성 하나의 점·행 색이다. OMV가 평가한 속성은 그 평가를, 평가하지 않는 섹터 수(197·198)는
// 0보다 크면 주의로 본다.
export function attributeLevel(a: SmartAttribute): Level {
  if (a.status === 'failing') return 'crit'
  if (a.status === 'warning' || a.status === 'unknown') return 'warn'
  if (a.status === undefined && (a.id === 197 || a.id === 198) && (a.rawNum ?? 0) > 0) return 'warn'
  return 'normal'
}

function sectorTile(detail: SmartDetail, id: TileId, attrId: number): Tile {
  const a = attribute(detail, attrId)
  return {
    id,
    value: a?.rawNum,
    unit: 'count',
    level: a ? (attributeLevel(a) === 'normal' && (a.rawNum ?? 0) > 0 ? 'warn' : attributeLevel(a)) : 'normal',
    sub: { kind: 'attribute', id: attrId },
  }
}

// overviewTiles는 개요의 요약 타일 여섯 개다. 앞의 셋(온도, 사용 시간, 전원 켠 횟수)은 종류와 상관없이 같다.
export function overviewTiles(detail: SmartDetail): Tile[] {
  const temp: Tile = { id: 'temperature', value: detail.celsius, unit: 'celsius', level: 'normal' }
  if (detail.kind === 'ata') {
    const m = attribute(detail, 194)?.raw.match(MIN_MAX) ?? attribute(detail, 190)?.raw.match(MIN_MAX)
    if (m) temp.sub = { kind: 'minMax', min: Number(m[1]), max: Number(m[2]) }
  }
  const sensors = detail.health.filter((f) => f.key === 'temperatureSensor' && f.num !== undefined).map((f) => f.num ?? 0)
  if (sensors.length > 1) temp.sub = { kind: 'sensorMax', value: Math.max(...sensors) }

  const cycles: Tile = { id: 'powerCycles', value: detail.powerCycles, unit: 'count', level: 'normal' }
  const unsafe = healthNum(detail, 'unsafeShutdowns')
  if (unsafe !== undefined) cycles.sub = { kind: 'unsafeShutdowns', value: unsafe }
  const common: Tile[] = [temp, { id: 'powerOnHours', value: detail.powerOnHours, unit: 'hours', level: 'normal' }, cycles]

  if (detail.kind === 'nvme') {
    const used = healthNum(detail, 'percentageUsed')
    const spare = healthNum(detail, 'availableSpare')
    const threshold = healthNum(detail, 'availableSpareThreshold')
    const media = healthNum(detail, 'mediaErrors')
    const errorLog = healthNum(detail, 'errorLogEntries')
    return [
      ...common,
      {
        id: 'percentageUsed',
        value: used,
        unit: 'percent',
        level: healthLevelOf('percentageUsed', used, detail) ?? 'normal',
        meter: used === undefined ? undefined : Math.min(100, Math.max(0, used)),
      },
      {
        id: 'availableSpare',
        value: spare,
        unit: 'percent',
        level: healthLevelOf('availableSpare', spare, detail) ?? 'normal',
        sub: threshold === undefined ? undefined : { kind: 'threshold', value: threshold },
      },
      {
        id: 'mediaErrors',
        value: media,
        unit: 'count',
        level: healthLevelOf('mediaErrors', media, detail) ?? 'normal',
        sub: errorLog === undefined ? undefined : { kind: 'errorLog', value: errorLog },
      },
    ]
  }
  if (detail.kind === 'ata') {
    return [
      ...common,
      sectorTile(detail, 'reallocated', 5),
      sectorTile(detail, 'pending', 197),
      sectorTile(detail, 'uncorrectable', 198),
    ]
  }
  return common
}

// healthLevelOf는 NVMe 건강 정보 중 상태를 판단하는 항목의 점 색이다(OMV 8.6의 NVMe 평가와 같은 기준). 판단하지
// 않는 항목이면 undefined다.
function healthLevelOf(key: string, num: number | undefined, detail: SmartDetail): Level | undefined {
  if (num === undefined) return undefined
  switch (key) {
    case 'criticalWarning':
      return num !== 0 ? 'crit' : 'normal'
    case 'availableSpare': {
      const threshold = healthNum(detail, 'availableSpareThreshold')
      return threshold !== undefined && threshold > 0 && num < threshold ? 'crit' : 'normal'
    }
    case 'percentageUsed':
      return num >= 100 ? 'crit' : 'normal'
    case 'mediaErrors':
      return num > 0 ? 'warn' : 'normal'
  }
  return undefined
}

export function healthLevel(field: SmartField, detail: SmartDetail): Level | undefined {
  return healthLevelOf(field.key, field.num, detail)
}

// HealthGroup은 NVMe 건강 정보 표의 묶음이다.
export type HealthGroupId = 'status' | 'usage' | 'power' | 'errors' | 'temperature' | 'other'

const HEALTH_GROUPS: Record<string, HealthGroupId> = {
  criticalWarning: 'status',
  availableSpare: 'status',
  availableSpareThreshold: 'status',
  percentageUsed: 'status',
  dataUnitsRead: 'usage',
  dataUnitsWritten: 'usage',
  hostReadCommands: 'usage',
  hostWriteCommands: 'usage',
  controllerBusyTime: 'usage',
  powerCycles: 'power',
  powerOnHours: 'power',
  unsafeShutdowns: 'power',
  mediaErrors: 'errors',
  errorLogEntries: 'errors',
  temperature: 'temperature',
  warningTempTime: 'temperature',
  criticalTempTime: 'temperature',
  temperatureSensor: 'temperature',
}

const GROUP_ORDER: HealthGroupId[] = ['status', 'usage', 'power', 'errors', 'temperature', 'other']

export function healthGroups(fields: readonly SmartField[]): { id: HealthGroupId; fields: SmartField[] }[] {
  const groups = new Map<HealthGroupId, SmartField[]>()
  for (const f of fields) {
    const id = HEALTH_GROUPS[f.key] ?? 'other'
    groups.set(id, [...(groups.get(id) ?? []), f])
  }
  return GROUP_ORDER.filter((id) => groups.has(id)).map((id) => ({ id, fields: groups.get(id) ?? [] }))
}

// smartctl이 NVMe 건강 정보에 쓰는 원래 이름. 화면이 번역한 이름 아래에 작게 둔다.
const HEALTH_SMARTCTL: Record<string, string> = {
  criticalWarning: 'Critical Warning',
  availableSpare: 'Available Spare',
  availableSpareThreshold: 'Available Spare Threshold',
  percentageUsed: 'Percentage Used',
  dataUnitsRead: 'Data Units Read',
  dataUnitsWritten: 'Data Units Written',
  hostReadCommands: 'Host Read Commands',
  hostWriteCommands: 'Host Write Commands',
  controllerBusyTime: 'Controller Busy Time',
  powerCycles: 'Power Cycles',
  powerOnHours: 'Power On Hours',
  unsafeShutdowns: 'Unsafe Shutdowns',
  mediaErrors: 'Media and Data Integrity Errors',
  errorLogEntries: 'Error Information Log Entries',
  temperature: 'Temperature',
  warningTempTime: 'Warning Comp. Temperature Time',
  criticalTempTime: 'Critical Comp. Temperature Time',
  temperatureSensor: 'Temperature Sensor',
}

export function smartctlName(field: SmartField): string {
  const base = HEALTH_SMARTCTL[field.key] ?? field.key
  return field.n ? `${base} ${field.n}` : base
}

// 번역 키가 있는 장치 정보·건강 정보 항목(백엔드 smart 애드온의 infoFields·healthFields와 같다). 목록에 없는
// 키는 번역하지 않고 키를 그대로 보인다.
export const INFO_KEYS = [
  'modelFamily',
  'model',
  'vendor',
  'serial',
  'firmware',
  'capacity',
  'sectorSize',
  'lbaSize',
  'rotationRate',
  'formFactor',
  'ataVersion',
  'sataVersion',
  'nvmeVersion',
  'namespaces',
  'pciVendor',
  'trim',
  'smartSupport',
  'selfAssessment',
] as const
export type InfoKey = (typeof INFO_KEYS)[number]

export function isInfoKey(key: string): key is InfoKey {
  return (INFO_KEYS as readonly string[]).includes(key)
}

export const HEALTH_KEYS = [
  'criticalWarning',
  'availableSpare',
  'availableSpareThreshold',
  'percentageUsed',
  'dataUnitsRead',
  'dataUnitsWritten',
  'hostReadCommands',
  'hostWriteCommands',
  'controllerBusyTime',
  'powerCycles',
  'powerOnHours',
  'unsafeShutdowns',
  'mediaErrors',
  'errorLogEntries',
  'temperature',
  'warningTempTime',
  'criticalTempTime',
  'temperatureSensor',
] as const
export type HealthKey = (typeof HEALTH_KEYS)[number]

export function isHealthKey(key: string): key is HealthKey {
  return (HEALTH_KEYS as readonly string[]).includes(key)
}

// 번역 이름이 있는 ATA 속성(smartctl 이름). 제조사마다 뜻이 다른 이름은 넣지 않는다. 목록 밖의 속성은 smartctl
// 이름만 보인다.
export const ATTR_NAMES = [
  'Raw_Read_Error_Rate',
  'Throughput_Performance',
  'Spin_Up_Time',
  'Start_Stop_Count',
  'Reallocated_Sector_Ct',
  'Seek_Error_Rate',
  'Seek_Time_Performance',
  'Power_On_Hours',
  'Spin_Retry_Count',
  'Calibration_Retry_Count',
  'Power_Cycle_Count',
  'Helium_Level',
  'Wear_Leveling_Count',
  'Used_Rsvd_Blk_Cnt_Tot',
  'Program_Fail_Cnt_Total',
  'Erase_Fail_Count_Total',
  'Runtime_Bad_Block',
  'End-to-End_Error',
  'Reported_Uncorrect',
  'Command_Timeout',
  'High_Fly_Writes',
  'Airflow_Temperature_Cel',
  'G-Sense_Error_Rate',
  'Power-Off_Retract_Count',
  'Load_Cycle_Count',
  'Temperature_Celsius',
  'Hardware_ECC_Recovered',
  'Reallocated_Event_Count',
  'Current_Pending_Sector',
  'Offline_Uncorrectable',
  'UDMA_CRC_Error_Count',
  'Multi_Zone_Error_Rate',
  'Unsafe_Shutdown_Count',
  'Head_Flying_Hours',
  'Total_LBAs_Written',
  'Total_LBAs_Read',
] as const
export type AttrName = (typeof ATTR_NAMES)[number]

export function isAttrName(name: string): name is AttrName {
  return (ATTR_NAMES as readonly string[]).includes(name)
}

// problemCount는 주의 이상인 속성 수다.
export function problemCount(attrs: readonly SmartAttribute[]): number {
  return attrs.filter((a) => attributeLevel(a) !== 'normal').length
}

// selfAssessmentKey는 자체 평가 값(PASSED 등)을 번역할 수 있으면 그 키다.
export function selfAssessmentKey(value: string): 'passed' | 'failed' | null {
  const v = value.trim().toUpperCase()
  if (v === 'PASSED' || v === 'OK') return 'passed'
  if (v.startsWith('FAILED')) return 'failed'
  return null
}

// testLevel은 자가 진단 결과의 점 색이다.
export function testLevel(result: SmartSelfTest['result']): Level | 'idle' {
  if (result === 'ok') return 'normal'
  if (result === 'failed') return 'crit'
  if (result === 'other') return 'warn'
  return 'idle'
}
