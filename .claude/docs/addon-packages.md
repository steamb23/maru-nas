# 애드온 패키지

저장소 밖에서 만든 애드온을 패키지 파일(`.maru-addon`)로 설치한다. 내장 애드온(`.claude/docs/addons.md`)은 지금처럼 Go 코드와 정적 프론트 목록으로 함께 빌드하고, 패키지는 그 옆에 런타임으로 더한다. 사용자 설명은 `docs/ko/addons.md`(애드온 설치), 개발자 설명은 `docs/ko/addon-dev.md`(애드온 개발)부터 시작하는 "애드온 개발" 절에 있다. 규칙은 `.claude/rules/addon-packages.md`.

- 패키지 = manifest + 샌드박스 iframe 보기(창·위젯) + 선택적 백엔드 프로세스
- 관리자가 설정 앱의 "애드온" 페이지에서 파일을 올리고, 선언한 권한을 검토해 승인한다. 재시작하지 않는다
- 코드 이름은 모두 `addonpkg`다(백엔드 `internal/addonpkg`, 프론트 `src/addonpkg/`, 앱 종류 `'addonpkg'`, 번역 `addonPackages.*`, API 타입 `AddonPackage*`). `packages`는 시스템 업데이트 애드온이 쓰고 있다
- SDK(`sdk/`)는 MIT다(`sdk/LICENSE`). 대시보드 본체는 GPL-3.0 그대로이며, SDK가 대시보드와 함께 쓰는 파일(`sdk/js/src/protocol.ts`, `tokens.css`, `maru.css`)도 원본은 `sdk/` 안에 있다

## 구성

```
[관리자] 설정 앱 "애드온" 페이지
  POST /api/site/addons/uploads (원시 본문) → 검토(권한·위험·이전 버전과 차이·sha256)
  POST /api/site/addons/uploads/{pid}/install (관리 창 + 필요하면 단계 확인)
    → internal/addonpkg.Manager
       ├ <state>/addons/<id>/<rev>/ 풀기, <id>/current 교체, installed.json
       ├ 레지스트리 = 내장(builtin.New) + 검사를 통과한 패키지 → atomic.Pointer 교체
       └ 백엔드가 있으면 <state>/addon-backends.json(원하는 상태) + <state>/addon-sync.json(요청)
            → maru-addons.path(PathExists) → root oneshot maru-addons.service
            → /usr/local/lib/maru/maru-install --addon-sync
               systemctl enable|disable --now maru-addon[-lan|-loopback|-lan-loopback]@<id>.socket, try-restart, clean
               → /var/lib/maru-addons/status.json(root 소유, maru는 읽기만)

[사용자] 데스크톱
  GET /api/capabilities → addons[](패키지 포함), addonPackages[](실행 manifest, 패키지별 uiBase)
  창·위젯 = <AddonFrame>: <iframe sandbox="allow-scripts" src="api/addon-ui/<토큰>/<rev>/<entry>?theme=…">
  postMessage 브리지 → 호스트(대시보드)가 쿠키로 대신 호출
     data    GET  /api/addons/{id}?…           (백엔드로 프록시)
     rpc     POST /api/addons/{id}/rpc          (읽기만)
     action  POST /api/addons/{id}/actions/{a}  (백엔드로 프록시)
     storage GET|PUT /api/preferences/addons/{id}
                ↓ HTTP/1.1, 유닉스 소켓 /run/maru-addon/<id>.sock(소켓 활성화)
         maru-addon…@<id>.service (DynamicUser)
                ↓ OMV가 필요하면 유닛 안 /run/maru-host/host.sock = /run/maru-addon-host/<id>/host.sock
         POST /v1/rpc | /v1/rpc-bg | /v1/apply + 요청마다 만든 호출 토큰
         → rpcCaller{addon: id} → OMV rpc.php (OMV 세션 ID는 maru 밖으로 나가지 않는다)
```

## 패키지 파일

`.maru-addon`은 gzip으로 압축한 tar다. 최상위에는 아래만 둔다.

| 경로 | 내용 |
|---|---|
| `manifest.json` | 필수. 아래 "manifest" |
| `icon.svg` | 필수. 앱 아이콘(컬러 `<img>`로만 그린다) |
| `i18n/<lang>.json` | `en` 필수. 평평한 키 → 문자열. 지원 언어(`languages.ts`)만 |
| `ui/` | 정적 프론트. 진입 HTML은 `ui/` 바로 아래만 |
| `backend/linux-amd64`, `backend/linux-arm64` | 선택. 정적 ELF 실행 파일 |
| `LICENSE`, `README.md` | 선택 |

