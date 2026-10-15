// "모든 앱" 화면의 앱 정렬. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 추천순(recommended)은 대시보드가 정한 순서(windows/apps.ts의 APP_IDS: 비슷한 기능의 앱끼리 모인다),
// 이름순(name)은 지금 화면 언어의 이름 순서다. 언어를 바꾸면 이름순도 그 언어에 맞게 바뀐다.
import type { ParseKeys } from 'i18next'
import { DrawerSortName, DrawerSortRecommended } from '../api/types.gen'

export type AppSort = typeof DrawerSortRecommended | typeof DrawerSortName

// 정렬 메뉴 순서
export const APP_SORTS: readonly AppSort[] = [DrawerSortRecommended, DrawerSortName]

export const DEFAULT_APP_SORT: AppSort = DrawerSortRecommended

export const APP_SORT_LABELS: Readonly<Record<AppSort, ParseKeys>> = {
  recommended: 'drawer.sortRecommended',
  name: 'drawer.sortName',
}

// toAppSort는 저장된 값을 정렬 방식으로 바꾼다. 없거나 모르는 값(다른 버전이 저장한 값 등)이면 추천순이다.
export function toAppSort(v: string | undefined): AppSort {
  return APP_SORTS.find((s) => s === v) ?? DEFAULT_APP_SORT
}

// sortApps는 ids를 sort 방식으로 늘어놓는다. ids는 추천순이어야 한다. 이름이 같으면 추천순을 따른다.
export function sortApps<T extends string>(
  ids: readonly T[],
  sort: AppSort,
  titleOf: (id: T) => string,
  locale: string,
): T[] {
  if (sort !== DrawerSortName) return [...ids]
  const collator = new Intl.Collator(locale, { sensitivity: 'base', numeric: true })
  return ids
    .map((id, index) => ({ id, index, title: titleOf(id) }))
    .sort((a, b) => collator.compare(a.title, b.title) || a.index - b.index)
    .map((a) => a.id)
}
