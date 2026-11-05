# 애드온 패키지

저장소 밖에서 만든 애드온을 패키지 파일(`.maru-addon`)로 설치한다. 내장 애드온(`.claude/docs/addons.md`)은 지금처럼 Go 코드와 정적 프론트 목록으로 함께 빌드하고, 패키지는 그 옆에 런타임으로 더한다. 사용자 설명은 `docs/ko/addons.md`(애드온 설치), 개발자 설명은 `docs/ko/addon-dev.md`(애드온 개발)부터 시작하는 "애드온 개발" 절에 있다. 규칙은 `.claude/rules/addon-packages.md`.

- 패키지 = manifest + 샌드박스 iframe 보기(창·위젯) + 선택적 백엔드 프로세스
- 관리자가 설정 앱의 "애드온" 페이지에서 파일을 올리고, 선언한 권한을 검토해 승인한다. 재시작하지 않는다
- 코드 이름은 모두 `addonpkg`다(백엔드 `internal/addonpkg`, 프론트 `src/addonpkg/`, 앱 종류 `'addonpkg'`, 번역 `addonPackages.*`, API 타입 `AddonPackage*`). `packages`는 시스템 업데이트 애드온이 쓰고 있다
- SDK(`sdk/`)는 MIT다(`sdk/LICENSE`). 대시보드 본체는 GPL-3.0 그대로이며, SDK가 대시보드와 함께 쓰는 파일(`sdk/js/src/protocol.ts`, `tokens.css`, `maru.css`)도 원본은 `sdk/` 안에 있다
    - Go 모듈(`sdk/go`)에는 `sdk/LICENSE`의 사본 `sdk/go/LICENSE`(링크가 아닌 일반 파일)를 둔다. 하위 폴더 모듈에 `LICENSE`가 없으면 `go` 명령이 모듈 압축 파일에 저장소 루트의 `LICENSE`(GPL-3.0)를 넣기 때문이다. 두 파일이 같은지와 모듈 안 Go 소스(예제 포함, `testdata` 제외) 첫 줄의 `// SPDX-License-Identifier: MIT`는 `sdk/go`의 `TestLicense`가 검사한다

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

`format: 1`. 모르는 필드는 거부한다(`DisallowUnknownFields`). 원본은 Go 구조체(`internal/addonpkg/format`)이고, 구조 스키마 `sdk/js/schema/manifest.schema.json`(npm 패키지에 함께 넣으려고 `sdk/js` 아래에 둔다)은 `cmd/genaddons`가 `format.Schema`와 압축 파일 규칙(`x-maru`, `addonpkg.Schema`)으로 만든다(골든 테스트). 의미 검사의 기준은 서버이고, SDK CLI는 스키마와 `x-maru`의 파일 배치 규칙만 본다. 둘의 경계는 공유 벡터 `sdk/testdata/manifests/`(`valid`, `invalid`, `server-only`)가 정한다

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
- 받지 않은 이유(`addonpkg.Error.Reason`): `archive_invalid`, `too_large`, `manifest_invalid`, `incompatible`, `arch_missing`, `backend_invalid`, 그리고 레지스트리의 거부 코드. 백엔드 패키지는 이 설치에서 백엔드가 꺼져 있어도 받는다(검토 화면의 경고 `backends_disabled`)
- 경고: `author_changed`, `downgrade`, `reinstall`, `same_digest`, `backends_disabled`(백엔드 패키지인데 이 설치에서 백엔드가 꺼져 있음, `Options.BackendsEnabled`)
- 보증 수준
    - 설치·업데이트·다시 켜기: 관리 창(`requireManage`, 비밀번호와 등록한 수단). 쓰기·백엔드·네트워크가 있는 패키지는 거기에 아래 쓰기 조작과 같은 단계도 필요하다
    - 쓰기를 선언한 패키지의 조작: 실행할 때마다 세션이 최소 `password`(세션 중 비밀번호 재확인)인지 본다. 확인은 세션마다 한 번이고 세션이 끝날 때까지 남는다(`Session.Assurance`는 올라가기만 하고 따로 만료가 없다). root 권한 상승 정책(`policy.Elevation`)이 `twoFactor`면 `twoFactor`. 정책이 기본 `off`일 때 빼앗은 관리자 쿠키만으로 패키지의 OMV 쓰기가 되지 않게 하려는 것이다. 모자라면 403 `security_required`
    - 끄기는 확인하지 않는다(기능을 줄이기만 한다). 제거는 파일만 지우고, 백엔드 데이터 지우기는 따로 고르며 관리 창이 필요하다

## API

| 경로 | 내용 |
|---|---|
| `GET /api/site/addons` | 관리자. 설치본·개발본·잘못됨·충돌·호환 안 됨과 이유 코드, 권한 요약, sha256, 설치할 수 있는지(`canInstall`), 설치 기록 오류(`indexError`), 올리기 상한, SDK 수준. 백엔드 동기화 상태(이 설치에서 백엔드가 켜졌는지, 항목별 동기화 결과, 동기화 멈춤)는 M3(root 동기화)에서 더한다 |
| `POST /api/site/addons/uploads` | 관리자. 원시 `application/octet-stream`, 64MiB. 200 `{pendingId, expiresInSeconds, package, installed?, diff, warnings}`. 대기는 세션에 묶이고 2개까지 |
| `POST /api/site/addons/uploads/{pid}/install` | 관리 창(+단계). 스테이징 → rename → current 교체 → installed.json → 레지스트리 교체 → 백엔드 갱신 |
| `DELETE /api/site/addons/uploads/{pid}` | 대기 버리기 |
| `PUT /api/site/addons/{id}` | `{enabled}`. 켜기는 관리 창(+단계) |
| `DELETE /api/site/addons/{id}` | 제거. `?purge=1`이면 백엔드 데이터도 지운다(관리 창) |
| `GET /api/addons/{id}/icon` | 세션. `image/svg+xml`, CSP `default-src 'none'; style-src 'unsafe-inline'; sandbox` |
| `GET /api/addons/{id}/i18n/{lang}` | 세션. 패키지 번역(지원 언어만) |
| `GET /api/addon-ui/{token}/{rev}/{path...}` | 세션 밖(토큰). 아래 "프레임" |
| `POST /api/addons/{id}/rpc` | 세션. 패키지만, 64KiB, 읽기만 |
| `GET·PUT /api/preferences/addons/{id}` | 세션. 사용자별 저장 |
| `GET /api/addons/{id}`, `POST /api/addons/{id}/actions/{action}` | 내장과 같은 경로. 패키지는 백엔드로 프록시 |