- 일반 파일과 디렉터리만 받는다. 심볼릭·하드 링크, 장치, FIFO는 거부한다
- 경로 조각은 `^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`, 깊이 16 이하. 절대 경로·`..`·역슬래시·NUL·제어 문자·중복·숨김 이름(`.`으로 시작)·예약 이름 `_maru`를 거부한다
- 상한: 압축 64MiB, 풀린 전체 160MiB, 항목 2000개, ui 파일 하나 8MiB, 백엔드 하나 64MiB, manifest 64KiB, 번역 파일 128KiB, 아이콘 128KiB
- 백엔드는 `debug/elf`로 ELF64이고 아키텍처(`EM_X86_64`, `EM_AARCH64`)가 이름과 맞아야 한다. 실행 중인 아키텍처의 백엔드가 없으면 설치하지 않는다(`arch_missing`)
- sha256은 압축 파일 전체로 계산해 검토 화면과 감사 로그에 보인다. 서명은 없다

## manifest

`format: 1`. 모르는 필드는 거부한다(`DisallowUnknownFields`). 원본은 Go 구조체(`internal/addonpkg/format`)이고, 구조 스키마 `sdk/schema/manifest.schema.json`은 `cmd/genaddons`가 만든다(골든 테스트). 의미 검사의 기준은 서버이고, SDK CLI는 구조만 본다.

| 필드 | 내용 |
|---|---|
| `format` | 1 |
| `id` | `idPattern`(소문자·숫자·`-`), 24자 이하(`User=maru-a-%i`가 31자 한도 안에 들어야 한다). 예약 id·내장 id와 겹치면 안 된다 |
| `version` | SemVer 코어 + 선택적 `-pre`(빌드 메타데이터 `+` 없음), 32자 이하 |
| `sdk` | 필요한 SDK API 수준(정수). 호스트의 `api.AddonSDKLevel`보다 크면 "호환 안 됨" |
| `name`, `description` | 언어별 맵, `en` 필수, 40자·200자 |
| `author` | `{name, url}`. 같은 id인데 이름이 바뀌면 검토 화면에 `author_changed` |
| `icon` | `icon.svg` |
| `views.window` | `{entry, defaultSize, minSize}`. `entry`는 `ui/` 바로 아래 `.html`, 크기는 픽셀 |
| `views.widget` | `{entry, defaultSize, minSize, maxSize}`. 크기는 칸, 한 변 1~12 |
| `requires.admin` | 관리자만 보이는지 |
| `requires.plugins` | `[{package, rpcFile}]`. `package`는 `^openmediavault-[a-z0-9.+-]+$`, `rpcFile`은 `^[a-z0-9_-]+\.inc$`. 감지기 표에 더한다(`Detector.SetExtra`). `knownPlugins`에 있는 패키지는 `rpcFile`이 같아야 한다 |
| `rpc.read`, `rpc.write` | `Service.method`. `write`는 조작이 있을 때만. 아래 "금지 표" |
| `actions` | `[{name, admin, confirm, timeoutSeconds, label, confirmText}]`. 백엔드가 있어야 한다. `timeoutSeconds`는 1~600(없으면 60). `label`·`confirmText`는 언어별 맵(40자·300자), `confirm`이면 `confirmText.en` 필수 |
| `backend` | 있으면 백엔드 패키지. `{network: []}`, 원소는 `lan`·`loopback`(비면 네트워크 없음) |

- 모든 문구에서 제어 문자와 서식 문자(양방향 재정의 등 유니코드 Cf)를 거부한다
- 패키지 manifest는 `addonpkg/convert.go`가 `addons.Addon{Kind: KindPackage}`로 바꾼다. title·icon은 번역 키·아이콘 이름이 아니므로 `api.Manifest`에는 id와 크기·요구·RPC·조작 선언만 쓰고, 이름·아이콘 주소는 `addonPackages[]`가 따로 준다

## 저장

| 경로(`<state>` = 상태 디렉터리) | 내용 |
|---|---|
| `addons/installed.json` | `{version:1, packages:[{id, version, rev, sha256, author, enabled, installedBy, installedAt, updatedAt, installUID, grant}]}`. `grant`는 승인한 권한 요약의 digest |
| `addons/<id>/<rev>/` | 풀린 파일. rev = `<version>-<sha256 앞 8자>`. 실행 중인 아키텍처의 백엔드만 `backend/run`(0755)으로 둔다 |
| `addons/<id>/current` | `<rev>`를 가리키는 심볼릭 링크(임시 링크를 rename) |
| `addons/.staging-<난수>/`, `addons/.uploads/<pid>` | 설치 중·검토 대기. 시작할 때와 올릴 때 정리한다 |
| `addon-data/<사용자>.json` | 패키지별 사용자 저장값(아래 "사용자별 저장") |
| `addon-backends.json`, `addon-sync.json` | 백엔드의 원하는 상태와 동기화 요청(아래 "백엔드 프로세스") |

- 디렉터리는 0755, 파일은 0644, `backend/run`만 0755로 맞춘다. 동적 uid가 자기 패키지 디렉터리를 지나가야 한다
- 쓰기는 임시 파일 + fsync + rename이다. installed.json을 읽을 수 없으면 `.corrupt`로 옮기지 않고 오류로 둔다(패키지가 조용히 사라지면 안 된다). 관리자 페이지가 오류를 보인다
- 옛 rev는 새 레지스트리로 바꾼 뒤 지운다. 열린 프레임은 capabilities의 rev가 바뀌면 새 주소로 다시 뜬다
- 상태 디렉터리가 없으면(메모리 모드) 설치할 수 없고 개발 패키지만 쓴다(409 `addons_unavailable`)

