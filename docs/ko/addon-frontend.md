# 화면 만들기

패키지의 창과 위젯은 `ui/` 아래의 HTML 페이지입니다. 대시보드는 각 페이지를 샌드박스 iframe에 띄우고, 같은 폴더의 `_maru/`에 SDK 런타임과 공통 스타일을 넣어 줍니다.

## 진입 HTML

```html
<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <link rel="stylesheet" href="./_maru/maru.css" />
    <script src="./_maru/sdk.js"></script>
  </head>
  <body>
    <main class="addon-window">
      <h1 id="title"></h1>
    </main>
    <script type="module" src="./window.js"></script>
  </body>
</html>
```

- `./_maru/sdk.js`는 `<head>`에서 먼저 싣습니다. 실으면 바로 테마를 적용하므로 어두운 화면에서 흰 화면이 깜빡이지 않습니다.
- 스크립트는 파일로 둡니다. 인라인 스크립트와 `eval`은 막힙니다.
- 경로는 모두 상대 경로(`./`)로 씁니다. 대시보드가 세션마다 다른 주소로 파일을 내주기 때문입니다.
- `ui/_maru`는 대시보드가 쓰는 이름이라 패키지에 둘 수 없습니다.

## 연결과 문맥

```js
const app = await maru.connect()
document.getElementById('title').textContent = app.t('title')
app.on('context', (ctx) => render(ctx))
```

`maru.connect()`는 대시보드와 연결하고 앱 객체를 돌려줍니다. 여러 번 불러도 같은 객체입니다. 대시보드 밖에서 페이지를 열면 `bridge_closed`로 실패합니다.

`app.context`의 값은 바뀔 때마다 `'context'` 이벤트로 옵니다.

| 값 | 내용 |
|---|---|
| `view` | `'window'` 또는 `'widget'` |
| `active` | 보기를 그려야 하는지. 창을 최소화하거나 브라우저 탭을 숨기면 `false`입니다. `false`인 동안은 주기 요청을 멈추세요 |
| `size` | 프레임 크기(`width`, `height`, 위젯이면 `cols`, `rows`) |
| `theme`, `animations` | `'light'`·`'dark'`, 애니메이션을 켰는지 |
| `language`, `messages` | 화면 언어(예: `ko`, `zh-Hans`)와 패키지 번역 표 |
| `admin` | 로그인한 사용자가 OMV 관리자인지 |
| `addonId`, `sdkLevel` | 패키지 id, 대시보드의 SDK 수준 |

## API

| 함수 | 내용 |
|---|---|
| `app.t(key, params)` | 패키지 번역 문구. `{{name}}`을 `params`로 바꿉니다. 없는 키는 키 그대로 돌려줍니다 |
| `app.rpc(service, method, params)` | manifest `rpc.read`에 밝힌 OMV 메서드를 부르고 결과를 돌려줍니다 |
| `app.storage.get()`, `app.storage.set(data)` | 이 사용자의 패키지 저장값(JSON, 64KiB까지). 없으면 `null`. `set`은 1초에 한 번 |
| `app.confirm({ message, title, confirmLabel, danger })` | 대시보드의 확인 대화상자. 확인하면 `true`. 제목은 "<패키지 이름> 애드온의 요청"이고 `title`은 문구 앞에 붙습니다 |
| `app.notify(level, key, params)` | 대시보드 알림(토스트와 알림 센터). `level`은 `info`·`success`·`warning`·`error`, 문구는 번역 키입니다. 분당 3개까지. 알림을 누르면 패키지 창이 열립니다 |
| `app.openWorkbench(route)` | OMV 워크벤치의 라우트를 이름 붙은 탭으로 엽니다(앞 슬래시 없이, 예: `storage/smart/devices`) |
| `app.data(query)`, `app.action(name)`, `app.query(name)` | 패키지 백엔드의 데이터와 조작. 이후 버전에서 씁니다 |

- 요청은 프레임마다 동시에 16개, 메시지 하나는 256KiB까지입니다. 그 밖의 요청은 1초에 50개까지 받습니다.
- `confirm`과 `notify`는 창 보기에서만 씁니다.

## 창과 위젯

- 위젯은 바탕화면의 칸에 붙는 작은 보기입니다. 누르거나 Tab으로 들어갈 수 없고, 누르면 패키지 창이 열립니다(창 보기가 있을 때). 끌기와 크기 조절은 대시보드가 합니다.
- 위젯은 `rpc`, `storage.get`, `data`만 쓸 수 있습니다. 위젯의 배경은 투명해서 위젯 틀이 보입니다.
- 창이 최소화되거나 탭이 숨겨지면 `context.active`가 `false`가 됩니다. 위젯·창 모두 주기 요청을 멈추세요.

## 사용자 조작이 필요한 요청

`openWorkbench`(과 이후의 `action`)는 사용자가 그 프레임 안을 방금 눌렀을 때만 받습니다. 단추의 `click` 처리기 안에서 바로 부르세요.

- 프레임 안을 누르거나 Enter·Space 키를 누른 뒤 5초 안이어야 합니다.
- 그 프레임에 포커스가 있고, 그 창이 맨 앞이고, 대화상자에 막혀 있지 않아야 합니다.
- 1초에 한 번까지 받습니다.
- 페이지를 열자마자 부르거나 타이머로 부르면 `bridge_gesture_required`로 거절합니다.

## 번역

