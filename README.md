# <img src="frontend/src/assets/logo/onmaru-lockup.svg" alt="온마루 (Onmaru)" height="80">

> A suspiciously normal desktop-like dashboard for openmediavault.

OpenMediaVault 8 위에 올리는 데스크톱형 웹 대시보드입니다. 창·작업표시줄·바탕화면 위젯으로 NAS 가동 현황을 확인하고, 컨테이너와 핵심 서비스를 간단히 켜고 끌 수 있습니다. 심층 설정은 OMV 기본 웹 UI에서 합니다.

- 로컬 네트워크 전용입니다. 인터넷에 노출하는 구성을 전제로 하지 않으며, 인터넷에 노출되었을 때 어떤 보안 취약점이 드러날지 충분히 검증되지 않았습니다.
- 로그인은 OMV 계정으로 합니다. 백엔드가 OMV `rpc.php`에 로그인을 위임하며, 설정 변경도 로그인한 사용자의 OMV 세션으로만 합니다.
- Go 실행 파일 하나(프론트엔드 포함)를 systemd 서비스로 실행하고, 앞단의 리버스 프록시가 HTTPS를 맡습니다.

## 설치

빌드에는 Go, Node.js(npm), make, git이 필요합니다.

```sh
./install.sh
```

일반 사용자 계정이라면 **sudo 없이** 실행해 주세요. 인터랙티브 설치 스크립트이며 다음 순서로 진행됩니다.

1. **설정 입력**: 서비스 사용자, 수신 방식, OMV 주소, SMART 갱신 시각, OMV 워크벤치 주소를 묻고 요약을 보여 준 뒤 확인을 받습니다. 입력은 이 단계에서 모두 끝납니다.
2. **빌드**: 실행한 계정으로 빌드합니다. 처음 클론한 상태라면 아이콘 서브모듈과 npm 의존성부터 준비합니다(`make build` → `bin/onmaru`).
3. **설치**: root 권한이 필요한 단계입니다. 일반 사용자라면 스크립트가 1단계의 설정을 옵션으로 넘겨 스스로 `sudo ./install.sh --service-install ...`을 실행하므로 비밀번호만 한 번 입력하면 됩니다. root 계정만 있는 환경이라면 sudo 없이 그대로 이어서 진행합니다.

설치 단계에서는 다음 작업을 합니다.

- 시스템 사용자를 만들고(기본값 `onmaru`), `bin/onmaru`를 `/usr/local/bin/onmaru`에 설치합니다.
- 설정 파일 `/etc/default/onmaru`와 systemd 유닛 `/etc/systemd/system/onmaru.service`를 작성한 뒤 서비스를 시작합니다.
- 사용자 설정(위젯 배치, 바탕화면 바로가기, 화면 설정 등), 관리자의 전역 설정, 올린 배경 이미지는 `/var/lib/onmaru`(`StateDirectory=`)에 저장됩니다.

다시 실행하면 기존 설정을 기본값으로 보여 주며, 업그레이드할 때도 같은 명령을 사용합니다. 설정을 바꾸지 않고 업그레이드만 하려면 `./install.sh --upgrade`를 사용합니다(아래 "업그레이드" 참고). 설정이 바뀌면 기존 `/etc/default/onmaru`는 `/etc/default/onmaru.bak-<시각>`으로 백업됩니다. 안내 문구는 로캘에 따라 영어 또는 한국어로 표시되며, `--lang en|ko`로 직접 고를 수도 있습니다.