## 레지스트리

- `addons.Extend(base, cands) (*Registry, []Rejection)`: 후보마다 `validatePackage`(id 길이, 예약 id, 금지 표, 쓰기는 조작이 있을 때만, 플러그인 표)를 거친 뒤 `NewRegistry(base.All()..., 받은 것..., 후보)`가 통과하면 받는다. 하나가 나빠도 나머지와 내장은 그대로다
- 거부 코드: `conflict_builtin`, `conflict_package`, `rpc_denied`, `rpc_kind`(다른 애드온과 읽기·쓰기 종류가 다름), `reserved_id`, `invalid`, `incompatible`
- 시작할 때는 설치 순서로 더한다. 이미 받은 패키지를 밀어낼 설치·켜기는 거절한다. 내장이 나중에 같은 id를 쓰면 내장이 이기고 패키지는 "충돌"로 보인다
- `Addon.Kind`(`KindBuiltin` 0값, `KindPackage`). `validateActions`는 종류를 안다: 패키지는 선언만 있고 구현(`Run`·`Query`)이 없다. "조작이 있으면 `rpc.write` 1개 이상"은 내장 테스트(`TestBuiltinActionsHaveWrites`)로 옮겼다
- `server.Config.Addons`(내장)는 그대로 두고, `h.addons()`가 `Manager`의 교체된 레지스트리를 준다. `/api/rpc` 중계는 내장 레지스트리만 본다
- 예약 id(`addons/reserved.go`): 프론트에만 있는 셸·링크·준비 중 앱 id와 미래용 이름. `genaddons`가 `RESERVED_APP_IDS`로 내보내고 Vitest가 프론트 표의 키가 모두 예약되어 있는지 본다

## 금지 표

`addons/packagerpc.go`, 항목마다 근거 주석. 패키지는 아래 메서드를 읽기로도 쓰기로도 선언할 수 없다(`rpc_denied`). OMV는 서비스 이름을 대소문자 구분 없이 찾고(`rpc/servicemanager.inc`의 `mb_strtolower`) 메서드 이름은 등록한 그대로 찾으므로(`rpc/serviceabstract.inc`의 `hasMethod`), 금지 표와 종류 충돌은 서비스 이름만 소문자로 맞춰 비교한다(`rpcKey`). 금지 표를 바꿀 때는 OMV 소스(`/usr/share/openmediavault/engined/rpc/*.inc`, 플러그인 포함)에서 근거를 확인하고 이 문서와 주석을 함께 고친다.

- 기준
    - 호출한 쪽이 명령·스크립트·패키지·파일 내용·호스트 경로·이스케이프하지 않는 CLI 인자를 정하는 메서드. OMV는 명령 인자를 이스케이프 없이 잇는다(`/usr/share/php/openmediavault/system/process.inc`의 `getCommandLine`)
    - 자격 증명을 돌려주거나 비밀번호를 확인하는 메서드
    - maru가 혼자 맡은 흐름(세션, 백그라운드 상태 파일, 설정 적용, APT 작업)
- 서비스 전체: `Session`, `Exec`(호스트 `/v1/rpc-bg`가 대신), `Config`(호스트 `/v1/apply`가 대신), `Cron`, `Scripts`, `PluginMgmt`, `Apttool`, `Omvextras`, `EnvVars`
- 메서드
    - `Apt.install`, `upgrade`, `update`, `upload`, `getChangeLog`
    - `Compose.doCommand`, `doCommandList`, `doPrune`, `doServiceCommand`, `doContainerCommand`, `getContainerLog`(이름은 get이지만 `id`를 그대로 셸에 넣는다), `doAutocompose`, `doDockerCmd`, `doDockerVolumeCmd`, `doDockerNetworkCmd`, `doDockerImageCmd`, `doNetworkConnect`, `doGit`, `doBuild`, `doJob`, `setJob`, `setFile`, `setDockerfile`, `setConfig`, `setGlobalEnv`, `doRestore`
    - `UserMgmt.set*`, `delete*`, `import*`, `authUser`(관리자 세션으로 아무 계정의 비밀번호를 대입할 수 있다)
    - `EmailNotification.get`(SMTP 비밀번호), `Rsync.get`(작업 비밀번호), `Network.getProxy`(프록시 비밀번호)
    - `Symlinks.setSymlink`, `Kvm.setVmXml`, `addFsPass`, `addHostDisk`
- 금지하지 않은 쓰기도 root에 이를 수 있다(서비스 설정의 `extraoptions` 등). 그래서 OMV 쓰기를 선언한 패키지는 검토 화면에 `omv_write`("root에 준하는 신뢰")로 보이고, 처음 보는 플러그인 메서드는 검토하지 않았다고 적는다

## 권한 요약과 승인

