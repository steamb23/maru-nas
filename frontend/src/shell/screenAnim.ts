// 로그인 화면과 데스크톱 사이 전환 애니메이션의 설정값과 계산(DOM 없음). 재생은 screenTransition.ts가 한다.
//
// - 로그인: 앞으로 들어가는 느낌. 로그인 카드가 커지며 흐려져 사라진 뒤, 데스크톱이 조금 작고 흐린 상태에서 커지며 또렷해진다.
// - 로그아웃·세션 만료: 뒤로 물러나는 느낌. 데스크톱이 줄어들며 흐려져 사라진 뒤, 로그인 카드가 크고 흐린 상태에서 줄어들며 또렷해진다.
// 두 화면은 함께 그려지지 않으므로 나가는 애니메이션이 끝난 뒤(auth 저장소의 beforeScreenChange) 들어오는 애니메이션이 이어진다.
// 배경(.desktop)은 두 화면이 같아 그대로 두고 내용만 움직인다.
// 창 애니메이션과 같은 화면 설정의 애니메이션·동작 줄이기 설정(animationsEnabled)을 따른다.
import type { Rect, Size } from '../windows/geometry'

export const SCREEN_LEAVE_MS = 180
export const SCREEN_ENTER_MS = 240
// 나가는 애니메이션이 어떤 이유로 끝나지 않아도(백그라운드 탭 등) 이 시간이 더 지나면 화면을 바꾼다.
export const SCREEN_LEAVE_GRACE_MS = 200

// ScreenPose는 화면 내용의 한 순간 모습이다. scale은 크기 비율, blur는 흐림 반경(px)이다.
export interface ScreenPose {
  scale: number
  opacity: number
  blur: number
}

// 제자리에 또렷하게 보이는 모습
export const SHOWN: ScreenPose = { scale: 1, opacity: 1, blur: 0 }
// 데스크톱 내용(창 영역, 작업표시줄)이 멀리 있어 보이지 않는 모습
export const DESKTOP_FAR: ScreenPose = { scale: 0.92, opacity: 0, blur: 12 }
// 로그인 카드가 가까이 지나가 보이지 않는 모습
export const LOGIN_NEAR: ScreenPose = { scale: 1.14, opacity: 0, blur: 12 }

// screenFrames는 from에서 to로 바뀌는 키프레임이다. origin은 transform-origin 값이다.
// 1배·흐림 0은 transform·filter를 두지 않는다(애니메이션이 끝난 뒤와 같은 값이 되게).
export function screenFrames(from: ScreenPose, to: ScreenPose, origin: string): Keyframe[] {
  const frame = ({ scale, opacity, blur }: ScreenPose): Keyframe => ({
    transform: scale === 1 ? 'none' : `scale(${scale})`,
    opacity,
    filter: blur === 0 ? 'none' : `blur(${blur}px)`,
    transformOrigin: origin,
  })
  return [frame(from), frame(to)]
}

// viewportOrigin은 rect 위치에 놓인 요소가 화면 가운데를 기준으로 커지고 줄어들게 하는 transform-origin 값이다.
// 창 영역과 작업표시줄을 따로 움직여도 한 장면처럼 같은 점을 향해 움직이게 한다.
export function viewportOrigin(rect: Rect, viewport: Size): string {
  return `${viewport.width / 2 - rect.x}px ${viewport.height / 2 - rect.y}px`
}
