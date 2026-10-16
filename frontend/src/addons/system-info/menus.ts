// 시스템 정보 창 우클릭 메뉴의 항목. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ContextMenuItem } from '../../shell/contextMenu'
import type { CpuGraph, CpuScale } from './sysinfoPrefsStore'

// cpuGraphMenuItems는 개요 CPU 카드의 메뉴다(그래프 모양).
export function cpuGraphMenuItems(cur: CpuGraph, select: (g: CpuGraph) => void): ContextMenuItem[] {
  return [
    {
      kind: 'action',
      id: 'overall',
      labelKey: 'sysinfo.menu.overall',
      checked: cur === 'overall',
      onSelect: () => select('overall'),
    },
    {
      kind: 'action',
      id: 'cores',
      labelKey: 'sysinfo.menu.perCore',
      checked: cur === 'cores',
      onSelect: () => select('cores'),
    },
  ]
}

// processMenuItems는 프로세스 표(머리글·행)의 메뉴다. 프로세스 조작이 생기면 하위 메뉴 앞에 더한다.
export function processMenuItems(cur: CpuScale, select: (s: CpuScale) => void): ContextMenuItem[] {
  return [
    {
      kind: 'submenu',
      id: 'cpu-scale',
      labelKey: 'sysinfo.menu.cpuScale',
      items: [
        {
          kind: 'action',
          id: 'machine',
          labelKey: 'sysinfo.menu.scaleMachine',
          checked: cur === 'machine',
          onSelect: () => select('machine'),
        },
        {
          kind: 'action',
          id: 'core',
          labelKey: 'sysinfo.menu.scaleCore',
          checked: cur === 'core',
          onSelect: () => select('core'),
        },
      ],
    },
  ]
}
