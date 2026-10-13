import { IconImage } from '../icons/IconImage'
import { getApp } from '../windows/apps'
import { type WindowState, selectActiveId, useWindowStore } from '../windows/store'

const { focus, minimize, restore } = useWindowStore.getState()

// 작업표시줄 버튼: 최소화된 창은 복원, 활성 창은 최소화, 그 밖의 창은 맨 앞으로.
function onTaskClick(w: WindowState, active: boolean) {
  if (w.minimized) restore(w.id)
  else if (active) minimize(w.id)
  else focus(w.id)
}

export function Taskbar() {
  const windows = useWindowStore((s) => s.windows)
  const activeId = useWindowStore(selectActiveId)

  return (
    <nav className="taskbar" aria-label="작업표시줄">
      <ul className="taskbar-items">
        {windows.map((w) => {
          const active = w.id === activeId
          const classes = ['task', active && 'active', w.minimized && 'minimized'].filter(Boolean).join(' ')
          return (
            <li key={w.id}>
              <button type="button" className={classes} aria-pressed={active} onClick={() => onTaskClick(w, active)}>
                <IconImage name={getApp(w.appId).icon} size={20} />
                <span className="task-title">{w.title}</span>
              </button>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
