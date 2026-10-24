// 시스템 업데이트 창을 작업 보기로 열어 달라는 요청. 알림(출력 보기, 백그라운드 작업의 열기)이 창을 열기 전에
// 남기고 창이 읽는다. 창 상태에 앱별 내용을 두지 않으므로 모듈 저장소에 둔다(미리보기 창의 viewerStore처럼).
import { create } from 'zustand'

interface PackagesNav {
  // jobId는 작업 보기에서 보일 작업이다. 진행 중인 작업이 있으면 그것이 먼저다.
  jobId: string | null
  // seq는 요청마다 늘어난다. 창이 이미 열려 있어도 다시 작업 보기로 바꾼다.
  seq: number
  showJob: (jobId: string | null) => void
}

export const usePackagesNav = create<PackagesNav>()((set) => ({
  jobId: null,
  seq: 0,
  showJob: (jobId) => set((s) => ({ jobId, seq: s.seq + 1 })),
}))