- 본문 없는 POST·PUT도 `{}` JSON 본문을 보낸다(`requireJSON`이 CSRF 보조 장치다). DELETE는 다른 출처에서 preflight 없이 보낼 수 없어 본문을 받지 않는다. 진행 중인 조작이 있는 패키지의 제거·끄기·업데이트는 409 `action_busy`
- 설치·켜기에 관리 창이나 단계 확인이 모자라면 대기를 남긴 채 403(`manage_required`, `security_required`)이고, 확인한 뒤 같은 대기로 다시 부른다. 대기는 세션마다 2개, 전체 4개, 30분이며 세션이 끝나면 버린다
- 개발 패키지와 같은 id는 설치할 수 없다(409 `addon_readonly`). 먼저 설치한 같은 id가 있으면 개발본이 이기고 설치본은 "충돌"이다
- 오류 코드: `addon_invalid`(+ `reason`, `detail`), `addon_too_large`, `addons_unavailable`, `upload_expired`, `addon_readonly`, `addon_no_backend`, 백엔드의 `addon_error`(+ `addonCode`), `addon_backend_unavailable`, `addon_backend_disabled`. capability 이유 `backend_unavailable`도 오류 코드로 쓴다(백엔드가 꺼진 설치에서 백엔드 패키지의 `POST /api/addons/{id}/rpc`가 403 `backend_unavailable`)
- 감사 로그: `addon: user=%q action=install id=%s version=%q sha256=%s result=%s`. manifest·브라우저 값은 `%q`

## capabilities와 프론트 목록

- 켜져 있고 유효한 패키지만 `addons[]`(capability)와 `addonPackages[]`(이 사용자에게 보이는 실행 manifest: id, rev, dev, 이름 맵, 아이콘 주소, 보기, 조작 선언, `uiBase`)에 넣는다. 꺼짐·잘못됨·충돌은 `/api/site/addons`에만 있다
- capability 이유에 `backend_unavailable`을 더했다(백엔드가 꺼진 설치, M3부터는 동기화 실패도). 서버의 `packageCapability`가 `addons.Capability` 위에 더하고(역할·플러그인 조건을 먼저 본다), capabilities·데이터·조작·`POST /api/addons/{id}/rpc`·프레임 경로가 쓴다. 아이콘·번역·사용자별 저장은 이 이유로 막지 않는다(바로가기 아이콘과 창 안 안내가 그대로 보이게)
- 프론트 검사는 `addonPackages`의 나쁜 항목을 하나씩 버린다. 응답 전체를 버리면 데스크톱의 앱이 모두 사라진다
- 프론트는 첫 capabilities 결과(받음 또는 실패, 짧은 시간 제한)를 기다린 뒤 바로가기·위젯을 그린다. 늦게 온 패키지 바로가기 때문에 아이콘 열이 늘어 위젯이 밀리지 않게 하기 위해서다. 첫 요청이 실패하면 빨리 다시 묻는다

## 프레임

- `<iframe sandbox="allow-scripts" allow="">`. `allow-same-origin`·popups·forms·top-navigation은 주지 않는다. 대시보드가 OMV와 같은 출처일 수 있으므로 반드시 불투명 출처여야 한다
- 프레임 문서와 자산은 토큰 경로 `api/addon-ui/<token>/<rev>/…`로 받는다. 불투명 출처의 하위 자원 요청에는 `SameSite=Strict` 쿠키가 붙지 않기 때문이다
    - 토큰은 (세션, 패키지)마다 하나를 `auth.Session`에 두고 세션이 끝날 때(`OnRemove`) 지운다. 고정 TTL이 없고 capabilities를 다시 받아도 바뀌지 않는다
    - 요청마다 패키지가 켜져 있고 그 세션이 capability를 만족하고 rev가 맞는지 본다. `os.Root`로 `ui/` 안만 연다
    - `_maru/sdk.js`, `_maru/maru.css`는 호스트가 준다(`web.SDKFile`, 개발은 `MARU_DEV_ADDON_SDK`)
- 모든 응답의 헤더
    - `Content-Security-Policy: sandbox allow-scripts; default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'`(CSP3의 `webrtc 'block'`은 크롬이 모르는 지시어라 두지 않았다)
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
- npm 패키지 `@maru-nas/addon-sdk`(`sdk/js`, 버전은 대시보드와 따로 `0.y.z`)는 손으로 쓴 공개 API 타입(`types/index.d.ts`, 런타임이 `satisfies`로 맞춘다), manifest 스키마(`schema/`), CLI(`cli/`)만 담는다. 런타임 소스(`src/`)는 넣지 않는다
- `sdk/js/src/tokens.css`: `:root` 토큰과 `data-theme`·`data-animations` 규칙. `frontend/src/base.css`가 불러온다
- `sdk/js/src/components.css`: 공통 클래스(`.addon-card`, `.badge`, `.meter`, `.status-dot`, `.addon-button`, `.row-*`, `.widget-line` 등). `frontend/src/addons/common/common.css`가 불러온다. 원본이 하나라 대시보드와 패키지의 모양이 어긋나지 않는다
- `sdk/js/src/frame.css`: 프레임 문서의 바탕(창 배경, 위젯은 투명). `_maru/maru.css`는 `tokens.css`, `components.css`, `frame.css`를 이어 붙인 파일이다(`vite.sdk.config.ts`)
- SDK 소스는 npm 패키지를 import하지 않는다. 검사는 프론트 도구가 한다(별칭 `@maru-sdk`, `tsconfig.app.json`의 `paths`, oxlint 경로, `frontend/src/addonpkg/*.test.ts`)

