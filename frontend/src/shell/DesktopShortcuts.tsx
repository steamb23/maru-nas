// 바탕화면 바로가기: 사용자가 고른 앱 아이콘을 왼쪽 위부터 열 단위로 놓는다. 위젯 레이어와 창 아래에 그린다.
// 목록은 shortcuts/store.ts(사용자별 저장), 순서 계산은 shortcuts/list.ts가 한다.
import { type PointerEvent, type Ref, type RefObject, useEffect, useImperativeHandle, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { appAccess, useCapabilitiesStore } from '../addons/capabilities'
import { placeShortcut, removeShortcut, visibleShortcuts } from '../shortcuts/list'
import { useShortcutStore } from '../shortcuts/store'
import { APP_IDS, type AppId, getApp, isAppId } from '../windows/apps'
import type { Size } from '../windows/geometry'
import { useWindowStore } from '../windows/store'
import type { ContextMenuAction, ContextMenuItem } from './contextMenu'
import { DesktopIcon } from './DesktopIcon'
import { useContextMenuTrigger } from './useContextMenuTrigger'

// DesktopShortcutsHandle은 바탕화면 메뉴(Desktop)가 바로가기에 묻는 것이다.
export interface DesktopShortcutsHandle {
  // 바탕화면 우클릭 메뉴에 넣을 항목("바로가기 추가 ▸"). 목록을 아직 불러오지 못했으면 빈 목록.
  desktopMenuItems: () => ContextMenuItem[]
}

interface Props {
  // 데스크톱 영역(작업표시줄 제외) 크기. 창을 열 때 쓴다.
  bounds: Size
  // 아이콘 목록 요소. 위젯 레이어가 크기를 재서 이 영역을 피하고, 바탕화면 메뉴가 빈 곳인지 판단한다.
  listRef: RefObject<HTMLUListElement | null>
  ref?: Ref<DesktopShortcutsHandle>
}

// 목록을 불러오지 못했을 때 다시 시도하는 간격
const RETRY_MS = 10_000

const { load, clear } = useShortcutStore.getState()

// 저장 목록을 바꾼다. 계산은 늘 저장소의 최신 목록으로 한다.
const update = (change: (apps: AppId[]) => AppId[]) => {
  const s = useShortcutStore.getState()
  s.set(change(s.apps))
}

export function DesktopShortcuts({ bounds, listRef, ref }: Props) {
  const { t } = useTranslation()
  const status = useShortcutStore((s) => s.status)
  const apps = useShortcutStore((s) => s.apps)
  const caps = useCapabilitiesStore((s) => s.caps)

  // 로그인한 동안만 그려진다. 로그아웃·만료로 언마운트되면 다음 사용자를 위해 비운다.
  useEffect(() => {
    void load()
    return clear
  }, [])

  useEffect(() => {
    if (status !== 'error') return
    const timer = setTimeout(() => void load(), RETRY_MS)
    return () => clearTimeout(timer)
  }, [status])

  // 관리자 전용 앱은 일반 사용자에게 그리지 않는다(저장 목록에는 남는다).
  const include = (id: AppId) => appAccess(caps, id).visible
  const visible = status === 'ready' ? visibleShortcuts(apps, include) : []

  const open = (id: AppId) => useWindowStore.getState().open(id, bounds)

  // 아이콘 우클릭·메뉴 키·길게 누르기 메뉴. 대상 앱은 입력을 받은 버튼의 data-app이다.
  const menu = useContextMenuTrigger({
    items: (el) => {
      const id = el.dataset.app
      if (!id || !isAppId(id)) return []
      return [
        { kind: 'action', id: 'open', labelKey: 'shortcuts.open', icon: 'document-open-symbolic', onSelect: () => open(id) },
        { kind: 'separator', id: 'sep' },
        {
          kind: 'action',
          id: 'remove',
          labelKey: 'shortcuts.remove',
          icon: 'list-remove-symbolic',
          onSelect: () => update((list) => removeShortcut(list, id)),
        },
      ]
    },
  })

  // 길게 눌러 메뉴를 연 누름 뒤에 오는 click은 무시한다(창을 열지 않는다). 다음 누름에서 풀린다.
  const suppressClick = useRef(false)
  const onPointerDown = (e: PointerEvent<HTMLElement>) => {
    menu.handlers.onPointerDown(e)
    suppressClick.current = false
  }
  const onPointerUp = (e: PointerEvent<HTMLElement>) => {
    menu.handlers.onPointerUp(e)
    if (menu.openedByPress()) suppressClick.current = true
  }
  const onClick = (id: AppId) => {
    if (suppressClick.current) suppressClick.current = false
    else open(id)
  }

  // "바로가기 추가 ▸" 하위 메뉴. 볼 수 있는 앱마다 하나씩, 이미 있는 앱은 체크 표시와 함께 비활성이다.
  useImperativeHandle(ref, () => ({
    desktopMenuItems: () => {
      if (status !== 'ready') return []
      const items = APP_IDS.filter(include).map((id): ContextMenuAction => {
        const app = getApp(id)
        const added = apps.includes(id)
        return {
          kind: 'action',
          id,
          labelKey: app.titleKey,
          icon: app.icon,
          checked: added,
          disabled: added,
          onSelect: () => update((list) => placeShortcut(list, id, Infinity, include)),
        }
      })
      if (items.length === 0) return []
      return [{ kind: 'submenu', id: 'add-shortcut', labelKey: 'shortcuts.add', icon: 'list-add-symbolic', items }]
    },
  }))

  return (
    <ul className="desktop-icons" aria-label={t('desktop.label')} ref={listRef}>
      {visible.map((id) => (
        <li key={id}>
          <DesktopIcon
            appId={id}
            {...menu.handlers}
            onPointerDown={onPointerDown}
            onPointerUp={onPointerUp}
            onClick={() => onClick(id)}
          />
        </li>
      ))}
    </ul>
  )
}