- `addonpkg/permissions.go`의 `Summary`: 관리자 전용, 플러그인, 읽기·쓰기 메서드, 조작, 백엔드, 네트워크. `Digest`는 요약의 해시로 installed.json의 `grant`에 둔다. `Diff(old, new)`는 업데이트 때 더해진·빠진 권한이다
- 위험 코드: `omv_write`, `backend`, `backend_shared_data`(백엔드가 OMV를 부르고 모든 사용자가 같은 백엔드를 쓴다), `net_lan`, `net_loopback`. 금지 표 밖의 메서드는 검토하지 않았다는 안내는 위험 코드 없이 검토 화면에 늘 보인다
- 받지 않은 이유(`addonpkg.Error.Reason`): `archive_invalid`, `too_large`, `manifest_invalid`, `incompatible`, `arch_missing`, `backend_invalid`, `backend_unsupported`, 그리고 레지스트리의 거부 코드
- 경고: `author_changed`, `downgrade`, `reinstall`, `same_digest`, `backends_disabled`
- 보증 수준
    - 설치·업데이트·다시 켜기: 관리 창(`requireManage`, 비밀번호와 등록한 수단). 쓰기·백엔드·네트워크가 있는 패키지는 거기에 아래 쓰기 조작과 같은 단계도 필요하다
    - 쓰기를 선언한 패키지의 조작: 실행할 때마다 최소 `password`(세션 중 비밀번호 재확인). root 권한 상승 정책(`policy.Elevation`)이 `twoFactor`면 `twoFactor`. 정책이 기본 `off`일 때 빼앗은 관리자 쿠키만으로 패키지의 OMV 쓰기가 되지 않게 하려는 것이다. 모자라면 403 `security_required`
    - 끄기는 확인하지 않는다(기능을 줄이기만 한다). 제거는 파일만 지우고, 백엔드 데이터 지우기는 따로 고르며 관리 창이 필요하다

## API

| 경로 | 내용 |
|---|---|
| `GET /api/site/addons` | 관리자. 설치본·개발본·잘못됨·충돌·호환 안 됨과 이유 코드, 권한 요약, sha256, 백엔드 상태, 백엔드 켜짐 여부, 상한 |
| `POST /api/site/addons/uploads` | 관리자. 원시 `application/octet-stream`, 64MiB. 200 `{pendingId, expiresInSeconds, package, installed?, diff, warnings}`. 대기는 세션에 묶이고 2개까지 |
| `POST /api/site/addons/uploads/{pid}/install` | 관리 창(+단계). 스테이징 → rename → current 교체 → installed.json → 레지스트리 교체 → 백엔드 갱신 |
| `DELETE /api/site/addons/uploads/{pid}` | 대기 버리기 |
| `PUT /api/site/addons/{id}` | `{enabled}`. 켜기는 관리 창(+단계) |
| `DELETE /api/site/addons/{id}` | `{purge}`. 제거. `purge`는 관리 창 |
| `GET /api/addons/{id}/icon` | 세션. `image/svg+xml`, CSP `default-src 'none'; style-src 'unsafe-inline'; sandbox` |
| `GET /api/addons/{id}/i18n/{lang}` | 세션. 패키지 번역(지원 언어만) |
| `GET /api/addon-ui/{token}/{rev}/{path...}` | 세션 밖(토큰). 아래 "프레임" |
| `POST /api/addons/{id}/rpc` | 세션. 패키지만, 64KiB, 읽기만 |
| `GET·PUT /api/preferences/addons/{id}` | 세션. 사용자별 저장 |
| `GET /api/addons/{id}`, `POST /api/addons/{id}/actions/{action}` | 내장과 같은 경로. 패키지는 백엔드로 프록시 |

- 본문 없는 POST·PUT·DELETE도 `requireJSON`(CSRF 가드). 진행 중인 조작이 있는 패키지의 제거·끄기·업데이트는 409 `action_busy`
- 오류 코드: `addon_error`(+ `addonCode`), `addon_backend_unavailable`, `addon_backend_disabled`, `addon_no_backend`, `addons_unavailable`, `addon_invalid`(+ 이유), `addon_too_large`, `upload_expired`
- 감사 로그: `addon: user=%q action=install id=%s version=%q sha256=%s result=%s`. manifest·브라우저 값은 `%q`

## capabilities와 프론트 목록

- 켜져 있고 유효한 패키지만 `addons[]`(capability)와 `addonPackages[]`(이 사용자에게 보이는 실행 manifest: id, rev, dev, 이름 맵, 아이콘 주소, 보기, 조작 선언, `uiBase`)에 넣는다. 꺼짐·잘못됨·충돌은 `/api/site/addons`에만 있다
- capability 이유에 `backend_unavailable`을 더했다(백엔드가 꺼진 설치, 동기화 실패)
- 프론트 검사는 `addonPackages`의 나쁜 항목을 하나씩 버린다. 응답 전체를 버리면 데스크톱의 앱이 모두 사라진다
- 프론트는 첫 capabilities 결과(받음 또는 실패, 짧은 시간 제한)를 기다린 뒤 바로가기·위젯을 그린다. 늦게 온 패키지 바로가기 때문에 아이콘 열이 늘어 위젯이 밀리지 않게 하기 위해서다. 첫 요청이 실패하면 빨리 다시 묻는다

