// 파일 목록의 고르기. 보기(정렬·찾기) 안의 자리로 두므로 보기나 목록이 바뀌면 지운다. 큰 폴더에서는 Shift·Ctrl+A로
// 아직 받지 않은 줄까지 고를 수 있어 항목이 아니라 자리의 구간으로 둔다. DOM에 의존하지 않아 node 환경에서 테스트한다.

// Span은 고른 자리의 구간 [start, end)다.
export interface Span {
  start: number
  end: number
}

export interface Selection {
  // spans는 겹치거나 이어지지 않게 합친 구간을 앞에서부터 둔다.
  spans: readonly Span[]
  // anchor는 Shift로 넓힐 때의 기준이다.
  anchor: number | null
  // focus는 마지막으로 누른 자리이자 키보드 이동의 기준이다(고르지 않은 자리일 수 있다: Ctrl로 뺀 줄).
  focus: number | null
}

export const NO_SELECTION: Selection = { spans: [], anchor: null, focus: null }

// selectOne은 i 하나만 고른다(그냥 누르기, 화살표).
export function selectOne(i: number): Selection {
  return { spans: [{ start: i, end: i + 1 }], anchor: i, focus: i }
}

// toggleAt은 i를 고르거나 뺀다(Ctrl·⌘ 누르기). 기준도 i로 옮긴다.
export function toggleAt(sel: Selection, i: number): Selection {
  const spans = isSelected(sel, i) ? without(sel.spans, i) : merged([...sel.spans, { start: i, end: i + 1 }])
  return { spans, anchor: i, focus: i }
}

// extendTo는 기준에서 i까지 고른다(Shift). 기준이 없으면 i 하나다. additive면 이미 고른 것을 남긴다(Ctrl+Shift).
export function extendTo(sel: Selection, i: number, additive = false): Selection {
  const a = sel.anchor ?? i
  const span = { start: Math.min(a, i), end: Math.max(a, i) + 1 }
  return { spans: additive ? merged([...sel.spans, span]) : [span], anchor: a, focus: i }
}

// selectAll은 보기의 모든 자리를 고른다(Ctrl+A). 기준과 위치는 그대로 둔다.
export function selectAll(sel: Selection, total: number): Selection {
  if (total <= 0) return NO_SELECTION
  return { spans: [{ start: 0, end: total }], anchor: sel.anchor ?? 0, focus: sel.focus ?? 0 }
}

export function isSelected(sel: Selection, i: number): boolean {
  return sel.spans.some((s) => s.start <= i && i < s.end)
}

export function selectedCount(sel: Selection): number {
  return sel.spans.reduce((n, s) => n + s.end - s.start, 0)
}

// soleIndex는 하나만 골랐을 때 그 자리다. 아니면 null이다.
export function soleIndex(sel: Selection): number | null {
  const [s] = sel.spans
  return sel.spans.length === 1 && s.end - s.start === 1 ? s.start : null
}

// selectedIndices는 고른 자리를 앞에서부터 모두 펼친다.
export function selectedIndices(sel: Selection): number[] {
  const out: number[] = []
  for (const s of sel.spans) for (let i = s.start; i < s.end; i++) out.push(i)
  return out
}

function merged(spans: readonly Span[]): Span[] {
  const sorted = [...spans].sort((a, b) => a.start - b.start)
  const out: Span[] = []
  for (const s of sorted) {
    const last = out[out.length - 1]
    if (last && s.start <= last.end) last.end = Math.max(last.end, s.end)
    else out.push({ ...s })
  }
  return out
}

function without(spans: readonly Span[], i: number): Span[] {
  const out: Span[] = []
  for (const s of spans) {
    if (i < s.start || i >= s.end) {
      out.push(s)
      continue
    }
    if (s.start < i) out.push({ start: s.start, end: i })
    if (i + 1 < s.end) out.push({ start: i + 1, end: s.end })
  }
  return out
}
