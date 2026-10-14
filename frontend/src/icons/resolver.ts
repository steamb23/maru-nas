// 아이콘 리졸버. 컴포넌트는 아이콘 경로 대신 freedesktop 아이콘 이름으로 getIcon()을 부른다.
// 테마를 바꿀 때는 이 파일의 매핑 테이블만 수정한다.
//
// 테이블에 import한 파일만 번들에 포함된다. 디렉터리 단위 glob을 쓰면 테마 전체가
// 실행 파일에 들어가므로, 아이콘을 쓸 때마다 여기에 한 줄씩 추가한다.

import documentOpen from '@colloid/actions/symbolic/document-open-symbolic.svg?url'
import listAdd from '@colloid/actions/symbolic/list-add-symbolic.svg?url'
import listRemove from '@colloid/actions/symbolic/list-remove-symbolic.svg?url'
import objectSelect from '@colloid/actions/symbolic/object-select-symbolic.svg?url'
import panEnd from '@colloid/actions/symbolic/pan-end-symbolic.svg?url'
import panStart from '@colloid/actions/symbolic/pan-start-symbolic.svg?url'
import systemLogOut from '@colloid/actions/symbolic/system-log-out-symbolic.svg?url'
import windowClose from '@colloid/actions/symbolic/window-close-symbolic.svg?url'
import windowMaximize from '@colloid/actions/symbolic/window-maximize-symbolic.svg?url'
import windowMinimize from '@colloid/actions/symbolic/window-minimize-symbolic.svg?url'
import windowRestore from '@colloid/actions/symbolic/window-restore-symbolic.svg?url'
import gnomeDiskUtility from '@colloid/apps/scalable/org.gnome.DiskUtility.svg?url'
import preferencesSystemNetworkEthernet from '@colloid/apps/scalable/preferences-system-network-ethernet.svg?url'
import preferencesVirtualizationContainer from '@colloid/apps/scalable/preferences-virtualization-container.svg?url'
import thermalMonitor from '@colloid/apps/scalable/thermal-monitor.svg?url'
import utilitiesSystemMonitor from '@colloid/apps/scalable/utilities-system-monitor.svg?url'
import preferencesDesktopLocale from '@colloid/categories/symbolic/preferences-desktop-locale-symbolic.svg?url'
import driveHarddisk from '@colloid/devices/scalable/drive-harddisk.svg?url'
import folder from '@colloid/places/scalable/folder.svg?url'
import avatarDefault from '@colloid/status/symbolic/avatar-default-symbolic.svg?url'

// color: 컬러 아이콘. <img>로 그대로 쓴다.
// symbolic: 단색 -symbolic 아이콘. IconImage가 mask-image로 currentColor를 입힌다.
export type IconKind = 'color' | 'symbolic'

export interface Icon {
  url: string
  kind: IconKind
}

const ICONS = {
  'window-close-symbolic': { url: windowClose, kind: 'symbolic' },
  'window-maximize-symbolic': { url: windowMaximize, kind: 'symbolic' },
  'window-minimize-symbolic': { url: windowMinimize, kind: 'symbolic' },
  'window-restore-symbolic': { url: windowRestore, kind: 'symbolic' },
  'system-log-out-symbolic': { url: systemLogOut, kind: 'symbolic' },
  'object-select-symbolic': { url: objectSelect, kind: 'symbolic' },
  'pan-start-symbolic': { url: panStart, kind: 'symbolic' },
  'pan-end-symbolic': { url: panEnd, kind: 'symbolic' },
  'list-add-symbolic': { url: listAdd, kind: 'symbolic' },
  'list-remove-symbolic': { url: listRemove, kind: 'symbolic' },
  'document-open-symbolic': { url: documentOpen, kind: 'symbolic' },
  'preferences-desktop-locale-symbolic': { url: preferencesDesktopLocale, kind: 'symbolic' },
  'avatar-default-symbolic': { url: avatarDefault, kind: 'symbolic' },
  'utilities-system-monitor': { url: utilitiesSystemMonitor, kind: 'color' },
  'preferences-system-network-ethernet': { url: preferencesSystemNetworkEthernet, kind: 'color' },
  'thermal-monitor': { url: thermalMonitor, kind: 'color' },
  'preferences-virtualization-container': { url: preferencesVirtualizationContainer, kind: 'color' },
  'org.gnome.DiskUtility': { url: gnomeDiskUtility, kind: 'color' },
  'drive-harddisk': { url: driveHarddisk, kind: 'color' },
  folder: { url: folder, kind: 'color' },
} as const satisfies Record<string, Icon>

export type IconName = keyof typeof ICONS

export function getIcon(name: IconName): Icon {
  return ICONS[name]
}
