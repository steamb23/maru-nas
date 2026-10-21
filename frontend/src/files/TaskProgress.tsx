// 파일 작업의 진행 부품. 파일 작업 창(TransfersWindow)의 작업 줄과 탐색기 창 안의 진행 배너(여러 항목 작업, 날짜·크기순
// 준비)가 같은 진행 막대와 지금 항목 줄을 쓴다.
import { IconImage } from '../icons/IconImage'

// ProgressMeter는 진행 막대다. 양을 모르면(fraction이 null) 짧은 막대가 오간다.
export function ProgressMeter({ fraction, label }: { fraction: number | null; label: string }) {
  const value = fraction === null ? null : Math.min(1, Math.max(0, fraction))
  return (
    <div
      className={`meter task-meter${value === null ? ' busy' : ''}`}
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={value === null ? undefined : Math.round(value * 100)}
    >
      <div className="meter-fill" style={value === null ? undefined : { width: `${value * 100}%` }} />
    </div>
  )
}

// TaskCurrent는 지금 하는 항목(이름이나 상대 경로) 한 줄이다. 길면 줄인다.
export function TaskCurrent({ name }: { name: string }) {
  return (
    <p className="task-current" title={name}>
      {name}
    </p>
  )
}

interface BannerProps {
  label: string
  // count는 진행 수("12 / 40")다.
  count: string
  fraction: number | null
  // current는 지금 하는 항목이다(없으면 줄을 그리지 않는다).
  current?: string | null
  // action은 멈추는 단추다.
  action: { label: string; title?: string; onClick: () => void }
}

// TaskBanner는 탐색기 창 안의 진행 배너다(여러 항목 작업, 날짜·크기순 준비).
export function TaskBanner({ label, count, fraction, current, action }: BannerProps) {
  return (
    <div className="task-banner">
      <div className="task-banner-line" role="status">
        <IconImage name="dialog-information-symbolic" size={16} />
        <span className="task-banner-label">{label}</span>
        <span className="task-banner-count">{count}</span>
        <button type="button" className="addon-button" title={action.title} onClick={action.onClick}>
          {action.label}
        </button>
      </div>
      <ProgressMeter fraction={fraction} label={label} />
      {current && <TaskCurrent name={current} />}
    </div>
  )
}
