---
paths:
  - "backend/internal/security/**"
  - "backend/internal/server/security*.go"
  - "backend/internal/server/passkey*.go"
  - "backend/internal/prefs/security*.go"
  - "frontend/src/security/**"
  - "frontend/src/settings/Security*.tsx"
---

# 보안 강화 모드

보안에 민감한 기능(웹 터미널, root 권한 상승)을 확인을 더 거친 세션에서만 쓰게 한다. 막는 것은 세션 탈취(열어 둔 브라우저, 훔친 쿠키)와 비밀번호 유출이고, onmaru 자체가 뚫린 경우는 막지 못한다. 사용자 설명은 `README.md`의 "보안 강화 모드". 로드맵의 "패스키로 비밀번호 없이 로그인"(PRF)은 아직 없다.

## 설계

- **확인은 onmaru가 한다.** 권한 도우미에 넣지 않고 도우미의 위협 모델도 바꾸지 않는다(`.claude/docs/helper.md`)
- **보증 수준은 세션 속성이다**(`auth.Session.Assurance`: `login` < `password` < `twoFactor`, `Raise`로 올라가기만 한다). 따로 만료가 없고 세션이 끝나면 함께 끝난다(OMV 자동 로그아웃이 꺼져 있으면 로그아웃까지 남으므로 정책 페이지가 경고한다)
  - `twoFactor`: OMV `Session.verify` 통과(`verify` 핸들러), 패스키·TOTP·복구 코드 확인, 수단을 새로 등록함(방금 소유를 증명)
  - `password`: 세션 중 비밀번호 재확인. 로그인할 때 넣은 비밀번호는 치지 않는다(그래야 세션 탈취를 막는다)
- **정책**은 기능별 `off`/`password`/`twoFactor`이고 `site.json`의 `security`(`api.SecurityPolicy`)에 둔다. 처리기는 메모리(`handler.policy`)에 올려 쓰고 저장할 때 함께 바꾼다. 모르는 값(손으로 고친 파일)은 `twoFactor`로 본다. 공개 `GET /api/site`에는 넣지 않는다
  - **정책 변경도 막는다**(`setSecurityPolicy`): 바꾸는 기능마다 이전·새 단계 중 높은 쪽을 그 관리자 세션이 거쳤어야 한다. 아니면 403 `security_required`(`ErrorResponse.Required`). 빼앗은 관리자 세션이 정책을 내리고 기능을 여는 것을 막는다
  - 정책을 올리면(`enforcePolicy`) 조건을 채우지 못하는 세션의 터미널을 `security_required`로 닫고(`terminal.Manager.CloseSession`) root 권한 상승을 끝낸다(도우미 `Drop`)
  - 새 대상(파일 탐색기 등)은 `api.SecurityPolicy`에 필드를 더하고 `normalizePolicy`·변경 가드의 목록·`enforcePolicy`·프론트 목록(`SecurityPolicyPage.tsx`의 `TARGETS`, `SecurityPage.tsx`의 `TARGET_KEYS`)에 넣는다
- **적용**: 웹 터미널은 WebSocket을 받은 뒤 `status closed reason=security_required`로 알린다(브라우저는 핸드셰이크의 HTTP 상태를 읽지 못한다). root 상승(`elevate`)은 비밀번호를 묻기 전에 403 `security_required`
- **로그인 단계**: 2차 인증을 요구하는 기능이 있고 사용자에게 수단이 있으면 로그인 응답에만 `stepUp`(쓸 수 있는 수단)을 담는다. 세션은 이미 만들어졌고 건너뛰면 일반 세션이다. `SessionInfo`에는 넣지 않는다(세션 복원이 다시 묻지 않게)

## 확인 수단

- **비밀번호 재확인**(`confirmPassword`): `OMV.Login`으로 새 OMV 세션을 받아 바로 `Logout`한다. 도우미가 없어도, LDAP 계정도 된다. `challengeRequired`(OMV 8.5 2차 인증 계정)도 비밀번호가 맞은 것이다. 실패는 로그인 제한기(`h.Limiter`, 신뢰 기기 토큰 포함)로 센다. 도우미 `Elevate`는 쓰지 않는다(관리자 전용이고 실제로 root를 붙인다)
- **2차 수단**(`checkFactor`)의 실패는 따로 둔 `securityLimiter`(IP·사용자 이름)로 세고, 틀린 값은 `FailDelay` 뒤 400 `factor_failed`
- **TOTP**: RFC 6238 SHA-1·6자리·30초, ±1 단계, `LastStep` 이하는 받지 않는다(같은 코드 재사용 금지). 설정 중 비밀은 세션별 메모리(`totpSetup`, 10분)에만 두고 코드로 확인한 뒤 저장한다. `otpauth://`의 계정 이름은 `<사용자>@<호스트 이름>`
- **복구 코드**: 10개, base32 10자(50비트), SHA-256 해시로만 저장, 한 번 쓰면 지운다. 첫 수단을 등록할 때 만들어 응답에만 평문을 담는다. 수단이 모두 없어지면 복구 코드도 지운다(`security.Normalize`)
- **패스키(WebAuthn)**: CBOR 파서 없이 표준 라이브러리만 쓴다
  - 등록은 증명 `none`이고 브라우저의 `getPublicKey()`(SPKI DER), `getPublicKeyAlgorithm()`, `getAuthenticatorData()`를 쓴다. 공개 키가 없으면(브라우저가 알고리즘을 모름) 거부한다. 인증기 데이터의 자격 증명 ID가 `rawId`와 같아야 한다. 증명이 없으므로 등록은 관리 창(비밀번호 + 기존 수단)으로 막는다
  - 확인은 clientDataJSON(type·challenge·origin, crossOrigin 거부), 인증기 데이터(rpIdHash, UP, **UV 필수**), 서명(authData‖sha256(clientDataJSON))을 본다. ES256·EdDSA·RS256(2048비트 이상)
  - challenge는 대시보드 세션과 목적(`assert`, `register`)에 묶인 1회용이다(`challenges`, 5분). 사용자 핸들(`Record.Handle`)은 무작위이고 응답의 userHandle이 있으면 같아야 한다. 서명 횟수가 늘지 않으면 기록만 남긴다(동기화 패스키는 늘 0)
  - RP는 `-public-origin`(`ONMARU_PUBLIC_ORIGIN`, `security.ParseOrigin`: `https://호스트[:포트]`, IP 거부)에서 정한다. 비우면 패스키를 숨긴다. 웹에서 덮어쓰지 않는다
