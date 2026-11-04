// 테스트용 패키지 목록 항목. 테스트 파일들이 함께 쓴다.
import type { AddonPackageView } from '../api/types.gen'

export function testPackage(id: string, edit: Partial<AddonPackageView> = {}): AddonPackageView {
  return {
    id,
    rev: '1.0.0-0123abcd',
    dev: false,
    version: '1.0.0',
    name: { en: `Pkg ${id}`, ko: `패키지 ${id}` },
    author: 'Example',
    icon: `api/addons/${id}/icon?rev=1.0.0-0123abcd`,
    uiBase: `api/addon-ui/token/1.0.0-0123abcd/`,
    views: { window: { entry: 'window.html', defaultSize: { width: 480, height: 320 } } },
    requires: { admin: false, plugins: [] },
    actions: [],
    languages: ['en'],
    sdk: 1,
    backend: false,
    elevated: false,
    ...edit,
  }
}
