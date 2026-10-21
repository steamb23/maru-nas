// 미리보기 창이 보이는 파일. 창 하나를 다시 쓰므로 파일 탐색기가 고른 파일을 여기에 두고 창을 연다(이미 열려 있으면
// 앞으로 가져온다). 창 상태(windows/store.ts)에는 앱별 내용을 두지 않으므로 파일 작업(transferStore.ts)처럼 모듈
// 저장소에 둔다. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { create } from 'zustand'
import type { PreviewKind } from '../files/kinds'
import type { Size } from '../windows/geometry'
import { useWindowStore } from '../windows/store'

export interface ViewerTarget {
  path: string
  name: string
  kind: PreviewKind
  // seq는 열 때마다 바뀐다(같은 파일을 다시 열면 내용을 다시 받는다).
  seq: number
}

interface ViewerState {
  target: ViewerTarget | null
}

export const useViewerStore = create<ViewerState>()(() => ({ target: null }))

let seq = 0

// openViewer는 미리보기 창에 파일을 보인다. bounds는 창을 열 데스크톱 영역 크기다.
export function openViewer(file: { path: string; name: string; kind: PreviewKind }, bounds: Size) {
  useViewerStore.setState({ target: { ...file, seq: ++seq } })
  useWindowStore.getState().open('viewer', bounds)
}
