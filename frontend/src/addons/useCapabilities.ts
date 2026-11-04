import { useEffect } from 'react'
import { clearPackages } from '../addonpkg/catalog'
import { registerPackageNames } from '../addonpkg/names'
import type { CapabilitiesResponse } from '../api/types.gen'
import { fetchCapabilities } from '../api/client'
import { useCapabilitiesStore } from './capabilities'
import { createPoller } from './common/poller'

// 플러그인 설치·제거를 반영하는 주기. 백엔드의 감지 결과 보관 시간(5분)과 같다.
export const CAPABILITIES_POLL_INTERVAL_MS = 5 * 60_000
// 첫 요청이 실패하면 받을 때까지 이 간격으로 다시 묻는다(패키지 앱·위젯이 5분 동안 비지 않게).
const FIRST_RETRY_MS = 5_000
const CAPABILITIES_TIMEOUT_MS = 10_000

function apply(caps: CapabilitiesResponse) {
  // 패키지 이름을 먼저 넣어 둔다. 저장소가 바뀌면 목록을 그리는 컴포넌트가 다시 그린다.
  registerPackageNames(caps.addonPackages)
  useCapabilitiesStore.getState().set(caps)
}

// refreshCapabilities는 바로 다시 받는다(애드온 패키지를 설치·켜기·끄기·제거한 뒤). 실패하면 마지막 값을 둔다.
export async function refreshCapabilities(): Promise<void> {
  try {
    apply(await fetchCapabilities())
  } catch {
    // 다음 주기에 다시 받는다.
  }
}

// useCapabilities는 로그인한 동안 capability를 가져와 저장소에 둔다. 언마운트(로그아웃·만료)되면
// 저장소를 비워 다음 사용자에게 이전 사용자의 값이 보이지 않게 한다.
// 실패하면 마지막 값을 유지한다(401은 API 클라이언트가 로그인 화면으로 보낸다).
export function useCapabilities() {
  useEffect(() => {
    const store = useCapabilitiesStore.getState()
    let received = false
    const poller = createPoller({
      fetch: fetchCapabilities,
      intervalMs: () => (received ? CAPABILITIES_POLL_INTERVAL_MS : FIRST_RETRY_MS),
      timeoutMs: CAPABILITIES_TIMEOUT_MS,
      onData: (caps: CapabilitiesResponse) => {
        received = true
        apply(caps)
      },
      onError: () => store.settle(),
    })
    poller.start()
    return () => {
      poller.stop()
      store.clear()
      clearPackages()
    }
  }, [])
}