| 옵션 | 설명 |
|---|---|
| (없음) | 빌드한 뒤 설치하거나 업그레이드합니다. |
| `--upgrade` | 기존 설치의 설정 그대로 묻지 않고 빌드한 뒤 업그레이드합니다. 기존 설치에 없는 설정(새 버전에서 생긴 설정)만 묻습니다. |
| `--no-build` | 빌드를 건너뛰고 기존 `bin/onmaru`를 설치합니다. |
| `--uninstall` | 서비스, 실행 파일, 설정을 제거합니다. |
| `--service-install` | 빌드 없이 설치 단계만 실행합니다(root 전용). 아래 설정 옵션을 모두 주면 묻지 않고 설치합니다. |
| `--lang en\|ko` | 안내 문구 언어를 고릅니다. |
| `--user NAME` | 서비스 사용자입니다(기본값 `onmaru`). |
| `--listen ADDR` | 수신 주소입니다. `127.0.0.1:8090`, `unix:/run/onmaru/onmaru.sock`, 또는 `host:port`를 지정합니다. |
| `--allow-non-loopback` | 루프백이 아닌 `--listen` 주소를 허용합니다(위험). |
| `--omv-url URL` | OMV 웹 UI 주소입니다(루프백). |
| `--smart-refresh-at HH:MM` | SMART 정보를 매일 다시 가져올 시각입니다. |
| `--workbench-url URL` | 브라우저에서 OMV 웹 UI를 열 주소입니다. 빈 값(`--workbench-url ''`)이면 "OMV 워크벤치" 앱을 숨깁니다. 질문에서는 `-`를 입력하면 끕니다. |

설정 옵션으로 준 항목은 묻지 않습니다. 옵션 없이 실행할 때는 설정을 모두 주더라도 요약을 보여 주고 진행 여부를 확인합니다. 확인까지 생략하는 것은 root로 `--service-install`에 설정을 모두 준 경우뿐이므로, 자동화에는 다음과 같이 사용합니다(빌드는 미리 해 두어야 합니다).

```sh
./install.sh --service-install --user onmaru --listen unix:/run/onmaru/onmaru.sock \
  --omv-url http://127.0.0.1 --smart-refresh-at 00:00 --workbench-url https://nas.example.lan/
```

root로 실행했는데 저장소의 소유자가 root가 아니면(`sudo ./install.sh`, `sudo -i`, `su` 등) 빌드를 건너뜁니다. root로 빌드하면 저장소에 root 소유 파일이 생기고, sudo 환경의 PATH에는 사용자가 설치한 Go·npm이 없을 수 있기 때문입니다. 이때는 기존 `bin/onmaru`로 설치만 할지 물어봅니다(`--upgrade`이면 묻지 않고 멈춥니다).

### 업그레이드

```sh
git pull
./install.sh --upgrade
```

`--upgrade`는 지금 설치된 설정(`/etc/systemd/system/onmaru.service`의 서비스 사용자, `/etc/default/onmaru`의 나머지 설정)을 그대로 써서 빌드하고 설치합니다. 설정 질문, 요약 확인, 포트 사용 중·OMV의 Force SSL/TLS 같은 경고의 확인을 모두 건너뛰므로(경고는 표시합니다) 일반 사용자라면 sudo 비밀번호만 입력하면 됩니다.

- 새 버전에서 설정이 추가되어 기존 설치에 그 값이 없으면 그 설정만 묻습니다.
- 설정 옵션(`--listen` 등)을 함께 주면 그 항목만 기존 값 대신 씁니다.
- 수신 주소와 서비스 사용자가 그대로면 리버스 프록시 설정도 그대로 맞으므로, 끝난 뒤 리버스 프록시 안내 없이 로그 위치만 알립니다.
- 기존 설치가 없거나, 기존 설정이 새 버전의 검사를 통과하지 못하면 멈춥니다. 이때는 `--upgrade` 없이 실행해 설정을 다시 입력합니다.

개발 서버(`make dev`)의 백엔드도 기본으로 `127.0.0.1:8090`을 사용합니다. 같은 머신에서 개발한다면 서비스는 Unix 소켓이나 다른 포트로 설치해 주세요. 설치 스크립트는 포트가 이미 사용 중이면 경고합니다.

제거하려면 `./install.sh --uninstall`을 실행합니다. `/var/lib/onmaru`와 서비스 사용자는 확인을 받은 뒤에만 삭제합니다. 제거하면 `/run/onmaru`도 지워지므로, 컨테이너 프록시에 이 디렉터리를 마운트해 두었다면 다시 설치한 뒤 프록시 컨테이너를 다시 만들어야 합니다.

리버스 프록시는 설치 스크립트가 설정하지 않습니다. 아래 "리버스 프록시" 항목을 참고해 주세요.

## 설정

`/etc/default/onmaru`의 환경 변수나 명령줄 플래그로 설정합니다. 값을 바꾼 뒤에는 `sudo systemctl restart onmaru`로 다시 시작해 주세요. 지금 쓰는 값과 그 출처(기본값, 환경 변수, 명령줄 플래그)는 관리자가 설정 앱의 "서버" 페이지에서 볼 수 있습니다.

