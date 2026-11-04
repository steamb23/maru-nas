// 애드온 패키지 SDK 런타임 빌드. sdk/js/src/client.ts를 의존성 없는 고전 스크립트 하나(sdk.js)로, 공통 CSS
// (tokens.css, components.css, frame.css)를 하나(maru.css)로 묶어 backend/internal/web/static/dist/addon-sdk/에 둔다.
// 호스트가 애드온 프레임 경로의 _maru/로 이 두 파일을 준다(backend/internal/server/addon_ui.go).
// 대시보드 빌드(vite build)가 dist를 비우므로 그 뒤에 실행한다(package.json의 build). 개발 서버는 scripts/dev.sh가
// --watch --outDir로 상태 디렉터리에 만든다(MARU_DEV_ADDON_SDK).
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { type Plugin, defineConfig } from 'vite'

const fromHere = (p: string) => fileURLToPath(new URL(p, import.meta.url))
const sdkSrc = fromHere('../sdk/js/src')
const CSS_PARTS = ['tokens.css', 'components.css', 'frame.css']

// maruCSS는 공통 CSS를 이어 붙여 maru.css로 내보낸다. 파일마다 머리 주석은 그대로 둔다.
function maruCSS(): Plugin {
  return {
    name: 'maru-sdk-css',
    buildStart() {
      for (const f of CSS_PARTS) this.addWatchFile(`${sdkSrc}/${f}`)
    },
    generateBundle() {
      const source = CSS_PARTS.map((f) => readFileSync(`${sdkSrc}/${f}`, 'utf8')).join('\n')
      this.emitFile({ type: 'asset', fileName: 'maru.css', source })
    },
  }
}

export default defineConfig({
  // 이 빌드에는 public 폴더가 없다(대시보드의 public을 다시 복사하지 않게).
  publicDir: false,
  plugins: [maruCSS()],
  build: {
    outDir: fromHere('../backend/internal/web/static/dist/addon-sdk'),
    emptyOutDir: true,
    target: 'es2022',
    minify: true,
    sourcemap: false,
    lib: {
      entry: `${sdkSrc}/client.ts`,
      formats: ['iife'],
      name: 'maruSdk',
      fileName: () => 'sdk.js',
    },
  },
})
