// Colloid 아이콘 테마(vendor/icons/colloid, GPL-3.0). 대시보드의 기본 아이콘 출처다.
// 테이블에 import한 파일만 번들에 포함된다. 디렉터리 단위 glob을 쓰면 테마 전체가
// 실행 파일에 들어가므로, 아이콘을 쓸 때마다 여기에 한 줄씩 추가한다.

import documentOpen from '@colloid/actions/symbolic/document-open-symbolic.svg?url'
import editCopy from '@colloid/actions/symbolic/edit-copy-symbolic.svg?url'
import editDelete from '@colloid/actions/symbolic/edit-delete-symbolic.svg?url'
import editFind from '@colloid/actions/symbolic/edit-find-symbolic.svg?url'
import helpAbout from '@colloid/actions/symbolic/help-about-symbolic.svg?url'
import listAdd from '@colloid/actions/symbolic/list-add-symbolic.svg?url'
import listRemove from '@colloid/actions/symbolic/list-remove-symbolic.svg?url'
import mediaPlaybackStart from '@colloid/actions/symbolic/media-playback-start-symbolic.svg?url'
import mediaPlaybackStop from '@colloid/actions/symbolic/media-playback-stop-symbolic.svg?url'
import objectSelect from '@colloid/actions/symbolic/object-select-symbolic.svg?url'
import openMenu from '@colloid/actions/symbolic/open-menu-symbolic.svg?url'
import panEnd from '@colloid/actions/symbolic/pan-end-symbolic.svg?url'
import panStart from '@colloid/actions/symbolic/pan-start-symbolic.svg?url'
import systemLockScreen from '@colloid/actions/symbolic/system-lock-screen-symbolic.svg?url'
import systemLogOut from '@colloid/actions/symbolic/system-log-out-symbolic.svg?url'
import viewAppGrid from '@colloid/actions/symbolic/view-app-grid-symbolic.svg?url'
import viewRefresh from '@colloid/actions/symbolic/view-refresh-symbolic.svg?url'
import viewSortAscending from '@colloid/actions/symbolic/view-sort-ascending-symbolic.svg?url'
import windowClose from '@colloid/actions/symbolic/window-close-symbolic.svg?url'
import windowMaximize from '@colloid/actions/symbolic/window-maximize-symbolic.svg?url'
import windowMinimize from '@colloid/actions/symbolic/window-minimize-symbolic.svg?url'
import windowRestore from '@colloid/actions/symbolic/window-restore-symbolic.svg?url'
import gnomeDiskUtility from '@colloid/apps/scalable/org.gnome.DiskUtility.svg?url'
import preferencesSystemNetworkEthernet from '@colloid/apps/scalable/preferences-system-network-ethernet.svg?url'
import preferencesSystemNetworkServer from '@colloid/apps/scalable/preferences-system-network-server.svg?url'
import preferencesSystem from '@colloid/apps/scalable/preferences-system.svg?url'
import preferencesVirtualizationContainer from '@colloid/apps/scalable/preferences-virtualization-container.svg?url'
import thermalMonitor from '@colloid/apps/scalable/thermal-monitor.svg?url'
import utilitiesSystemMonitor from '@colloid/apps/scalable/utilities-system-monitor.svg?url'
import preferencesDesktopAppearance from '@colloid/apps/symbolic/preferences-desktop-appearance-symbolic.svg?url'
import preferencesSystemSymbolic from '@colloid/apps/symbolic/preferences-system-symbolic.svg?url'
import systemUsers from '@colloid/apps/symbolic/system-users-symbolic.svg?url'
import preferencesDesktopLocale from '@colloid/categories/symbolic/preferences-desktop-locale-symbolic.svg?url'
import driveHarddisk from '@colloid/devices/scalable/drive-harddisk.svg?url'
import folder from '@colloid/places/scalable/folder.svg?url'
import networkServer from '@colloid/places/symbolic/network-server-symbolic.svg?url'
import avatarDefault from '@colloid/status/symbolic/avatar-default-symbolic.svg?url'
import type { Icon } from '../types'

export const COLLOID_ICONS = {
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
  'media-playback-start-symbolic': { url: mediaPlaybackStart, kind: 'symbolic' },
  'media-playback-stop-symbolic': { url: mediaPlaybackStop, kind: 'symbolic' },
  'view-refresh-symbolic': { url: viewRefresh, kind: 'symbolic' },
  'view-app-grid-symbolic': { url: viewAppGrid, kind: 'symbolic' },
  'edit-find-symbolic': { url: editFind, kind: 'symbolic' },
  'view-sort-ascending-symbolic': { url: viewSortAscending, kind: 'symbolic' },
  'preferences-desktop-locale-symbolic': { url: preferencesDesktopLocale, kind: 'symbolic' },
  'avatar-default-symbolic': { url: avatarDefault, kind: 'symbolic' },
  'open-menu-symbolic': { url: openMenu, kind: 'symbolic' },
  'edit-delete-symbolic': { url: editDelete, kind: 'symbolic' },
  'edit-copy-symbolic': { url: editCopy, kind: 'symbolic' },
  'help-about-symbolic': { url: helpAbout, kind: 'symbolic' },
  'system-lock-screen-symbolic': { url: systemLockScreen, kind: 'symbolic' },
  'preferences-desktop-appearance-symbolic': { url: preferencesDesktopAppearance, kind: 'symbolic' },
  'preferences-system-symbolic': { url: preferencesSystemSymbolic, kind: 'symbolic' },
  'system-users-symbolic': { url: systemUsers, kind: 'symbolic' },
  'network-server-symbolic': { url: networkServer, kind: 'symbolic' },
  'utilities-system-monitor': { url: utilitiesSystemMonitor, kind: 'color' },
  'preferences-system-network-ethernet': { url: preferencesSystemNetworkEthernet, kind: 'color' },
  'thermal-monitor': { url: thermalMonitor, kind: 'color' },
  'preferences-virtualization-container': { url: preferencesVirtualizationContainer, kind: 'color' },
  'preferences-system-network-server': { url: preferencesSystemNetworkServer, kind: 'color' },
  'org.gnome.DiskUtility': { url: gnomeDiskUtility, kind: 'color' },
  'drive-harddisk': { url: driveHarddisk, kind: 'color' },
  folder: { url: folder, kind: 'color' },
  'preferences-system': { url: preferencesSystem, kind: 'color' },
} as const satisfies Record<string, Icon>