## 프레임

- `<iframe sandbox="allow-scripts" allow="">`. `allow-same-origin`·popups·forms·top-navigation은 주지 않는다. 대시보드가 OMV와 같은 출처일 수 있으므로 반드시 불투명 출처여야 한다
- 프레임 문서와 자산은 토큰 경로 `api/addon-ui/<token>/<rev>/…`로 받는다. 불투명 출처의 하위 자원 요청에는 `SameSite=Strict` 쿠키가 붙지 않기 때문이다
    - 토큰은 (세션, 패키지)마다 하나를 `auth.Session`에 두고 세션이 끝날 때(`OnRemove`) 지운다. 고정 TTL이 없고 capabilities를 다시 받아도 바뀌지 않는다
    - 요청마다 패키지가 켜져 있고 그 세션이 capability를 만족하고 rev가 맞는지 본다. `os.Root`로 `ui/` 안만 연다
    - `_maru/sdk.js`, `_maru/maru.css`는 호스트가 준다(`web.SDKFile`, 개발은 `MARU_DEV_ADDON_SDK`)
- 모든 응답의 헤더
    - `Content-Security-Policy: sandbox allow-scripts; default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'; webrtc 'block'`
    - `X-Content-Type-Options: nosniff`, `Access-Control-Allow-Origin: *`(불투명 출처의 모듈 스크립트·글꼴은 CORS 로드다. 토큰이 권한이다), `Referrer-Policy: no-referrer`, `X-DNS-Prefetch-Control: off`
    - MIME은 고정 표(html, js, mjs, css, json, svg, png, jpg, webp, woff2, txt)다. `/etc/mime.types`에 기대지 않는다
    - 설치본은 `private, max-age=31536000, immutable`, 개발본은 `no-cache`
- 대시보드 문서(`index.html`, `/`)에 `Content-Security-Policy: frame-src 'self'`만 더했다. 샌드박스 프레임은 스스로 다른 주소로 이동할 수 있고, 그것을 막는 것은 부모 문서의 `frame-src`뿐이다. Vite 개발 서버도 같은 헤더를 준다
- 프레임 문서의 `load`가 두 번째로 오면(스스로 이동) 브리지를 끊고 프레임을 다시 띄운다. 그래서 보기 하나는 페이지 하나다
- 테마는 진입 URL 쿼리(`theme`, `anim`)로 넘기고 `sdk.js`가 `<head>`에서 바로 적용한다. iframe 요소에도 `color-scheme`을 둔다(어두운 화면에서 흰 바탕이 깜빡이지 않게)

## 브리지

원본은 `sdk/js/src/protocol.ts`(메시지 버전 1). 호스트 구현은 `frontend/src/addonpkg/bridge.ts`(순수 함수)와 `hostServices.ts`.

- `event.source === iframe.contentWindow`인 메시지만 받는다. 애드온 id는 프레임 바인딩에서 정하고 메시지의 값은 쓰지 않는다. 응답은 targetOrigin `'*'`(불투명 출처라 다른 값을 쓸 수 없다). 메시지 256KiB, 동시 요청 16개
- `hello` → `init{addonId, view, active, size, theme, language, messages, admin, sdkLevel}` → `context`(바뀐 값)
- 요청: `data`, `rpc`, `action`, `query`, `storage.get`, `storage.set`, `openWorkbench`, `confirm`, `notify`, `activity`, `pointer`
- 사용자 조작 게이트(`action`, `openWorkbench`): 호스트 `navigator.userActivation.isActive`, 그 iframe이 `document.activeElement`, 그 창이 앞 창이고 창 모달로 막히지 않음, 포커스가 몇 초 안에 들어옴, 초당 1개. 활성화는 문서 전체에 퍼지므로 활성화만 보면 다른 곳을 누른 직후 뒤쪽 패키지가 조작을 보낼 수 있다. `query`는 막지 않는다(창을 열 때 데이터를 읽는 데 쓴다)
- `confirm`·`notify`는 창 보기에서만 받는다. 호스트 대화상자·토스트에 패키지 이름·아이콘·배지를 붙인다. `notify`는 패키지 번역 키 + 인수이고 분당 3개, 동작은 "앱 열기"만
- `storage.set`은 초당 1개
- 위젯 프레임은 `inert` + `pointer-events: none`이고(끌기·크기 조절·창 열기는 틀이 한다) `data`·`rpc`·`storage.get`만 받는다
- 입력 전달: iframe 요소의 `focus` 이벤트와 SDK의 `pointer` 전달마다 iframe 요소에 합성 `pointerdown`(버블)을 보낸다. 창 올리기(`onPointerDownCapture`), 바깥 클릭으로 닫는 메뉴·패널, `useIdle`이 그대로 동작한다. 키 입력은 `maru-activity`로 바꿔 세션 유지(`useSessionKeeper`)에 쓴다
- 호스트 컨텍스트 메뉴 요청과 키(Shift+F10) 전달은 만들지 않았다. 프레임이 스스로 메뉴를 그린다
- 패키지 창 안에서 호스트는 비밀번호를 묻지 않는다. 패키지가 그린 가짜 입력 화면은 막을 수 없으므로 문서로 알린다