- 화면 문구는 `i18n/<언어>.json`에 둡니다. 키와 문자열 값만 있는 평평한 JSON이고, `en.json`은 반드시 있어야 합니다.
- 키는 영문자·숫자·`_`·`.`·`-`로 된 100자 이하, 값은 2000바이트 이하, 파일마다 키 2000개까지입니다.
- 대시보드는 `en` 위에 화면 언어의 문구를 덮어 `context.messages`로 줍니다. 사용자가 언어를 바꾸면 `'context'` 이벤트가 오므로 문구를 다시 그리세요.
- 치환자는 `{{name}}`입니다(`app.t('host', { name })`). 복수형은 없습니다.

## 모양

- `./_maru/maru.css`는 대시보드의 디자인 토큰과 공통 클래스를 담습니다. 대시보드 내장 앱과 같은 모양이 됩니다.
- 토큰: `--text`, `--text-muted`, `--accent`, `--window-bg`, `--window-border`, `--hover-bg`. 테마는 `<html data-theme="light|dark">`, 애니메이션을 끄면 `data-animations="off"`, 보기는 `data-view="window|widget"`입니다.
- 클래스: `.addon-window`(창의 가운데 열), `.addon-card`·`.addon-card-head`·`.addon-card-title`·`.addon-card-value`·`.addon-card-sub`, `.addon-section-title`, `.addon-hint`, `.addon-list`, `.addon-banner`, `.addon-empty`, `.addon-button`(`.primary`, `.danger`), `.badge`, `.meter`·`.meter-fill`, `.status-dot`, `.addon-widget`·`.widget-line`·`.widget-name`·`.widget-value`
- 인라인 `style`과 `<style>`은 쓸 수 있습니다.

## 프레임의 제약

패키지 화면은 출처가 없는(불투명 출처) 샌드박스에서 돌고, 응답마다 대시보드의 Content-Security-Policy가 붙습니다.

- 네트워크 요청(`fetch`, XHR, WebSocket, EventSource)을 보낼 수 없습니다. OMV는 `app.rpc`로 부릅니다.
- 스크립트·이미지·글꼴은 패키지 파일만 씁니다. 이미지와 글꼴은 `data:`·`blob:`도 됩니다. 다른 사이트의 파일(CDN 등)은 막힙니다.
- `document.cookie`, `localStorage`, `sessionStorage`, IndexedDB는 `SecurityError`를 던집니다. 사용자별 값은 `app.storage`에 둡니다.
- 폼은 제출되지 않고 `submit` 이벤트도 일어나지 않습니다. `<form>` 대신 단추의 `click`과 입력란의 Enter 키를 직접 처리하세요.
- `alert`, `confirm`, `prompt`, 팝업(`window.open`), 대시보드 페이지 자체를 다른 주소로 보내기, 클립보드처럼 권한이 필요한 브라우저 기능은 쓸 수 없습니다. 확인은 `app.confirm`, 워크벤치는 `app.openWorkbench`로 엽니다.
- 프레임이 다른 주소로 이동하면 대시보드가 연결을 끊고 패키지 화면을 처음부터 다시 띄웁니다.

## 오류

실패한 요청은 `MaruError`(`status`, `code`, `addonCode`, `message`)로 거부됩니다.

| `code` | 뜻 |
|---|---|
| `bridge_not_allowed` | 이 보기에서 쓸 수 없는 요청(위젯의 `confirm` 등) |
| `bridge_gesture_required` | 사용자 조작이 필요한 요청을 조작 없이 불렀습니다 |
| `bridge_rate_limited`, `bridge_busy` | 너무 자주, 너무 많이 보냈습니다 |
| `bridge_invalid` | 형식이 틀린 요청 |
| `bridge_cancelled` | 사용자가 대시보드의 확인 대화상자에서 취소했습니다 |
| `bridge_timeout`, `bridge_closed` | 대시보드가 답하지 않았거나 연결이 끊겼습니다 |
| `rpc_forbidden` | manifest에 밝히지 않았거나 허용하지 않는 메서드입니다 |
| `omv_error`, `omv_unavailable` | OMV가 오류를 돌려주었거나 OMV에 닿지 못했습니다 |

일반 사용자에게 관리자 전용 메서드를 부르면 OMV가 오류를 돌려줍니다. `context.admin`을 보고 부를지 정하세요.

## TypeScript와 Vite

`maru-addon init <디렉터리> --ts`는 TypeScript + Vite 프로젝트를 만듭니다. `addon/`이 패키지 디렉터리이고 `npm run build`가 `addon/ui/`에 씁니다.

- 타입은 `@maru-nas/addon-sdk`에 있습니다(`tsconfig.json`의 `"types": ["@maru-nas/addon-sdk"]`). 전역 `maru`와 `MaruApp` 등의 타입이 들어 있습니다.
- `vite.config.ts`: `base: './'`(상대 경로), `build.outDir: 'addon/ui'`, `build.assetsInlineLimit: 0`(자산을 파일로), 보기마다 HTML 페이지 하나(`rollupOptions.input`)
- HTML의 `_maru/` 참조에는 `vite-ignore` 속성을 붙여 Vite가 묶지 않게 합니다.
- 개발 중에는 `npm run watch`(`vite build --watch`)로 다시 빌드하고 `MARU_DEV_ADDONS`에 `addon/` 디렉터리를 줍니다. Vite 개발 서버(HMR)는 쓸 수 없습니다.
