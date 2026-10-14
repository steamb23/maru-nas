// requestAnimationFrame 콜백 중 fps에 맞는 프레임만 고르는 계산(DOM 없음).
//
// - 간격 경계를 마지막으로 그린 시각이 아니라 일정한 간격으로 이어 가 지연이 쌓이지 않는다.
// - 60Hz 화면에서 30fps면 rAF 두 번(약 33.2ms)이 간격(33.3ms)보다 조금 짧을 수 있다. 그대로 거르면 세 번째 rAF까지
//   기다려 20fps가 되므로 SLACK_MS만큼 일찍 와도 그린다.
// - 탭 숨김 등으로 오래 멈췄다 돌아오면 연출 시간이 한꺼번에 뛰지 않도록 한 프레임의 경과 시간을 maxStepMs로 자른다.

const SLACK_MS = 1

export interface FrameGate {
  // tick은 이번 rAF에서 그려야 하면 지난번 그린 뒤 흐른 시간(ms, 첫 프레임은 0)을, 건너뛰어야 하면 null을 돌려준다.
  tick: (now: number) => number | null
}

export function createFrameGate(fps: number, maxStepMs = 100): FrameGate {
  const interval = 1000 / fps
  // 다음 프레임 간격이 시작된 시각
  let slot: number | undefined
  // 마지막으로 그린 시각
  let drawn = 0

  return {
    tick(now) {
      if (slot === undefined) {
        slot = drawn = now
        return 0
      }
      if (now - slot < interval - SLACK_MS) return null
      slot += interval
      // 한 간격 넘게 밀렸으면 지금을 새 기준으로 삼는다.
      if (now - slot > interval) slot = now
      const step = Math.min(now - drawn, maxStepMs)
      drawn = now
      return step
    },
  }
}
