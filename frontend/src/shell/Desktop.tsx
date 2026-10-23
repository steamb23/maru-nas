// 데스크톱 셸: 바탕화면 바로가기, 위젯 레이어, 창 레이어, "모든 앱" 화면, 알림 센터, 작업표시줄, 토스트,
// 컨텍스트 메뉴, 데스크톱 범위 모달.
import { useEffect, useEffectEvent, useLayoutEffect, useRef, useState } from 'react'
import { useShallow } from 'zustand/react/shallow'
import type { NoticeAction } from '../api/types.gen'
import { useNotificationStore } from '../notifications/store'
import type { SettingsPageId } from '../settings/pages'
import { useSettingsNavStore } from '../settings/navStore'
import { useAuthStore } from '../auth/store'
import { useShortcutStore } from '../shortcuts/store'
import { clearUpgradeHint, hasUpgradeHint } from '../upgrade/resultHint'
import type { Size } from '../windows/geometry'
import { type WindowState, type WindowStore, stableOrder, useWindowStore } from '../windows/store'
import { AppDrawer } from './AppDrawer'
import { useAppDrawerStore } from './appDrawerStore'
import { ContextMenu } from './ContextMenu'
import type { ContextMenuItem } from './contextMenu'
import { DialogOutlet } from './DialogOutlet'
import { NotificationCenter } from './NotificationCenter'
import { ToastLayer } from './ToastLayer'
import { runNoticeAction } from './noticeAction'
import { type DesktopShortcutsHandle, DesktopShortcuts } from './DesktopShortcuts'
import { Taskbar } from './Taskbar'
import { TileDivider } from './TileDivider'
import { type WidgetLayerHandle, WidgetLayer } from './WidgetLayer'
import { launchApp } from './launchApp'
import { enterDesktop } from './screenTransition'
import { type ClosingWindow, Window } from './Window'
import { useContextMenuStore } from './contextMenuStore'
import { DesktopModalContext, useModalHost } from './modalScope'
import { useClosingWindows } from './useClosingWindows'
import { useContextMenuTrigger } from './useContextMenuTrigger'

// 닫힌 창은 마지막 상태와 그때의 타일 그룹 비율로 그린다.
const closingSnapshot = (win: WindowState, prev: WindowStore): ClosingWindow => ({ win, tileSplit: prev.tileSplit })

const DESKTOP_SCOPE = { kind: 'desktop' } as const

