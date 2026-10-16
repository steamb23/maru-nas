---
paths:
  - "backend/internal/omv/**"
  - "backend/internal/auth/**"
  - "backend/internal/server/**"
  - "backend/cmd/**"
  - "frontend/src/auth/**"
  - "frontend/src/api/**"
---

# 인증과 OMV 세션

## 확인된 OMV 8 동작

OMV 공식 저장소 소스(2026-10 기준)에서 확인한 내용이다. OMV 업데이트 후 동작이 다르면 이 섹션을 갱신한다. 로그인 관련 소스는 `/var/www/openmediavault/rpc/session.inc`.

- **로그인 흐름은 두 단계**: `Session.login`(`username`, `password`) 응답의 `status`가 `authenticated`면 완료, `challengeRequired`면 TOTP 등 2차 인증이 필요하다. 이때 같은 세션으로 `Session.verify`(`challengeresponse`)를 호출한다. 2FA를 쓰지 않더라도 이 분기를 항상 구현한다
- **8.5.0 이전 형식**: `status` 필드와 2차 인증 분기는 OMV 8.5.0(Issue #2235)에서 생겼다. 8.4 이하는 `{"authenticated": true, "username", "permissions", "sessionid"}` 형식이라 백엔드가 `omv login: unexpected response`로 거절한다. 그래서 OMV 8.5 이상만 지원한다(2026-10, OMV 8.3.1 ISO로 설치한 VM에서 확인. 최신 ISO도 8.3.1이라 설치 직후에는 업데이트가 필요하다)
- **2차 인증 시 세션 ID**: `challengeRequired`일 때 OMV는 세션 ID를 바꾸고(`session_regenerate_id`) 새 ID를 응답 본문 없이 `Set-Cookie`로만 준다. `Session.verify`는 그 ID로 호출해야 한다. `challengeresponse`의 형식은 2FA 플러그인이 정한다(이 NAS에는 2FA 플러그인이 없어 미검증)
- **대기 중 세션 주의**: 인증되지 않은 세션으로 엔진 RPC(Session 밖의 서비스)를 부르면 OMV가 세션을 파기하므로, 2차 인증 대기 중인 세션으로는 다른 RPC를 부르지 않는다
- **세션 쿠키**: 이름 `OPENMEDIAVAULT-SESSIONID`, HttpOnly, SameSite=Strict, HTTPS일 때 Secure. 쿠키 대신 `X-OpenMediaVault-SessionID` 헤더도 받는다
- **세션 바인딩**: 세션은 User-Agent에 묶이며 다르면 서버가 세션을 파기한다(`Session.logout` 같은 로컬 서비스 호출 시 검사). IP 검사(`validateIpAddress`)는 OMV 소스에서 꺼져 있다. 한 주체가 받은 세션을 다른 주체에게 넘겨 쓰는 설계는 하지 않는다
- **클라이언트 IP**: 인증 로그의 IP는 `X-Forwarded-For` 등 프록시 헤더를 그대로 믿어 정한다(세션 검증에는 쓰지 않음). 백엔드는 브라우저 IP를 `X-Forwarded-For`로 넘긴다
- **세션 만료**: OMV 웹 관리 설정의 timeout 값(기본 5분)을 따른다. 인증된 호출마다 마지막 접근 시각이 갱신되며, `options.updatelastaccess=false`면 갱신하지 않는다
- **세션 확인용 RPC**: `WebGui.getAutoLogoutSettings`. 엔진 RPC라 인증된 세션이 필요하고 역할 제한이 없으며, 자동 로그아웃 시간(`timeout`, 분, 0이면 꺼짐)을 돌려준다. OMV workbench도 이 값으로 브라우저에서 자동 로그아웃 타이머를 돈다. `System.noop`은 admin 전용이다
- **로그인 실패 제한**: 비밀번호 실패에는 OMV 쪽 지연이 없다(2차 인증 실패만 제한, 429). 그래서 백엔드가 자체 제한을 둔다
- **새 브라우저 로그인 메일**: 로그인 성공 시 `OPENMEDIAVAULT-LOGIN-*` 쿠키가 없으면 "새 브라우저 로그인" 알림 메일을 보낸다. 백엔드가 이 쿠키(비밀 아님)를 사용자별로 메모리에 보관해 다음 로그인에 실어 보내므로, 백엔드 재시작 후 첫 로그인에만 메일이 간다
- **권한**: 로그인 응답의 `permissions.role`에 admin 포함 여부가 들어 있다. 관리자가 아니면 설정 관련 창과 버튼을 숨긴다
- **workbench의 탭별 로그인**: OMV workbench는 사용자 이름과 권한을 sessionStorage에 저장하고 그것으로 로그인 여부를 판단해서, 새 탭마다 다시 로그인해야 한다
- **대시보드 로그인을 OMV 탭에 넘길 수 없다**: Session 서비스에는 `login`·`verify`·`logout`만 있고 토큰·SSO 같은 비밀번호 없는 로그인이 없다. 백엔드의 OMV 세션은 고정 User-Agent에 묶여 브라우저에 넘기면 파기되고(대시보드도 로그아웃된다), 쿠키가 있어도 새 탭은 sessionStorage가 비어 로그인 화면이 뜬다. 그래서 OMV 워크벤치 링크 앱은 이름 붙은 탭 하나를 다시 써서 그 탭의 로그인을 유지한다(OMV 8.5 소스 기준)
- **보안 헤더**: 기본값이 `X-Frame-Options: SAMEORIGIN`, CSP `default-src 'self'`. OMV 화면을 iframe에 넣는 건 같은 출처에서만 가능하다. 보안 헤더를 끄는 방식은 쓰지 않는다

## 인증 설계

- 로그인 화면은 자체 디자인하고, 인증은 백엔드가 `Session.login`(필요하면 `Session.verify`)을 호출해 위임한다. 계정·비밀번호를 따로 저장하지 않는다
- 로그인 성공 시 대시보드 자체 세션 쿠키(`onmaru_session`, HttpOnly, SameSite=Strict, Path는 `-cookie-path`, 기본 `/`)를 발급한다. 2차 인증 대기 중에는 `onmaru_pending`. Secure는 HTTPS로 들어온 요청(`X-Forwarded-Proto: https`)에만 붙인다(평문 HTTP 개발 서버에서는 브라우저가 Secure 쿠키를 저장하지 않으므로). 세션 저장소는 메모리이며 백엔드 재시작 시 모두 재로그인한다
- 백엔드의 OMV 클라이언트는 세션 ID를 쿠키가 아닌 `X-OpenMediaVault-SessionID` 헤더로 보내고, 응답 `Set-Cookie`로 바뀐 ID를 갱신한다
- `/api/*`는 대시보드 세션이 없으면 401을 반환한다(`/api/health`, 로그인 라우트, 로그인 화면이 쓰는 `GET /api/site`와 `GET /api/site/images/login-background` 제외). 본문이 있는 요청(POST, PUT)은 `Content-Type: application/json`만 받는다(CSRF 보조 장치)
  - 예외: 이미지 올리기(`PUT /api/preferences/wallpaper-image`, `…/avatar-image`, `PUT /api/site/images/{name}`)는 원시 이미지 본문(`image/png|jpeg|webp`)만 받는다. HTML 폼은 이 형식을 보낼 수 없고 다른 출처의 fetch는 preflight에서 막히므로 같은 보조 장치가 된다. multipart는 받지 않는다
  - 공개 로그인 배경은 대시보드 주소에 닿는 누구나 받을 수 있다(로그인 화면에 보이는 그림이다). 설정 앱이 관리자에게 이 점을 알린다
- 로그인 상태를 localStorage·sessionStorage에 저장하지 않는다. 앱 시작 시 `/api/session`으로 상태를 복원한다(workbench처럼 탭마다 로그인하지 않게 하기 위함)
- OMV가 중계 호출에 401을 반환하면 해당 대시보드 세션을 지우고 브라우저를 로그인 화면으로 보낸다
- 로그인 시도 제한: 실패 응답 전 1초 지연, IP·사용자 이름별로 15분 안에 5번째 실패부터 30초에서 두 배씩 최대 15분 잠금(429, `Retry-After`), 한 IP의 동시 시도는 2개까지
  - 사용자 이름별 잠금은 IP와 무관하므로 누구나 남의 계정을 잠글 수 있다. 그래서 인증을 끝까지 마친(2차 인증 포함) 브라우저에 신뢰 기기 쿠키(`onmaru_device`, HttpOnly, SameSite=Strict, 30일, 로그인할 때마다 새 토큰)를 주고, 같은 사용자 이름으로 이 토큰을 보낸 시도는 사용자 이름별 잠금을 건너뛴다. IP별 제한은 그대로 받고, 토큰으로 5번 틀리면 토큰을 폐기한다. 로그아웃해도 지우지 않는다. 토큰은 메모리에만 두므로 백엔드를 재시작하면 모두 사라진다(`auth.Limiter.Trust`)
  - 실패 기록은 1만 개까지 둔다. 넘치면 기간이 지난 기록, 잠기지 않은 기록, 잠긴 기록 순으로 오래된 것부터 지운다
- 로그에는 사용자 이름·IP·결과만 남긴다. 비밀번호, 대시보드·OMV 세션 ID, OMV 오류의 `trace`는 남기지 않고 브라우저에도 넘기지 않는다
- 로그아웃하거나 다른 사용자가 로그인하면 창을 모두 닫는다. 세션 만료 후 같은 사용자가 다시 로그인하면 창 배치를 남긴다

### 세션 만료는 브라우저 타이머로 처리한다

- 백엔드는 로그인 직후와 확인 때마다 `WebGui.getAutoLogoutSettings`로 timeout을 읽는다. OMV 세션의 마지막 접근 시각을 갱신하는 것은 이 백엔드뿐이므로(로그인, 활동 신호) 마지막 갱신 시각 + timeout이 만료 시각이다. `/api/session`, `/api/session/activity`, 로그인 응답의 `expiresInSeconds`(남은 초. 시계 차이를 피하려고 절대 시각을 쓰지 않음)로 알려 준다
- 프론트(`sessionKeeper.ts`의 순수 함수 `decide`)는 사용자 입력이 있으면 최대 60초에 한 번 `/api/session/activity`로 연장한다. 아직 보내지 않은 입력이 있으면 만료 10초 전에는 반드시 보낸다(timeout이 1분이어도 조작 중에는 만료되지 않게). 입력 없이 만료 시각이 지나면 2초 뒤 `/api/session`으로 확인하고, 401이면 로그인 화면으로 보낸다(다른 탭이 연장했으면 새 만료 시각을 받는다)
- 주기 요청(시스템 정보 갱신 등)과 세션 확인은 `updatelastaccess=false`라 OMV 세션을 연장하지 않는다
- 안전장치로 백엔드는 요청 처리 중 마지막 확인에서 60초가 지났거나 예상 만료 시각이 지났으면 OMV에 확인한다(OMV 설정 변경, OMV 재시작 등). OMV에 연결할 수 없을 때는 세션을 유지한다
- JWT는 쓰지 않는다. OMV 세션 ID 때문에 서버 쪽 세션 저장소가 어차피 필요하고, HttpOnly 쿠키 안의 `exp`는 브라우저가 읽을 수 없으며, 조작할 때마다 만료가 늘어나는 구조와 맞지 않는다

### 프론트 인증 저장소의 주의점

- `createAuthStore`로 API와 화면 전환 훅을 주입한다. 테스트는 가짜 API를 넣는다
- 로그아웃·만료로 화면을 떠나는 동안은 상태를 `loggedIn`으로 두고, 그 사이에 다시 들어온 만료·로그아웃은 무시한다
- API 클라이언트는 401을 받으면 `setUnauthorizedHandler`로 등록한 `expire()`를 부른다. 로그인 관련 요청은 제외한다
