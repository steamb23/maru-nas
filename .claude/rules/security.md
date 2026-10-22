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

보안에 민감한 기능(웹 터미널, root 권한 상승, 파일 탐색기)을 확인을 더 거친 세션에서만 쓰게 한다. 막는 것은 세션 탈취(열어 둔 브라우저, 훔친 쿠키)와 비밀번호 유출이고, maru 자체가 뚫린 경우는 막지 못한다. 사용자 설명은 `docs/security.md`와 `docs/passkey-login.md`. 패스키만으로 로그인하는 기능은 아래 "패스키로 로그인"에 있다.

## 설계

- **확인은 maru가 한다.** 권한 도우미에 넣지 않고 도우미의 위협 모델도 바꾸지 않는다(`.claude/docs/helper.md`)
- **보증 수준은 세션 속성이다**(`auth.Session.Assurance`: `login` < `password` < `twoFactor`, `Raise`로 올라가기만 한다). 따로 만료가 없고 세션이 끝나면 함께 끝난다(OMV 자동 로그아웃이 꺼져 있으면 로그아웃까지 남으므로 정책 페이지가 경고한다)
  - `twoFactor`: OMV `Session.verify` 통과(`verify` 핸들러), 패스키·TOTP·복구 코드 확인, 수단을 새로 등록함(방금 소유를 증명)
  - `password`: 세션 중 비밀번호 재확인. 로그인할 때 넣은 비밀번호는 치지 않는다(그래야 세션 탈취를 막는다)
- **정책**은 기능별 `off`/`password`/`twoFactor`이고 `site.json`의 `security`(`api.SecurityPolicy`)에 둔다. 처리기는 메모리(`handler.policy`)에 올려 쓰고 저장할 때 함께 바꾼다. 모르는 값(손으로 고친 파일)은 `twoFactor`로 본다. 공개 `GET /api/site`에는 넣지 않는다
  - **정책 변경도 막는다**(`setSecurityPolicy`): 바꾸는 기능마다 이전·새 단계 중 높은 쪽을 그 관리자 세션이 거쳤어야 한다. 아니면 403 `security_required`(`ErrorResponse.Required`). 빼앗은 관리자 세션이 정책을 내리고 기능을 여는 것을 막는다
  - 정책을 올리면(`enforcePolicy`) 조건을 채우지 못하는 세션의 터미널을 `security_required`로 닫고(`terminal.Manager.CloseSession`) root 권한 상승을 끝낸다(도우미 `Drop`)
  - 새 대상은 `api.SecurityPolicy`에 필드를 더하고 `normalizePolicy`·변경 가드의 목록·"모두 꺼짐" 조건·`stepUpOffer`·`enforcePolicy`(필요하면)·프론트 목록(`SecurityPolicyPage.tsx`의 `TARGETS`, `SecurityPage.tsx`의 `TARGET_KEYS`)·문구(`security.targets.*`, `settings.securityPolicy.*Desc`)에 넣는다
- **적용**: 웹 터미널은 WebSocket을 받은 뒤 `status closed reason=security_required`로 알린다(브라우저는 핸드셰이크의 HTTP 상태를 읽지 못한다). root 상승(`elevate`)은 비밀번호를 묻기 전에 403 `security_required`. 파일 탐색기는 `/api/files/*` 전부(목록·내려받기 포함)가 403 `security_required`이고, 요청마다 보므로 정책을 올려도 따로 닫지 않는다(이미 시작한 전송은 끝까지 간다). 창은 안내와 확인 단추를 두고 `requestStepUp({target: 'files'})` 뒤 다시 연다
  - **적용 범위**(`api.SecurityPolicy.FilesScope`, `all`·`writes`, 없거나 모르는 값은 `all`): `writes`면 읽기 라우트(`requireFiles`)는 열고 쓰기 라우트(`requireFilesWrite`)만 막는다(`.claude/rules/files.md`). 범위를 바꾸려면 그 세션이 이전·새 `Files` 단계 중 높은 쪽을 거쳤어야 한다(쓰기에만 적용하면 목록·내려받기가 열린다). "모두 꺼짐"으로 저장을 지우는 조건에도 `all`이 들어간다. 보안 정책 페이지는 `Files`가 꺼짐이 아닐 때 범위 행을 보이고, 보안 페이지는 "(쓰기·삭제에만)"을 붙인다. 쓰기 요청이 막히면 창이 `files/stepUp.ts`의 `withStepUp`으로 확인한 뒤 다시 한다
