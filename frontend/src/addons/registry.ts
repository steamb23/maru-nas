// 내장 애드온의 보기 컴포넌트 목록. 키는 manifest 목록(manifests.gen.ts)의 애드온 id와 같아야 한다.
// 애드온마다 manifest가 선언한 보기(창·위젯)의 컴포넌트가 모두 있어야 하며, 빠지거나 없는 id가 있으면
// tsc 오류가 난다. 컴포넌트를 import하므로 node 테스트 대상 모듈(windows/apps.ts 등)에서는 import하지 않는다.
import type { ComponentType } from 'react'
import { type AddonId, MANIFESTS } from './manifests.gen'
import { ContainersWidget } from './containers/ContainersWidget'
import { ContainersWindow } from './containers/ContainersWindow'
import { NetworkWidget } from './network/NetworkWidget'
import { NetworkWindow } from './network/NetworkWindow'
import { ServicesWidget } from './services/ServicesWidget'
import { ServicesWindow } from './services/ServicesWindow'
import { SmartWidget } from './smart/SmartWidget'
import { SmartWindow } from './smart/SmartWindow'
import { StorageWidget } from './storage/StorageWidget'
import { StorageWindow } from './storage/StorageWindow'
import { TemperatureWidget } from './temperature/TemperatureWidget'
import { TemperatureWindow } from './temperature/TemperatureWindow'
import { SystemInfoWidget } from './system-info/SystemInfoWidget'
import { SystemInfoWindow } from './system-info/SystemInfoWindow'
import type { WidgetViewProps, WindowViewProps } from './types'

// ViewsOf는 manifest의 views에 맞춰 필요한 보기 컴포넌트를 정한다.
type ViewsOf<V> = (V extends { window: object } ? { Window: ComponentType<WindowViewProps> } : unknown) &
  (V extends { widget: object } ? { Widget: ComponentType<WidgetViewProps> } : unknown)

export type AddonViews<K extends AddonId> = ViewsOf<(typeof MANIFESTS)[K]['views']>

export const ADDON_VIEWS: { [K in AddonId]: AddonViews<K> } = {
  'system-info': { Window: SystemInfoWindow, Widget: SystemInfoWidget },
  storage: { Window: StorageWindow, Widget: StorageWidget },
  network: { Window: NetworkWindow, Widget: NetworkWidget },
  temperature: { Window: TemperatureWindow, Widget: TemperatureWidget },
  containers: { Window: ContainersWindow, Widget: ContainersWidget },
  smart: { Window: SmartWindow, Widget: SmartWidget },
  services: { Window: ServicesWindow, Widget: ServicesWidget },
}