- **관리 창**(`ManageUntil`, 5분): 수단 등록·삭제·복구 코드 다시 만들기 전에 비밀번호와(등록한 수단이 있으면) 그 수단을 함께 확인한다. 없으면 403 `manage_required`. 세션을 빼앗은 쪽이 자기 수단을 등록하지 못하게 한다
- **관리자 초기화**(`DELETE /api/security/users/{name}`): 관리자이면서 `twoFactor` 세션만 한다. 기록을 지우고 그 사용자의 대시보드 세션을 모두 끝낸다(`auth.Store.DeleteUser`, 삭제 훅이 도우미 티켓·터미널을 정리한다). 손상된 기록도 지운다

## 저장

- 타입(`security.Record`)은 `internal/security`에 둔다. TOTP 비밀·복구 해시가 들어 있으므로 `internal/api`(tygo 원본)에 두지 않고 응답·로그에 넣지 않는다
- `prefs.FileStore`가 `security/<인코딩한 이름>.json`(디렉터리 `0700`, 파일 `0600`, `version` 1)에 쓴다. **읽을 수 없는 파일은 오류로 돌려준다**(`ErrSecurityRecord`). 다른 설정처럼 `.corrupt`로 옮기고 빈 값으로 시작하면 2차 인증이 조용히 사라지기 때문이다. 그 사용자는 500 `security_unavailable`을 받고, 관리자 초기화나 root의 파일 삭제로 되살린다
- 감사: 확인 성공·실패, 등록·삭제, 정책 변경, 초기화를 `log.Printf("security: …")`로 남긴다(사용자, IP, 수단, 이름. 비밀 값·코드 없음)

## 프론트

- `src/security/`: `StepUpForm.tsx`(로그인 카드와 대화상자가 함께 쓰는 확인 폼, `variant`), `stepUpDialog.tsx`(`requestStepUp`, `requestManage`) + 내용 `StepUpBody.tsx`, `securityDialogs.tsx`(패스키·인증 앱 등록, 복구 코드 보기) + 내용 `PasskeyAdd.tsx`·`TOTPSetup.tsx`·`RecoveryCodes.tsx`, 순수 모듈 `messages.ts`(오류 문구, 수단 고르기, QR 경로)·`webauthn.ts`(base64url, 옵션 변환)
  - 여는 함수와 컴포넌트를 다른 파일에 둔다(oxlint `only-export-components`)
- 로그인 직후 단계는 인증 저장소의 `stepUp` 상태(`finishStepUp`)다. 보안에 민감한 기능은 `security_required`를 받으면 `requestStepUp`으로 확인하고 다시 시도한다(터미널은 닫힘 막대의 확인 단추, 상승은 `ElevationForm`이 같은 비밀번호로 다시 시도)
- 패스키는 `passkeysSupported()`(보안 컨텍스트 + `PublicKeyCredential`)일 때만 보인다. 개발 서버(평문 HTTP)에서는 TOTP·복구 코드만 시험할 수 있다
- 인증 앱 QR은 `uqr`의 `encode`로 모듈 배열을 받아 SVG path로 그린다(`qrPath`, innerHTML 없음). 등록 대화상자를 열 때 `import('uqr')`로 받고 vendor 청크에서 뺀다(`vite.config.ts`)

## 테스트

- Go: TOTP는 RFC 6238 SHA-1 벡터, WebAuthn은 테스트 안에서 합성한 인증기(P-256·ed25519·RSA 키로 authData·clientDataJSON·서명을 만든다). 서버 테스트(`security_test.go`, `passkey_test.go`)는 `envOptions{security, rp}`와 omvfake로 정책 가드, 로그인 단계, 터미널·상승 적용, 관리 창, 초기화, 손상된 기록을 시험한다
- 브라우저: TOTP·화면은 `make dev-fake`, 패스키는 HTTPS 도메인으로 연결한 테스트 VM에서 Playwright 가상 인증기(CDP `WebAuthn.addVirtualAuthenticator`, UV 켬)로 확인한다. 실제 기기 패스키는 사용자가 확인한다