워크벤치 주소(`-workbench-url`)와 SMART 확인 시각(`-smart-refresh-at`)은 설정 앱의 "서버" 페이지에서 바꿀 수도 있습니다. 여기서 바꾼 값은 상태 디렉터리의 `site.json`에 저장되어 플래그·환경 변수보다 우선하며, 다시 시작하지 않아도 바로 적용됩니다. `install.sh`로 다시 설치하거나 업그레이드해도 그대로 남으므로, 플래그 값으로 돌아가려면 그 페이지에서 "서버 값 사용"을 눌러 주세요.

| 플래그 | 환경 변수 | 기본값 | 설명 |
|---|---|---|---|
| `-listen` | `ONMARU_LISTEN` | `127.0.0.1:8090` | 수신 주소입니다. 루프백 `host:port` 또는 `unix:/절대/경로.sock`(Unix 소켓, 권한 `0660`)을 지정합니다. |
| `-allow-non-loopback` | `ONMARU_ALLOW_NON_LOOPBACK` | `false` | 루프백이 아닌 TCP 주소(`0.0.0.0:8090` 등)도 허용합니다. 위험하니 아래 설명을 먼저 읽어 주세요. |
| `-omv-url` | `ONMARU_OMV_URL` | `http://127.0.0.1` | OMV 웹 UI(nginx) 주소입니다. 백엔드가 `<주소>/rpc.php`로 로그인을 위임합니다. 루프백 주소만 허용합니다. OMV 웹 UI 포트를 바꿨다면 포트까지 적어 주세요(설치 스크립트는 nginx 설정에서 포트를 찾아 제안합니다). 백엔드는 평문 HTTP로만 접속하므로 OMV의 "Force SSL/TLS"는 꺼져 있어야 합니다. |
| `-cookie-path` | `ONMARU_COOKIE_PATH` | `/` | 브라우저가 보는 대시보드 경로(세션 쿠키의 Path)입니다. 경로 접두사 아래에 둘 때 좁힐 수 있습니다. |
| `-state-dir` | `ONMARU_STATE_DIR`, `STATE_DIRECTORY` | (없음) | 대시보드 설정을 저장할 디렉터리입니다(사용자 설정 `users/<사용자 이름>.json`, 전역 설정 `site.json`, 배경 이미지 `images/`). 비워 두면 메모리에만 보관하므로 재시작하면 사라집니다. systemd의 `StateDirectory=`가 `STATE_DIRECTORY`를 넘겨줍니다. |
| `-smart-refresh-at` | `ONMARU_SMART_REFRESH_AT` | `00:00` | SMART 정보를 하루 한 번 다시 가져오는 기준 시각입니다(`HH:MM`, 서버 로컬 시간). 마지막 조회가 이 시각 이전이면 다음 접속 때 다시 가져옵니다. SMART 조회는 대기 중인 디스크를 깨우므로 자동 주기 조회는 하지 않습니다. |
| `-workbench-url` | `ONMARU_WORKBENCH_URL` | (없음) | 브라우저에서 OMV 웹 UI(workbench)를 열 주소입니다. 아래 "OMV 워크벤치 열기"를 참고해 주세요. 비워 두면 "OMV 워크벤치" 앱과 프로필 메뉴 항목이 보이지 않습니다. |

세션은 백엔드 메모리에만 있으므로 백엔드를 재시작하면 모든 사용자가 다시 로그인해야 합니다. `/var/lib/onmaru`에는 대시보드 자신의 상태만 들어 있으며 OMV 설정과는 관계가 없습니다. 백업할 때 이 디렉터리를 함께 복사하면 사용자 설정, 전역 설정, 올린 배경 이미지가 유지됩니다.

### OMV 워크벤치 열기

`-workbench-url`을 설정하면 "모든 앱"·바탕화면 바로가기의 "OMV 워크벤치"와 프로필 메뉴의 "OMV 워크벤치 열기"가 OMV 웹 UI를 브라우저의 새 탭으로 엽니다.

