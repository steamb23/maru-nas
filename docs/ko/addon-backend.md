# 애드온 백엔드

패키지에는 백엔드를 넣을 수 있습니다. 백엔드는 서버에서 대시보드와 함께 도는 프로그램입니다. 패키지 화면이 `app.data()`, `app.action()`, `app.query()`로 백엔드를 부르고, 백엔드는 manifest에 밝힌 OMV 메서드를 대시보드를 거쳐 부릅니다. OMV 쓰기(조작)가 필요하거나, 여러 OMV 호출을 모아 한 번에 답하거나, 서버에 데이터를 두거나, `/proc` 같은 시스템 파일을 읽어야 하는 패키지에 씁니다.

> [!NOTE]
> `install.sh`로 설치한 대시보드는 관리자가 애드온 백엔드를 켠 설치(`./install.sh --addon-backends`)에서만 백엔드를 실행합니다. 켜지 않은 설치도 백엔드가 있는 패키지를 받지만 그 앱과 위젯은 "쓸 수 없음"으로 보입니다([설치본의 백엔드](#설치본의-백엔드)).

## 동작 방식

```
패키지 화면     app.data(), app.action(), app.query()
  → 대시보드     세션, manifest, 사용자 권한을 확인한다
    → 백엔드     GET /v1/data, POST /v1/actions/<이름>
      → 대시보드 호스트 소켓   POST /v1/rpc, /v1/rpc-bg, /v1/apply
        → OMV    로그인한 사용자의 OMV 세션으로
```

- 대시보드는 유닉스 소켓 위의 HTTP/1.1로 백엔드와 주고받습니다. 브라우저는 백엔드에 직접 연결하지 않습니다.
- 요청마다 로그인한 사용자가 누구인지, OMV 관리자인지, 어떤 화면 언어를 쓰는지와 그 요청의 호출 토큰을 백엔드에 알려 줍니다.
- 백엔드는 호출 토큰을 대시보드의 호스트 소켓에 보내 OMV를 부릅니다. 대시보드가 로그인한 사용자의 OMV 세션으로 대신 부르므로, 백엔드는 OMV 세션 ID나 비밀번호를 보지 못하고 그 사용자의 OMV 권한을 넘지 못합니다.
- manifest의 `rpc.read`(데이터 요청과 조작)와 `rpc.write`(조작만)에 밝힌 메서드만 부를 수 있습니다. 패키지에 허용하지 않는 메서드([애드온 보안](addon-security.md#허용하지-않는-메서드))는 처음부터 선언할 수 없습니다.
- 대시보드의 모든 사용자가 백엔드 프로세스 하나를 함께 씁니다.

## 백엔드 선언

```json
{
  "rpc": {
    "read": ["System.getInformation", "PerfStats.get"],
    "write": ["PerfStats.set"]
  },
  "actions": [
    {
      "name": "perfstats",
      "admin": true,
      "confirm": true,
      "timeoutSeconds": 300,
      "label": { "en": "Change statistics", "ko": "통계 설정 바꾸기" },
      "confirmText": { "en": "Change OMV's performance statistics setting and apply it now?" }
    }
  ],
  "backend": { "network": [] }
}
```

- `backend`가 있으면 백엔드가 있는 패키지입니다. `network`는 설치본의 백엔드가 샌드박스 안에서 연결할 수 있는 곳이고, 빈 배열이면 네트워크가 없습니다([샌드박스](#샌드박스)).
- `actions`와 `rpc.write`는 백엔드가 있어야 씁니다. 필드 설명은 [manifest](addon-manifest.md#조작과-백엔드)에 있습니다.
- 실행 파일은 패키지의 `backend/linux-amd64`, `backend/linux-arm64`에 둡니다([묶기와 배포](addon-packaging.md#파일-구성)).

## Go SDK

Go SDK(모듈 `github.com/steamb23/maru-nas/sdk/go`, 패키지 `maruaddon`)가 프로토콜의 백엔드 쪽을 구현합니다. 서버, 요청 헤더, 응답과 오류 형식, 호스트 소켓 클라이언트가 들어 있습니다. MIT 라이선스이고 Go 표준 라이브러리만 쓰며 Go 1.23 이상으로 빌드합니다. 전체 API는 [SDK README](https://github.com/steamb23/maru-nas/blob/main/sdk/go/README.md)(영어)에 있습니다.

모듈은 아직 공개하지 않았습니다. 첫 공개 릴리스 전까지는 마루 NAS 저장소 체크아웃에서 백엔드의 `go.mod`에 `replace` 지시어를 두고 씁니다. `maru-addon init <디렉터리> --backend`가 그 `go.mod`를 만들어 줍니다([개발](#개발)).

### 가장 작은 백엔드

```go
package main

import (
	"context"
	"log"

	"github.com/steamb23/maru-nas/sdk/go/maruaddon"
)

func main() {
	err := maruaddon.Serve(maruaddon.Handler{
		// 화면의 app.data(): GET /v1/data
		Data: func(ctx context.Context, r *maruaddon.Request) (any, error) {
			var info struct {
				Hostname string `json:"hostname"`
			}
			// manifest의 rpc.read에 밝힌 메서드만 부를 수 있다.
			if err := r.Host.RPCInto(ctx, "System", "getInformation", nil, &info); err != nil {
				return nil, err
			}
			return map[string]string{"hostname": info.Hostname, "user": r.User}, nil
		},
		// 화면의 app.action('reset'): POST /v1/actions/reset
		Actions: map[string]maruaddon.HandlerFunc{
			"reset": func(ctx context.Context, r *maruaddon.Request) (any, error) {
				return nil, nil // 204, 결과 없음
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

`Serve`는 환경이 정한 소켓에서 받고, SIGTERM이나 SIGINT를 받을 때까지 요청에 답한 뒤 진행 중인 요청을 60초까지 기다립니다.

### 빌드

아키텍처마다 정적 실행 파일을 패키지의 `backend/`에 빌드합니다.

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o ../addon/backend/linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o ../addon/backend/linux-arm64 .
```

`CGO_ENABLED=0`이면 실행 파일이 NAS에 설치된 라이브러리에 기대지 않습니다.

### 데이터와 조작

`Handler.Data`가 데이터 요청에, `Handler.Actions`가 이름별로 조작에 답합니다. 처리기는 `*maruaddon.Request`를 받습니다.

| 필드 | 내용 |
|---|---|
| `User` | 로그인한 사용자의 OMV 사용자 이름 |
| `Admin` | 사용자가 OMV 관리자인지 |
| `Lang` | 대시보드의 화면 언어(`en`, `ko`, `zh-Hans` 등. 대시보드가 지원하지 않는 언어면 `en`) |
| `Query` | 데이터 요청의 쿼리(`app.data({ name: 'x' })`는 `name=x`) |
| `Action`, `Target`, `Input` | 조작 이름, 대상 문자열, 입력 객체(원시 JSON). `r.DecodeInput(&v)`가 입력을 엄격하게(모르는 필드는 오류) 풉니다 |
| `Host` | 이 요청 대신 OMV를 부릅니다(아래). 처리기가 도는 동안만 쓸 수 있습니다 |

| 처리기 결과 | 화면이 받는 것 |
|---|---|
| 데이터 `(v, nil)` | `v`의 JSON(`nil`은 `null`) |
| 조작 `(nil, nil)` | 없음(`app.action()`이 끝납니다) |
| 조작 `(v, nil)` | `v`의 JSON(`app.query()`가 이 값으로 끝납니다) |
| 상태 400, 404, 409, 422, 429의 `maruaddon.NewError(status, code, message)` | 그 상태의 `MaruError`. `code`는 `addon_error`, `addonCode`는 백엔드의 코드, `message`는 백엔드의 문구입니다([화면 만들기](addon-frontend.md#오류)) |
| 그 밖의 오류, panic | `code`가 `addon_error`이고 `addonCode`가 없는 `MaruError`. 오류는 표준 오류로 나갑니다 |

코드는 `^[a-z][a-z0-9_]{0,31}$`에 맞아야 합니다. 대시보드는 문구를 번역하지 않으므로 영어로 쓰거나 `r.Lang`을 보고 고릅니다.

### OMV 부르기

`r.Host`는 요청이 도는 동안 대시보드를 거쳐 OMV를 부릅니다.

| 메서드 | 쓰임 |
|---|---|
| `RPC(ctx, service, method, params)` | 선언한 메서드를 부르고 결과를 원시 JSON으로 돌려줍니다 |
| `RPCInto(ctx, service, method, params, &out)` | 같은 호출이고 결과를 `out`에 풉니다(`out`에 없는 필드는 무시) |
| `RPCBg(ctx, service, method, params, output)` | OMV에서 백그라운드로 도는 선언한 메서드(백그라운드 상태 파일 이름을 돌려주는 메서드)를 부르고 끝날 때까지 기다립니다. `output`이 참이면 작업 출력(1MiB까지)을 돌려줍니다. 관리자 세션만 |
| `Apply(ctx, calls...)` | 선언한 쓰기 호출(`maruaddon.Call{Service, Method, Params}`)을 16개까지 실행한 뒤 설정 변경을 적용하고, 적용이 끝날 때까지 기다립니다. 조작에서만 |

- 데이터 요청에서는 `rpc.read`의 메서드만 부를 수 있습니다. 조작에서는 `rpc.write`의 메서드도 부를 수 있습니다.
- OMV 설정을 바꾸는 메서드는 `Apply`로 부르세요. `RPC`로 부른 쓰기는 OMV 설정 DB에 변경만 남기고 적용 대기로 두므로, 누군가 워크벤치에서 적용할 때까지 설정이 반영되지 않습니다.
- OMV에 이미 적용 대기가 있으면 `Apply`는 `config_dirty` 코드로 거절합니다. 다른 사람이 만든 변경을 패키지가 대신 배포하지 않게 하기 위해서입니다. 호출 하나가 실패하면 적용하지 않고, 앞 호출이 바꾼 설정은 적용 대기로 남습니다.
- OMV는 관리자 세션의 적용만 받으므로 `Apply`를 부르는 조작은 관리자 전용(`"admin": true`)으로 둡니다.
- 대시보드가 준 오류는 `*maruaddon.HostError`이고 `maruaddon.IsCode(err, "config_dirty")`로 확인합니다([호스트 소켓의 오류](#호스트-소켓의-오류)). 그대로 돌려주면 500이 되어 화면은 `addonCode` 없는 `addon_error`를 받습니다. 화면에 이유를 보이려면 직접 만든 오류를 돌려줍니다.

```go
// http는 net/http다.
_, err := r.Host.Apply(ctx, maruaddon.Call{Service: "PerfStats", Method: "set", Params: map[string]bool{"enable": in.Enable}})
if maruaddon.IsCode(err, "config_dirty") {
	return nil, maruaddon.NewError(http.StatusConflict, "config_dirty",
		"OMV has pending configuration changes; apply or revert them in the OMV workbench first")
}
if err != nil {
	return nil, err
}
```

사용자의 OMV 세션이 끝났으면 호스트가 `unauthorized`로 답하고, 백엔드가 무엇으로 답하든 대시보드가 그 사용자를 로그아웃시킵니다.

### 데이터 디렉터리

`maruaddon.StateDir()`는 백엔드 자신의 데이터 디렉터리이고, 없으면 빈 문자열입니다. 모든 사용자가 함께 쓰며, 업데이트해도 남고, 관리자가 패키지를 제거할 때 지울 수 있습니다. 지웠다가 다시 설치한 패키지는 빈 디렉터리에서 시작합니다([데이터와 제거](#데이터와-제거)). 파일은 작게 두고 원자적으로 씁니다(임시 파일에 쓴 뒤 이름 바꾸기).

## 다른 언어를 위한 프로토콜

백엔드를 꼭 Go로 만들 필요는 없습니다. 다른 언어의 백엔드는 아래 요청을 구현합니다. Go SDK도 이 계약을 따릅니다.

### 대시보드가 보내는 요청

유닉스 소켓 위의 HTTP/1.1이고 `Host: addon`입니다.

| 요청 | 내용 |
|---|---|
| `GET /v1/data?<쿼리>` | 데이터 요청. 쿼리 문자열은 화면이 `app.data()`에 준 값이고, 출력 가능한 ASCII로 2KiB까지입니다 |
| `POST /v1/actions/<이름>` | 조작. `Content-Type: application/json`, 본문 `{"target": 문자열, "input": 객체 또는 null}`. `input`은 8KiB까지의 JSON 객체입니다 |

모든 요청에 아래 헤더가 붙습니다.

| 헤더 | 내용 |
|---|---|
| `X-Maru-User` | 로그인한 사용자의 OMV 사용자 이름 |
| `X-Maru-Admin` | OMV 관리자면 `1`, 아니면 `0` |
| `X-Maru-Lang` | 대시보드의 화면 언어. 대시보드가 지원하는 언어이고 아니면 `en` |
| `X-Maru-Call` | 호스트 소켓에 쓸 호출 토큰. 로그에 남기지 마세요 |

모르는 경로와 메서드에도 JSON 오류 본문으로 답합니다(예: 404 `{"code": "not_found", "message": "not found"}`). 텍스트 본문의 404는 잘못된 응답으로 봅니다.

### 응답

| 백엔드의 응답 | 화면이 받는 것 |
|---|---|
| 200과 JSON 값(4MiB까지) | 그 값(데이터, 또는 `app.query()`의 결과) |
| 204 | 조작이면 없음, 데이터 요청이면 `null` |
| 400, 404, 409, 422, 429와 JSON 객체 `{"code": 문자열, "message": 문자열}` | 같은 상태의 `MaruError`. `code`는 `addon_error`, `addonCode`는 백엔드의 코드(`^[a-z][a-z0-9_]{0,31}$`에 맞을 때만), `message`는 백엔드의 문구에서 제어 문자와 양방향 제어 문자를 지우고 300자로 자른 것(비면 `addon backend error`) |
| 그 밖의 상태, JSON 객체 본문이 없는 위 상태, 4MiB를 넘는 응답, 잘못된 JSON, 리디렉션 | `addonCode` 없는 502 `addon_error` |

대시보드는 백엔드의 응답 헤더를 브라우저에 넘기지 않고, 백엔드의 401·403·5xx를 그대로 넘기지 않습니다. 그래서 백엔드가 사용자를 로그아웃시키거나 대시보드 자신의 오류 코드를 흉내 낼 수 없습니다.

### 시간 제한

- 백엔드 연결: 2초. 백엔드가 떠 있지 않으면 데이터 요청과 조작은 503 `addon_backend_unavailable`로 실패합니다.
- 데이터 요청: 15초. 넘으면 504 `addon_backend_unavailable`입니다.
- 조작: 그 조작의 `timeoutSeconds`(없으면 60초, 600초까지). 넘으면 504 `action_timeout`입니다. 조작은 사용자가 창을 닫아도 끝나거나 시간 제한에 이를 때까지 돕니다. 패키지와 대상마다 조작은 한 번에 하나만 돕니다(409 `action_busy`).

### 호스트 소켓

유닉스 소켓 위의 HTTP/1.1이고 `Host: maru`입니다. 설치본의 백엔드는 `/run/maru-host/host.sock`으로 닿고, 개발 서버에서는 경로가 `MARU_ADDON_HOST`에 있습니다. 백엔드마다 자기 호스트 소켓만 보입니다.

요청은 모두 `POST`이고 `Content-Type: application/json`과 `X-Maru-Call: <처리 중인 요청의 토큰>`을 붙입니다. 본문은 64KiB까지이고 엄격하게 읽습니다(모르는 필드는 오류).

| 경로 | 본문 | 응답 |
|---|---|---|
| `/v1/rpc` | `{"service", "method", "params"}` | `{"result": <OMV 결과>}`. 선언한 읽기 메서드, 조작 중이면 선언한 쓰기 메서드도 |
| `/v1/rpc-bg` | `{"service", "method", "params", "output": 불}` | `{"output": 문자열, "truncated": 불}`. 관리자 세션만. 메서드의 결과가 백그라운드 상태 파일 이름이어야 하고, 대시보드가 작업이 끝날 때까지 기다린 뒤 `output`이 참이면 출력을 1MiB까지 돌려줍니다 |
| `/v1/apply` | `{"calls": [{"service", "method", "params"}, …]}`, 호출 1~16개 | `{"results": [<OMV 결과>, …]}`. 조작에서만, 호출은 모두 선언한 쓰기 메서드여야 합니다. 대시보드가 호출을 실행하고 설정 변경을 적용한 뒤 적용이 끝날 때까지 기다립니다 |

- 호출 토큰은 대시보드가 백엔드에 보낸 요청이 끝날 때까지만 쓸 수 있습니다. 데이터 요청의 토큰은 읽기, 조작의 토큰은 쓰기 토큰이고, 다른 패키지의 토큰은 거절합니다.
- 한도: 헤더는 5초 안에 8KiB까지, 본문은 5초 안에 보내야 하고, 유휴 연결은 30초 뒤 닫습니다. 동시 연결 16개, 동시 요청 8개까지이며 넘는 요청은 429 `busy`입니다.

### 호스트 소켓의 오류

오류는 `{"error": 문구, "code": 코드}`입니다.

| 상태 | 코드 | 뜻 |
|---|---|---|
| 400 | `bad_request` | 본문 형식이 틀렸거나 너무 크거나 호출이 너무 많습니다 |
| 401 | `call_invalid` | 토큰이 없거나, 모르는 토큰이거나, 만료되었거나, 다른 패키지의 토큰입니다 |
| 401 | `unauthorized` | 사용자의 OMV 세션이 끝났습니다. 백엔드가 무엇으로 답하든 대시보드가 사용자를 로그아웃시키고 요청을 401로 끝냅니다 |
| 403 | `rpc_forbidden` | 선언하지 않은 메서드이거나, 데이터 요청에서 쓰기나 `apply`를 불렀습니다 |
| 403 | `admin_required` | 관리자가 아닌 세션의 `rpc-bg` |
| 409 | `config_dirty` | OMV에 이미 적용 대기가 있습니다(`apply`) |
| 429 | `busy` | 동시 요청이 너무 많습니다 |
| 502 | `omv_error` | OMV가 오류를 돌려주었습니다. 문구는 OMV의 오류 문구(1KiB까지)입니다 |
| 503 | `omv_unavailable` | OMV에 닿지 못했습니다 |
| 504 | `timeout` | 요청의 시간 제한이 지났습니다 |

### 프로세스 환경

| 항목 | 설치본의 백엔드 | 개발 서버 |
|---|---|---|
| 받을 소켓 | 소켓 활성화: `LISTEN_FDS=1`, `LISTEN_PID`는 백엔드의 프로세스 ID, 소켓은 파일 기술자 3 | `MARU_ADDON_SOCKET`이 소켓 경로입니다. 권한 0600으로 만들고 끝날 때 지웁니다. 남은 소켓 파일은 연결이 거부될 때(아무도 받지 않을 때)만 바꾸고, 소켓이 아닌 파일이나 다른 프로세스가 받고 있는 소켓은 오류로 봅니다 |
| 호스트 소켓 | `/run/maru-host/host.sock` | `MARU_ADDON_HOST` |
| 데이터 디렉터리 | `STATE_DIRECTORY` | `MARU_ADDON_STATE`(선택) |

- SIGTERM을 받으면 연결을 더 받지 않고 진행 중인 요청을 마친 뒤 끝냅니다.
- 로그는 표준 오류에 씁니다. `X-Maru-Call` 헤더는 로그에 남기지 마세요.
- 개발 서버에서 백엔드의 로그는 백엔드를 띄운 터미널에 나옵니다(예제는 `make dev-fake`를 띄운 터미널). 데이터 요청이나 조작이 실패하면 대시보드의 로그(개발 서버 터미널)에도 패키지 id와 오류 코드가 남습니다. 설치본의 백엔드는 systemd journal에 로그를 남깁니다([로그](#로그)).

## 개발

개발 서버(`make dev`, `make dev-fake`)는 개발 백엔드를 켭니다. 개발 스크립트가 `MARU_DEV_ADDON_BACKENDS=1`을 넣습니다(`0`으로 두면 백엔드가 꺼진 설치에서 패키지가 어떻게 보이는지 확인할 수 있습니다). 상태 디렉터리가 있어야 하며, 개발 스크립트가 기본값을 정합니다.

대시보드는 백엔드가 있는 개발 패키지마다 `<state>/addon-host/<id>.sock`에 호스트 소켓을 열고 `<state>/addon-dev/<id>.sock`의 백엔드에 연결합니다. `<state>`는 개발 상태 디렉터리(`MARU_STATE_DIR`)이고, 기본은 주 체크아웃의 `.dev-state/fake`(`make dev-fake`)나 `.dev-state/real`(`make dev`)입니다. 대시보드는 백엔드를 스스로 띄우지 않습니다. 개발자가 띄우며, 띄우기 전에는 데이터 요청과 조작이 `addon_backend_unavailable`로 실패합니다. 개발 패키지는 `backend/`의 실행 파일 없이도 불러옵니다.

### 백엔드 실행

1. Go 백엔드가 있는 패키지를 만듭니다. `addon/`(패키지 디렉터리)과 `backend/`(백엔드의 Go 모듈, `go.mod`가 체크아웃의 SDK를 가리킵니다)가 나란히 만들어집니다. TypeScript + Vite 화면이면 `--ts`를 더합니다.

    ```sh
    node sdk/js/cli/maru-addon.mjs init ~/addons/my-backend --backend
    ```

2. 패키지 디렉터리를 주고 개발 서버를 띄웁니다.

    ```sh
    MARU_DEV_ADDONS=$HOME/addons/my-backend/addon make dev-fake
    ```

3. 다른 터미널에서 개발 소켓으로 백엔드를 띄웁니다.

    ```sh
    cd ~/addons/my-backend/backend
    MARU_ADDON_SOCKET=<state>/addon-dev/my-backend.sock \
    MARU_ADDON_HOST=<state>/addon-host/my-backend.sock \
    MARU_ADDON_STATE=/tmp/my-backend-data \
    go run .
    ```

4. 백엔드 코드를 고치면 백엔드를 다시 띄웁니다. `addon/`을 고치면 페이지를 새로 고칩니다.
5. 실행 파일을 빌드하고([빌드](#빌드)) `maru-addon pack`으로 `addon/`을 묶습니다. 백엔드를 선언했는데 실행 파일이 없으면 묶지 않습니다.

- 유닉스 소켓 경로는 107바이트까지입니다. 상태 디렉터리의 경로가 길면(`MARU_STATE_DIR`가 git 워크트리 깊은 곳을 가리킬 때 등) SDK가 시작하지 않으므로 `MARU_STATE_DIR`를 짧은 경로로 둡니다.
- 개발 서버의 백엔드는 설치본의 샌드박스 없이 개발자 계정으로 돕니다. 백엔드가 쓰는 것을 [샌드박스](#샌드박스)와 비교해 확인하세요.

### 예제와 가짜 OMV

- `FAKEADDONS=examples make dev-fake`는 저장소의 예제를 불러오고, 백엔드가 있는 예제의 백엔드를 `go run`으로 띄웁니다(데이터 디렉터리는 `<state>/addon-state/` 아래). `MARU_DEV_ADDONS`로 더한 패키지는 띄우지 않습니다.
- [hello-backend](https://github.com/steamb23/maru-nas/tree/main/sdk/examples/hello-backend)(백엔드는 [sdk/go/examples/hello-backend](https://github.com/steamb23/maru-nas/tree/main/sdk/go/examples/hello-backend))는 `Host.RPCInto`로 읽은 데이터, 데이터 디렉터리에 둔 값, `Host.Apply`로 OMV 설정을 바꾸고 `config_dirty`를 `addonCode`로 바꾸는 관리자 조작을 보여 줍니다.
- `make sdk-examples`는 예제 백엔드의 정적 실행 파일을 그 패키지의 `backend/`에 빌드합니다(커밋하지 않습니다). 개발 서버에는 필요 없고 묶을 때 씁니다.
- `make dev-fake`의 가짜 OMV는 일부 메서드만 흉내 냅니다. 그 밖의 메서드(플러그인 메서드 등)는 패키지 디렉터리의 `dev/fake-rpc.json`에 정한 응답을 둡니다. `make dev-fake`가 `MARU_DEV_ADDONS` 디렉터리에서 이 파일을 모으고, `FAKEOMV_EXTRA_RPC`에 콜론으로 나눈 파일 목록을 직접 줄 수도 있습니다.

    ```json
    {
      "Example.getStatus": { "result": { "running": true }, "admin": true },
      "Example.getList": { "result": [] }
    }
    ```

    - `result`는 반드시 있어야 하고(아무 JSON 값, `null`도 됩니다) 파라미터와 상관없이 늘 이 값을 돌려줍니다. `"admin": true`면 관리자 세션만 받습니다.
    - 가짜 OMV가 구현하지 않은 메서드에만 답하고, 세션 검사는 다른 메서드와 같습니다. `Session` 메서드와 모르는 필드는 오류이고, 읽지 못한 파일이 있으면 가짜 OMV가 시작하지 않습니다. 여러 파일이 같은 메서드를 적으면 앞 파일의 것을 씁니다.
    - 공개하기 전에 플러그인 메서드는 `make dev`로 실제 OMV에서 확인하세요.

## 제약

- **실행 파일**: `backend/linux-amd64`와 `backend/linux-arm64`. 각각 그 아키텍처의 64비트 리눅스 ELF이고 64MiB까지입니다. 정적으로 빌드합니다. 서버 아키텍처의 실행 파일이 있어야 설치되므로("이 서버의 아키텍처용 백엔드가 패키지에 없습니다.") 둘 다 넣으세요.
- **시간**: 데이터 요청은 15초, 조작은 그 조작의 `timeoutSeconds`(600초까지)입니다. 그보다 오래 걸리는 일은 할 수 없습니다.
- **OMV는 요청 중에만**: 호출 토큰은 대시보드의 요청이 도는 동안만 쓸 수 있으므로, 백엔드가 스스로(시작할 때, 타이머로, 답한 뒤에) OMV를 부를 수 없습니다. `/proc`나 자기 파일 읽기처럼 OMV가 필요 없는 일은 언제든 할 수 있습니다.
- **모든 사용자가 백엔드 하나를 씀**: 모든 사용자의 요청이 같은 프로세스로, 동시에 올 수도 있습니다. 요청마다 사용자와 관리자 여부를 보고 돌려줄 값을 정하고, 관리자 세션으로 읽은 값을 두었다가 일반 사용자에게 돌려주지 마세요. 데이터 디렉터리도 함께 씁니다.
- **화면은 그대로**: 화면은 여전히 네트워크에 닿지 못합니다. 연결은 백엔드만 할 수 있고, 설치본의 백엔드는 `backend.network`가 허용한 곳에만 닿습니다.

## 설치본의 백엔드

`install.sh`로 설치한 대시보드는 관리자가 애드온 백엔드를 켠 설치에서만 패키지 백엔드를 실행합니다. 켜지 않은 설치에서는 백엔드가 있는 패키지를 설치할 수는 있지만, 검토 화면이 경고하고 그 앱과 위젯은 "쓸 수 없음"으로 보입니다.

### 켜고 끄기

- 관리자가 SSH로 저장소에서 `./install.sh --addon-backends`를 실행합니다([설치](install.md#애드온-백엔드), [설치 옵션](install-options.md)). 기본값은 꺼짐이고 설치 스크립트가 묻지 않습니다. 한 번 켜면 이후 업그레이드와 `--reinstall`에서도 켜진 채로 둡니다.
- 끌 때는 `./install.sh --no-addon-backends`를 실행합니다. 돌고 있는 백엔드를 모두 멈춥니다. 백엔드 데이터는 지우지 않지만, 다시 켜면 그 데이터가 어느 설치의 것인지 확인할 수 없으므로 패키지의 백엔드를 다시 켜기 전에 그 패키지의 데이터를 지웁니다.
- 설치 스크립트가 대시보드를 다시 시작하므로 모든 사용자가 다시 로그인합니다.

### 백엔드를 켜는 방식

- 대시보드는 백엔드 프로그램을 직접 실행하지 않습니다. 패키지를 설치하거나, 켜거나 끄거나, 지울 때마다 켜 둘 백엔드 목록을 상태 디렉터리에 적고, root로 도는 동기화 서비스가 그 목록에 맞춰 백엔드를 켜고 끕니다. 동기화 서비스는 패키지 파일을 읽지 않고, 목록을 믿지 않는 입력으로 보고 형식을 엄격하게 검사합니다.
- 켠 백엔드는 첫 요청이 올 때 시작하고, 실패하면 다시 시작합니다. 업데이트하면 새 버전으로 다시 시작합니다.
- 설치, 업데이트, 다시 설치한 뒤에는 동기화 결과가 이 설치의 이 버전을 켰다고 알릴 때까지 대시보드가 백엔드로 요청을 보내지 않습니다. 그동안 데이터 요청과 조작은 503 `addon_backend_unavailable`로 실패하므로, 같은 id의 이전 설치나 이전 버전에서 남은 백엔드에 요청이 가지 않습니다.
- 백엔드가 있는 패키지는 32개까지 켤 수 있습니다. 개발 패키지는 세지 않습니다.
- 설정 앱의 "애드온" 페이지에서 마지막 동기화 결과를 봅니다. 켜지 못한 백엔드는 패키지 목록에 "백엔드 실패" 배지와 이유가 붙고 그 앱은 "쓸 수 없음"으로 보입니다([애드온 패키지](addons.md#애드온-백엔드)).

### 샌드박스

백엔드마다 따로 아래 샌드박스에서 돕니다.

- 백엔드가 도는 동안만 있는 자기만의 시스템 사용자로 돌고, root나 다른 특권이 없습니다.
- 자기 패키지 파일(읽기 전용)과 자기 데이터 디렉터리만 씁니다. 대시보드의 상태(사용자 설정, 보안 기록, 다른 패키지), OMV 설정, 공유 폴더와 데이터 디스크(`/srv`, `/export`, `/mnt`, `/media`), 홈 폴더(`/home`)에 닿지 못합니다. `/run`에는 자기 호스트 소켓만 있어 D-Bus 같은 시스템 서비스의 소켓에 닿지 않습니다. 다른 프로세스는 보이지 않습니다.
- 대시보드는 호스트 소켓에 연결한 프로세스의 사용자를 보고, root나 백엔드가 쓰는 동적 시스템 사용자가 아니면 바로 끊습니다. 그래서 이 서버의 일반 로컬 계정은 호스트 소켓을 쓸 수 없습니다. 백엔드의 `/run`에는 자기 호스트 소켓만 있으므로 다른 패키지의 호스트 소켓에도 닿지 않습니다.
- `/tmp`와 공유 메모리(`/dev/shm`)도 백엔드마다 따로 둡니다.
- 메모리 256MiB, 작업(프로세스와 스레드) 64개까지이고 CPU 우선순위가 낮습니다.
- 쓸 수 있는 메모리를 실행할 수 있게 바꿀 수 없으므로, 실행하면서 코드를 컴파일하는 런타임(JIT 컴파일러)은 돌지 않습니다. 정적으로 빌드한 프로그램(Go, Rust, C)은 돕니다.
- 장치, 커널 설정, 시계를 바꿀 수 없고, 보통 서비스가 쓰는 시스템 호출만 씁니다.
- 네트워크는 manifest의 `backend.network`가 허용한 곳입니다.

    | `network` | 백엔드가 연결할 수 있는 곳 |
    |---|---|
    | `[]` | 없음. 유닉스 소켓만 |
    | `["lan"]` | 사설 주소(10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7), 링크 로컬·멀티캐스트 주소 |
    | `["loopback"]` | 이 서버 자신의 서비스(127.0.0.0/8, ::1) |
    | `["lan", "loopback"]` | 둘 다 |

    인터넷을 포함한 그 밖의 주소는 막습니다. 인터넷을 허용하는 값은 없습니다. `lan`·`loopback`의 위험은 [애드온 보안](addon-security.md#대시보드가-막지-못하는-것)에 있습니다.

이 제한은 systemd가 적용합니다. 아래는 서버의 구성에 따라 다를 수 있으므로, 쓰는 서버에서 무엇이 막히는지는 시험 패키지 [probe](https://github.com/steamb23/maru-nas/tree/main/sdk/testdata/probe)(관리자만 쓰는 시험 도구이며 예제가 아닙니다)로 확인하세요.

- 주소 제한은 커널과 systemd가 IP 주소 필터를 지원해야 적용됩니다.
- `/run` 아래의 시스템 파일이 숨겨지므로, DNS 설정을 `/run`에 두는 서버에서는 네트워크가 있는 백엔드도 호스트 이름을 풀지 못할 수 있습니다. 그때는 IP 주소로 연결하세요.

### 데이터와 제거

- 데이터 디렉터리는 `/var/lib/private/maru-addon/<id>`이고 root만 읽을 수 있습니다. 백엔드에는 `STATE_DIRECTORY`로 보입니다.
- 업데이트(같은 id의 새 파일 설치)와 패키지 끄기는 데이터를 그대로 둡니다. 설치에서 애드온 백엔드를 끄면 데이터는 남지만, 다시 켤 때 지워집니다(위 "켜고 끄기").
- 제거할 때 "백엔드가 저장한 데이터도 지우기"를 고르면 동기화 서비스가 데이터를 지웁니다.
- 고르지 않으면 데이터는 서버에 남다가, 같은 id의 패키지를 다시 설치하면 백엔드를 켜기 전에 지웁니다. 다시 설치한 패키지(같은 id를 쓰는 다른 작성자의 패키지일 수 있습니다)가 이전 데이터를 읽지 못하게 하기 위해서입니다. 사용자별 저장값과 같은 규칙입니다.

### 로그

- 백엔드가 표준 출력과 표준 오류에 쓴 내용은 systemd journal에 남습니다. root와 `adm` 그룹은 `journalctl -u 'maru-addon*@<id>.service'`로 읽습니다.
- 데이터 요청이나 조작이 실패하면 대시보드 로그(`journalctl -u maru`)에도 패키지 id와 오류 코드가 남습니다.