## CLI(maru-addon)

- `sdk/js/cli/maru-addon.mjs`(진입)와 `lib.mjs`(동작). Node 내장 모듈만 쓰는 `.mjs`라 빌드 없이 Node 20.19 이상에서 돈다. JSDoc 타입은 `frontend/tsconfig.node.json`(`checkJs`)이, 동작은 `frontend/src/addonpkg/cli.node.test.ts`가 검사한다(Node API를 쓰는 테스트는 `*.node.test.ts`로 두고 node 설정이 본다)
- 상수(상한, 경로 규칙, 언어 목록, SDK 수준)는 스키마 파일의 `x-maru`에서 읽고 CLI에 다시 적지 않는다
- `init <dir> [--id] [--ts] [--backend]`: 빌드 없는 JS(창·위젯) 또는 TypeScript + Vite 프로젝트(`addon/`이 패키지 디렉터리, `vite build`가 `addon/ui/`에 쓴다, `_maru/` 참조는 `vite-ignore`). 저장소 체크아웃의 SDK면 TS 템플릿의 SDK 의존성을 `file:`로 둔다(공개 전에는 npm에 없다)
    - `--backend`: Go 백엔드를 더한다. 패키지 최상위에는 `go.mod`·소스를 둘 수 없으므로(압축 파일 규칙) 패키지 디렉터리는 `<dir>/addon`, 백엔드는 그 옆의 Go 모듈 `<dir>/backend`(`main.go`가 `maruaddon`으로 데이터와 조작 하나)이고 빌드 결과를 `addon/backend/linux-*`에 쓴다(`.gitignore`). manifest는 `backend: {network: []}`, 읽기 `System.getInformation`, 조작 `reset`(쓰기 없음)이고 창이 `app.data()`·`app.action()`을 부른다. `--ts`와 함께 쓰면 TS 프로젝트(`addon/`이 원래 패키지 디렉터리)에 `backend/`를 더한다. 템플릿은 `templates/backend`(패키지 파일, Go 모듈)에 `backend-plain` 또는 `ts` + `backend-ts`를 겹쳐 쓴다(`_go.mod`는 `go.mod`로 바꾼다. 저장소에 자리표시자가 든 `go.mod`를 두지 않는다)
    - Go SDK는 공개 전이라 저장소 체크아웃의 SDK면 `go.mod`에 `replace`로 그 `sdk/go`를 가리킨다(새 디렉터리가 저장소 안이면 상대 경로, 밖이면 절대 경로, `goModuleDeps`). npm으로 받은 SDK면 의존성 줄을 비우고 README가 `go get`을 안내한다
    - README는 빌드(`GOOS=linux GOARCH=amd64|arm64 CGO_ENABLED=0 go build -trimpath -o ../addon/backend/linux-*`), 개발 서버에서 실행(`MARU_ADDON_SOCKET`·`MARU_ADDON_HOST`로 `go run .`), 묶기 순서다
- `validate <dir|file>`: 스키마와 파일 배치(Read·ReadDir와 같은 규칙). 개발 디렉터리의 백엔드 패키지는 실행 파일이 없어도 된다(알림만). 개발 디렉터리의 최상위 `dev/`는 보지 않는다
- `pack <dir> [-o]`: 검사한 뒤 묶는다. 백엔드를 선언했는데 `backend/linux-*` 실행 파일이 없으면 묶지 않는다. ustar, 경로 바이트 순, 수정 시각·소유자 0, 권한 0644(`backend/*`는 0755), 디렉터리 항목 없음, 100바이트가 넘는 경로는 prefix 칸. gzip 머리의 수정 시각 0, 운영체제 바이트 3. 같은 입력이면 같은 tar가 나오지만 gzip 바이트(그래서 sha256과 rev)는 zlib 버전에 따라 기기마다 다를 수 있다. 개발 디렉터리의 `dev/`는 넣지 않는다
- 고정 파일 `sdk/testdata/packed/dev-min.maru-addon`은 `sdk/testdata/dev-min`을 묶은 것이다. Go(`fixture_test.go`)가 `Read`로 받고, 프론트 테스트가 다시 묶은 tar와 풀린 tar를 비교한다. dev-min을 바꾸면 `node sdk/js/cli/maru-addon.mjs pack sdk/testdata/dev-min -o sdk/testdata/packed/dev-min.maru-addon`으로 다시 만든다

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

HTTP/1.1, 유닉스 소켓(운영 `config.AddonSocketDir`의 `<id>.sock`, 개발 `<state>/addon-dev/<id>.sock`). 요청의 Host는 `addon`이다. 이 절과 아래 "호스트 소켓"이 Go SDK(`sdk/go`)와 다른 언어 백엔드가 따르는 계약이며, 사용자 문서 `docs/ko/addon-backend.md`에도 같은 내용을 둔다.

- 요청
    - `GET /v1/data?<쿼리>`: 브라우저가 보낸 쿼리 문자열 그대로(2KiB 넘으면 maru가 400)
    - `POST /v1/actions/{name}`: `Content-Type: application/json`, 본문 `{"target": 문자열, "input": 객체 또는 null}`. 브라우저 본문은 16KiB, `input`은 8KiB JSON 객체까지
    - 헤더: `X-Maru-User`(OMV 사용자 이름), `X-Maru-Admin`(`1`·`0`), `X-Maru-Lang`(대시보드 화면 언어. 브라우저가 같은 이름의 헤더로 보낸 값이 지원 언어(`format.Languages`)면 그 값, 아니면 `en`), `X-Maru-Call`(호출 토큰, 아래)
