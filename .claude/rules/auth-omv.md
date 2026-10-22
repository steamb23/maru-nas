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
- **2차 인증 시 세션 ID**: `challengeRequired`일 때 OMV는 세션 ID를 바꾸고(`session_regenerate_id`) 새 ID를 응답 본문 없이 `Set-Cookie`로만 준다. `Session.verify`는 그 ID로 호출해야 한다. `challengeresponse`의 형식은 2FA 플러그인이 정한다(이 NAS에는 2FA 플러그인이 없어 미검증)
- **대기 중 세션 주의**: 인증되지 않은 세션으로 엔진 RPC(Session 밖의 서비스)를 부르면 OMV가 세션을 파기하므로, 2차 인증 대기 중인 세션으로는 다른 RPC를 부르지 않는다
- **세션 쿠키**: 이름 `OPENMEDIAVAULT-SESSIONID`, HttpOnly, SameSite=Strict, HTTPS일 때 Secure. 쿠키 대신 `X-OpenMediaVault-SessionID` 헤더도 받는다
- **세션 바인딩**: 세션은 User-Agent에 묶이며 다르면 서버가 세션을 파기한다(`Session.logout` 같은 로컬 서비스 호출 시 검사). IP 검사(`validateIpAddress`)는 OMV 소스에서 꺼져 있다. 한 주체가 받은 세션을 다른 주체에게 넘겨 쓰는 설계는 하지 않는다
- **클라이언트 IP**: 인증 로그의 IP는 `X-Forwarded-For` 등 프록시 헤더를 그대로 믿어 정한다(세션 검증에는 쓰지 않음). 백엔드는 브라우저 IP를 `X-Forwarded-For`로 넘긴다
- **세션 만료**: OMV 웹 관리 설정의 timeout 값(기본 5분)을 따른다. 인증된 호출마다 마지막 접근 시각이 갱신되며, `options.updatelastaccess=false`면 갱신하지 않는다
- **세션 확인용 RPC**: `WebGui.getAutoLogoutSettings`(8.2.0부터 있다. 아래 "OMV 8 버전별 차이"). 엔진 RPC라 인증된 세션이 필요하고 역할 제한이 없으며, 자동 로그아웃 시간(`timeout`, 분, 0이면 꺼짐)을 돌려준다. OMV workbench도 이 값으로 브라우저에서 자동 로그아웃 타이머를 돈다. `System.noop`은 admin 전용이다
- **로그인 실패 제한**: 비밀번호 실패에는 OMV 쪽 지연이 없다(2차 인증 실패만 제한, 429). 그래서 백엔드가 자체 제한을 둔다
- **새 브라우저 로그인 메일**: 로그인 성공 시 `OPENMEDIAVAULT-LOGIN-*` 쿠키가 없으면 "새 브라우저 로그인" 알림 메일을 보낸다. 백엔드가 이 쿠키(비밀 아님)를 사용자별로 메모리에 보관해 다음 로그인에 실어 보내므로, 백엔드 재시작 후 첫 로그인에만 메일이 간다
- **권한**: 로그인 응답의 `permissions.role`에 admin 포함 여부가 들어 있다. 관리자가 아니면 설정 관련 창과 버튼을 숨긴다
- **workbench의 탭별 로그인**: OMV workbench는 사용자 이름과 권한을 sessionStorage에 저장하고 그것으로 로그인 여부를 판단해서, 새 탭마다 다시 로그인해야 한다
- **대시보드 로그인을 OMV 탭에 넘길 수 없다**: Session 서비스에는 `login`·`verify`·`logout`만 있고 토큰·SSO 같은 비밀번호 없는 로그인이 없다. 백엔드의 OMV 세션은 고정 User-Agent에 묶여 브라우저에 넘기면 파기되고(대시보드도 로그아웃된다), 쿠키가 있어도 새 탭은 sessionStorage가 비어 로그인 화면이 뜬다. 그래서 OMV 워크벤치 링크 앱은 이름 붙은 탭 하나를 다시 써서 그 탭의 로그인을 유지한다(OMV 8.5 소스 기준)
- **보안 헤더**: 기본값이 `X-Frame-Options: SAMEORIGIN`, CSP `default-src 'self'`. OMV 화면을 iframe에 넣는 건 같은 출처에서만 가능하다. 보안 헤더를 끄는 방식은 쓰지 않는다

## OMV 8 버전별 차이

