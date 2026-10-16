// 작업표시줄 창 버튼 넘침 계산(DOM 없음). 너비 측정과 그리기는 Taskbar.tsx가 한다.
//
// 버튼이 다 들어가지 않으면 단계적으로 줄인다.
//  1. 제목을 말줄임으로 줄인다. 모든 버튼에 같은 최대 너비(cap)를 걸어 긴 제목부터 줄어든다.
//  2. cap이 TASK_LABEL_MIN보다 작아지면 모든 버튼을 아이콘만으로 그린다.
//  3. 아이콘만으로도 넘치면 앞에서부터 들어가는 만큼만 보이고, 나머지는 "더 보기" 버튼의 메뉴로 접는다.
//     순서(windows[])는 바꾸지 않으므로 숨는 쪽은 늘 뒤쪽이다.
//
// 입력은 결과에 의존하지 않아야 한다(결과로 그린 화면을 다시 재도 입력이 같아 흔들리지 않게).
// 그래서 "더 보기" 버튼 자리는 available에 넣지 않고 moreSpace로 따로 받아 여기서 뺀다.

// 아래 값은 Taskbar.css의 .task(padding 0 10px, gap 6px, max-width)와 아이콘 크기(Taskbar.tsx의 ICON_SIZE 24)와 맞춘다.
// 버튼 최대 너비. 제목이 이보다 길면 공간이 남아도 말줄임으로 자른다.
export const TASK_MAX_WIDTH = 200
// 아이콘만 그린 버튼 너비: 아이콘 24 + 좌우 padding 10
export const TASK_ICON_WIDTH = 44
// 아이콘과 제목 사이 간격
export const TASK_TITLE_GAP = 6
// 제목을 보이는 버튼의 최소 너비. cap이 이보다 작아지면 아이콘만 그린다.
// 제목 칸 56px: 14px 글꼴에서 한글 세 글자(14px씩)와 말줄임표(14px), 라틴 문자는 다섯 글자 안팎이 들어간다.
export const TASK_LABEL_MIN = TASK_ICON_WIDTH + TASK_TITLE_GAP + 56

export interface FitInput {
  // 창 버튼 목록이 쓸 수 있는 너비. "더 보기" 버튼 자리는 빼지 않은 값이다.
  available: number
  // 창 버튼 사이 간격
  gap: number
  // "더 보기" 버튼이 차지하는 너비(앞 간격 포함)
  moreSpace: number
  // 버튼마다 줄이지 않았을 때의 너비(windows[] 순서, 퇴장 중인 버튼 제외). TASK_MAX_WIDTH보다 클 수 있다.
  natural: readonly number[]
}

export type TaskbarFit =
  // 제목을 보인다. cap은 버튼 최대 너비, truncated는 제목이 잘리는 버튼이다(툴팁을 붙인다).
  | { mode: 'labels'; cap: number; truncated: boolean[] }
  // 아이콘만 보인다. 앞에서 visible개만 보이고 나머지는 "더 보기" 메뉴로 접는다.
  | { mode: 'icons'; visible: number }

// fitTasks는 버튼을 어떻게 그릴지 정한다.
export function fitTasks({ available, gap, moreSpace, natural }: FitInput): TaskbarFit {
  const n = natural.length
  const cap = labelCap(natural, available - gap * Math.max(0, n - 1))
  if (cap >= TASK_LABEL_MIN) return { mode: 'labels', cap, truncated: natural.map((w) => w > cap) }

  const step = TASK_ICON_WIDTH + gap
  if (n * step - gap <= available) return { mode: 'icons', visible: n }
  const visible = Math.floor((available - moreSpace + gap) / step)
  return { mode: 'icons', visible: Math.min(n, Math.max(0, visible)) }
}

// sameFit은 두 결과가 같은지 비교한다. 다시 잰 결과가 같으면 다시 그리지 않는다.
export function sameFit(a: TaskbarFit, b: TaskbarFit): boolean {
  if (a.mode === 'icons') return b.mode === 'icons' && a.visible === b.visible
  return (
    b.mode === 'labels' &&
    a.cap === b.cap &&
    a.truncated.length === b.truncated.length &&
    a.truncated.every((t, i) => t === b.truncated[i])
  )
}

// labelCap은 Σ min(natural_i, cap) ≤ budget인 가장 큰 cap(TASK_MAX_WIDTH 이하, 정수)이다.
// 짧은 버튼은 그대로 두고 긴 버튼만 같은 너비로 줄인다. 들어갈 수 없으면 음수일 수 있다.
function labelCap(natural: readonly number[], budget: number): number {
  const widths = natural.map((w) => Math.min(w, TASK_MAX_WIDTH)).sort((a, b) => a - b)
  let rest = budget
  for (let i = 0; i < widths.length; i++) {
    // 남은 버튼(i번째부터)이 모두 cap으로 줄었을 때의 cap
    const cap = rest / (widths.length - i)
    if (cap < widths[i]) return Math.floor(cap)
    rest -= widths[i]
  }
  return TASK_MAX_WIDTH
}