- 값은 브라우저에서 OMV에 접속하는 주소입니다(예: `https://nas.example.lan/`). `http://`·`https://`로 시작하는 주소 또는 대시보드 기준 경로(`/`, `./`, `../`)를 받습니다. 대시보드를 OMV와 같은 호스트의 경로 접두사(예: `/desk/`) 아래에 두었다면 `/`도 됩니다. 쿼리·프래그먼트는 받지 않으며, `nas.example.lan/`처럼 `http(s)://`가 없는 주소는 거부합니다.
- 같은 탭을 다시 씁니다. 이미 연 OMV 탭이 있으면 새 탭을 만들지 않고 그 탭에서 OMV 첫 화면으로 이동합니다.
- OMV 로그인은 그 탭에서 따로 합니다. OMV는 탭마다 로그인 상태를 따로 두고 비밀번호 없이 로그인하는 방법을 제공하지 않으므로, 대시보드 로그인을 OMV 탭에 넘길 수 없습니다. 탭을 닫지 않고 다시 쓰는 동안에는 로그인이 유지됩니다. 처음 로그인하는 브라우저라면 OMV가 "새 브라우저 로그인" 알림 메일을 보낼 수 있습니다.
- 대시보드 안의 창(iframe)으로 띄우지 않는 이유: OMV 웹 UI는 `X-Frame-Options: SAMEORIGIN`을 보내므로 대시보드와 출처(호스트·포트)가 같을 때만 iframe에 들어갈 수 있습니다. 보안 헤더를 끄는 방식은 쓰지 않습니다.

## 리버스 프록시

대시보드는 따로 설치한 리버스 프록시 뒤에 둡니다. 아래 예시는 Caddy 기준이며, 다른 프록시를 쓰더라도 "프록시 요구 사항"을 만족하면 됩니다. 대시보드를 둘 호스트와 경로는 프록시 설정에서 정합니다. 별도 서브도메인의 루트에 두어도 되고, 같은 도메인의 경로 접두사(예: `/desk/`) 아래에 두어도 됩니다. 프론트엔드는 상대 경로로 빌드되어 있습니다.

### 수신 방식 고르기

| 프록시 위치 | 수신 방식 | 프록시 대상 |
|---|---|---|
| 이 호스트에서 직접 실행 | 루프백 TCP `127.0.0.1:8090` | `http://127.0.0.1:8090` |
| Docker 컨테이너(bridge 네트워크) | Unix 소켓 `unix:/run/onmaru/onmaru.sock` | 컨테이너에 `/run/onmaru`를 바인드 마운트하고 소켓으로 연결 |
| 그 밖에 꼭 필요한 경우 | `-allow-non-loopback`과 `0.0.0.0:8090` 등 | 해당 주소 |

- 컨테이너 안의 `127.0.0.1`은 컨테이너 자기 자신을 가리키므로 호스트의 루프백에 닿지 않습니다. 그래서 Docker 안의 프록시에는 Unix 소켓을 권장합니다.
- 소켓은 서비스 사용자와 그 기본 그룹(기본값 `onmaru:onmaru`) 소유에 권한 `0660`입니다. root로 실행되는 컨테이너는 접속할 수 있습니다. 비root 사용자로 실행되는 프록시 이미지나 userns-remap 환경에서는 접속할 수 없으니, 프록시 프로세스가 그 그룹 권한을 갖도록 설정해 주세요. 호스트에서 실행하는 프록시라면 프록시 사용자를 서비스 그룹에 추가합니다.
- systemd 유닛은 `RuntimeDirectoryPreserve=yes`로 `/run/onmaru`를 재시작 사이에도 유지합니다. 디렉터리가 새로 만들어지면 컨테이너의 바인드 마운트가 이전 디렉터리를 계속 가리켜 연결이 끊기기 때문입니다.
- **루프백이 아닌 주소에서 수신하는 것은 위험합니다.** 그 주소에 닿을 수 있는 누구나 프록시와 HTTPS를 거치지 않고 접속할 수 있으며, 비밀번호가 평문 HTTP로 전송됩니다. 또한 `X-Forwarded-For`·`X-Forwarded-Proto`를 위조해 로그인 시도 제한을 우회하거나 OMV 인증 로그(fail2ban)에 가짜 IP를 남길 수 있습니다. 이 방식을 사용한다면 방화벽으로 접근을 제한해 주세요.

### 프록시 요구 사항

