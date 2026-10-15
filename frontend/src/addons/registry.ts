// 내장 애드온의 보기 컴포넌트 목록. 키는 manifest 목록(manifests.gen.ts)의 애드온 id와 같아야 한다.
// 애드온마다 manifest가 선언한 보기(창·위젯)의 컴포넌트가 모두 있어야 하며, 빠지거나 없는 id가 있으면
// tsc 오류가 난다. 보기 모듈을 동적 import하고 React를 쓰므로 node 테스트 대상 모듈(windows/apps.ts 등)에서는
// import하지 않는다.
import { type ComponentType, lazy } from 'react'
import { type AddonId, MANIFESTS } from './manifests.gen'
import type { WidgetViewProps, WindowViewProps } from './types'

// ViewsOf는 manifest의 views에 맞춰 필요한 보기 컴포넌트를 정한다.
type ViewsOf<V> = (V extends { window: object } ? { Window: ComponentType<WindowViewProps> } : unknown) &
  (V extends { widget: object } ? { Widget: ComponentType<WidgetViewProps> } : unknown)

export type AddonViews<K extends AddonId> = ViewsOf<(typeof MANIFESTS)[K]['views']>

// lazyView는 보기 컴포넌트를 처음 그릴 때 내려받게 한다. 동적 import마다 청크가 나뉘어, 바탕화면에 놓거나
// 연 애드온의 코드만 받는다. 목록과 id는 그대로 컴파일 타임에 정해진다. 그리는 쪽은 shell/ViewBoundary로
// 감싸야 한다(받는 동안의 Suspense, 받지 못했을 때의 안내).
export function lazyView<M, P>(load: () => Promise<M>, pick: (m: M) => ComponentType<P>): ComponentType<P> {
  return lazy(async () => ({ default: pick(await load()) }))
}

export const ADDON_VIEWS: { [K in AddonId]: AddonViews<K> } = {
  'system-info': {
    Window: lazyView(() => import('./system-info/SystemInfoWindow'), (m) => m.SystemInfoWindow),
    Widget: lazyView(() => import('./system-info/SystemInfoWidget'), (m) => m.SystemInfoWidget),
  },
  storage: {
    Window: lazyView(() => import('./storage/StorageWindow'), (m) => m.StorageWindow),
    Widget: lazyView(() => import('./storage/StorageWidget'), (m) => m.StorageWidget),
  },
  network: {
    Window: lazyView(() => import('./network/NetworkWindow'), (m) => m.NetworkWindow),
    Widget: lazyView(() => import('./network/NetworkWidget'), (m) => m.NetworkWidget),
  },
  temperature: {
    Window: lazyView(() => import('./temperature/TemperatureWindow'), (m) => m.TemperatureWindow),
    Widget: lazyView(() => import('./temperature/TemperatureWidget'), (m) => m.TemperatureWidget),
  },
  containers: {
    Window: lazyView(() => import('./containers/ContainersWindow'), (m) => m.ContainersWindow),
    Widget: lazyView(() => import('./containers/ContainersWidget'), (m) => m.ContainersWidget),
  },
  smart: {
    Window: lazyView(() => import('./smart/SmartWindow'), (m) => m.SmartWindow),
    Widget: lazyView(() => import('./smart/SmartWidget'), (m) => m.SmartWidget),
  },
  services: {
    Window: lazyView(() => import('./services/ServicesWindow'), (m) => m.ServicesWindow),
    Widget: lazyView(() => import('./services/ServicesWidget'), (m) => m.ServicesWidget),
  },
}
