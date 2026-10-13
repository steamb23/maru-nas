// 작업표시줄 버튼 드래그 정렬 계산.
// 드래그를 시작할 때 한 번 잰 버튼 위치(Slot)만 입력으로 받아 DOM에 의존하지 않는다.
// 드래그 중에는 버튼을 옮기지 않고 transform으로만 움직이므로, 시작 시점의 위치가 끝까지 기준이 된다.

// Slot은 버튼 하나의 가로 위치와 너비다. 좌표계는 호출하는 쪽이 정하며 서로 같기만 하면 된다.
export interface Slot {
  left: number
  width: number
}

export interface DragLayout {
  // 범위 안으로 제한한 드래그 버튼의 이동량
  dx: number
  // 드래그 버튼이 놓일 자리
  to: number
  // 각 버튼의 이동량. 드래그 버튼 자리는 0이다.
  shifts: number[]
  // 드래그를 놓았을 때 드래그 버튼이 도착할 위치의 이동량
  settle: number
}

// dragLayout은 slots[from] 버튼을 dx만큼 끌었을 때의 배치를 계산한다. gap은 버튼 사이 간격이다.
export function dragLayout(slots: readonly Slot[], gap: number, from: number, dx: number): DragLayout {
  const src = slots[from]
  const first = slots[0]
  const last = slots[slots.length - 1]
  // 드래그 버튼은 버튼 목록이 차지하는 범위 밖으로 나가지 않는다.
  const minDx = first.left - src.left
  const maxDx = last.left + last.width - (src.left + src.width)
  const clamped = Math.min(Math.max(dx, minDx), maxDx)

  // 드래그 버튼의 앞쪽 가장자리(오른쪽으로 끌면 오른쪽 끝, 왼쪽으로 끌면 왼쪽 끝)가
  // 다른 버튼의 원래 중심을 넘으면 그 버튼과 자리를 바꾼다. 중심끼리 비교하면 너비가 넓은
  // 버튼은 양 끝 자리에 닿지 못한다. 시작 시점의 위치와 비교하므로 비켜난 버튼 때문에 판정이 흔들리지 않는다.
  const left = src.left + clamped
  const right = left + src.width
  let to = from
  slots.forEach((s, i) => {
    const mid = s.left + s.width / 2
    if (i > from && right > mid) to++
    if (i < from && left < mid) to--
  })

  // 사이에 낀 버튼은 드래그 버튼 너비와 간격만큼 비켜난다.
  const step = src.width + gap
  const shifts = slots.map((_, i) => {
    if (from < i && i <= to) return -step
    if (to <= i && i < from) return step
    return 0
  })

  let settle = 0
  if (to > from) settle = slots[to].left + slots[to].width - src.width - src.left
  else if (to < from) settle = slots[to].left - src.left

  return { dx: clamped, to, shifts, settle }
}
