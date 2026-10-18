---
paths:
  - "frontend/**"
---

# 프론트엔드 공통

- Vite `base`는 상대 경로 `./`다. 자산과 API(`${BASE_URL}api`)를 문서 URL 기준으로 찾으므로 어떤 경로 접두사 아래에서도 동작한다. 절대 경로(`/...`)로 자산·API를 참조하지 않는다. 개발 서버는 `/`에서 서빙하고 `/api/*`를 백엔드로 프록시한다
- `server.fs.allow`는 `frontend/`와 `vendor/icons/`만 허용한다. 개발 서버가 LAN에 열려 있으므로 저장소 루트 전체를 허용하지 않는다
- 개인 환경 값(`DEV_HOST`, `DEV_ALLOWED_HOSTS`, `ONMARU_LISTEN`, `ONMARU_OMV_URL`, `ONMARU_WORKBENCH_URL`)은 커밋하지 않는 `frontend/.env.local`에 둔다. `scripts/dev.sh`가 `ONMARU_*`·`DEV_*` 값을 환경 변수로 내보내므로 git 워크트리에서 실행하거나 `make dev BRANCH=<브랜치>`로 워크트리를 골라도 주 체크아웃의 값이 적용된다(워크트리의 `frontend/.env.local`이 우선). `make dev-fake`는 `ONMARU_OMV_URL`·`ONMARU_WORKBENCH_URL`을 가짜 OMV로 덮는다
- `src/api/types.gen.ts`, `src/addons/manifests.gen.ts`는 생성물이다. 직접 수정하지 말고 `make types`로 다시 만든다
- 컴포넌트 테스트용 DOM 환경(jsdom 등)은 쓰지 않는다. 로직은 DOM 없는 순수 함수·저장소로 분리해 node 환경의 Vitest로 테스트하고, 화면 동작은 브라우저에서 확인한다
- 주기 요청은 `src/addons/common/poller.ts` 방식을 따른다(응답 후 다음 요청 예약, 중첩 없음). 탭이 보이지 않으면 멈춘다(창·위젯 틀이 `usePageVisible`을 보기의 `active`에 넣는다). 예외는 시스템 정보 창의 사용량 기록뿐이다: 창이 열려 있으면 숨겨도 Worker 타이머로 계속 가져온다(`.claude/docs/addons.md`의 "시스템 정보")
  - 애드온 데이터는 엔드포인트마다 모듈 수준의 공유 폴러(`sharedPoll.ts`의 `createSharedPoll`) 하나를 두고 보기는 `usePolledData`로 붙는다. 같은 엔드포인트를 보는 창·위젯은 요청 하나를 가장 짧은 주기로 함께 쓴다. 보기별로 달라야 하는 값(네트워크 속도)은 `usePolledData`의 `reduce`로, 보이지 않는 동안에도 쌓아야 하는 값(시스템 정보 창의 기록)은 기록기(`recorder.ts`)의 `reduce`로 만든다. 폴러의 타이머는 `schedule`로 바꿀 수 있다(기본 `setTimeout`, 시스템 정보 창은 Worker 타이머). 누적 카운터의 차이로 값을 구하는 엔드포인트는 `warmupMs`로 시작 직후 두 번째 요청을 빨리 보낼 수 있다
- 텍스트 선택·드래그·브라우저 컨텍스트 메뉴는 기본 금지다(`shell/nativeGuard.ts`, `base.css`의 `user-select`). `selectable` 클래스는 텍스트 선택이 꼭 필요한 영역에만 붙인다. 현황 수치는 선택 불가로 둔다. 텍스트 입력 요소는 자동 허용된다

## CSS 배치

- 일반 전역 CSS다(CSS Modules 아님). 진입점은 `src/index.css` 하나이고 주석과 `@import` 목록만 둔다. 컴포넌트는 CSS를 직접 import하지 않는다(적용 순서를 모듈 그래프가 아니라 이 목록이 정하게)
  - 예외: 지연 청크에서만 쓰는 제3자 스타일(`@xterm/xterm/css/xterm.css`)은 그 보기 컴포넌트(`terminal/TerminalWindow.tsx`)가 import한다. 자기 클래스(`.xterm*`)만 다뤄 순서에 영향이 없고, 진입 CSS에 넣지 않기 위해서다. 덮어쓰는 규칙은 우리 CSS에서 명시도를 높여 둔다
- 파일 위치는 클래스를 보고 짐작할 수 있게 정한다
  1. 컴포넌트 폴더(`shell/`, `auth/`, `security/`, `icons/`, `settings/`, `terminal/`, `addons/` 루트): 클래스를 그리는 컴포넌트와 같은 이름으로 옆에 둔다(`Taskbar.tsx` → `Taskbar.css`)
  2. 애드온 폴더(`addons/<id>/`, `addons/common/`): 폴더 이름의 파일 하나에 창·위젯 스타일을 함께 둔다(`addons/network/network.css`). 쓸 스타일이 없으면 만들지 않는다
  3. 여러 곳이 쓰는 클래스는 주인(이름 접두사를 가진 쪽) 파일 한 곳에만 둔다. 애드온 공통 클래스(`.addon-*`, `.widget-line` 등, `.status-dot`, `.row-*`)는 `addons/common/common.css`
  - 전역 토대(디자인 토큰, reset, `user-select`)는 `src/base.css`
- 같은 명시도로 덮어쓰는 규칙은 덮이는 쪽보다 뒤 파일에 둔다(`index.css`의 import 순서: base → icons → shell → auth → security → 애드온(루트·공통·애드온별) → terminal → settings). 새 CSS 파일은 `index.css`에 반드시 추가한다. `src/styles.test.ts`가 빠진 파일을 잡는다