## SDK 런타임과 CSS

- `_maru/sdk.js`: 의존성 없는 고전 스크립트, `globalThis.maru`. `frontend/vite.sdk.config.ts`가 `sdk/js/src/client.ts`에서 파일 하나로 빌드해 `backend/internal/web/static/dist/addon-sdk/`에 둔다(`npm run build`에 포함). 패키지에 넣지 않으므로 프로토콜은 늘 호스트 빌드와 같다
- npm 패키지 `@maru-nas/addon-sdk`(`sdk/js`)는 손으로 쓴 공개 API 타입(`types/index.d.ts`, 런타임이 `satisfies`로 맞춘다)과 CLI만 담는다
- `sdk/js/src/tokens.css`: `:root` 토큰과 `data-theme`·`data-animations` 규칙. `frontend/src/base.css`가 불러온다
- `sdk/js/src/maru.css`: 공통 클래스(`.addon-card`, `.badge`, `.meter`, `.status-dot`, `.addon-button`, `.row-*`, `.widget-line` 등). `frontend/src/addons/common/common.css`가 불러온다. 원본이 하나라 대시보드와 패키지의 모양이 어긋나지 않는다
- SDK 소스는 npm 패키지를 import하지 않는다. 검사는 프론트 도구가 한다(별칭 `@maru-sdk`, `tsconfig.app.json`의 `paths`, oxlint 경로, `frontend/src/addonpkg/*.test.ts`)

## 사용자별 저장

- `addon-data/<사용자>.json`, 사용자 설정 파일과 따로 잠근다(위젯을 저장할 때마다 큰 값을 다시 쓰지 않게)
- 패키지마다 64KiB, 사용자마다 1MiB
- 값은 설치 ID(`installUID`, 처음 설치할 때 만들고 업데이트해도 유지)에 묶는다. 다시 설치하면 이전 값이 보이지 않고 다음 저장 때 지운다. 같은 id를 쓰는 다른 작성자의 패키지가 이전 데이터를 읽지 못한다

## 바로가기와 위젯 배치

- 모르는 id(꺼지거나 지운 패키지)는 지우지 않고 그리지만 않는다. 다시 켜면 돌아온다
- 저장할 목록이 한도(64개)를 넘으면 모르는 id부터 지운다
- 위젯 크기가 manifest 범위를 벗어나면(업데이트로 범위가 줄어듦) 불러올 때 맞춘다
- `ValidateWidgets`는 모르는 id를 형식·크기 한도·좌표·겹침만 본다
- 창이 열린 채 패키지가 사라지면 묘비(마지막으로 본 이름·아이콘·크기)로 창을 유지하고 본문에 "쓸 수 없음"을 보인다

## 백엔드 프로세스

- maru는 프로세스를 직접 실행하지 않는다. 설치기가 켠 systemd 템플릿만 쓴다(maru의 자식은 maru와 같은 uid라 상태 디렉터리·도우미 소켓에 닿는다)
- 템플릿 원본 하나를 네트워크 단계 4개로 그린다: `maru-addon@`(없음), `maru-addon-lan@`, `maru-addon-loopback@`, `maru-addon-lan-loopback@`(각 `.socket`, `.service`). root는 드롭인을 쓰지 않는다
- 서비스 공통
    - `DynamicUser=yes`, `User=maru-a-%i`, `StateDirectory=maru-addon/%i`
    - `ExecStart=/var/lib/maru/addons/%i/current/backend/run`
    - `TemporaryFileSystem=/run:ro` + `BindPaths=/run/maru-addon-host/%i:/run/maru-host`(D-Bus, resolved varlink, avahi, tailscale 같은 누구나 붙는 소켓을 숨긴다)
    - `TemporaryFileSystem=/var/lib/maru:ro` + `BindReadOnlyPaths=/var/lib/maru/addons/%i`
    - `ProtectHome=yes`, `InaccessiblePaths=-/srv -/export -/mnt -/media -/var/lib/openmediavault -/etc/openmediavault -/var/lib/maru-upgrade -/var/lib/maru-addons`(공유 폴더는 다른 uid에게도 읽힐 수 있다)
    - `ProtectProc=invisible`, `PrivateUsers=yes`. `ProcSubset=pid`는 두지 않는다(`/proc/meminfo` 같은 시스템 값을 읽는 백엔드를 위해서)
    - 도우미 템플릿의 하드닝(`CapabilityBoundingSet=`, `PrivateDevices`, `ProtectKernel*`, `ProtectClock`, `ProtectHostname`, `RestrictNamespaces`, `MemoryDenyWriteExecute`, `SystemCallFilter=@system-service`, `UMask=0077`)
    - `MemoryMax=256M`, `TasksMax=64`, `CPUWeight=20`, `Restart=on-failure`
