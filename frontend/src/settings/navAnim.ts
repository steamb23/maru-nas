// 좁은 설정 창에서 사이드바를 내용 위에 덮어 여닫는 애니메이션 설정값(DOM 없음).
// 왼쪽에서 조금 미끄러지며 나타나고, 닫을 때는 더 짧게 사라진다. 뒤 배경(어둡게)은 투명도만 바꾼다.
export const NAV_OPEN_MS = 180
export const NAV_CLOSE_MS = 130
export const NAV_HIDDEN: Keyframe = { opacity: 0, transform: 'translateX(-24px)' }
export const NAV_SHOWN: Keyframe = { opacity: 1, transform: 'none' }
export const SCRIM_HIDDEN: Keyframe = { opacity: 0 }
export const SCRIM_SHOWN: Keyframe = { opacity: 1 }
