---
paths:
  - "frontend/**"
---

# 프론트엔드 공통

- Vite `base`는 `/desk/`. 개발 서버는 `/desk/api/*`를 `/desk`를 뗀 뒤 백엔드로 프록시한다
- `server.fs.allow`는 `frontend/`와 `vendor/icons/`만 허용한다. 개발 서버가 LAN에 열려 있으므로 저장소 루트 전체를 허용하지 않는다
- 개인 환경 값(`DEV_HOST`, `DEV_ALLOWED_HOSTS`, `OMVDESK_LISTEN`, `OMVDESK_OMV_URL`)은 커밋하지 않는 `frontend/.env.local`에 둔다
- `src/api/types.gen.ts`, `src/addons/manifests.gen.ts`는 생성물이다. 직접 수정하지 말고 `make types`로 다시 만든다
- 컴포넌트 테스트용 DOM 환경(jsdom 등)은 쓰지 않는다. 로직은 DOM 없는 순수 함수·저장소로 분리해 node 환경의 Vitest로 테스트하고, 화면 동작은 브라우저에서 확인한다
- 주기 요청은 `src/addons/common/poller.ts` 방식을 따른다(응답 후 다음 요청 예약, 중첩 없음). 탭이 보이지 않으면 멈춘다(창·위젯 틀이 `usePageVisible`을 보기의 `active`에 넣는다)
- 텍스트 선택·드래그·브라우저 컨텍스트 메뉴는 기본 금지다(`shell/nativeGuard.ts`, `index.css`의 `user-select`). `selectable` 클래스는 텍스트 선택이 꼭 필요한 영역에만 붙인다. 현황 수치는 선택 불가로 둔다. 텍스트 입력 요소는 자동 허용된다