- 네트워크
    - 없음: `PrivateNetwork=yes`, `RestrictAddressFamilies=AF_UNIX`
    - 그 밖: 호스트 네트워크, `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`, `IPAddressDeny=any`. `lan`은 `IPAddressAllow=10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7 link-local multicast`, `loopback`은 `IPAddressAllow=localhost`
- 소켓: `ListenStream=/run/maru-addon/%i.sock`, `SocketUser=maru`, `SocketMode=0600`, `RemoveOnStop=yes`, `Service=`(같은 단계의 서비스)
- 켜기: 설치기 `--addon-backends`(기본 꺼짐, 질문 없음)가 템플릿과 `/var/lib/maru-addons/`를 만든다. maru는 `/var/lib/maru-addons/status.json`이 있는지로 안다. 플래그·환경 변수는 없다(`/etc/default/maru`를 바꾸지 않는다)
- `maru.service`는 켜졌을 때 `RuntimeDirectory=maru maru-addon-host`(Preserve가 함께 적용되어 디렉터리 바인드가 maru 재시작 뒤에도 이어진다)

### root 동기화

- maru가 `<state>/addon-backends.json`(원하는 상태: `{format, backends:[{id, version, rev, network}], purge:[]}`)을 쓰고 `<state>/addon-sync.json`(요청)을 남긴다. 대기 중인 요청이 있으면 다시 쓰지 않고, 1분이 넘게 처리되지 않으면 관리자 페이지에 "동기화 멈춤"을 보인다
- `maru-addons.path`(`PathExists`) → `maru-addons.service`(root oneshot, `StartLimitIntervalSec=0`) → `/usr/local/lib/maru/maru-install --addon-sync`
- 동기화는 요청을 `O_NOFOLLOW|O_NONBLOCK` + `IsRegular`로 읽고 지운다. 원하는 상태는 64KiB, 엄격한 스키마(id·rev·네트워크 열거값·32개)다. 자기 기록(`/var/lib/maru-addons/state.json`)과 비교해 `enable|disable --now`, `try-restart`(rev가 바뀜), 이전 단계 끄기 + 새 단계 켜기(네트워크가 바뀜), `clean --what=state`(purge, 같은 id가 backends에 있으면 무시)만 한다. 끝나면 원하는 상태의 해시를 다시 보고 3번까지 맞춘다
- 결과는 `/var/lib/maru-addons/status.json`(0644, `updatedAt`, 항목별 상태·오류)이다. 실패해도 0으로 끝낸다. maru는 status가 지운 것을 확인한 purge 항목을 원하는 상태에서 뺀다
- root는 패키지 파일을 읽지 않는다. id 정규식이 `systemctl` 인자 주입을 막는다

### maru ↔ 백엔드

- `GET /v1/data?<쿼리 2KiB>`, `POST /v1/actions/{name}`(`{target, input}`, `input`은 8KiB JSON 객체)
- 헤더: `X-Maru-User`, `X-Maru-Admin`, `X-Maru-Lang`, `X-Maru-Call`(호출 토큰)
- 시간 제한: 연결 2초, 데이터 15초, 조작은 manifest의 `timeoutSeconds`
- 응답: 200·204·400·404·409·422·429만 통과하고 나머지는 502 `addon_error`. 백엔드 코드는 `addonCode`(`^[a-z][a-z0-9_]{0,31}$`)로 따로 준다(가짜 401로 로그아웃시키거나 호스트 오류 코드를 사칭하지 못하게). 응답 4MiB, `json.Valid`, 헤더는 넘기지 않는다
- 연결 실패 503 `addon_backend_unavailable`, 백엔드가 꺼진 설치 503 `addon_backend_disabled`, 백엔드 없는 패키지의 `data`는 404 `addon_no_backend`. 내장 조작 본문에 `input`이 오면 400
- capability 확인, `decl.Admin`, 진행 중 표시(`inflight`), `WithoutCancel` + 시간 제한, 상태 코드 변환, 로그 한 줄은 내장 조작과 같은 코드를 쓴다

### 호스트 소켓

