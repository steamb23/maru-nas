# 애드온 개발

마루 NAS 저장소 밖에서 애드온을 만들어 패키지 파일(`.maru-addon`)로 배포할 수 있습니다. 패키지는 HTML과 JavaScript로 만든 창·위젯 화면, 패키지가 부를 OMV 메서드를 밝힌 `manifest.json`, 그리고 필요하면 서버에서 도는 백엔드로 이루어집니다. 관리자는 설정 앱에서 파일을 올려 설치합니다([애드온 패키지](addons.md)).

SDK(`sdk/`)는 MIT 라이선스입니다. 대시보드 자체는 GPL-3.0이지만 패키지는 원하는 라이선스로 배포할 수 있습니다.

## 동작 방식

- 패키지 화면은 대시보드 안의 샌드박스 iframe에서 돕니다. 대시보드의 쿠키·저장소·화면에 닿지 않고, 직접 네트워크 요청(fetch 등)을 보낼 수 없습니다.
- 화면은 대시보드가 프레임에 넣어 주는 SDK 런타임(`./_maru/sdk.js`, 전역 `maru`)으로 대시보드에 요청합니다. 대시보드가 로그인한 사용자의 세션으로 대신 부르고 결과를 돌려줍니다.
- 화면은 `manifest.json`의 `rpc.read`에 밝힌 OMV RPC 메서드만 부를 수 있고, 백엔드는 조작을 실행하는 동안 `rpc.write`에 밝힌 메서드도 부를 수 있습니다. 로그인한 사용자의 OMV 권한으로 부르므로, 일반 사용자는 OMV가 일반 사용자에게 주는 값만 받습니다.
- 사용자마다 작은 값(64KiB까지의 JSON)을 저장할 수 있습니다.
- 패키지에는 백엔드를 넣을 수 있습니다. 백엔드는 서버에서 도는 프로그램으로, 화면의 데이터 요청과 조작에 답하고 조작의 OMV 쓰기를 포함해 대시보드를 거쳐 OMV를 부릅니다([애드온 백엔드](addon-backend.md)).
- 지금 버전에서 백엔드는 개발 서버에서만 돕니다. 설치한 대시보드도 백엔드가 있는 패키지를 받지만, 백엔드를 켜는 설치 옵션이 이후 버전에서 나올 때까지 그 앱은 "쓸 수 없음"으로 보입니다.

## 준비물

- 마루 NAS 저장소 체크아웃(개발 서버를 띄웁니다). SDK는 아직 npm에 공개하지 않았으므로 CLI도 체크아웃에서 씁니다
- Node.js 20.19 이상(CLI). TypeScript로 만들려면 npm
- Go SDK로 백엔드를 만들려면 Go 1.23 이상
- 개발 서버의 빌드 도구(Go, Node.js)는 저장소의 [CONTRIBUTING.ko.md](https://github.com/steamb23/maru-nas/blob/main/CONTRIBUTING.ko.md)를 보세요

## 빠른 시작

저장소 루트에서 실행합니다.

1. 새 패키지를 만듭니다. 빌드 없는 HTML·JavaScript 패키지가 만들어집니다(`--ts`면 TypeScript + Vite 프로젝트, `--backend`면 [애드온 백엔드](addon-backend.md#백엔드-실행)처럼 Go 백엔드를 더합니다).

    ```sh
    node sdk/js/cli/maru-addon.mjs init ~/addons/my-addon
    ```

2. 그 디렉터리를 개발 패키지로 불러 가짜 OMV 개발 서버를 띄웁니다. 경로는 절대 경로로 씁니다.

    ```sh
    MARU_DEV_ADDONS=$HOME/addons/my-addon make dev-fake
    ```

3. `http://<호스트>:5173/`에 가짜 계정 `admin`(비밀번호 `admin`)으로 로그인하고 "모든 앱"에서 "My Addon"을 엽니다. 바탕화면 오른쪽 클릭 메뉴로 위젯도 더할 수 있습니다.
4. 파일을 고친 뒤 페이지를 새로 고치면 바뀐 내용이 보입니다. 대시보드가 검사하는 내용(manifest 오류 등)은 설정 앱의 "애드온" 페이지에서 봅니다.
5. 다 만들었으면 검사하고 묶습니다. `my-addon-0.1.0.maru-addon`과 그 SHA-256이 나옵니다.

    ```sh
    node sdk/js/cli/maru-addon.mjs pack ~/addons/my-addon
    ```

저장소에 있는 예제는 `FAKEADDONS=examples make dev-fake`로 함께 띄웁니다. [hello-widget](https://github.com/steamb23/maru-nas/tree/main/sdk/examples/hello-widget)은 OMV 읽기, 사용자별 저장, 확인 대화상자, 알림, 워크벤치 열기, 위젯을 보여 줍니다. [hello-backend](https://github.com/steamb23/maru-nas/tree/main/sdk/examples/hello-backend)는 데이터와 OMV 설정을 바꾸는 관리자 조작이 있는 Go 백엔드를 보여 주며, `make dev-fake`가 그 백엔드도 띄웁니다.

## 개발 서버

- `MARU_DEV_ADDONS`는 콜론으로 나눈 패키지 디렉터리 목록입니다. 개발 서버(`make dev`, `make dev-fake`)에서만 씁니다. 같은 id의 설치본이 있으면 개발 패키지를 씁니다.
- 개발 패키지는 파일이 바뀌면 새 판으로 보고, 대시보드가 앱 목록을 다시 받을 때(페이지를 새로 고칠 때 등) 열린 창과 위젯을 다시 띄웁니다.
- 디렉터리의 `dev/`는 개발용 파일을 두는 곳이라 패키지에 묶이지 않습니다.
- `make dev-fake`는 가짜 OMV에 연결합니다. 가짜 OMV는 일부 메서드만 흉내 내므로(예: `System.getInformation`), 그 밖의 메서드는 패키지 디렉터리의 `dev/fake-rpc.json`에 정한 응답을 둘 수 있습니다([예제와 가짜 OMV](addon-backend.md#예제와-가짜-omv)). 플러그인 메서드는 `make dev`로 실제 OMV에 연결해 확인합니다.
- 개발 서버는 개발 백엔드를 켭니다. 패키지의 백엔드는 개발자가 개발 소켓으로 직접 띄웁니다([개발](addon-backend.md#개발)).
- 가짜 계정은 `admin`(관리자), `user`(일반 사용자), `totp`(관리자, 2차 인증 코드 `123456`)입니다. 일반 사용자 화면도 확인하세요.

## 문서

| 문서 | 내용 |
|---|---|
| [manifest](addon-manifest.md) | `manifest.json`의 모든 필드와 규칙 |
| [화면 만들기](addon-frontend.md) | 진입 HTML, SDK API, 창과 위젯, 프레임의 제약, TypeScript + Vite |
| [애드온 백엔드](addon-backend.md) | Go SDK, 데이터와 조작, OMV 부르기, 다른 언어를 위한 프로토콜, 개발, 제약, 설치본 백엔드의 샌드박스 |
| [묶기와 배포](addon-packaging.md) | 패키지 파일 구성과 한도, `maru-addon` CLI, 버전과 업데이트 |
| [애드온 보안](addon-security.md) | 대시보드가 막는 것과 막지 못하는 것, 허용하지 않는 메서드, 백엔드의 위험, 작성자가 지킬 것 |
