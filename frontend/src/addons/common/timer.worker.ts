// workerTimer.ts가 쓰는 Worker. 메인 스레드 대신 여기서 setTimeout을 건다.
// 브라우저는 숨긴 탭의 메인 스레드 타이머를 늦추지만(크롬은 5분 뒤 1분에 한 번) Worker의 타이머는 늦추지 않는다.
// 메시지: {id, ms}는 ms 뒤에 id를 돌려보낸다. {id}(ms 없음)는 그 예약을 거둔다.
const timers = new Map<number, ReturnType<typeof setTimeout>>()

// DOM 타입의 self.postMessage는 창의 것(targetOrigin 필요)이라 Worker 쪽 모양으로 좁힌다.
const scope = self as unknown as { postMessage: (message: number) => void }

self.onmessage = (e: MessageEvent<{ id: number; ms?: number }>) => {
  const { id, ms } = e.data
  clearTimeout(timers.get(id))
  timers.delete(id)
  if (ms === undefined) return
  timers.set(
    id,
    setTimeout(() => {
      timers.delete(id)
      scope.postMessage(id)
    }, ms),
  )
}
