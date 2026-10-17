// 시스템 정보 창 우클릭 메뉴의 항목. DOM에 의존하지 않아 node 환경에서 테스트한다.
// 설정을 불러오기 전이나 불러오지 못했으면(disabled) 항목을 보이되 고를 수 없다.
import type { ContextMenuAction, ContextMenuItem } from '../../shell/contextMenu'
import type { CpuGraph, CpuScale } from './sysinfoPrefsStore'

function choice<T>(
  id: string,
  labelKey: ContextMenuAction['labelKey'],
  value: T,
  cur: T,
  select: (v: T) => void,
  disabled: boolean,
): ContextMenuAction {
  return { kind: 'action', id, labelKey, checked: cur === value, disabled, onSelect: () => select(value) }
}

// cpuGraphMenuItems는 성능 페이지 CPU 그래프의 메뉴다(그래프 모양, 커널 시간 표시).
export function cpuGraphMenuItems(
  cur: CpuGraph,
  kernel: boolean,
  select: (g: CpuGraph) => void,
  setKernel: (on: boolean) => void,
  disabled = false,
): ContextMenuItem[] {
  return [
    choice<CpuGraph>('overall', 'sysinfo.menu.overall', 'overall', cur, select, disabled),
    choice<CpuGraph>('cores', 'sysinfo.menu.perCore', 'cores', cur, select, disabled),
    { kind: 'separator', id: 'sep-kernel' },
    {
      kind: 'action',
      id: 'kernel',
      labelKey: 'sysinfo.menu.showKernel',
      checked: kernel,
      disabled,
      onSelect: () => setKernel(!kernel),
    },
  ]
}

// processMenuItems는 프로세스 표(머리글·행)의 메뉴다. 프로세스 조작이 생기면 하위 메뉴 앞에 더한다.
export function processMenuItems(cur: CpuScale, select: (s: CpuScale) => void, disabled = false): ContextMenuItem[] {
  return [
    {
      kind: 'submenu',
      id: 'cpu-scale',
      labelKey: 'sysinfo.menu.cpuScale',
      disabled,
      items: [
        choice<CpuScale>('machine', 'sysinfo.menu.scaleMachine', 'machine', cur, select, disabled),
        choice<CpuScale>('core', 'sysinfo.menu.scaleCore', 'core', cur, select, disabled),
      ],
    },
  ]
}