- `X-Forwarded-Proto`에 브라우저가 접속한 프로토콜(`https` 또는 `http`)을 전달해야 합니다. 백엔드는 이 값이 `https`일 때만 세션 쿠키에 `Secure`를 붙입니다.
- `X-Forwarded-For`의 마지막 항목이 브라우저 IP여야 합니다. 백엔드는 마지막 항목만 로그인 시도 제한과 OMV 인증 로그에 사용합니다. 앞쪽 항목은 브라우저가 임의로 보낼 수 있으므로 쓰지 않습니다.
- 경로 접두사 아래에 둘 때는 접두사를 떼고 전달하고, 끝 슬래시가 없는 주소(`/desk`)를 `/desk/`로 리다이렉트해 주세요. 상대 경로가 `/desk/`를 기준으로 해석되기 때문입니다.
- 응답 대기 시간이 3분보다 길어야 합니다. 서비스 켜기·끄기 같은 조작은 OMV 작업이 끝날 때까지 최대 3분 동안 응답을 기다리며, 프록시가 먼저 연결을 끊으면 작업은 계속 진행되지만 화면에는 시간 초과 오류가 표시됩니다. nginx 계열 프록시는 기본값이 60초이므로 `proxy_read_timeout`을 늘려 주세요.

Caddy의 `reverse_proxy`는 기본 설정으로 이 조건을 모두 만족합니다. `X-Forwarded-Proto`와 `X-Forwarded-For`를 붙이고, 응답 대기 시간에 제한이 없습니다. Caddy 앞에 다른 프록시가 없다면 `trusted_proxies`는 설정하지 마세요. 그래야 브라우저가 보낸 `X-Forwarded-For`를 Caddy가 덮어씁니다.

### Caddy 예시: OMV 기본 포트를 유지하는 경우

OMV 웹 UI가 설치할 때의 기본값대로 80번 포트(HTTP)를 쓰고 있다면, Caddy는 443번 포트(HTTPS)만 사용하게 합니다. OMV 설정은 바꾸지 않으며, OMV 웹 UI 설정(시스템 → Workbench)의 SSL/TLS는 443번 포트가 겹치지 않도록 꺼 둔 상태(기본값)여야 합니다.

Caddy는 기본적으로 80번 포트에서 HTTP를 HTTPS로 리다이렉트하므로 OMV nginx와 포트가 겹칩니다. 전역 옵션 `auto_https disable_redirects`를 주면 이 리다이렉트를 만들지 않고 80번 포트를 열지 않습니다. 따라서 `http://` 주소로 접속하면 OMV 웹 UI가 응답하고, 대시보드는 반드시 `https://` 주소로 접속해야 합니다. Caddy를 Docker로 실행한다면 443번 포트만 publish합니다.

80번 포트를 쓸 수 없으므로 HTTP-01 방식의 인증서 발급은 할 수 없습니다. 로컬 네트워크에서는 다음 중 하나를 사용합니다.

- `tls internal`: Caddy가 자체 CA로 인증서를 발급합니다. 브라우저 경고를 없애려면 Caddy의 루트 인증서(`<Caddy 데이터 디렉터리>/pki/authorities/local/root.crt`)를 접속하는 기기에 신뢰할 인증서로 등록합니다.
- DNS-01 방식: 실제로 소유한 도메인이 있다면 DNS 공급자 모듈을 포함한 Caddy로 공인 인증서를 받을 수 있습니다. 설정 방법은 Caddy 문서의 `tls` 지시어 `dns` 항목을 참고해 주세요.

서브도메인 루트에 두는 경우:

```caddyfile
{
	auto_https disable_redirects
}

dash.example.lan {
	tls internal
	reverse_proxy 127.0.0.1:8090
}
```

OMV와 같은 이름의 경로 접두사 아래에 두는 경우입니다. `https://nas.example.lan/`은 Caddy를 거쳐 OMV 웹 UI로, `https://nas.example.lan/desk/`는 대시보드로 연결됩니다. 기존 `http://nas.example.lan/`도 그대로 OMV 웹 UI에 접속됩니다.

```caddyfile
{
	auto_https disable_redirects
}

nas.example.lan {
	tls internal
	redir /desk /desk/ 308
	handle_path /desk/* {
		reverse_proxy 127.0.0.1:8090
	}
	handle {
		reverse_proxy 127.0.0.1:80   # OMV 웹 UI(기본 포트)
	}
}
```

