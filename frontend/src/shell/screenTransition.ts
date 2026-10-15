// 로그인 화면과 데스크톱 사이 전환 애니메이션 재생. 설정값과 방향은 screenAnim.ts에 있다.
// 나가는 애니메이션은 auth 저장소가 화면을 바꾸기 전에 leaveScreen으로 재생하고,
// 들어오는 애니메이션은 각 화면이 처음 그려질 때(Desktop, LoginScreen) 재생한다.
import type { ScreenChange } from '../auth/store'
import {
  DESKTOP_FAR,
  LOGIN_NEAR,
  SCREEN_ENTER_MS,
  SCREEN_LEAVE_GRACE_MS,
  SCREEN_LEAVE_MS,
  SHOWN,
  type ScreenPose,
  screenFrames,
  viewportOrigin,
} from './screenAnim'
import { animationsEnabled } from './useWindowAnimation'
import { CLOSE_EASING, EASING } from './windowAnim'

// 데스크톱에서 움직이는 부분. 배경(.desktop)은 그대로 둔다.
// 작업표시줄과 "모든 앱" 화면의 backdrop-filter가 제대로 보이도록 한 부모로 묶지 않고 따로 움직인다.
const DESKTOP_PARTS = '.desktop-area, .app-drawer, .taskbar'

function originOf(el: Element): string {
  const r = el.getBoundingClientRect()
  return viewportOrigin(
    { x: r.left, y: r.top, width: r.width, height: r.height },
    { width: window.innerWidth, height: window.innerHeight },
  )
}

// play는 els를 같은 방향으로 움직인다. 이미 재생 중인 애니메이션은 지운다(개발 모드에서 두 번 불릴 때 등).
function play(els: Element[], from: ScreenPose, to: ScreenPose, options: KeyframeAnimationOptions): Animation[] {
  return els.map((el) => {
    el.getAnimations().forEach((a) => a.cancel())
    return el.animate(screenFrames(from, to, originOf(el)), options)
  })
}

// leaveScreen은 지금 화면의 내용을 나가는 방향으로 움직이고, 끝나면 resolve한다.
// 끝난 뒤 화면이 바뀔 때까지 마지막 상태를 유지하고 클릭·포커스를 받지 않는다.
// 애니메이션이 꺼져 있거나 움직일 요소가 없으면 바로 resolve한다.
export function leaveScreen(change: ScreenChange): Promise<void> {
  if (!animationsEnabled()) return Promise.resolve()
  const root =
    change === 'signIn' ? document.querySelector('.login-card') : document.querySelector('.desktop-area')?.parentElement
  if (!root) return Promise.resolve()
  root.setAttribute('inert', '')
  const options: KeyframeAnimationOptions = { duration: SCREEN_LEAVE_MS, easing: CLOSE_EASING, fill: 'forwards' }
  const anims =
    change === 'signIn'
      ? play([root], SHOWN, LOGIN_NEAR, options)
      : play(Array.from(root.querySelectorAll(DESKTOP_PARTS)), SHOWN, DESKTOP_FAR, options)
  return Promise.race([
    Promise.allSettled(anims.map((a) => a.finished)).then(() => {}),
    new Promise<void>((resolve) => window.setTimeout(resolve, SCREEN_LEAVE_MS + SCREEN_LEAVE_GRACE_MS)),
  ])
}

// enterDesktop은 데스크톱 내용을 조금 작은 상태에서 커지며 나타나게 한다.
export function enterDesktop(root: HTMLElement | null) {
  if (!root || !animationsEnabled()) return
  play(Array.from(root.querySelectorAll(DESKTOP_PARTS)), DESKTOP_FAR, SHOWN, {
    duration: SCREEN_ENTER_MS,
    easing: EASING,
  })
}

// enterLogin은 로그인 카드를 큰 상태에서 줄어들며 나타나게 한다.
export function enterLogin(card: HTMLElement | null) {
  if (!card || !animationsEnabled()) return
  play([card], LOGIN_NEAR, SHOWN, { duration: SCREEN_ENTER_MS, easing: EASING })
}
