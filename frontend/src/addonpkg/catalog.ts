// 애드온 패키지 목록(capabilities의 addonPackages). DOM·i18next에 의존하지 않아 node 환경에서 테스트한다.
//
// 앱·위젯 표(windows/apps.ts, widgets/catalog.ts)는 내장 표 다음에 이 목록을 본다. 지금 목록(packages)은 보이는
// 앱·위젯을 정하고, 이 탭에서 한 번이라도 본 패키지(known)는 사라진 뒤에도 남아 열린 창의 제목·아이콘·크기를 그린다
// (묘비). 그래야 패키지를 끄거나 지운 순간 그 창을 그리던 컴포넌트가 정의를 잃지 않는다.
import { create } from 'zustand'
import type { AddonPackageView } from '../api/types.gen'

// PackageAppId는 애드온 패키지의 앱 id다. 내장 앱 id(컴파일 타임 유니언)와 섞이지 않게 표시를 붙인 문자열이다.
declare const packageBrand: unique symbol
export type PackageAppId = string & { readonly [packageBrand]: true }

// packageTitleKey는 패키지 이름을 넣은 번역 키다(addonpkg/names.ts가 i18next에 넣는다). en.json에는 없는 이름공간이다.
export function packageTitleKey(id: string): string {
  return `addonPackageNames.${id}`
}

interface CatalogState {
  // packages는 지금 capabilities가 준 목록이다(이 사용자에게 보이는 패키지).
  packages: readonly AddonPackageView[]
  // known은 이 탭에서 본 모든 패키지의 마지막 정의다(id → 정의).
  known: ReadonlyMap<string, AddonPackageView>
}

export const usePackageCatalog = create<CatalogState>(() => ({ packages: [], known: new Map() }))

// setPackages는 capabilities를 받을 때 부른다.
export function setPackages(list: readonly AddonPackageView[]) {
  const known = new Map(usePackageCatalog.getState().known)
  for (const p of list) known.set(p.id, p)
  usePackageCatalog.setState({ packages: list, known })
}

// clearPackages는 로그아웃·만료 때 부른다(다음 사용자에게 이전 사용자의 패키지가 보이지 않게).
export function clearPackages() {
  usePackageCatalog.setState({ packages: [], known: new Map() })
}

// currentPackage는 지금 목록의 id 패키지다.
export function currentPackage(id: string): AddonPackageView | undefined {
  return usePackageCatalog.getState().packages.find((p) => p.id === id)
}

// knownPackage는 이 탭에서 본 id 패키지의 마지막 정의다.
export function knownPackage(id: string): AddonPackageView | undefined {
  return usePackageCatalog.getState().known.get(id)
}

// currentPackages는 지금 목록이다.
export function currentPackages(): readonly AddonPackageView[] {
  return usePackageCatalog.getState().packages
}

// isKnownPackage는 id가 이 탭에서 본 패키지인지다.
export function isKnownPackage(id: string): id is PackageAppId {
  return usePackageCatalog.getState().known.has(id)
}

// localizedText는 언어별 문구에서 lang을, 없으면 en을, 그것도 없으면 첫 값을 고른다.
export function localizedText(text: Readonly<Record<string, string>> | undefined, lang: string): string {
  if (!text) return ''
  return text[lang] ?? text.en ?? Object.values(text)[0] ?? ''
}