- **로그인 단계**: 2차 인증을 요구하는 기능이 있고 사용자에게 수단이 있으면 로그인 응답에만 `stepUp`(쓸 수 있는 수단)을 담는다. 세션은 이미 만들어졌고 건너뛰면 일반 세션이다. `SessionInfo`에는 넣지 않는다(세션 복원이 다시 묻지 않게)

## 확인 수단

- **비밀번호 재확인**(`confirmPassword`): `OMV.Login`으로 새 OMV 세션을 받아 바로 `Logout`한다. 도우미가 없어도, LDAP 계정도 된다. `challengeRequired`(OMV 8.5 2차 인증 계정)도 비밀번호가 맞은 것이다. 실패는 로그인 제한기(`h.Limiter`, 신뢰 기기 토큰 포함)로 센다. 도우미 `Elevate`는 쓰지 않는다(관리자 전용이고 실제로 root를 붙인다)
- **2차 수단**(`checkFactor`)의 실패는 따로 둔 `securityLimiter`(IP·사용자 이름)로 세고, 틀린 값은 `FailDelay` 뒤 400 `factor_failed`
- **TOTP**: RFC 6238 SHA-1·6자리·30초, ±1 단계, `LastStep` 이하는 받지 않는다(같은 코드 재사용 금지). 설정 중 비밀은 세션별 메모리(`totpSetup`, 10분)에만 두고 코드로 확인한 뒤 저장한다. `otpauth://`의 계정 이름은 `<사용자>@<호스트 이름>`
- **복구 코드**: 10개, base32 10자(50비트), SHA-256 해시로만 저장, 한 번 쓰면 지운다. 첫 수단을 등록할 때 만들어 응답에만 평문을 담는다. 수단이 모두 없어지면 복구 코드도 지운다(`security.Normalize`)
- **패스키(WebAuthn)**: CBOR 파서 없이 표준 라이브러리만 쓴다
  - 등록은 증명 `none`이고 브라우저의 `getPublicKey()`(SPKI DER), `getPublicKeyAlgorithm()`, `getAuthenticatorData()`를 쓴다. 공개 키가 없으면(브라우저가 알고리즘을 모름) 거부한다. 인증기 데이터의 자격 증명 ID가 `rawId`와 같아야 한다. 증명이 없으므로 등록은 관리 창(비밀번호 + 기존 수단)으로 막는다
  - 등록할 때 `extensions.prf`를 요청한다(`webauthn.ts`의 `creationOptions`, 아래 "패스키로 로그인"). 보안 키의 CTAP2 hmac-secret은 만들 때 요청해야 생기므로, 요청하지 않고 만든 패스키는 확인할 때 PRF 결과를 주지 않는다
  - 확인은 clientDataJSON(type·challenge·origin, crossOrigin 거부), 인증기 데이터(rpIdHash, UP, **UV 필수**), 서명(authData‖sha256(clientDataJSON))을 본다. ES256·EdDSA·RS256(2048비트 이상). UV는 서명까지 맞은 뒤에 보고, UV만 없으면 `security.ErrUserNotVerified`(`ErrVerify`를 감싼다)다. 플래그는 서명한 인증기 데이터 안에 있어 바꿀 수 없다
  - **등록은 두 단계다.** `POST /api/security/passkeys`는 등록 값을 확인하되 저장하지 않고 세션의 대기(`pendingPasskeys`, 5분, 세션 ID)에 둔 뒤, 그 자격 증명 하나만 허용하는 시험 확인 옵션을 준다. `POST /api/security/passkeys/confirm`이 대기의 패스키로 확인(UV 필수)해 통과해야 저장한다(중복·개수·핸들 검사, 첫 수단의 복구 코드, `twoFactor` 올리기도 이때). 두 요청 모두 관리 창이 필요하고 대기는 한 번만 쓴다. 등록할 때만 UV를 켜고 확인할 때는 켜 주지 않는 저장소(잠금이 풀리면 확인을 건너뛰는 비밀번호 관리자, Windows에서 기기 저장을 끄고 Microsoft Password Manager에만 둔 패스키)를 등록 응답의 플래그로는 걸러 낼 수 없기 때문이다
    - 시험 확인에서 UV만 없으면 400 `passkey_uv_unsupported`이고 `securityLimiter`에 세지 않는다(`Neutral`, 값을 맞히는 시도가 아니다). 그 밖의 실패는 지금처럼 `Failure`·`factor_failed`. 세션 중 확인(`checkFactor`)의 UV 실패는 그대로 `factor_failed`·`Failure`다
    - 프론트(`PasskeyAdd.tsx`)는 `create()` 뒤 바로 `get()`을 부른다. `get()`이 취소되면 대기가 남으므로 "다시 확인"으로 확인만 다시 한다. 백엔드가 거절해 저장하지 않은 것이 확실할 때(400·403·429, `passkeyNotSaved`)와 시험 확인을 보내기 전에 닫았을 때 `PublicKeyCredential.signalUnknownCredential`로 저장소에서 지우게 한다(`forgetPasskey`, 없으면 직접 지우라고 안내). 409(이미 등록됐을 수 있음)·연결 실패는 저장 여부를 모르므로 지우게 하지 않는다
  - 진단: 패스키 확인·등록 실패 로그에 인증기 플래그(`UP=… UV=… BE=… BS=…`, `security.AuthFlags`)를, 등록 로그에 AAGUID(`security.AAGUID`)를 남긴다. AAGUID는 증명 none에서 브라우저가 0으로 지울 수 있어 믿지 않고 로그에만 쓴다(차단 목록으로 쓰지 않는다)
  - 등록할 때의 BE 플래그를 `Passkey.BackupEligible`에 두고 보안 페이지 목록에 "동기화됨"으로 표시한다. 막지 않는다(대부분의 패스키가 동기화 저장소다)
  - challenge는 대시보드 세션과 목적(`assert`, `register`)에 묶인 1회용이다(`challenges`, 5분). 사용자 핸들(`Record.Handle`)은 무작위이고 응답의 userHandle이 있으면 같아야 한다. 서명 횟수가 늘지 않으면 기록만 남긴다(동기화 패스키는 늘 0)
  - RP는 `-public-origin`(`MARU_PUBLIC_ORIGIN`, `security.ParseOrigin`: `https://호스트[:포트]`, IP 거부)에서 정한다. 비우면 패스키를 숨긴다. 웹에서 덮어쓰지 않는다