두 예시 모두 Caddy가 호스트에서 실행되는 경우입니다. Docker 컨테이너의 Caddy라면 대시보드는 아래의 Unix 소켓 예시처럼 연결하고, OMV 웹 UI 주소는 컨테이너에서 닿는 호스트 주소로 바꿔 주세요.

### Caddy 예시: Caddy를 메인 리버스 프록시로 쓰는 경우

Caddy가 80·443번 포트를 모두 맡아 HTTPS 리다이렉트와 인증서 발급·갱신을 하고, OMV 웹 UI와 온마루를 모두 Caddy 뒤에 두는 구성입니다. 이 경우 OMV 웹 UI 설정(시스템 → Workbench)에서 OMV 포트를 다른 값으로 옮기고 SSL/TLS는 끕니다. 아래 예시에서는 `8001`로 옮겼다고 가정합니다. 온마루의 OMV 주소도 바꾼 포트에 맞춰 `http://127.0.0.1:8001`로 설정해야 합니다(설치 스크립트는 nginx 설정에서 포트를 찾아 제안합니다).

서브도메인 루트에 두고, 프록시가 호스트에서 실행되는 경우:

```caddyfile
dash.example.com {
	reverse_proxy 127.0.0.1:8090
}
```

Docker 컨테이너의 Caddy에서 Unix 소켓으로 연결하는 경우(컨테이너에 `- /run/onmaru:/run/onmaru` 볼륨을 추가해야 합니다):

```caddyfile
dash.example.com {
	reverse_proxy unix//run/onmaru/onmaru.sock
}
```

OMV와 같은 도메인의 경로 접두사 아래에 두는 경우:

```caddyfile
nas.example.com {
	redir /desk /desk/ 308
	handle_path /desk/* {
		reverse_proxy 127.0.0.1:8090
	}
	handle {
		reverse_proxy 127.0.0.1:8001   # OMV 웹 UI(옮긴 포트)
	}
}
```

## 설정 앱

"모든 앱"의 "설정"이나 작업표시줄 프로필 메뉴의 머리글(사용자 이름)을 누르면 열립니다. 창이 좁으면 왼쪽 메뉴가 숨고 ☰ 단추로 엽니다.

| 페이지 | 내용 | 권한 |
|---|---|---|
| 화면 | 테마(자동·라이트·다크)와 바탕화면(기본 배경 또는 올린 이미지) | 모든 사용자 |
| 일반 | 언어(이 브라우저에만 적용)와 애니메이션 | 모든 사용자 |
| 로그인 화면 | 로그인 배경과 대기 연출 켜기·끄기 | 관리자 |
| 기본 화면 | 화면 설정을 정하지 않은 사용자에게 적용할 기본 테마·바탕화면 | 관리자 |
| 서버 | 워크벤치 주소·SMART 확인 시각 바꾸기, 가동 설정 현황과 바꾸는 방법 | 관리자 |
| 정보 | 버전, 라이선스, 오픈 소스 고지 | 모든 사용자 |

- 관리자 페이지는 OMV 관리자 계정에만 보이며, 모든 사용자에게 적용됩니다.
- 배경 이미지는 PNG, JPEG, WebP를 20MB까지 올릴 수 있습니다. 리버스 프록시에 요청 본문 크기 제한이 있다면 20MB 이상으로 맞춰 주세요(nginx의 기본값은 1MB입니다).
- 로그인 배경은 로그인하기 전에 보이는 그림이므로, 대시보드 주소에 접속할 수 있는 누구나 볼 수 있습니다.
- OMV와 운영체제 설정은 설정 앱에서 다루지 않습니다. OMV 웹 UI에서 바꿔 주세요.

## 개발

개발 명령과 설계 문서는 `CLAUDE.md`와 `.claude/` 디렉터리에 있습니다. `make dev-fake`를 실행하면 가짜 OMV에 연결된 개발 서버가 뜹니다.

## 라이선스

`LICENSE` 파일을 참고해 주세요. 함께 포함된 아이콘 등 제3자 에셋의 출처와 라이선스는 `vendor/icons/README.md`에 정리되어 있습니다.
