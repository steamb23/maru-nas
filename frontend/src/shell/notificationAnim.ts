// 알림 센터와 토스트의 애니메이션 설정값(DOM 없음). 창 애니메이션과 같은 켜기/끄기·동작 줄이기 설정
// (animationsEnabled)을 따른다.
// 알림 센터는 화면 오른쪽 가장자리 쪽에서 미끄러져 들어오고, 닫을 때는 더 짧게 같은 쪽으로 빠진다.
export const CENTER_OPEN_MS = 200
export const CENTER_CLOSE_MS = 140
export const CENTER_OPEN_FROM: Keyframe = { opacity: 0, transform: 'translateX(24px)' }
export const CENTER_CLOSE_TO: Keyframe = { opacity: 0, transform: 'translateX(12px)' }
export const CENTER_SHOWN: Keyframe = { opacity: 1, transform: 'none' }
// 토스트는 작업표시줄 쪽(아래)에서 조금 떠오르며 나타난다.
export const TOAST_ENTER_MS = 180
export const TOAST_ENTER_FROM: Keyframe = { opacity: 0, transform: 'translateY(8px) scale(0.98)' }
export const TOAST_SHOWN: Keyframe = { opacity: 1, transform: 'none' }