- **관리 창**(`ManageUntil`, 5분): 수단 등록·삭제·복구 코드 다시 만들기 전에 비밀번호와(등록한 수단이 있으면) 그 수단을 함께 확인한다. 없으면 403 `manage_required`. 세션을 빼앗은 쪽이 자기 수단을 등록하지 못하게 한다
- **기기 추가 코드**(`POST /api/security/device-code`): 동기화되지 않는 패스키(Windows Hello 등)만 등록한 사용자는 새 기기에서 관리 창을 열 수단이 없다(복구 코드는 하나를 써 버린다). 그래서 수단이 있는 기기가 1회용 코드를 만들어 주고, 새 기기의 관리 확인이 등록한 수단 대신 받는다
  - 관리 창이 열린 세션만(아니면 403 `manage_required`), 수단을 등록한 사용자만(아니면 400) 만든다. 공개 주소(RP)와는 묶지 않는다(화면이 패스키를 쓸 수 없으면 행을 막는다)
  - 대문자 base32 8자(`XXXX-XXXX`, 40비트, 소문자 `xxxxx-xxxxx`인 복구 코드와 모양이 다르다). SHA-256 해시만 메모리(`deviceCodes`, 사용자 이름 키, 10분)에 두고 평문은 발급 응답에만 담는다. 다시 만들면 이전 코드는 버리고, 관리자 초기화도 지운다
  - 관리 확인(`SecurityManageRequest.deviceCode`, `Factor`와 함께 보내면 400)에서 비밀번호와 함께 확인한다. 맞을 때만 지운다(`Ephemeral.TakeIf`, 같은 코드를 동시에 두 번 쓰지 못한다). 틀린 값은 지우지 않는다(비밀번호를 가진 쪽이 아무 값이나 보내 정상 사용자의 코드를 없애지 못하게). 실패는 `securityLimiter`·`FailDelay`·400 `factor_failed`
  - 2차 수단(`Factor*`)이 아니다. `verify`에서 받지 않고 `methods`에도 넣지 않는다. 관리 창만 열고 세션은 `password`까지만 올린다(이 세션에서 수단 소유를 증명하지 않았다). 새 패스키를 등록하면 그때 `twoFactor`가 된다
  - 이름과 달리 관리 창 전체(수단 삭제, 복구 코드 다시 만들기 포함)를 연다. 복구 코드와 같은 성질이고 코드를 만든 세션이 이미 관리 창을 거쳤으므로 범위를 나누지 않는다
  - 로그에는 발급·사용·실패의 사용자와 IP만 남긴다
