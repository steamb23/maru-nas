// "모든 앱" 화면의 애니메이션 설정값(DOM 없음). transform·opacity만 움직인다.
// 흐린 배경은 투명도만 바꾸고, 내용(검색 상자·격자)은 조금 작은 상태에서 커지며 나타나 살짝 줄며 사라진다.
// 창 애니메이션과 같은 켜기/끄기·동작 줄이기 설정(animationsEnabled)을 따른다.
export const DRAWER_OPEN_MS = 200
export const DRAWER_CLOSE_MS = 140
export const DRAWER_HIDDEN: Keyframe = { opacity: 0 }
export const DRAWER_CONTENT_FROM: Keyframe = { opacity: 0, transform: 'scale(0.96)' }
export const DRAWER_CONTENT_CLOSE_TO: Keyframe = { opacity: 0, transform: 'scale(0.98)' }
export const DRAWER_SHOWN: Keyframe = { opacity: 1, transform: 'none' }
