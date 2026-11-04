import { describe, expect, it } from 'vitest'
import { ApiError } from '../../api/client'
import type { SmartAttribute, SmartDetail, SmartDisk, SmartField } from '../../api/types.gen'
import en from '../../i18n/locales/en.json'
import ko from '../../i18n/locales/ko.json'
import {
  ATTR_NAMES,
  HEALTH_KEYS,
  INFO_KEYS,
  attributeLevel,
  detailProblem,
  detailTabs,
  healthGroups,
  healthLevel,
  isSmartDetail,
  isSmartRaw,
  mediaBadges,
  overviewTiles,
  problemCount,
  selfAssessmentKey,
  smartDeviceRoute,
  smartctlName,
  statusBanner,
  testLevel,
} from './detail'

const field = (key: string, value: string, num?: number, unit?: SmartField['unit'], n?: number): SmartField => ({
  key,
  value,
  num,
  unit,
  n,
})

const attr = (id: number, name: string, rawNum: number, status?: SmartAttribute['status'], prefailure = false): SmartAttribute => ({
  id,
  name,
  flags: prefailure ? 'PO--CK' : '-O--CK',
  prefailure,
  value: 100,
  worst: 100,
  threshold: prefailure ? 10 : 0,
  whenFailed: '-',
  raw: String(rawNum),
  rawNum,
  status,
})

function ata(attrs: SmartAttribute[]): SmartDetail {
  return {
    device: 'sdb',
    deviceFile: '/dev/disk/by-id/fake-sdb',
    kind: 'ata',
    readAt: '2026-11-04T05:32:08Z',
    celsius: 34,
    powerOnHours: 31482,
    powerCycles: 142,
    info: [field('model', 'Fake HDD'), field('sataVersion', 'SATA 3.3'), field('rotationRate', '7200 rpm')],
    health: [],
    attributes: attrs,
    selfTests: [
      { num: 1, type: 'short', typeText: 'Short offline', result: 'ok', statusText: 'Completed without error', remaining: 0, powerOnHours: 41800 },
    ],
  }
}

function nvme(health: SmartField[]): SmartDetail {
  return {
    device: 'nvme0n1',
    deviceFile: '/dev/disk/by-id/fake-nvme0n1',
    kind: 'nvme',
    readAt: '2026-11-04T05:33:10Z',
    celsius: 38,
    info: [field('model', 'Fake NVMe SSD')],
    health,
    attributes: [],
    selfTests: [],
    raw: 'smartctl 7.4',
  }
}

const disk = (overallStatus: string, status: SmartDisk['status']): SmartDisk => ({
  device: 'sdb',
  model: 'Fake HDD',
  sizeBytes: 1,
  status,
  overallStatus,
  monitored: true,
})

describe('응답 검사', () => {
  it('상세와 원문을 받는다', () => {
    expect(isSmartDetail(ata([attr(5, 'Reallocated_Sector_Ct', 8, 'warning', true)]))).toBe(true)
    expect(isSmartDetail(nvme([field('percentageUsed', '3%', 3, 'percent')]))).toBe(true)
    expect(isSmartRaw({ device: 'sda', readAt: 'x', text: 'smartctl', truncated: true })).toBe(true)
  })

  it('모양이 다르면 거절한다', () => {
    expect(isSmartDetail({ ...ata([]), kind: 'sas' })).toBe(false)
    expect(isSmartDetail({ ...ata([]), info: [{ key: 'model', value: 1 }] })).toBe(false)
    expect(isSmartDetail({ ...ata([]), info: [field('model', 'x', 1, 'kelvin' as SmartField['unit'])] })).toBe(false)
    expect(isSmartDetail({ ...ata([{ ...attr(1, 'x', 0), status: 'bad' as SmartAttribute['status'] }]) })).toBe(false)
    expect(isSmartRaw({ device: 'sda', readAt: 'x' })).toBe(false)
  })
})

describe('탭과 오류', () => {
  it('종류마다 탭이 다르다', () => {
    expect(detailTabs('ata')).toEqual(['overview', 'attributes', 'selfTests', 'raw'])
    expect(detailTabs('nvme')).toEqual(['overview', 'health', 'selfTests', 'raw'])
    expect(detailTabs('other')).toEqual(['overview', 'raw'])
    expect(detailTabs(undefined)).toEqual(['overview', 'raw'])
  })

  it('오류 코드를 화면 상태로 나눈다', () => {
    const err = (code: ConstructorParameters<typeof ApiError>[2]) => new ApiError(400, 'x', code)
    expect(detailProblem(err('action_busy'))).toBe('pending')
    expect(detailProblem(err('smart_unsupported'))).toBe('unsupported')
    expect(detailProblem(err('not_found'))).toBe('gone')
    expect(detailProblem(err('action_timeout'))).toBe('timeout')
    expect(detailProblem(err('omv_error'))).toBe('failed')
    expect(detailProblem(new Error('network'))).toBe('failed')
  })

  it('워크벤치 경로는 장치 파일을 인코딩한다', () => {
    expect(smartDeviceRoute('/dev/disk/by-id/ata-Fake_HDD_1')).toBe('storage/smart/devices/details/%2Fdev%2Fdisk%2Fby-id%2Fata-Fake_HDD_1')
  })
})

