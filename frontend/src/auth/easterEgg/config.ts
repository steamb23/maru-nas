// 로그인 대기 이스터에그 설정값. 로그인 화면에서 입력 없이 기다리면 화면을 어둡게 덮고 캔버스 연출을 보여 준다.
// OMV workbench 로그인 화면의 매트릭스 효과(30초 대기, 동작 줄이기면 끔)에 대응한다.

// 이스터에그 사용 여부. 시스템의 '동작 줄이기'(prefers-reduced-motion)가 켜져 있으면 이 값과 관계없이 끈다.
// TODO: 설정 기능이 생기면 이 값을 사용자 설정에서 읽어 끄고 켤 수 있게 한다.
export const LOGIN_EASTER_EGG = true

// 입력 없이 이만큼 지나면 연출을 시작한다. OMV workbench 로그인 화면과 같은 30초다.
export const IDLE_DELAY_MS = 30_000

// 연출의 최대 프레임레이트. 화면 주사율과 관계없이 이 값을 넘겨 그리지 않는다. 15로 줄여도 된다.
export const EGG_FPS = 30

// 배경이 어두워지며 나타나는 시간과, 입력으로 해제할 때 사라지는 시간
export const EGG_FADE_IN_MS = 1200
export const EGG_FADE_OUT_MS = 200

// easterEggEnabled는 LOGIN_EASTER_EGG가 켜져 있고 '동작 줄이기'가 꺼져 있을 때 true다.
export function easterEggEnabled(): boolean {
  return LOGIN_EASTER_EGG && !window.matchMedia('(prefers-reduced-motion: reduce)').matches
}