- 응답
    - 200 + JSON 값(`json.Valid`, 4MiB까지): 데이터, 또는 결과를 돌려주는 조작. 브라우저에 그대로 준다(`Cache-Control: no-store`)
    - 204: 결과 없는 조작. 데이터 요청의 204는 200 `null`로 준다
    - 400·404·409·422·429 + `{"code": 코드, "message": 문구}`: 같은 상태로 `{"error": 문구, "code": "addon_error", "addonCode": 코드}`를 준다. 코드는 `^[a-z][a-z0-9_]{0,31}$`일 때만 넘기고, 문구는 제어 문자와 양방향 제어 문자를 지우고 300자로 자른다(비면 `addon backend error`). 이 문구는 그 패키지 프레임의 `MaruError.message`로 간다
    - 그 밖의 상태, 4MiB 초과, 잘못된 JSON: 502 `addon_error`(`addonCode` 없음). 백엔드가 가짜 401로 사용자를 로그아웃시키거나 호스트 오류 코드를 사칭하지 못하게 상태와 코드를 그대로 넘기지 않는다. 응답 헤더는 넘기지 않는다
- 시간 제한: 연결 2초, 데이터 15초(넘으면 504 `addon_backend_unavailable`), 조작은 manifest의 `timeoutSeconds`(기본 60초, 넘으면 504 `action_timeout`)
- 오류: 연결 실패 503 `addon_backend_unavailable`, 백엔드가 꺼진 설치 503 `addon_backend_disabled`, 백엔드 없는 패키지 404 `addon_no_backend`. 내장 조작 본문에 `input`이 오면 400
- 조작은 내장과 같은 순서로 본다: 백엔드 있음·켜짐 → capability → `decl.Admin` → 쓰기를 선언한 패키지면 단계(`packageWriteLevel`, 모자라면 403 `security_required`) → 진행 중 표시(`inflight`, 409 `action_busy`) → `WithoutCancel` + 시간 제한 → 감사 로그 한 줄(`action: user=%q addon=%s action=%s target=%q result=%s`)
- 백엔드가 호스트 소켓으로 부른 OMV가 401을 주면, 백엔드 응답과 상관없이 바깥 요청이 대시보드 세션을 지우고 401로 끝난다(내장과 같다)
- 구현(`internal/addonpkg/backend.go`의 `BackendClient`·`Translate`, `internal/server/addon_backend.go`). 계약이 정하지 않은 것은 아래처럼 정했다
    - 소켓마다 `http.Transport` 하나를 두되 keep-alive를 끈다(요청마다 새 연결). 백엔드가 닫은 유휴 연결을 다시 쓰다 조작(POST, 다시 보내지 않음)이 실패하지 않게 하고, 지운 패키지의 연결이 남지 않게 하기 위해서다. 레지스트리가 바뀌면 빠진 패키지의 Transport를 버린다(`Retain`). 리디렉션은 따라가지 않는다(3xx는 502)
    - 연결 단계(`DialContext`)의 실패는 제한 시각이 지났어도 503 `addon_backend_unavailable`이다. 연결한 뒤 응답 없이 끊기면 502 `addon_error`다
    - 400·404·409·422·429인데 본문이 JSON 객체가 아니면(빈 본문, 텍스트, `null`, 배열) 위의 "잘못된 JSON"으로 보고 502 `addon_error`다. JSON 객체이면 `code`·`message`가 없거나 형식이 틀려도 같은 상태로 주고, 틀린 값만 버린다(`addonCode` 없음, 기본 문구)
    - 문구에서 지우는 것은 `unicode.IsControl`(Cc)과 `unicode.Bidi_Control`(U+061C, U+200E·U+200F, U+202A~U+202E, U+2066~U+2069)이다. 그 밖의 서식 문자(Cf)는 남긴다
    - 쿼리 문자열은 2KiB 안이고 출력 가능한 ASCII(0x21~0x7E)만 받는다(요청 줄을 깨지 않게, 브라우저는 그 밖을 퍼센트 인코딩한다). 아니면 400
    - 확인 순서는 데이터도 조작과 같다(백엔드 있음 → 켜짐 → capability). 조작 본문의 형식·크기(400)는 백엔드 켜짐 다음, capability 앞에서 본다(내장 조작과 같은 자리)
    - 감사 로그의 `result`는 `ok`, `unauthorized`, 그 밖은 브라우저에 준 오류 코드(`addon_error` 등)다. 토큰은 남기지 않는다
    - 프론트(`hostServices.ts`)는 데이터·조작 요청에 대시보드 화면 언어를 `X-Maru-Lang`으로 보낸다. 데이터의 204도 `null`로 받는다
    - 프론트(`api/client.ts`)는 `addon_error`의 `error`를 `ApiError.addonMessage`(300자까지)로 두고, 브리지(`toBridgeError`)가 그것을 프레임의 `message`로 준다. 다른 오류의 메시지는 `<경로>: HTTP <상태>`다. 이 문구는 호스트 화면(토스트, 대화상자)에 보이지 않는다

### 호스트 소켓

HTTP/1.1, 유닉스 소켓. 요청의 Host는 `maru`다.

