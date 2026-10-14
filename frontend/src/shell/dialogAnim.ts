// 모달 열기·닫기 애니메이션 설정값(DOM 없음). 상자는 조금 작은 크기에서 커지며 나타나고, 닫을 때는 더 짧게
// 살짝 줄어들며 사라진다. 배경은 투명도만 바뀐다. 창 애니메이션과 같은 켜기/끄기 설정(animationsEnabled)을 따른다.
export const DIALOG_OPEN_MS = 180
export const DIALOG_CLOSE_MS = 120
export const DIALOG_OPEN_FROM: Keyframe = { opacity: 0, transform: 'scale(0.96)' }
export const DIALOG_CLOSE_TO: Keyframe = { opacity: 0, transform: 'scale(0.98)' }
export const DIALOG_SHOWN: Keyframe = { opacity: 1, transform: 'none' }
export const BACKDROP_HIDDEN: Keyframe = { opacity: 0 }
export const BACKDROP_SHOWN: Keyframe = { opacity: 1 }
