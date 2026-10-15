// 설정 저장 큐. 위젯 배치·바로가기·"모든 앱" 설정 저장소가 함께 쓴다. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 저장은 한 번에 하나만 보낸다. 보내는 동안 바뀐 값은 마지막 것만 기다렸다가 이어서 보낸다.
// 실패하면 onResult(false)로 알리고, 값은 다음 push 때 다시 보낸다(부르는 쪽이 로컬 값을 그대로 둔다).

export interface SaveQueue<T> {
  // value를 저장한다. 보내는 중이면 기다렸다 보내고, 그사이 들어온 값이 있으면 마지막 것만 보낸다.
  push: (value: T) => void
  // 기다리던 값을 버리고 보내는 중인 요청의 결과도 무시한다(로그아웃, 다시 불러오기).
  reset: () => void
}

export function createSaveQueue<T>(
  save: (value: T) => Promise<void>,
  onResult: (ok: boolean) => void,
): SaveQueue<T> {
  // reset마다 바뀐다. 그 전에 보낸 요청의 결과를 버리는 데 쓴다.
  let generation = 0
  let saving = false
  // 값 자체가 거짓값일 수 있어 감싼다.
  let pending: { value: T } | null = null

  const flush = async () => {
    if (saving || !pending) return
    saving = true
    const gen = generation
    const { value } = pending
    pending = null
    try {
      await save(value)
      if (gen === generation) onResult(true)
    } catch {
      // 401은 API 클라이언트가 로그인 화면으로 보낸다.
      if (gen === generation) onResult(false)
    } finally {
      saving = false
    }
    void flush()
  }

  return {
    push: (value) => {
      pending = { value }
      void flush()
    },
    reset: () => {
      generation++
      pending = null
    },
  }
}