- 위치: 운영은 maru가 백엔드 패키지마다 `config.AddonHostDir/<id>/host.sock`(디렉터리 0755, 소켓 0666)을 열고, 유닛 안에서는 `config.AddonUnitHostSocket`(`/run/maru-host/host.sock`)이다. 유닛에는 그 디렉터리만 바인드되므로 다른 애드온의 소켓은 보이지 않는다. 개발은 `<state>/addon-host/<id>.sock`(0600, 같은 uid)이고 백엔드는 `MARU_ADDON_HOST`로 받는다
- 수신: 소켓마다 그 애드온 id에 묶인다. 헤더 읽기 5초, 헤더 8KiB, 유휴 30초, 동시 연결 16개, 동시 요청 8개(넘으면 429 `busy`), 본문 64KiB. 토큰 없는 연결도 받는다(소켓이 0666이다)
- 호출 토큰: 바깥 요청마다 32바이트 무작위 값(base64url)을 만들어 전용 표에 (애드온 id, 대시보드 세션, 범위, 바깥 요청의 호출기)로 둔다. 범위는 데이터 요청이면 `read`, 조작이면 `write`다. 만료는 바깥 요청의 제한 + 5초이고 바깥 요청이 끝나면 지운다. 표는 256개까지이고 가득 차면 바깥 요청이 503 `addon_backend_unavailable`이다. 그래서 백엔드는 요청을 처리하는 동안에만 OMV를 부를 수 있다. 토큰은 로그·응답에 남기지 않는다
- 모든 요청: `POST`, `Content-Type: application/json`, 헤더 `X-Maru-Call: <받은 토큰>`
    - `POST /v1/rpc` `{"service", "method", "params"}` → 200 `{"result": OMV 결과}`. 그 애드온이 선언한 메서드만 부른다(`rpcCaller{addon: id}`, `read` 범위면 읽기만, `write` 범위면 선언한 쓰기도)
    - `POST /v1/rpc-bg` `{"service", "method", "params", "output": 불}` → 200 `{"output": 문자열, "truncated": 불}`. 관리자 세션만. 선언한 메서드(범위 규칙은 `/v1/rpc`와 같다)를 부르고, 결과가 상태 파일 이름(`^/tmp/bgstatus[A-Za-z0-9]+$`)이어야 한다(아니면 502 `omv_error`). 그 이름에만 `Exec.isRunning`·`getOutput`을 쓰는 내부 호출기로 `addons.WaitBg`(`output`이 거짓)나 `addons.ReadBg`(1MiB까지)로 끝까지 기다린다
    - `POST /v1/apply` `{"calls": [{"service", "method", "params"}…]}`(16개까지) → 200 `{"results": [OMV 결과…]}`. `write` 범위만, 호출은 모두 선언한 쓰기여야 한다. `addons.ApplyChange`(잠금, 적용 대기가 있으면 409 `config_dirty`, `Config.applyChangesBg` → `WaitBg`)로 실행한다
    - `rpc-bg`·`apply`는 금지 표의 `Exec`·`Config`를 쓰므로 그 처리기 안에서만 만드는 내부 호출기(선언한 메서드 + 고정 보조 메서드)를 쓴다. `/v1/rpc`는 선언하지 않은 메서드를 모두 거부하므로 `Config.*`·`Exec.*`도 부를 수 없다
- 오류는 `{"error": 문구, "code": 코드}`다
    - 400 `bad_request`: 본문 형식, 크기, 개수
    - 401 `call_invalid`: 토큰이 없음·모름·만료·다른 애드온
    - 401 `unauthorized`: OMV 세션이 끝남. 바깥 요청이 대시보드 세션을 지운다
    - 403 `rpc_forbidden`: 선언하지 않은 메서드, `read` 범위의 쓰기, `read` 범위의 `apply`
    - 403 `admin_required`: 관리자가 아닌 세션의 `rpc-bg`
    - 409 `config_dirty`, 429 `busy`
    - 502 `omv_error`: OMV 오류. 문구는 OMV 오류 문구(1KiB까지, `trace`는 넣지 않는다)
    - 503 `omv_unavailable`, 504 `timeout`(바깥 요청의 제한이 지남)