describe('개요', () => {
  it('OMV 평가로 안내 띠를 고른다', () => {
    expect(statusBanner(disk('GOOD', 'good'))).toBeNull()
    expect(statusBanner(disk('BAD_SECTOR', 'warning'))).toEqual({ key: 'badSector', level: 'warn' })
    expect(statusBanner(disk('BAD_ATTRIBUTE_IN_THE_PAST', 'failing'))).toEqual({ key: 'attributeInPast', level: 'crit' })
    expect(statusBanner(disk('BAD_STATUS', 'unknown'))).toEqual({ key: 'unknown', level: 'warn' })
  })

  it('ATA 타일은 섹터 속성을 보인다', () => {
    const d = ata([
      attr(5, 'Reallocated_Sector_Ct', 8, 'warning', true),
      attr(197, 'Current_Pending_Sector', 0),
      { ...attr(194, 'Temperature_Celsius', 34), raw: '34 (Min/Max 19/46)' },
    ])
    const tiles = overviewTiles(d)
    expect(tiles.map((t) => t.id)).toEqual(['temperature', 'powerOnHours', 'powerCycles', 'reallocated', 'pending', 'uncorrectable'])
    expect(tiles[0]).toMatchObject({ value: 34, sub: { kind: 'minMax', min: 19, max: 46 } })
    expect(tiles[3]).toMatchObject({ value: 8, level: 'warn', sub: { kind: 'attribute', id: 5 } })
    expect(tiles[4]).toMatchObject({ value: 0, level: 'normal' })
    // 속성이 없으면 값을 모른다.
    expect(tiles[5].value).toBeUndefined()
    expect(mediaBadges(d)).toEqual(['SATA', 'HDD'])
  })

  it('NVMe 타일은 건강 정보로 상태를 판단한다', () => {
    const d = nvme([
      field('percentageUsed', '100%', 100, 'percent'),
      field('availableSpare', '5%', 5, 'percent'),
      field('availableSpareThreshold', '10%', 10, 'percent'),
      field('mediaErrors', '2', 2, 'count'),
      field('errorLogEntries', '7', 7, 'count'),
      field('unsafeShutdowns', '23', 23, 'count'),
      field('temperatureSensor', '38 Celsius', 38, 'celsius', 1),
      field('temperatureSensor', '45 Celsius', 45, 'celsius', 2),
    ])
    const tiles = overviewTiles(d)
    expect(tiles.map((t) => t.id)).toEqual(['temperature', 'powerOnHours', 'powerCycles', 'percentageUsed', 'availableSpare', 'mediaErrors'])
    expect(tiles[0].sub).toEqual({ kind: 'sensorMax', value: 45 })
    expect(tiles[2].sub).toEqual({ kind: 'unsafeShutdowns', value: 23 })
    expect(tiles[3]).toMatchObject({ level: 'crit', meter: 100 })
    expect(tiles[4]).toMatchObject({ level: 'crit', sub: { kind: 'threshold', value: 10 } })
    expect(tiles[5]).toMatchObject({ level: 'warn', sub: { kind: 'errorLog', value: 7 } })
    expect(mediaBadges(d)).toEqual(['NVMe', 'SSD'])
  })
})

describe('표', () => {
  it('속성 색은 OMV 평가를 따르고 평가 없는 섹터 수는 0보다 크면 주의다', () => {
    expect(attributeLevel(attr(3, 'Spin_Up_Time', 0, 'failing', true))).toBe('crit')
    expect(attributeLevel(attr(5, 'Reallocated_Sector_Ct', 8, 'warning', true))).toBe('warn')
    expect(attributeLevel(attr(198, 'Offline_Uncorrectable', 2))).toBe('warn')
    expect(attributeLevel(attr(9, 'Power_On_Hours', 31482))).toBe('normal')
    expect(problemCount([attr(198, 'x', 2), attr(9, 'y', 1), attr(3, 'z', 0, 'failing', true)])).toBe(2)
  })

  it('NVMe 건강 정보를 묶음 순서대로 나누고 판단 항목에만 점을 단다', () => {
    const fields = [
      field('temperature', '38', 38, 'celsius'),
      field('criticalWarning', '0x00', 0, 'hex'),
      field('somethingNew', 'x'),
      field('powerCycles', '412', 412, 'count'),
    ]
    expect(healthGroups(fields).map((g) => g.id)).toEqual(['status', 'power', 'temperature', 'other'])
    const d = nvme(fields)
    expect(healthLevel(fields[1], d)).toBe('normal')
    expect(healthLevel({ ...fields[1], num: 4 }, d)).toBe('crit')
    expect(healthLevel(fields[3], d)).toBeUndefined()
    expect(smartctlName(field('temperatureSensor', '', 1, 'celsius', 2))).toBe('Temperature Sensor 2')
    expect(smartctlName(field('somethingNew', ''))).toBe('somethingNew')
  })

  it('자체 평가와 자가 진단 결과를 나눈다', () => {
    expect(selfAssessmentKey('PASSED')).toBe('passed')
    expect(selfAssessmentKey('OK')).toBe('passed')
    expect(selfAssessmentKey('FAILED!')).toBe('failed')
    expect(selfAssessmentKey('Unknown')).toBeNull()
    expect(testLevel('ok')).toBe('normal')
    expect(testLevel('failed')).toBe('crit')
    expect(testLevel('aborted')).toBe('idle')
  })
})

describe('번역 키', () => {
  it('목록의 모든 이름에 en·ko 문구가 있다', () => {
    for (const cat of [en, ko]) {
      for (const n of ATTR_NAMES) expect(cat.smart.attr[n], n).toBeTruthy()
      for (const k of INFO_KEYS) expect(cat.smart.info[k], k).toBeTruthy()
      for (const k of HEALTH_KEYS) expect(cat.smart.health[k], k).toBeTruthy()
    }
  })
})