OMV 8의 모든 버전을 지원한다. OMV 패키지(`packages.openmediavault.org/public/pool/main/o/openmediavault/`)를 버전별로 풀어 온마루가 쓰는 RPC를 비교했고(2026-10), 아래만 다르다. 나머지 메서드는 8.0부터 의미가 같고 set 스키마(`rpc.*`)도 같다. 새 RPC를 쓰면 옛 버전 소스에도 있는지 같은 방법으로 확인한다. `omvfake.Version`(`SetVersion`, 개발 서버는 `FAKEOMV_VERSION`)이 이 경계를 흉내 낸다

- **`Session.login` 응답**: `status`와 2차 인증(`Session.verify`)은 8.5.0(Issue #2235)에서 생겼다. 8.4 이하는 `{"authenticated": true, "username", "permissions", "sessionid"}`이고 실패는 똑같이 HTTP 400이다. 백엔드는 `status`가 없고 `authenticated`가 true이면 `authenticated` 상태로 바꿔 같은 흐름을 탄다(`omv.loginResult`). 8.5의 verify 응답에도 `authenticated`가 들어 있을 수 있으므로 옛 형식은 `status`가 없는 것으로 판단한다
- **8.5.0-1의 `Session.authenticate`**: 이 한 릴리스만 `login`을 `authenticate`로 바꿨고 8.5.0-2에서 되돌렸다. 지원하지 않는다
- **`ShareMgmt.enumerateSharedFolders`**(파일 탐색기의 공유 폴더 스냅숏, 관리자 전용): 항목의 `absdirpath`와 `status`(`OK`, `ENOENT`, `ENOTDIR`, `ENOENT_DB_MNTENT`)는 8.1.0에서 생겼다. 8.0은 `mntent.dir` + `reldirpath`로 경로를 만들고 폴더가 있는지는 모른다(`files.ParseShares`). 항목의 공유 폴더 권한(`privileges.privilege[]{type, name, perms}`, 파일 탐색기의 공유 폴더 권한 따르기)은 8.0부터 같다. 워크벤치의 ACL 화면 라우트(`storage/shared-folders/acl/<uuid>`)는 8.0부터 같다
- **`WebGui.getAutoLogoutSettings`**: 8.2.0에서 생겼다. 8.0·8.1에서는 HTTP 404(없는 메서드, `rpc/rpc.inc`)라 `Probe`가 세션은 살아 있고 timeout은 모른다고 본다. 이때 브라우저에 남은 시간을 주지 않고 만료는 확인 주기의 Probe(401)로 안다

## 인증 설계

- 로그인 화면은 자체 디자인하고, 인증은 백엔드가 `Session.login`(필요하면 `Session.verify`)을 호출해 위임한다. 계정·비밀번호를 따로 저장하지 않는다
- 로그인 성공 시 대시보드 자체 세션 쿠키(`maru_session`, HttpOnly, SameSite=Strict, Path는 `-cookie-path`, 기본 `/`)를 발급한다. 2차 인증 대기 중에는 `maru_pending`. Secure는 HTTPS로 들어온 요청(`X-Forwarded-Proto: https`)에만 붙인다(평문 HTTP 개발 서버에서는 브라우저가 Secure 쿠키를 저장하지 않으므로). 세션 저장소는 메모리이며 백엔드 재시작 시 모두 재로그인한다
- 백엔드의 OMV 클라이언트는 세션 ID를 쿠키가 아닌 `X-OpenMediaVault-SessionID` 헤더로 보내고, 응답 `Set-Cookie`로 바뀐 ID를 갱신한다
- `/api/*`는 대시보드 세션이 없으면 401을 반환한다(`/api/health`, 로그인 라우트, 로그인 화면이 쓰는 `GET /api/site`와 `GET /api/site/images/login-background` 제외). 본문이 있는 요청(POST, PUT)은 `Content-Type: application/json`만 받는다(CSRF 보조 장치)
  - 예외: 이미지 올리기(`PUT /api/preferences/wallpaper-image`, `…/avatar-image`, `PUT /api/site/images/{name}`)는 원시 이미지 본문(`image/png|jpeg|webp`)만 받는다. HTML 폼은 이 형식을 보낼 수 없고 다른 출처의 fetch는 preflight에서 막히므로 같은 보조 장치가 된다. multipart는 받지 않는다
  - 예외: 파일 탐색기의 올리기(`PUT /api/files/upload`)는 원시 본문(`application/octet-stream`)만 받는다. 이 형식도 HTML 폼이 보낼 수 없고 다른 출처의 fetch는 preflight에서 막힌다. 내려받기(`GET /api/files/download`)는 같은 사이트 이동(`<a download>`)이라 SameSite=Strict 쿠키가 간다
  - 공개 로그인 배경은 대시보드 주소에 닿는 누구나 받을 수 있다(로그인 화면에 보이는 그림이다). 설정 앱이 관리자에게 이 점을 알린다
- 로그인 상태를 localStorage·sessionStorage에 저장하지 않는다. 앱 시작 시 `/api/session`으로 상태를 복원한다(workbench처럼 탭마다 로그인하지 않게 하기 위함)
  - 패스키로 로그인의 브라우저 항목(localStorage `maru.passkeyLogin`)은 로그인 상태가 아니다. 서버 share와 패스키 PRF가 함께 있어야 풀리는 비밀번호 암호문이며, 있어도 세션은 생기지 않는다(`.claude/rules/security.md`의 "패스키로 로그인")
- OMV가 중계 호출에 401을 반환하면 해당 대시보드 세션을 지우고 브라우저를 로그인 화면으로 보낸다
- 로그인 시도 제한: 실패 응답 전 1초 지연, IP·사용자 이름별로 15분 안에 5번째 실패부터 30초에서 두 배씩 최대 15분 잠금(429, `Retry-After`), 한 IP의 동시 시도는 2개까지
  - 사용자 이름별 잠금은 IP와 무관하므로 누구나 남의 계정을 잠글 수 있다. 그래서 인증을 끝까지 마친(2차 인증 포함) 브라우저에 신뢰 기기 쿠키(`maru_device`, HttpOnly, SameSite=Strict, 30일, 로그인할 때마다 새 토큰)를 주고, 같은 사용자 이름으로 이 토큰을 보낸 시도는 사용자 이름별 잠금을 건너뛴다. IP별 제한은 그대로 받고, 토큰으로 5번 틀리면 토큰을 폐기한다. 로그아웃해도 지우지 않는다. 토큰은 메모리에만 두므로 백엔드를 재시작하면 모두 사라진다(`auth.Limiter.Trust`)
  - 실패 기록은 1만 개까지 둔다. 넘치면 기간이 지난 기록, 잠기지 않은 기록, 잠긴 기록 순으로 오래된 것부터 지운다
- 로그에는 사용자 이름·IP·결과만 남긴다. 비밀번호, 대시보드·OMV 세션 ID, OMV 오류의 `trace`는 남기지 않고 브라우저에도 넘기지 않는다
- 로그아웃하거나 다른 사용자가 로그인하면 창을 모두 닫는다. 세션 만료 후 같은 사용자가 다시 로그인하면 창 배치를 남긴다
- 권한 도우미(`-helper-socket`)가 있으면 OMV 로그인에 성공한 뒤 같은 비밀번호로 도우미 티켓을 받는다(도우미가 `unix_chkpwd`로 직접 확인, 5초 제한). 실패해도 로그인은 진행하고 도우미 기능만 없다. 2차 인증이면 대기 티켓을 `auth.Pending`에 두고, `Session.verify`가 성공하면 `activate`해 세션으로 옮긴다(`Store.ClaimPending`). 세션·대기가 지워지면(로그아웃, 만료, 밀려남, 유휴 정리, 대기 만료) 저장소의 삭제 훅이 티켓을 폐기한다
  - root 권한 상승(`POST /api/helper/elevation`, 관리자)은 비밀번호를 다시 받아 도우미가 직접 확인하고, 그 대시보드 세션에만 5분 붙는다. 비밀번호는 저장·기록하지 않는다. 도우미의 실패 제한이 따로 있다(사용자별 5분 안 3번 실패부터 잠금, 429 `Retry-After`). 상세는 `.claude/docs/helper.md`
- 보안 강화 모드: 세션마다 보증 수준(`Session.Assurance`)이 있고 `SessionInfo.assurance`로 알린다. OMV 2차 인증(`Session.verify`)을 마친 로그인은 `twoFactor`로 시작한다. 2차 인증을 요구하는 기능이 있고 사용자에게 수단이 있으면 로그인 응답에만 `stepUp`을 담는다(세션은 이미 만들어졌고 건너뛸 수 있다). 비밀번호 재확인은 `Session.login` → 바로 `Logout`이고 로그인 실패 제한을 함께 쓴다. 상세는 `.claude/rules/security.md`

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