- 백엔드 프로세스의 환경: 운영은 소켓 활성화(`LISTEN_FDS=1`, `LISTEN_PID`, fd 3)와 `STATE_DIRECTORY`(systemd), 개발은 `MARU_ADDON_SOCKET`(백엔드가 받을 소켓 경로), `MARU_ADDON_HOST`, `MARU_ADDON_STATE`(선택, 데이터 디렉터리)
- 구현(`internal/server/addon_host.go`). 계약이 정하지 않은 것은 아래처럼 정했다
    - 소켓은 레지스트리에 들어간 백엔드 패키지(켜져 있고 받은 설치본, 개발본)마다 하나다. `addonpkg.Manager.OnChange`가 레지스트리를 바꿀 때마다(Manager의 잠금을 쥔 채로) 맞추고, `server.NewServer`가 돌려준 `Server.Close`가 모두 닫는다(소켓 파일도 지운다). 열지 못한 소켓은 로그만 남기고 다음 변경 때 다시 연다. 남은 소켓 파일은 `cmd/maru`의 `listenOn`과 같은 규칙으로 지운다(소켓이 아닌 파일, 쓰고 있는 소켓은 그대로)
    - 위치·권한은 `server.Config.AddonHost`(`addonpkg.BackendPaths`, nil이면 꺼짐)가 정한다. 개발은 `addonpkg.DevBackendPaths`, 운영 경로(`InstalledBackendPaths`)는 M3에서 쓴다
    - 확인 순서: 경로(모르면 404 `not_found`)·메서드(POST가 아니면 405 `bad_request`) → 토큰(그 애드온의 것) → OMV 세션이 이미 끝났으면 401 `unauthorized`(OMV에 더 보내지 않는다) → 동시 요청(429) → `Content-Type` → 본문. 토큰 없는 요청은 동시 요청 칸을 쓰지 않는다
    - 본문은 처리기가 시작한 뒤 5초 안에 읽어야 한다(처리기 첫 줄의 `ResponseController.SetReadDeadline`, 본문을 다 읽으면 푼다). 서버의 `ReadTimeout`은 오래 도는 요청(rpc-bg)의 문맥을 끝내므로 쓰지 않는다. 제한 시각을 본문 읽기 직전이 아니라 처리기 첫 줄에 거는 것은, `net/http`가 처리기가 읽지 않은 본문을 응답 전후에 읽어 버리기 때문이다(제한 시각이 없으면 본문을 보내지 않고 거절받은 연결이 동시 연결 칸을 끝없이 쥔다)
    - 본문을 읽기 전의 거절(404, 405, 401 `call_invalid`·`unauthorized`, 429, `Content-Type`의 400)과 본문을 읽지 못한 400에는 `Connection: close`를 붙인다. 응답은 바로 가고, 남은 본문은 위 제한 시각까지만 읽은 뒤 연결을 닫는다. 그래서 토큰 없는 연결 하나가 칸을 쥐는 시간은 헤더 읽기 5초 또는 본문 읽기 5초까지다. 소켓이 0666이 되는 M3에서 연결을 계속 다시 맺는 로컬 클라이언트는 이 시간만큼씩 칸을 쥘 수 있으므로, 그때 연결한 uid 확인(`SO_PEERCRED`) 같은 방어를 따로 정한다
    - 헤더 8KiB는 요청 줄과 헤더를 합친 크기다. `net/http`는 `MaxHeaderBytes`에 4096바이트를 더해 읽으므로 `MaxHeaderBytes`를 `8<<10 - 4096`으로 둔다(넘으면 431)
    - 본문은 엄격하게 읽는다(모르는 필드, 뒤에 남은 값은 400). `/v1/apply`의 `calls`는 1~16개다
    - 호스트 요청의 문맥은 바깥 요청의 문맥이 끝나면(제한 시각, 바깥 요청 완료) 함께 끝난다(504 `timeout`)
    - 토큰 만료는 문맥의 제한 시각(실제 시각)과 비교하므로 서버의 `Config.Now`(테스트 시계)가 아니라 `time.Now`를 쓴다. 대시보드 세션이 끝나면 그 세션의 토큰도 지운다
    - `/v1/rpc`·`/v1/rpc-bg`·`/v1/apply`의 메서드는 `rpcCaller.Call`(그 애드온의 선언, 범위)에 앞서 금지 표와 이름 형식을 한 번 더 본다
    - `/v1/apply`의 호출 하나가 실패하면 적용하지 않고 그 오류를 준다. 앞의 호출이 바꾼 설정은 적용 대기로 남는다(내장 `ApplyChange`와 같다). 관리자가 아닌 세션은 OMV가 `Config.isDirty`를 거절해 502 `omv_error`다
    - rpc-bg는 `bgCaller`(받은 상태 파일 이름의 `Exec.isRunning`·`getOutput`만), apply는 `applyCaller`(`Config.isDirty`, `Config.applyChangesBg`, 그것이 돌려준 이름의 `Exec.isRunning`만)를 그 처리기 안에서만 만든다. 둘 다 바깥 요청의 호출기로 OMV를 불러 401 표시를 함께 쓴다

### 켜짐과 꺼짐

- 운영에서 백엔드가 켜져 있는지는 `config.AddonSyncStatusFile`이 있는지로 안다(M3의 설치기 설정 `--addon-backends`). 그 전까지와 꺼진 설치에서는 백엔드 패키지를 설치할 수는 있지만 capability가 `backend_unavailable`이고, 데이터·조작은 503 `addon_backend_disabled`이며, 검토 화면에 경고 `backends_disabled`가 보인다
- 개발 서버는 `MARU_DEV_ADDON_BACKENDS=1`(개발 전용 환경 변수, `scripts/dev.sh`가 `make dev`·`make dev-fake` 모두에 넣는다)이면 위의 개발 경로로 켠다. 상태 디렉터리가 있어야 하고(없으면 로그를 남기고 꺼짐), `<state>/addon-dev`·`<state>/addon-host`를 0700으로 만든다(`cmd/maru/devaddons.go`). 서버의 `Config.AddonHost`와 패키지 관리자의 `Options.BackendsEnabled`는 같은 값에서 정한다
- M2의 운영 실행 파일은 백엔드가 늘 꺼져 있다(`config.AddonSyncStatusFile`로 켜는 것은 M3)

## 개발 루프