- 확인(`verify`)·관리 확인(`manage`)이 성공하면 OMV 세션도 연장한다(`touchSession`, 활동 신호와 같다). 로그인 직후 단계에서는 활동 신호가 돌지 않기 때문이다. 그 단계에서 세션이 끝나면 프론트의 `expire`가 로그인 화면으로 돌린다
- 첫 수단은 비밀번호만으로 등록된다(관리 창에 확인할 기존 수단이 없다). 그래서 비밀번호 유출은 수단을 이미 등록한 계정만 막는다(`docs/security.md`의 "설계와 한계")
- **관리자 초기화**(`DELETE /api/security/users/{name}`): 관리자이면서 `twoFactor` 세션만 한다. 기록을 지우고 그 사용자의 대시보드 세션을 모두 끝낸다(`auth.Store.DeleteUser`, 삭제 훅이 도우미 티켓·터미널을 정리한다). 손상된 기록도 지운다

## 패스키로 로그인

패스키 확인만으로 로그인한다. OMV `Session.login`과 도우미 티켓에는 비밀번호가 필요하고 OMV에는 토큰·SSO가 없으므로, 브라우저가 비밀번호를 암호화해 두었다가 패스키 확인 뒤 풀어 평소처럼 보낸다.

- **분할 키**: 키 = HKDF-SHA256(ikm = PRF 결과 ‖ share, salt = 등록 ID, info `"maru passkey login v1"`) → AES-GCM-256(추출 불가, 저장하지 않음). 암호문은 브라우저 localStorage(`maru.passkeyLogin`)에만, share(등록마다 서버가 만든 32바이트)는 서버 기록(`Record.PasskeyLogins`)에만 둔다
  - PRF만으로 키를 만들면 복호화에 서버가 끼지 않는다. 브라우저 프로필 사본과 인증기(+UV)를 가진 쪽이 maru 모르게(로그·시도 제한 없이) OMV 비밀번호를 꺼낼 수 있고, 패스키를 지워도 그 사본은 막지 못한다. share는 패스키 서명(UV 필수)을 확인한 뒤에만 내주므로 복호화마다 서버를 거친다
  - 상태 디렉터리가 새도 share만으로는 풀 수 없고, 브라우저 항목이 새도 share와 PRF가 없으면 풀 수 없다. 동기화 패스키라도 켠 브라우저에서만 된다
  - PRF 입력(salt)은 등록마다 브라우저가 만들어 항목에 둔다. 확인은 `allowCredentials = [그 자격 증명]`이라 검색 가능한 자격 증명이 아니어도 되고, 사용자 이름 없이 찾는 단계가 없다
  - AAD = rpId ‖ 사용자 이름 ‖ 자격 증명 ID ‖ 등록 ID. 다른 계정·패스키·등록의 share로 풀 수 없다. 평문은 1KiB까지