export function Desktop() {
  const rootRef = useRef<HTMLDivElement>(null)
  const areaRef = useRef<HTMLDivElement>(null)
  const iconsRef = useRef<HTMLUListElement>(null)
  const widgetsRef = useRef<WidgetLayerHandle>(null)
  const shortcutsRef = useRef<DesktopShortcutsHandle>(null)
  // 바탕화면 빈 곳(아이콘 사이 포함)의 우클릭 메뉴. 창·위젯·아이콘 위에서는 열지 않는다.
  // 항목은 기능을 가진 쪽이 만든다: "바로가기 추가 ▸"는 바로가기, "위젯 추가 ▸"는 위젯 레이어.
  // 맨 아래에 구분선을 두고 설정의 화면 페이지를 여는 항목을 둔다.
  const desktopMenu = useContextMenuTrigger({
    items: () => {
      const add = [
        ...(shortcutsRef.current?.desktopMenuItems() ?? []),
        ...(widgetsRef.current?.desktopMenuItems() ?? []),
      ]
      const settings: ContextMenuItem = {
        kind: 'action',
        id: 'screen-settings',
        labelKey: 'desktop.openScreenSettings',
        icon: 'preferences-desktop-appearance-symbolic',
        onSelect: () => openSettings('screen'),
      }
      return add.length > 0 ? [...add, { kind: 'separator', id: 'screen-settings-separator' }, settings] : [settings]
    },
    accept: (e) => e.target === e.currentTarget || e.target === iconsRef.current,
  })
  // 데스크톱 범위 모달. 열려 있으면 창·위젯·아이콘·작업표시줄·토스트를 모두 막고, 열린 컨텍스트 메뉴와 "모든 앱" 화면,
  // 알림 센터, 진행 중인 바로가기 끌기는 끝낸다.
  const { setHost: setModalHost, layer: modalLayer, blocked } = useModalHost()
  useEffect(() => {
    if (!blocked) return
    useContextMenuStore.getState().close()
    useAppDrawerStore.getState().close()
    useNotificationStore.getState().setCenterOpen(false)
    shortcutsRef.current?.cancelDrag()
  }, [blocked])
  // "모든 앱" 화면이 열려 있으면 그 뒤의 데스크톱 영역(창·위젯·아이콘)은 입력을 받지 않는다. 작업표시줄은 그대로 쓴다.
  const drawerOpen = useAppDrawerStore((s) => s.open)
  // 데스크톱 영역(작업표시줄 제외) 크기. 창 위치 제한의 기준이다.
  const [bounds, setBounds] = useState<Size>(() => ({ width: window.innerWidth, height: window.innerHeight }))
  // 창을 옮길 때마다 Desktop 전체가 다시 그려지지 않도록 id 목록만 구독한다.
  const ids = useWindowStore(useShallow((s) => s.windows.map((w) => w.id)))
  // 바로가기 목록을 불러오기(또는 실패하기) 전에는 아이콘 영역 크기를 알 수 없으므로 위젯을 그리지 않는다.
  // 먼저 그리면 위젯이 아이콘 자리에 놓였다가 바로가기가 들어오며 밀려난다.
  // 한 번 불러온 뒤에는 다시 시도하는 동안에도 위젯을 그대로 둔다(loaded).
  const iconsReady = useShortcutStore((s) => s.loaded)
  // 창은 처음 열린 순서대로 그린다. 앞뒤는 z-index가 정하므로, 작업표시줄에서 순서를 바꿔도
  // 창 DOM이 옮겨지지 않게 한다(옮겨지면 창 안의 스크롤 위치나 iframe이 초기화된다).
  // 닫힌 창은 저장소에서 바로 지워지지만, 닫기 애니메이션이 끝나 onClosed가 불릴 때까지
  // 마지막 상태(closing)로 계속 그린다.
  const [closing, onClosed] = useClosingWindows(closingSnapshot)
  const [order, setOrder] = useState<string[]>([])
  const renderIds = stableOrder(order, ids, closing)
  if (renderIds.length !== order.length || renderIds.some((id, i) => id !== order[i])) setOrder(renderIds)

  useLayoutEffect(() => {
    const el = areaRef.current
    if (!el) return
    const measure = () => {
      const b = { width: el.clientWidth, height: el.clientHeight }
      setBounds(b)
      // 브라우저 창이 줄어 화면 밖으로 밀려난 창을 다시 끌어온다.
      useWindowStore.getState().clampAll(b)
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // 설정 창을 그 페이지로 연다. 이미 열려 있으면 페이지만 바꾸고 앞으로 가져온다.
  const openSettings = (page: SettingsPageId) => {
    useSettingsNavStore.getState().show(page)
    launchApp('settings', bounds)
  }

  // 알림 센터·토스트의 동작(앱·설정 열기, 워크벤치, 전원 대화상자)을 실행한다.
  const runNotice = (action: NoticeAction) =>
    runNoticeAction(action, { openApp: (id) => launchApp(id, bounds), openSettings })

  // 로그인하거나 세션을 복원해 처음 그려질 때 내용이 커지며 나타난다.
  useLayoutEffect(() => enterDesktop(rootRef.current), [])

  // 업그레이드로 다시 시작한 뒤 로그인했으면(upgrade/resultHint.ts) 관리자에게 설정의 업데이트 페이지로 결과를 보인다.
  // 처음 그릴 때 한 번만 본다. 관리자가 아니면 힌트만 지운다.
  const admin = useAuthStore((s) => s.user?.admin ?? false)
  const showUpgradeResult = useEffectEvent(() => {
    if (admin) openSettings('update')
  })
  useEffect(() => {
    if (!hasUpgradeHint()) return
    clearUpgradeHint()
    showUpgradeResult()
  }, [])

  return (
    <DesktopModalContext value={modalLayer}>
      <div className="desktop" ref={rootRef}>
        <div className="desktop-area" ref={areaRef} inert={blocked || drawerOpen} {...desktopMenu.handlers}>
          <DesktopShortcuts bounds={bounds} listRef={iconsRef} ref={shortcutsRef} />
          <WidgetLayer bounds={bounds} iconsRef={iconsRef} iconsReady={iconsReady} ref={widgetsRef} />
          <TileDivider bounds={bounds} />
          {renderIds.map((id) => (
            <Window key={id} id={id} bounds={bounds} closingState={closing.get(id)} onClosed={onClosed} />
          ))}
        </div>
        <AppDrawer bounds={bounds} shortcuts={shortcutsRef} />
        <NotificationCenter onAction={runNotice} />
        <Taskbar inert={blocked} onOpenSettings={openSettings} onNewWindow={(appId) => launchApp(appId, bounds)} />
        <ToastLayer inert={blocked} onAction={runNotice} />
        <ContextMenu />
        <div className="modal-host modal-host-desktop" ref={setModalHost} />
        <DialogOutlet scope={DESKTOP_SCOPE} />
      </div>
    </DesktopModalContext>
  )
}
