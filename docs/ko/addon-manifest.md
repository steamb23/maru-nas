# manifest

패키지 루트의 `manifest.json`은 패키지의 id, 이름, 보기, 권한을 밝힙니다. 모르는 필드가 있으면 설치하지 않습니다. 편집기 자동 완성에는 SDK의 JSON 스키마([`sdk/js/schema/manifest.schema.json`](https://github.com/steamb23/maru-nas/blob/main/sdk/js/schema/manifest.schema.json))를 `$schema`로 걸어 씁니다. 스키마는 구조만 담고, 필드 사이의 조건은 대시보드가 설치할 때 봅니다.

```json
{
  "$schema": "https://raw.githubusercontent.com/steamb23/maru-nas/main/sdk/js/schema/manifest.schema.json",
  "format": 1,
  "id": "hello-widget",
  "version": "0.1.0",
  "sdk": 1,
  "name": { "en": "Hello widget", "ko": "안녕 위젯" },
  "description": { "en": "Shows the host name." },
  "author": { "name": "Example Author", "url": "https://example.com" },
  "icon": "icon.svg",
  "views": {
    "window": { "entry": "window.html", "defaultSize": { "width": 460, "height": 400 } },
    "widget": { "entry": "widget.html", "defaultSize": { "cols": 2, "rows": 1 } }
  },
  "rpc": { "read": ["System.getInformation"] }
}
```

## 필드

| 필드 | 필수 | 내용 |
|---|---|---|
| `$schema` | | 편집기가 쓰는 스키마 주소. 대시보드는 무시합니다 |
| `format` | 예 | `1` |
| `id` | 예 | 소문자로 시작하고 소문자·숫자와 `-`로 이은 24자 이하(예: `hello-widget`). 내장 애드온·다른 패키지·대시보드가 예약한 id(`files`, `settings`, `maru` 등)와 겹치면 안 됩니다. 설치한 뒤에는 바꾸지 마세요(다른 패키지가 됩니다) |
| `version` | 예 | SemVer(`1.2.3`, `1.2.3-beta.1`). 빌드 메타데이터(`+`)는 쓰지 않습니다. 32자 이하 |
| `sdk` | 예 | 필요한 SDK 수준. 지금은 `1`. 대시보드가 지원하는 수준보다 크면 "호환 안 됨"입니다 |
| `name` | 예 | 언어별 이름(40자 이하). `en`은 반드시 있어야 합니다 |
| `description` | | 언어별 설명(200자 이하). 있으면 `en`이 있어야 합니다 |
| `author` | 예 | `name`(60자 이하)과 `url`(선택, `http`·`https`). 같은 id의 업데이트에서 이름이 바뀌면 검토 화면에 경고가 뜹니다 |
| `icon` | 예 | `"icon.svg"`. 패키지 루트의 SVG 파일입니다 |
| `views` | 예 | 창(`window`)과 위젯(`widget`) 중 하나 이상. 아래 "보기" |
| `requires` | | `admin`(`true`면 관리자에게만 보입니다)과 `plugins`(필요한 OMV 플러그인) |
| `rpc` | | `read`(부를 OMV 읽기 메서드)와 `write`(조작이 부를 쓰기 메서드). 둘 합쳐 64개까지 |
| `actions` | | 조작 선언(32개까지). 백엔드가 있어야 합니다 |
| `backend` | | 백엔드 선언(`network`). 아래 "조작과 백엔드" |

## 문구

- `name`, `description`, 조작의 `label`·`confirmText`는 언어 코드를 키로 둔 객체입니다. 언어는 대시보드가 지원하는 언어(`en`, `ko`, `de`, `fr`, `es`, `it`, `pt-PT`, `nl`, `pl`, `cs`, `ru`, `uk`, `ja`, `zh-Hans`, `zh-Hant`)만 씁니다. 화면 언어의 문구가 없으면 `en`을 보입니다.
- 공백만 있는 문구, 제어 문자·서식 문자(방향 바꾸기 문자 포함), `{{`, `}}`, `$t(`는 쓸 수 없습니다(대시보드의 번역 문법과 섞이지 않게).
- 화면 안 문구는 manifest가 아니라 번역 파일(`i18n/<언어>.json`)에 둡니다([화면 만들기](addon-frontend.md#번역)).

## 보기

| 필드 | 내용 |
|---|---|
| `window.entry` | 창 화면의 HTML 파일. `ui/` 바로 아래의 `.html` 파일 이름입니다(예: `window.html`) |
| `window.defaultSize` | 처음 여는 창 크기 `{width, height}`(픽셀, 1~4096) |
| `window.minSize` | 줄일 수 있는 최소 크기. 기본 크기보다 크면 안 됩니다 |
| `widget.entry` | 위젯 화면의 HTML 파일 |
| `widget.defaultSize` | 위젯 크기 `{cols, rows}`(바탕화면 칸, 1~12) |
| `widget.minSize`, `widget.maxSize` | 크기 조절 범위. 없으면 기본 크기로 고정됩니다. 최소는 기본 이하, 최대는 기본 이상이어야 합니다 |

## 플러그인

`requires.plugins`는 `[{ "package": "openmediavault-nut", "rpcFile": "nut.inc" }]`처럼 OMV 플러그인의 Debian 패키지 이름과 그 플러그인이 설치하는 RPC 파일(`/usr/share/openmediavault/engined/rpc/<파일>`)입니다. 둘 다 있어야 설치된 것으로 봅니다. 대시보드가 이미 아는 플러그인이면 `rpcFile`이 대시보드의 값과 같아야 합니다. 플러그인이 없으면 앱과 위젯이 "쓸 수 없음"으로 보입니다.

## OMV 메서드

- `rpc.read`의 메서드만 화면(`app.rpc`)에서 부를 수 있습니다. 백엔드도 이 메서드를 부를 수 있고, 조작 중에는 `rpc.write`의 메서드도 부릅니다. 형식은 `"Service.method"`이고 중복은 받지 않습니다.
- OMV 서비스·메서드 이름과 돌려주는 값은 OMV 소스(`/usr/share/openmediavault/engined/rpc/*.inc`)에서 확인하세요. OMV 8의 모든 버전(8.0 이상)에 있는 메서드를 쓰는 것이 좋습니다.
- 명령 실행, 스크립트, 패키지 설치, 자격 증명 읽기처럼 애드온 패키지에 허용하지 않는 메서드가 있습니다([애드온 보안](addon-security.md#허용하지-않는-메서드)). 선언하면 설치하지 않습니다.
- 다른 애드온이 읽기로 선언한 메서드를 쓰기로(또는 그 반대로) 선언하면 "허용 안 됨"입니다.
- `rpc.write`는 조작이 있을 때만 쓸 수 있습니다. 쓰기 메서드는 백엔드가 조작을 실행하는 동안에만 부릅니다. OMV 설정을 바꾸는 메서드는 `Host.Apply`로 불러 적용 대기로 남기지 않고 적용까지 합니다([애드온 백엔드](addon-backend.md#omv-부르기)).

## 조작과 백엔드

조작은 패키지의 백엔드에서 돕니다([애드온 백엔드](addon-backend.md)). 지금 버전에서 백엔드는 개발 서버에서만 돕니다. 설치한 대시보드도 백엔드가 있는 패키지를 받지만, 백엔드를 켜는 설치 옵션이 이후 버전에서 나올 때까지 그 앱은 "쓸 수 없음"으로 보입니다.

| 필드 | 내용 |
|---|---|
| `actions[].name` | 조작 이름(`id`와 같은 형식, 32자 이하) |
| `actions[].admin` | 관리자만 실행할 수 있는지 |
| `actions[].confirm` | 실행하기 전에 대시보드가 물을지. `true`면 `confirmText`가 있어야 합니다 |
| `actions[].timeoutSeconds` | 조작 전체에 주는 시간(1~600초, 없으면 60초) |
| `actions[].label`, `confirmText` | 언어별 문구(40자, 300자 이하) |
| `backend.network` | 백엔드가 연결할 수 있는 곳: 빈 배열(연결 없음), `"lan"`, `"loopback"`, 또는 둘 다 |

- `admin`: 일반 사용자는 그 조작을 실행할 수 없습니다. OMV는 관리자 세션의 설정 적용만 받으므로 OMV 설정을 바꾸는 조작은 관리자 전용으로 둡니다.
- `confirm`: 패키지 이름이 붙은 대시보드의 확인 대화상자가 조작을 실행할 때마다 뜹니다. 화면이 건너뛸 수 없습니다.
- `rpc.write`를 선언한 패키지의 조작은 비밀번호를 다시 확인한 세션(root 권한 상승의 보안 정책이 2차 인증이면 2차 인증을 거친 세션)이어야 합니다. 세션이 아직 확인하지 않았으면 첫 실행 전에 한 번 묻고, 그 확인은 세션이 끝날 때까지 남습니다([보안 강화 모드](security.md#보안-정책)). 같은 세션에서 패키지를 설치했다면 이미 확인했을 수 있습니다.
- `timeoutSeconds`: 조작은 사용자가 창을 닫아도 끝나거나 이 시간에 이를 때까지 돕니다.
- `backend.network`는 설치본의 백엔드가 샌드박스에서 돌 때 적용됩니다([설치본의 백엔드](addon-backend.md#설치본의-백엔드)). `"lan"`과 `"loopback"`은 검토 화면에 위험으로 표시합니다([애드온 보안](addon-security.md#대시보드가-막지-못하는-것)).
