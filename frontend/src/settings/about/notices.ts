// 설정의 정보 페이지에 보여 줄 제3자 소프트웨어·에셋 고지. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 번들(프론트엔드)이나 실행 파일(백엔드)에 들어가는 것만 둔다. 개발 도구(Vite, TypeScript 등)는 넣지 않는다.
// 프론트 런타임 의존성을 더하면 여기에도 넣는다(notices.test.ts가 package-lock.json과 비교한다).
// 아이콘·로고 출처는 vendor/icons/README.md, 로고 글꼴은 .claude/rules/icons.md와 맞춘다.
import type { ParseKeys } from 'i18next'

export interface Notice {
  name: string
  // SPDX 라이선스 식별자
  license: string
  copyright: string
  url: string
  // npm 패키지 이름. 프론트 번들에 들어가는 패키지면 적는다(검사용).
  packages?: readonly string[]
  // 쓰임새나 조건에 대한 짧은 설명의 번역 키
  noteKey?: ParseKeys
}

export const NOTICES: readonly Notice[] = [
  {
    name: 'Colloid icon theme',
    license: 'GPL-3.0',
    copyright: 'Vince Liuice',
    url: 'https://github.com/vinceliuice/Colloid-icon-theme',
  },
  {
    name: 'openmediavault logo',
    license: 'GPL-3.0',
    copyright: 'Volker Theile',
    url: 'https://github.com/openmediavault/openmediavault',
    noteKey: 'settings.about.notes.omvLogo',
  },
  {
    name: 'Pretendard',
    license: 'OFL-1.1',
    copyright: 'Kil Hyung-jin',
    url: 'https://github.com/orioncactus/pretendard',
    noteKey: 'settings.about.notes.pretendard',
  },
  {
    name: 'React',
    license: 'MIT',
    copyright: 'Meta Platforms, Inc. and affiliates',
    url: 'https://github.com/facebook/react',
    packages: ['react', 'react-dom', 'scheduler', 'use-sync-external-store'],
  },
  {
    name: 'i18next',
    license: 'MIT',
    copyright: 'i18next',
    url: 'https://github.com/i18next/i18next',
    packages: ['i18next'],
  },
  {
    name: 'react-i18next',
    license: 'MIT',
    copyright: 'i18next',
    url: 'https://github.com/i18next/react-i18next',
    packages: ['react-i18next'],
  },
  {
    name: 'html-parse-stringify',
    license: 'MIT',
    copyright: 'Henrik Joreteg, i18next',
    url: 'https://github.com/i18next/html-parse-stringify',
    packages: ['html-parse-stringify'],
  },
  {
    name: 'Zustand',
    license: 'MIT',
    copyright: 'Paul Henschel',
    url: 'https://github.com/pmndrs/zustand',
    packages: ['zustand'],
  },
  {
    name: 'Babel runtime',
    license: 'MIT',
    copyright: 'Sebastian McKenzie and other contributors',
    url: 'https://github.com/babel/babel',
    packages: ['@babel/runtime'],
  },
  {
    name: 'xterm.js',
    license: 'MIT',
    copyright: 'The xterm.js authors, SourceLair Private Company, Christopher Jeffrey',
    url: 'https://github.com/xtermjs/xterm.js',
    packages: ['@xterm/xterm', '@xterm/addon-fit'],
    noteKey: 'settings.about.notes.xterm',
  },
  {
    name: 'Go',
    license: 'BSD-3-Clause',
    copyright: 'The Go Authors',
    url: 'https://go.dev/',
    noteKey: 'settings.about.notes.go',
  },
  {
    name: 'Go x/crypto',
    license: 'BSD-3-Clause',
    copyright: 'The Go Authors',
    url: 'https://cs.opensource.google/go/x/crypto',
    noteKey: 'settings.about.notes.goX',
  },
  {
    name: 'websocket',
    license: 'ISC',
    copyright: 'Coder',
    url: 'https://github.com/coder/websocket',
    noteKey: 'settings.about.notes.websocket',
  },
]

// 형식 정보만 들어 있어 번들에 들어가지 않는 런타임 의존성(타입 선언 등).
const TYPES_ONLY = new Set(['@types/react', 'csstype', 'typescript'])

// missingNotices는 lock 파일의 런타임(개발용이 아닌) 패키지 중 고지에 없는 것이다.
export function missingNotices(lockPackages: Record<string, { dev?: boolean }>): string[] {
  const covered = new Set(NOTICES.flatMap((n) => n.packages ?? []))
  return Object.entries(lockPackages)
    .filter(([path, p]) => path.startsWith('node_modules/') && !p.dev)
    .map(([path]) => path.slice('node_modules/'.length))
    .filter((name) => !name.includes('/node_modules/') && !TYPES_ONLY.has(name) && !covered.has(name))
}