- **등록**(`POST /api/security/passkey-logins`): 비밀번호(`confirmPassword`)와 그 패스키(`checkFactor`, PRF를 함께 요청한 `get()`)를 확인한 뒤 서버가 ID·share를 만들어 응답에만 담는다. 브라우저가 같은 `get()`의 PRF 결과로 암호화해 저장한다. PRF 결과가 없으면 요청을 보내지 않는다. 세션은 `twoFactor`로 오른다
  - PRF는 패스키를 등록할 때 `extensions.prf`로 함께 요청해 둔다(보안 키의 hmac-secret은 만들 때 요청해야 생긴다). 그 전에 등록한 패스키는 PRF 결과를 주지 않을 수 있어 PRF 미지원 문구가 다시 등록하라고 안내한다
  - 항목을 저장하면 그 사용자를 "다시 묻지 않기"(`dismissed`)에서 뺀다(`rememberPasskeyLogin`)
  - 비밀번호 로그인 직후 로그인 화면이 권한다(`LoginResponse.passkeyLoginOffer`, 로그인 응답에만). 비밀번호는 LoginScreen 상태에만 있다. 보안 페이지에서도 켠다
  - 해제(`DELETE /api/security/passkey-logins/{id}`)는 관리 창을 요구하지 않는다. 기능을 줄이기만 하고 수단을 지우지 않기 때문이다. 패스키 삭제·관리자 초기화는 그 등록도 지운다(`security.Normalize`)
- **로그인**(인증 없음, 의식 쿠키 `maru_passkey_login`, HttpOnly, SameSite=Strict)
  1. `POST /api/login/passkey/options {username, enrollment}`: 없는 사용자·등록·읽을 수 없는 기록은 모두 400 `passkey_login_unknown`(`Neutral`), 만료면 400 `passkey_login_expired`(등록 삭제). 유효한 등록 ID가 있어야 상태가 생긴다
  2. `POST /api/login/passkey {passkey}`: `securityLimiter`(IP·사용자 이름). 그 등록의 자격 증명·서명·UV를 확인하면 `{share, nextShare}`(아래 "회전")
  3. `POST /api/login/passkey/finish {password, rotated}`: `h.Limiter`로 센다(브라우저가 보낸 비밀번호라 복호화 결과인지 서버가 알 수 없다). 성공하면 일반 로그인과 같은 마무리를 `twoFactor`로 한다(`completeLogin`. OMV 2차 인증이면 대기 → `verify`). OMV가 400이면 비밀번호를 바꾼 것이므로 등록을 지우고 400 `passkey_login_stale`. 브라우저가 풀지 못하면 `{discard: true}`로 등록을 지운다
  - 브라우저는 `passkey_login_unknown`·`passkey_login_expired`·`passkey_login_stale`이면 항목을 지운다. 1단계 뒤 패스키가 PRF 결과를 주지 않아도 항목을 지운다(그 자격 증명 하나만 허용하므로 다시 해도 풀 수 없다). share를 받기 전이라 서버 등록은 지우지 못하고 만료나 보안 페이지의 해제로 정리된다
- **회전**: 패스키로 로그인이 성공할 때마다 share를 바꿔, 복사된 브라우저 프로필 사본(옛 share용 암호문)을 다음 정상 로그인 뒤로 쓸 수 없게 한다. 두 단계로 확정한다
  - 풀림 단계가 지금 share와 함께 다음 share(`PasskeyLogin.NextShare`)를 준다. 대기가 없을 때만 새로 만들어 저장하고, 있으면 그 값을 다시 준다(확정 전에는 바꾸지 않는다)
  - 브라우저는 지금 share로 풀고, 안 되면 다음 share로 푼다(직전 회전의 확정 응답만 사라진 경우). 풀어 낸 비밀번호를 다음 share로 다시 암호화해(새 IV) **finish를 보내기 전에** 항목을 바꾸고, finish에 `rotated: true`를 보낸다. 저장하지 못했으면 `rotated: false`(`unlockPasskeyLogin`이 `{ password, rotated }`를 돌려준다). 서버 기록은 `PasskeyLogin.PrepareRotation`·`Rotate`로 고치고, 로그에는 `rotated=`만 남긴다
  - finish에서 OMV가 비밀번호를 받으면(OMV 2차 인증 대기여도) `rotated`일 때 다음 share를 지금 share로 확정하고 대기를 지운다. 그래서 어느 응답이 사라져도 브라우저 항목은 늘 지금 share나 다음 share 중 하나로 풀린다
  - 두 share 모두로 풀리지 않는 항목은 정상 브라우저에서는 생기지 않으므로 회전 전에 복사된 사본으로 본다. 지금처럼 discard로 서버 등록을 지우고 로그에 사본일 수 있다고 남긴다(정상 브라우저도 다시 켜야 하지만 사본이 나도는 등록을 바로 막는다)
  - 만료 시각은 회전해도 늘리지 않는다