- maru가 백엔드 패키지마다 `/run/maru-addon-host/<id>/host.sock`(디렉터리 0755, 소켓 0666)을 연다. 유닛에는 그 디렉터리만 바인드되므로 다른 애드온의 소켓은 보이지 않는다. 헤더 읽기 시간 제한과 동시 연결 상한을 둔다(토큰 없는 연결도 받는다)
- 호출 토큰은 전용 표(항목별 만료 = 요청 제한 + 5초, 요청이 끝나면 삭제, 가득 차면 거절)에 (애드온 id, 세션, 범위 read/write)를 묶는다. 범위는 데이터 요청이면 read, 조작이면 write다
- `POST /v1/rpc {service, method, params}`: 그 애드온이 선언한 메서드만(`rpcCaller{addon: id}`), read 범위면 읽기만
- `POST /v1/rpc-bg {service, method, params, output}`: 선언한 백그라운드 메서드를 부르고, 그 메서드가 돌려준 상태 파일 이름(`^/tmp/bgstatus[A-Za-z0-9]+$`)에만 `Exec.isRunning`·`getOutput`을 써서 `addons.WaitBg`·`ReadBg`로 기다린다. 출력 1MiB, 관리자 세션만
- `POST /v1/apply {calls[]}`: write 범위만. 선언한 쓰기들을 `addons.ApplyChange`의 잠금과 `config_dirty` 규칙으로 실행한다
- `rpc-bg`·`apply`는 금지 표의 `Exec`·`Config`를 쓰므로, 그 처리기 안에서만 만드는 내부 호출기(선언한 메서드 + 고정 보조 메서드)를 쓴다. `/v1/rpc`는 `Config.applyChangesBg`를 거부한다
- OMV 401이면 바깥 요청이 대시보드 세션을 지우고 401로 끝난다(내장과 같다)
- 개발에서는 `<state>/addon-host/<id>.sock`이고 uid 구분이 없다

## 개발 루프

- 저장소 체크아웃이 필요하다(`make dev`, `make dev-fake`)
- `MARU_DEV_ADDONS`: 콜론으로 나눈 풀린 패키지 디렉터리. 개발 전용이며 플래그·가동 설정·설치기에 넣지 않는다. capabilities를 받을 때 manifest와 `ui/`의 mtime을 보고 다시 읽으며 rev는 `dev-<해시>`다. 관리자 페이지에 "개발"로 보이고 바꿀 수 없다. 제목 표시줄 메뉴에 "다시 불러오기"가 있다
- `FAKEADDONS=examples`(선택, 기본 `none`): `make dev-fake`가 `sdk/examples`를 `MARU_DEV_ADDONS` 뒤에 더한다
- `MARU_DEV_ADDON_SDK`: `scripts/dev.sh`가 `vite build -c vite.sdk.config.ts --watch`로 만든 디렉터리. 프레임의 `_maru/`를 여기서 준다(embed된 빌드는 개발 중에 없거나 오래되었다)
- 개발 백엔드: 개발자가 `MARU_ADDON_SOCKET=<state>/addon-dev/<id>.sock`으로 백엔드를 띄운다. 호스트 소켓은 `<state>/addon-host/<id>.sock`(`MARU_ADDON_HOST`)
- `FAKEOMV_EXTRA_RPC`: 가짜 OMV가 패키지의 `dev/fake-rpc.json`에 적힌 메서드에 정해진 응답을 준다. 그 밖의 플러그인 RPC는 실제 OMV(`make dev`)에서 확인한다

## 위험과 한계

- OMV 관리자 쓰기는 root에 준한다. 금지 표는 직접적인 경로만 막는다
- 프레임 유출: WebRTC와 DNS 미리 가져오기는 CSP로 완전히 막지 못한다. 패키지에 보여 준 데이터는 작성자도 볼 수 있다고 본다
- 네트워크 단계: `lan`은 LAN의 리졸버·프록시를 거쳐 인터넷과 Docker 망(172.16.0.0/12 안)에 닿는다. `loopback`은 OMV nginx·sshd·maru 수신 주소·resolved에 닿는다(로그인 대입 가능성). 네트워크가 있는 단계는 호스트 네트워크 이름공간이라 추상 유닉스 소켓(예: `@lxc/…`)에도 닿는다
- 백엔드는 모든 사용자가 함께 쓴다. 관리자 세션으로 읽은 값을 일반 사용자에게 보여 줄 수 있다
- 프레임 토큰이 경로에 있어 프록시 접근 로그에 남을 수 있다. 그 세션이 볼 수 있는 그 패키지의 정적 파일만 읽을 수 있고 세션이 끝나면 무효다
- 서명·카탈로그·자동 업데이트가 없다. 대시보드 업그레이드는 패키지를 다시 빌드하지 않는다. SDK 수준은 하위 호환을 지키며 올린다
- 옛 실행 파일로 되돌리면 패키지는 무시된다(상태 디렉터리 형식이 늘어난다: `addons/`, `addon-data/`, `addon-backends.json`)
- Go 모듈은 공개 전까지 `replace`로 쓴다. 공개 릴리스 절차에 `sdk/go/vX.Y.Z` 태그와 npm 배포를 더해야 한다

## 향후 고려(구현하지 않음)

- 선언형(JS 없는) 보기: `.claude/docs/addons.md`의 "선언형 애드온"
- 패키지 서버 알림 출처, 패키지 백그라운드 작업(동적 종류·슬롯 정책), 패키지 관리자 설정 페이지
- 서명과 카탈로그, `internet` 네트워크 권한, 파일 탐색기 API를 거친 파일 접근, 호스트 컨텍스트 메뉴 요청
- 백엔드가 요청 밖에서 OMV를 부르는 방법(지금 호출 토큰은 요청 동안만 있다)

## 시험

구현하면서 채운다. 브라우저(dev-fake)와 VM(desk-test)에서 확인한 범위와 날짜, 확인하다 바뀐 결정을 적는다.
