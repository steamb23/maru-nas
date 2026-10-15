// 프로필 사진 저장소를 로그인한 동안 불러온다. SignedIn에서 부른다.
import { useEffect } from 'react'
import { useAvatarStore } from './store'

// 불러오지 못했을 때 다시 시도하는 간격
const RETRY_MS = 10_000

export function useAvatarLoader() {
  const failed = useAvatarStore((s) => s.status === 'error')
  useEffect(() => {
    const { load, clear } = useAvatarStore.getState()
    void load()
    return clear
  }, [])
  useEffect(() => {
    if (!failed) return
    const id = window.setTimeout(() => void useAvatarStore.getState().load(), RETRY_MS)
    return () => window.clearTimeout(id)
  }, [failed])
}