- **만료**: 관리자가 정책의 `passkeyLoginDays`(7·30·90·180·365, 기본 30, 모르는 값은 7)로 정한다. 등록할 때 `ExpiresAt`을 두고, 실제 만료는 `min(ExpiresAt, Created + 지금 기간)`이다(줄이면 기존 등록에도 바로, 늘리면 새 등록부터). 바꾸려면 `password` 이상의 세션이어야 한다. 만료된 등록은 상태 응답에서 빼고, 쓰는 김에 지운다
- 로그에는 사용자, IP, 패스키 이름, 등록 ID 앞 6자만 남긴다. share·암호문·PRF 결과는 남기지 않는다

## 저장

- 타입(`security.Record`)은 `internal/security`에 둔다. TOTP 비밀·복구 해시가 들어 있으므로 `internal/api`(tygo 원본)에 두지 않고 응답·로그에 넣지 않는다
- `prefs.FileStore`가 `security/<인코딩한 이름>.json`(디렉터리 `0700`, 파일 `0600`, `version` 1)에 쓴다. **읽을 수 없는 파일은 오류로 돌려준다**(`ErrSecurityRecord`). 다른 설정처럼 `.corrupt`로 옮기고 빈 값으로 시작하면 2차 인증이 조용히 사라지기 때문이다. 그 사용자는 500 `security_unavailable`을 받고, 관리자 초기화나 root의 파일 삭제로 되살린다
- 감사: 확인 성공·실패, 등록·삭제, 정책 변경, 초기화를 `log.Printf("security: …")`로 남긴다(사용자, IP, 수단, 이름. 비밀 값·코드 없음)

## 프론트

- `src/security/`: `StepUpForm.tsx`(로그인 카드와 대화상자가 함께 쓰는 확인 폼, `variant`), `stepUpDialog.tsx`(`requestStepUp`, `requestManage`) + 내용 `StepUpBody.tsx`, `securityDialogs.tsx`(패스키·인증 앱 등록, 복구 코드 보기) + 내용 `PasskeyAdd.tsx`·`TOTPSetup.tsx`·`RecoveryCodes.tsx`, 순수 모듈 `messages.ts`(오류 문구, 수단 고르기, QR 경로)·`webauthn.ts`(base64url, 옵션 변환)
  - 여는 함수와 컴포넌트를 다른 파일에 둔다(oxlint `only-export-components`)
- 확인 폼에서 고를 수 있는 수단과 순서는 `messages.ts`의 `factorChoices`다. 패스키 등록(`requestManage`의 `purpose: 'addPasskey'`, 보안 페이지의 추가 단추)은 인증 앱, 기기 추가 코드, 패스키, 복구 코드 순이다(새 기기에는 그 패스키가 없는 경우가 많다). 그 밖의 관리 확인과 세션 중 확인은 패스키, 인증 앱, 복구 코드 순이고 기기 추가 코드를 보이지 않는다
  - 기기 추가 코드 입력 아래 안내가 "이 기기에 패스키가 있으면 패스키 사용"을 알린다(인증 앱이 없으면 같은 기기에 패스키를 하나 더 등록할 때도 코드 입력이 먼저 나온다)
  - 발급은 보안 페이지의 "다른 기기에서 패스키 추가" 행이 관리 창을 연 뒤 코드를 받아 `deviceCodeDialog`(내용 `DeviceCode.tsx`, 남은 시간)로 보인다. 이 브라우저의 패스키 지원과 관계없이 수단이 있고 서버가 패스키를 쓸 수 있으면 켠다