- 저장소 체크아웃이 필요하다(`make dev`, `make dev-fake`)
- `MARU_DEV_ADDONS`: 콜론으로 나눈 풀린 패키지 디렉터리. 개발 전용이며 플래그·가동 설정·설치기에 넣지 않는다. maru와 가짜 OMV는 `backend/`에서 실행되므로 `scripts/dev.sh`가 항목마다 부른 디렉터리 기준의 절대 경로로 바꾸고, 디렉터리가 아닌 항목은 알리고 뺀다. capabilities를 받을 때 manifest와 `ui/`의 mtime을 보고 다시 읽으며 rev는 `dev-<해시>`다. 관리자 페이지에 "개발"로 보이고 바꿀 수 없다. 제목 표시줄 메뉴에 "다시 불러오기"가 있다
- `FAKEADDONS=examples`(선택, 기본 `none`): `make dev-fake`가 `sdk/examples`를 `MARU_DEV_ADDONS` 뒤에 더하고 백엔드 예제의 백엔드를 띄운다(아래 "예제와 시험 패키지")
- `MARU_DEV_ADDON_SDK`: `scripts/dev.sh`가 `vite build -c vite.sdk.config.ts --watch`로 만든 디렉터리. 프레임의 `_maru/`를 여기서 준다(embed된 빌드는 개발 중에 없거나 오래되었다)
- 개발 백엔드: 개발자가 `MARU_ADDON_SOCKET=<state>/addon-dev/<id>.sock`과 `MARU_ADDON_HOST=<state>/addon-host/<id>.sock`(선택 `MARU_ADDON_STATE`)으로 백엔드를 띄운다. maru는 `MARU_DEV_ADDON_BACKENDS=1`일 때 그 호스트 소켓을 열고 백엔드 소켓에 붙는다. 백엔드가 떠 있지 않으면 데이터·조작은 503 `addon_backend_unavailable`이다. 개발 패키지는 실행 파일(`backend/linux-*`) 없이도 불러온다(`ReadDir`의 `needBackend=false`). 상태 디렉터리 경로가 길면 소켓 경로가 유닉스 소켓 한도(107바이트)를 넘어 SDK가 시작하지 않는다(`helper.sock`과 같은 주의)
- 예제와 시험 패키지: UI 패키지 `sdk/examples/<id>/`(또는 시험 패키지 `sdk/testdata/<id>/`)의 백엔드 소스는 `sdk/go/examples/<id>/`(`sdk/go` 모듈 안, `go.mod` 없음, 모든 `.go` 첫 줄 SPDX MIT, `TestLicense`)다. 그래서 `cd sdk/go && go vet ./... && go test ./...`(`make lint`·`make test`)가 예제도 검사한다
    - `make sdk-examples`(`build`에 넣지 않음): 백엔드 예제마다 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64|arm64 go build -trimpath`로 정적 실행 파일을 그 UI 패키지의 `backend/linux-*`에 둔다. 결과물은 `.gitignore`(`/sdk/examples/*/backend/`, `/sdk/testdata/*/backend/`)다. 실행하지 않아도 개발 서버는 예제를 불러온다
    - `scripts/dev.sh`: `FAKEADDONS=examples`(가짜 OMV)이고 `MARU_DEV_ADDON_BACKENDS=1`이면 `sdk/examples/<id>` 가운데 `sdk/go/examples/<id>/*.go`가 있는 것만 `go run`(`setsid`, 다른 개발 프로세스와 같은 정리)으로 띄운다. 소켓은 위의 개발 경로, 데이터 디렉터리는 `<state>/addon-state/<id>`다. `addon-dev`·`addon-host`는 maru보다 먼저 만든다(SDK는 소켓 디렉터리를 만들지 않는다). `MARU_DEV_ADDONS`로 더한 패키지와 `sdk/testdata/probe`의 백엔드는 띄우지 않는다
    - `hello-backend`(예제): 데이터는 `Host.RPCInto`의 `System.getInformation`과 관리자에게만 `PerfStats.get`(관리자 전용 메서드라 `r.Admin`으로 가른다), 백엔드가 데이터 디렉터리(없으면 메모리)에 두는 데이터 요청 수와 마지막 변경 기록(관리자에게만). 관리자·확인 조작 `perfstats`가 `Host.Apply`로 `PerfStats.set {enable}`을 바꾸고 적용한다(`timeoutSeconds: 300`, 다시 켤 때 그래프를 다시 그린다). 적용 대기가 있으면 호스트의 `config_dirty`를 409 `config_dirty`(`addonCode`)로 바꾼다
    - 쓰기를 `PerfStats.set`으로 고른 이유: OMV 코어(`openmediavault` 패키지의 `engined/rpc/perfstats.inc`, 8.0부터 같다. `.claude/rules/auth-omv.md`의 "OMV 8 버전별 차이"), 파라미터가 불 하나(`rpc.perfstats.set`), 같은 창에서 되돌릴 수 있다. 적용하면 collectd·rrdcached·monit 모듈만 다시 배포하고(성능 그래프가 그동안 멈출 뿐 쌓인 RRD는 남는다) 네트워크·인증·사용자·저장소·웹 UI·접속을 끊는 서비스에 닿지 않는다. 금지 표 밖이다. 가짜 OMV는 `serviceRPC`의 `PerfStats`(모듈 `collectd`)로 흉내 낸다
    - `probe`(시험 도구, `sdk/testdata/probe`, `requires.admin`): 관리자 `Query` 조작 `run`이 백엔드가 하지 못해야 하는 일(상태 디렉터리 안의 `users`·`security`·`site.json`, root 동기화 기록 `/var/lib/maru-addons`, `/run/maru`, 다른 애드온의 소켓 디렉터리 `/run/maru-addon`·`/run/maru-addon-host`, 도우미 소켓, 시스템 D-Bus, resolved varlink, OMV `config.xml`, `/srv`·`/home`, 보이는 추상 유닉스 소켓, `127.0.0.1:80`, 입력한 LAN 주소, `1.1.1.1:443`, 토큰 없는 호스트 호출)을 하나씩 시도해 표를 돌려준다. 행에는 `reachable`·`blocked`·`timeout`·`error`·`skipped`와 오류 종류(errno 이름, `call_invalid`, `http_<상태>`, `none_visible`)만 담고 읽은 내용은 담지 않는다. `EPERM`(IPAddressDeny)·`EAFNOSUPPORT`(RestrictAddressFamilies)·`EACCES`·`ENOENT`·`ENOTDIR`는 `blocked`, `ECONNREFUSED`와 연결 성공은 `reachable`, 시간 초과는 `timeout`(샌드박스 차단과 구별), `ENETUNREACH` 등은 `error`다. 입력의 `network`(manifest와 같게)가 기대 결과를 정하고, 네트워크가 있는 단계의 추상 소켓은 알려진 한계라 판정하지 않는다. 개발 서버에서는 대부분 닿는 것이 정상이고, M3의 VM 확인에 쓴다
- `FAKEOMV_EXTRA_RPC`(`make dev-fake`): `scripts/dev.sh`가 `MARU_DEV_ADDONS`(예제를 더한 뒤) 디렉터리의 `dev/fake-rpc.json`을 콜론 목록으로 모아 가짜 OMV 프로세스에 넘긴다(셸에서 준 값 뒤에 더하고, 셸에서 준 상대 경로는 부른 디렉터리 기준의 절대 경로로 바꾼다. 읽지 못한 파일이 있으면 가짜 OMV가 시작하지 않는다). 형식은 `{"Service.method": {"result": <JSON>, "admin": true}}`(`result` 필수, `admin`이면 관리자 세션만, 모르는 필드·`Session` 서비스는 오류, `omvfake.ParseExtraRPC`)다. 가짜 OMV가 구현하지 않은 메서드에만 답하고(구현이 이긴다) 세션 검사는 그대로다. 여러 파일이 같은 메서드를 적으면 앞 파일의 것을 쓰고 로그를 남긴다. 그 밖의 플러그인 RPC는 실제 OMV(`make dev`)에서 확인한다

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

브라우저(dev-fake)와 VM(desk-test)에서 확인한 범위와 날짜, 확인하다 바뀐 결정을 적는다.

- 자동 시험(M2): 예제 백엔드는 `sdk/go/examples/*`의 Go 시험이 SDK의 `ServeContext`와 가짜 호스트 소켓(임시 유닉스 소켓)으로 본다(네트워크·실제 경로 없음, `probe`는 경로·연결 함수를 바꿔 끼운다). 서버 쪽은 `TestAddonSDKExamples`(실행 파일 없이 개발 디렉터리로 불러옴)와 `TestAddonSDKBackendExample`(`hello-backend`의 manifest 선언으로 가짜 백엔드가 `System.getInformation`·`PerfStats.get`·`/v1/apply`의 `PerfStats.set`을 omvfake까지 부름, 일반 사용자·단계 없음 거절), `addonpkg`의 `TestReadDirBackendWithoutExecutable`(실행 파일 없는 백엔드 패키지와 `dev/`), CLI 시험(`init --backend`, `dev/` 건너뛰기)이다
- 아직 하지 않음(M2): 브라우저에서 `FAKEADDONS=examples`의 `hello-backend` 데이터와 쓰기 조작(호스트 확인 → 비밀번호 재확인 → 실행 → 데이터 다시 읽기, 일반 사용자에게는 성능 통계 칸이 없음, 백엔드를 멈추면 창에 연결 실패 문구), 실제 OMV에서 `PerfStats.set` 적용(사용자 확인), VM에서 `probe`의 네트워크 단계 4개(M3)

- 2026-11-05, 크롬(Playwright, dev-fake, 개발 패키지 `sdk/testdata/dev-min`)
    - 샌드박스 프레임 문서에서 CSP `'self'`는 문서 주소의 출처로 풀린다. `script-src 'self'`가 `./_maru/sdk.js`와 패키지의 모듈 스크립트를 허용한다(모듈 로드는 `Access-Control-Allow-Origin: *`로 된다)
    - 프레임 안 `document.cookie`·`localStorage`는 `SecurityError`를 던진다(불투명 출처). 외부 fetch·같은 출처 fetch는 `connect-src 'none'`, 외부 이미지는 `img-src`로 막힌다. 같은 출처 이미지도 쿠키가 붙지 않아 API에서 401이다
    - 호스트가 받는 `event.origin`은 `'null'`이다
    - 프레임이 스스로 `https://example.com/`으로 이동하면 대시보드의 `frame-src 'self'`가 막는다. 그 뒤의 두 번째 `load`로 프레임을 다시 만든다
    - 다른 창 뒤의 패키지 창 프레임을 누르면 그 창이 앞으로 오고 `document.activeElement`가 프레임이 된다(합성 `pointerdown`)
    - 호스트 테마를 바꾸면 프레임에 바로 반영된다(context). 위젯 프레임은 `inert`·`pointer-events: none`이고 배경이 투명하다
    - CSP3 `webrtc 'block'`은 크롬이 모르는 지시어라 콘솔 오류를 내서 뺐다
- 2026-11-05, 크롬(Playwright, dev-fake, 설정 앱과 `sdk/examples/hello-widget`, CLI로 묶은 패키지)
    - 설정 앱: 올리기 → 검토(권한, 업데이트 차이, `author_changed` 경고) → 관리 창(비밀번호) → 설치·업데이트, 끄기·다시 켜기, 제거. 열린 패키지 창은 끄면 "쓸 수 없는 애드온"이 되고 다시 켜면 프레임이 돌아온다. 거절 이유(`reserved_id`, `rpc_denied`, `arch_missing`)와 영어 설명이 보인다. 관리 확인을 취소하면 검토 대기를 버린다
    - `maru-addon init`(JS, `--ts` + Vite 8 빌드)으로 만든 패키지를 `pack`해 올리면 설치되고 창이 그려진다. Vite가 붙이는 `<script type="module" crossorigin>`도 `Access-Control-Allow-Origin: *`로 로드된다. `vite-ignore`를 단 `_maru/` 참조는 그대로 남는다
    - 사용자 조작 판정: 프레임 스크립트가 스스로 부른 `openWorkbench`는 `bridge_gesture_required`, 프레임의 단추를 누르면 받는다(워크벤치 안내 대화상자 뒤 새 탭)
    - `FAKEOMV_TIMEOUT=1m`에서 프레임 안 키 입력만으로 90초 넘게 세션이 이어지고, 입력을 멈추면 만료된다
    - 프레임 샌드박스(`allow-forms` 없음)는 폼 제출을 막고 `submit` 이벤트도 일어나지 않는다. 샌드박스를 넓히지 않고 개발 문서에 적었다(단추·Enter를 직접 처리)
    - 확인(`confirm`)은 "<이름> 애드온의 요청" 제목, 알림(`notify`)은 "애드온 <이름>" 제목으로 보인다. 저장(`storage`)은 사용자별로 남는다
    - 대시보드의 `frame-src 'self'` 아래에서도 파일 미리보기의 PDF(같은 출처 iframe, 크롬 내장 뷰어)가 그려진다
    - 프레임 주소를 새 탭에서 바로 열어도 출처가 `null`이고 `document.cookie`·`localStorage`가 `SecurityError`다(응답 CSP의 `sandbox`)
