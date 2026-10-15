// 관리자 페이지(로그인 화면, 기본 화면)가 전역 설정을 저장할 때 쓰는 훅.
// 고른 값을 바로 보여 주고(공개 전역 정보 저장소를 먼저 바꿈), 저장한 뒤 서버 값을 다시 불러온다.
// 실패하면 다시 불러와 원래 값으로 되돌리고 오류를 알린다.
import { useState } from 'react'
import type { SiteInfo } from '../api/types.gen'
import { useSiteStore } from '../appearance/siteStore'

export function useSiteSave() {
  const [saving, setSaving] = useState(false)
  const [failed, setFailed] = useState(false)

  const save = async (preview: (info: SiteInfo) => SiteInfo, send: () => Promise<unknown>) => {
    const { info, set, load } = useSiteStore.getState()
    if (info) set(preview(info))
    setSaving(true)
    setFailed(false)
    try {
      await send()
    } catch {
      setFailed(true)
    } finally {
      setSaving(false)
      await load()
    }
  }

  // reload는 이미지 올리기·지우기처럼 결과를 서버에서 다시 받아야 하는 변경 뒤에 부른다.
  const reload = () => useSiteStore.getState().load()

  return { saving, failed, save, reload }
}
