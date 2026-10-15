/// <reference types="vitest/config" />
import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'

const fromHere = (p: string) => fileURLToPath(new URL(p, import.meta.url))

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // 개인 환경 값은 커밋하지 않는 frontend/.env.local에 둔다.
  //   DEV_HOST            개발 서버 바인딩 주소 (기본: 모든 인터페이스)
  //   DEV_ALLOWED_HOSTS   IP가 아닌 호스트 이름으로 접속할 때 허용할 이름 (쉼표 구분)
  //   ONMARU_LISTEN      백엔드 주소 (기본: 127.0.0.1:8090)
  const env = { ...loadEnv(mode, fromHere('.'), ''), ...process.env }
  const backend = env.ONMARU_LISTEN || '127.0.0.1:8090'

  return {
    // 상대 경로로 빌드해 어떤 경로 접두사 아래에서도 동작하게 한다. 대시보드를 둘 호스트·경로는
    // 리버스 프록시가 정한다(접두사 아래에 둘 때는 프록시가 접두사를 떼고 넘긴다).
    base: './',
    plugins: [react()],
    resolve: {
      alias: {
        // 기본 아이콘 테마(Colloid)의 원본 SVG.
        '@colloid': fromHere('../vendor/icons/colloid/src'),
        // 테마 밖 제3자 아이콘 출처(vendor/icons/<출처>/). src/icons/sources/에서만 쓴다.
        '@vendor-icons': fromHere('../vendor/icons'),
      },
    },
    server: {
      host: env.DEV_HOST || true,
      port: 5173,
      strictPort: true,
      allowedHosts: env.DEV_ALLOWED_HOSTS ? env.DEV_ALLOWED_HOSTS.split(',') : [],
      // 아이콘 서브모듈이 Vite 루트 밖에 있으므로 vendor/icons만 추가로 허용한다.
      // 저장소 루트 전체를 허용하면 CLAUDE.local.md 등이 LAN에 노출된다.
      fs: { allow: [fromHere('.'), fromHere('../vendor/icons')] },
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
    },
    test: {
      environment: 'node',
      // Vitest는 CSS를 빈 문자열로 바꾼다. import 목록을 검사하는 styles.test.ts만 index.css 원문을 읽는다.
      css: { include: [/\/src\/index\.css/] },
    },
  }
})
