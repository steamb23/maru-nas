// 애드온 manifest의 프론트 타입. 원본은 백엔드 Go 구조체(api.Manifest)이고,
// 목록(manifests.gen.ts)은 `make types`로 생성한다.
import type { ParseKeys } from 'i18next'
import type { GridSize, Manifest } from '../api/types.gen'
import type { IconName } from '../icons/resolver'

// 생성된 목록은 `as const`라 배열이 readonly 튜플이 된다. 그대로 검사할 수 있게 깊게 readonly로 만든다.
type DeepReadonly<T> = T extends (infer U)[]
  ? readonly DeepReadonly<U>[]
  : T extends object
    ? { readonly [K in keyof T]: DeepReadonly<T[K]> }
    : T

// AddonManifest는 title을 번역 키로, icon을 리졸버에 있는 아이콘 이름으로 좁힌다.
// 없는 키·아이콘을 manifest에 쓰면 manifests.gen.ts에서 tsc 오류가 난다.
export type AddonManifest = DeepReadonly<Omit<Manifest, 'title' | 'icon'> & { title: ParseKeys; icon: IconName }>

export interface WindowViewProps {
  // 창이 최소화되거나 닫히는 중이거나 탭이 보이지 않으면 false. 이때는 주기 요청을 멈춘다.
  active: boolean
}

export interface WidgetViewProps {
  // 탭이 보이지 않으면 false. 이때는 주기 요청을 멈춘다.
  active: boolean
  // 위젯의 지금 크기(칸). 크기에 따라 보여 줄 내용을 고를 때 쓴다.
  size: GridSize
}
