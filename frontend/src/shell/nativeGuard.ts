// 브라우저 기본 동작 제한. 데스크톱 앱처럼 보이도록 텍스트·이미지 드래그와 브라우저 컨텍스트 메뉴를
// 막고, 선택 허용 영역(.selectable 하위)과 텍스트 입력 요소에서만 허용한다.
// 텍스트 선택 자체는 base.css의 user-select로 같은 범위에 맞춰 제한한다.

// 선택 허용 영역을 표시하는 클래스. 텍스트 선택이 꼭 필요한 영역에만 붙인다.
export const SELECTABLE_CLASS = 'selectable'

// 텍스트를 입력받지 않는 input 종류
const NON_TEXT_INPUT_TYPES = new Set([
  'button',
  'checkbox',
  'color',
  'file',
  'hidden',
  'image',
  'radio',
  'range',
  'reset',
  'submit',
])

// isTextInput은 요소가 텍스트를 입력받는 textarea·input인지 판단한다.
// tagName은 Element.tagName(HTML 문서에서는 대문자), type은 type 속성 값이다.
export function isTextInput(tagName: string, type: string | null): boolean {
  const tag = tagName.toLowerCase()
  if (tag === 'textarea') return true
  if (tag !== 'input') return false
  return type === null || !NON_TEXT_INPUT_TYPES.has(type.trim().toLowerCase())
}

// allowsNativeText는 이벤트 대상에서 브라우저 기본 텍스트 동작(드래그, 컨텍스트 메뉴)을 허용할지 판단한다.
export function allowsNativeText(target: EventTarget | null): boolean {
  // 선택한 텍스트를 끌 때 dragstart의 대상은 Text 노드일 수 있다.
  const el = target instanceof Element ? target : target instanceof Node ? target.parentElement : null
  if (!el) return false
  if (el.closest(`.${SELECTABLE_CLASS}`)) return true
  if (el instanceof HTMLElement && el.isContentEditable) return true
  return isTextInput(el.tagName, el.getAttribute('type'))
}

// installNativeGuard는 문서 전체에 제한을 건다. 해제 함수를 반환한다.
export function installNativeGuard(doc: Document): () => void {
  const guard = (e: Event) => {
    if (!allowsNativeText(e.target)) e.preventDefault()
  }
  doc.addEventListener('dragstart', guard, true)
  doc.addEventListener('contextmenu', guard, true)
  return () => {
    doc.removeEventListener('dragstart', guard, true)
    doc.removeEventListener('contextmenu', guard, true)
  }
}
