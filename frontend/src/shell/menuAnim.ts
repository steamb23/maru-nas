// 메뉴(프로필 메뉴, 컨텍스트 메뉴)의 열기·닫기 애니메이션 설정값(DOM 없음).
// 메뉴는 기준점(transform-origin) 쪽에서 조금 아래·작은 상태로 떠오르며 나타나고, 닫을 때는 더 짧게 살짝
// 가라앉으며 사라진다. 창 애니메이션과 같은 켜기/끄기·동작 줄이기 설정(animationsEnabled)을 따른다.
export const MENU_OPEN_MS = 160
export const MENU_CLOSE_MS = 110
export const MENU_OPEN_FROM: Keyframe = { opacity: 0, transform: 'translateY(8px) scale(0.96)' }
export const MENU_CLOSE_TO: Keyframe = { opacity: 0, transform: 'translateY(4px) scale(0.98)' }
export const MENU_SHOWN: Keyframe = { opacity: 1, transform: 'none' }
