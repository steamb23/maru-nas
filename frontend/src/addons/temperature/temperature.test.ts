import { describe, expect, it } from 'vitest'
import type { SensorGroup } from '../../api/types.gen'
import { headline, isTemperatureResponse, summaryRows, tempLevel } from './temperature'

const group = (over: Partial<SensorGroup>): SensorGroup => ({
  id: 'g',
  kind: 'cpu',
  name: 'coretemp',
  source: 'hwmon',
  sensors: [],
  ...over,
})

describe('isTemperatureResponse', () => {
  it('정상 응답을 받는다', () => {
    const g = group({ sensors: [{ label: 'Core 0', celsius: 40, critCelsius: 100 }], checkedAt: '2026-01-01T00:00:00Z' })
    expect(isTemperatureResponse({ sources: ['hwmon'], groups: [g] })).toBe(true)
  })
  it('모르는 종류는 거부한다', () => {
    expect(isTemperatureResponse({ sources: [], groups: [{ ...group({}), kind: 'gpu' }] })).toBe(false)
  })
})

describe('tempLevel', () => {
  it('칩의 경고 온도를 기준으로 쓴다', () => {
    expect(tempLevel({ celsius: 70, highCelsius: 80 }, 'disk')).toBe('warn')
    expect(tempLevel({ celsius: 80, highCelsius: 80 }, 'disk')).toBe('crit')
    expect(tempLevel({ celsius: 60, highCelsius: 80 }, 'disk')).toBe('normal')
  })
  it('기준이 없으면 종류별 기본값을 쓴다', () => {
    expect(tempLevel({ celsius: 52 }, 'disk')).toBe('warn')
    expect(tempLevel({ celsius: 52 }, 'cpu')).toBe('normal')
  })
})

describe('headline', () => {
  it('CPU 패키지 온도를 대표로 쓴다', () => {
    const g = group({ sensors: [{ label: 'Core 0', celsius: 60 }, { label: 'Package id 0', celsius: 55 }] })
    expect(headline(g)?.celsius).toBe(55)
  })
  it('대표 이름이 없으면 가장 뜨거운 센서', () => {
    const g = group({ sensors: [{ celsius: 30 }, { celsius: 41 }, { celsius: 35 }] })
    expect(headline(g)?.celsius).toBe(41)
    expect(headline(group({}))).toBeNull()
  })
})

describe('summaryRows', () => {
  it('CPU와 디스크만 보여 준다', () => {
    const rows = summaryRows([
      group({ id: 'c', sensors: [{ celsius: 50 }] }),
      group({ id: 'd', kind: 'disk', name: 'nvme', device: 'nvme0', sensors: [{ celsius: 40 }] }),
      group({ id: 'o', kind: 'other', name: 'acpitz', sensors: [{ celsius: 30 }] }),
    ])
    expect(rows.map((r) => [r.kind, r.name])).toEqual([
      ['cpu', 'coretemp'],
      ['disk', 'nvme0'],
    ])
  })
  it('CPU·디스크가 없으면 그 밖의 센서를 보여 준다', () => {
    const rows = summaryRows([group({ id: 'o', kind: 'other', name: 'acpitz', sensors: [{ celsius: 30 }] })])
    expect(rows.map((r) => r.name)).toEqual(['acpitz'])
  })
})
