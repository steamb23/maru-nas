// requestAnimationFrame 콜백 중 fps에 맞는 프레임만 고르는 계산(DOM 없음).
//
// - 간격 경계를 마지막으로 그린 시각이 아니라 일정한 간격으로 이어 가 지연이 쌓이지 않는다.
// - 60Hz 화면에서 30fps면 rAF 두 번(약 33.2ms)이 간격(33.3ms)보다 조금 짧을 수 있다. 그대로 거르면 세 번째 rAF까지
//   기다려 20fps가 되므로 SLACK_MS만큼 일찍 와도 그린다.

const SLACK_MS = 1

export interface FrameGate {
  // tick은 이번 rAF에서 그려야 하면 true다.
  tick: (now: number) => boolean
}

export function createFrameGate(fps: number): FrameGate {
  const interval = 1000 / fps
  // 다음 프레임 간격이 시작된 시각
  let slot: number | undefined

  return {
    tick(now) {
      if (slot === undefined) {
        slot = now
        return true
      }
      if (now - slot < interval - SLACK_MS) return false
      slot += interval
      // 한 간격 넘게 밀렸으면(탭 숨김 등) 지금을 새 기준으로 삼는다.
      if (now - slot > interval) slot = now
      return true
    },
  }
}