- 로그인 직후 단계는 인증 저장소의 `stepUp` 상태(`finishStepUp`)다. 보안에 민감한 기능은 `security_required`를 받으면 `requestStepUp`으로 확인하고 다시 시도한다(터미널은 닫힘 막대의 확인 단추, 상승은 `ElevationForm`이 같은 비밀번호로 다시 시도)
- 패스키는 `passkeysSupported()`(보안 컨텍스트 + `PublicKeyCredential`)일 때만 보인다. 개발 서버(평문 HTTP)에서는 TOTP·복구 코드만 시험할 수 있다
- 패스키로 로그인: 순수 모듈 `passkeyLogin.ts`(항목 형식, 분할 키, 두 share로 차례로 풀기 `openEntry`·다시 암호화 `resealEntry`), 저장 `passkeyLoginStorage.ts`(localStorage), 흐름 `passkeyLoginFlow.ts`(`enablePasskeyLogin`, `unlockPasskeyLogin`). 보안 페이지의 "패스키로 로그인하는 브라우저" 절은 관리 창 없이 해제하고 `enablePasskeyLoginDialog`(내용 `PasskeyLoginSetup.tsx`)로 켜며, 받은 상태마다 이 브라우저 항목을 서버 목록에 맞춘다(`syncPasskeyLogins`: 목록에 없으면 지우고, 있으면 `expiresAt`을 서버 값으로. 관리자가 기간을 줄인 경우). 만료 기간은 보안 정책 페이지가 고른다
  - 로그인 화면은 활성 항목마다 "패스키로 로그인" 단추를 두고(풀어 낸 비밀번호는 인증 저장소의 `completePasskeyLogin`에만 넘긴다), 비밀번호 로그인 직후 권유는 인증 저장소의 `passkeyOffer`(`shouldOfferPasskeyLogin`: 이 브라우저 항목·"다시 묻지 않기")다. 권유만 오면 `auth/PasskeyOffer.tsx` 카드, 2차 인증 권유와 함께 오면 `StepUpForm`의 확인란(`passkeyLogin`. PRF가 없으면 같은 확인 값으로 `verifySecurity`)이다. 비밀번호는 권유를 마칠 때까지 LoginScreen 상태에만 둔다
- 인증 앱 QR은 `uqr`의 `encode`로 모듈 배열을 받아 SVG path로 그린다(`qrPath`, innerHTML 없음). 등록 대화상자를 열 때 `import('uqr')`로 받고 vendor 청크에서 뺀다(`vite.config.ts`)

## 테스트

- Go: TOTP는 RFC 6238 SHA-1 벡터, WebAuthn은 테스트 안에서 합성한 인증기(P-256·ed25519·RSA 키로 authData·clientDataJSON·서명을 만든다). 서버 테스트(`security_test.go`, `passkey_test.go`)는 `envOptions{security, rp}`와 omvfake로 정책 가드, 로그인 단계, 터미널·상승 적용, 관리 창, 기기 추가 코드, 초기화, 손상된 기록을 시험한다
- 브라우저: TOTP·화면은 `make dev-fake`, 패스키는 HTTPS 도메인으로 연결한 테스트 VM에서 Playwright 가상 인증기(CDP `WebAuthn.addVirtualAuthenticator`, UV 켬)로 확인한다. 실제 기기 패스키는 사용자가 확인한다
  - 가상 인증기는 `WebAuthn.setUserVerified(false)`로 UV를 끄면 UV=0 응답을 만들지 않고 `get()`이 NotAllowedError로 끝난다(시험 확인의 "다시 확인" 경로). "등록은 UV=1, 확인은 UV=0"인 경우는 Go 테스트(`fakePasskey.noUV`)가 맡고, 화면의 `passkey_uv_unsupported` 안내는 confirm 응답을 `page.route`로 바꿔 확인한다
  - 패스키로 로그인은 가상 인증기를 `hasPrf: true`로 만들어 확인한다. 등록 옵션에 `extensions.prf`가 있어야 그 자격 증명에 PRF가 생긴다(create 결과 `prf.enabled: true`)
  - 이 Chrome에서는 `setUserVerified(false)`를 한 번 하면 그 인증기는 `setUserVerified(true)`로 다시 켜도 계속 실패한다. 취소·UV 실패 확인은 따로 만든 인증기로 하거나 마지막에 한다. 자격 증명을 새 인증기로 옮겨도(`getCredentials` → `addCredential`) PRF 상태는 옮겨지지 않는다
