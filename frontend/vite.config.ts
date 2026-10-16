/// <reference types="vitest/config" />
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import { type Plugin, defineConfig, loadEnv } from 'vite'

const fromHere = (p: string) => fileURLToPath(new URL(p, import.meta.url))

const vendorIcons = fromHere('../vendor/icons')
const locales = fromHere('src/i18n/locales')

// Colloid의 일부 SVG(external-link-symbolic 등)는 루트 <svg>에 xmlns가 없어 브라우저가 이미지로 읽지 못한다.
// 그러면 mask-image가 비어 아이콘이 보이지 않는다. 원본(서브모듈)은 고치지 않고, ?url로 불러올 때만
// SVG 이름공간을 붙여 data URL로 넣는다. 따로 내보낸 .svg 파일도 같은 이유로 깨지므로 크기와 관계없이 넣는다.
function svgNamespace(): Plugin {
  return {
    name: 'onmaru:svg-namespace',
    enforce: 'pre',
    async load(id) {
      const [file, query = ''] = id.split('?', 2)
      if (!file.endsWith('.svg') || !file.startsWith(`${vendorIcons}/`) || !/(^|&)url(&|$)/.test(query)) return null
      const src = await readFile(file, 'utf8')
      const root = src.match(/<svg\b[^>]*>/)
      if (!root || /\sxmlns=['"]http:\/\/www\.w3\.org\/2000\/svg['"]/.test(root[0])) return null
      const fixed = src.replace(/<svg\b/, '<svg xmlns="http://www.w3.org/2000/svg"')
      return `export default ${JSON.stringify(`data:image/svg+xml,${encodeURIComponent(fixed)}`)}`
    },
  }
}

// 번역 카탈로그는 고른 언어만 따로 받지만(i18n/index.ts), "모든 앱" 검색은 모든 언어의 앱 이름이 필요하다.
// locales/<언어>.json?apps는 카탈로그에서 apps 부분만 남긴 모듈이라 전체 카탈로그와 다른 모듈로 묶인다
// (i18n/appNames.ts).
function catalogApps(): Plugin {
  return {
    name: 'onmaru:catalog-apps',
    enforce: 'pre',
    async load(id) {
      const [file, query = ''] = id.split('?', 2)
      if (!file.endsWith('.json') || !file.startsWith(`${locales}/`) || !/(^|&)apps(&|$)/.test(query)) return null
      this.addWatchFile(file)
      const { apps } = JSON.parse(await readFile(file, 'utf8'))
      return { code: `export default ${JSON.stringify({ apps })}`, moduleType: 'js' }
    },
  }
}

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // 개인 환경 값은 커밋하지 않는 frontend/.env.local에 둔다.
  //   DEV_HOST            개발 서버 바인딩 주소 (기본: 모든 인터페이스)
  //   DEV_ALLOWED_HOSTS   IP가 아닌 호스트 이름으로 접속할 때 허용할 이름 (쉼표 구분)
  //   ONMARU_LISTEN      백엔드 주소 (기본: 127.0.0.1:8090)
  //   DEV_PORT           개발 서버 포트 (기본: 5173). scripts/dev.sh의 포트 검사와 같은 값을 쓴다
  const env = { ...loadEnv(mode, fromHere('.'), ''), ...process.env }
  const backend = env.ONMARU_LISTEN || '127.0.0.1:8090'

  return {
    // 상대 경로로 빌드해 어떤 경로 접두사 아래에서도 동작하게 한다. 대시보드를 둘 호스트·경로는
    // 리버스 프록시가 정한다(접두사 아래에 둘 때는 프록시가 접두사를 떼고 넘긴다).
    base: './',
    // DEV_PORT로 개발 서버를 하나 더 띄울 때 의존성 최적화 캐시(node_modules/.vite)를 함께 쓰지 않게 나눈다.
    cacheDir: env.DEV_PORT ? `node_modules/.vite-${env.DEV_PORT}` : undefined,
    plugins: [svgNamespace(), catalogApps(), react()],
    resolve: {
      alias: {
        // 기본 아이콘 테마(Colloid)의 원본 SVG.
        '@colloid': fromHere('../vendor/icons/colloid/src'),
        // 테마 밖 제3자 아이콘 출처(vendor/icons/<출처>/). src/icons/sources/에서만 쓴다.
        '@vendor-icons': vendorIcons,
      },
    },
    server: {
      host: env.DEV_HOST || true,
      port: Number(env.DEV_PORT) || 5173,
      strictPort: true,
      allowedHosts: env.DEV_ALLOWED_HOSTS ? env.DEV_ALLOWED_HOSTS.split(',') : [],
      // 아이콘 서브모듈이 Vite 루트 밖에 있으므로 vendor/icons만 추가로 허용한다.
      // 저장소 루트 전체를 허용하면 CLAUDE.local.md 등이 LAN에 노출된다.
      fs: { allow: [fromHere('.'), vendorIcons] },
      proxy: {
        // xfwd: 브라우저 IP를 X-Forwarded-For로 넘긴다(로그인 제한·OMV 인증 로그용).
        '/api': {
          target: `http://${backend}`,
          xfwd: true,
        },
      },
    },
    build: {
      // go:embed로 실행 파일에 포함하기 위해 Go 패키지 안에 빌드한다.
      outDir: fromHere('../backend/internal/web/static/dist'),
      emptyOutDir: true,
      rolldownOptions: {
        output: {
          // 라이브러리(React, i18next 등)를 앱 코드와 다른 청크로 나눈다. 진입 청크 하나가 500KB 경고 기준을
          // 넘지 않게 하려는 것이며 내려받는 총량은 같다.
          codeSplitting: {
            groups: [{ name: 'vendor', test: /node_modules[\\/]/ }],
          },
        },
      },
    },
    test: {
      environment: 'node',
      // Vitest는 CSS를 빈 문자열로 바꾼다. import 목록을 검사하는 styles.test.ts만 index.css 원문을 읽는다.
      css: { include: [/\/src\/index\.css/] },
      // 변환 결과를 파일로 캐시하면 import.meta.glob 결과도 함께 남아, 파일을 더하거나 지워도 테스트 파일이
      // 바뀌기 전까지 옛 목록으로 검사한다(catalog.test.ts의 카탈로그, resolver.test.ts·styles.test.ts의 목록).
      fsModuleCache: false,
    },
  }
})
