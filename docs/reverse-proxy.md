# 리버스 프록시

대시보드는 루프백 주소나 Unix 소켓에서만 받고, HTTPS는 앞단의 리버스 프록시가 맡습니다. 설치 스크립트는 프록시를 설정하지 않습니다. 아래 예시는 Caddy 기준이며, 다른 프록시도 [프록시 요구 사항](#프록시-요구-사항)을 채우면 됩니다.

대시보드를 둘 호스트와 경로는 프록시 설정에서 정합니다. 별도 서브도메인의 루트에 두어도 되고, OMV와 같은 도메인의 경로 접두사(예: `/desk/`) 아래에 두어도 됩니다.

## 수신 방식 고르기

| 프록시 위치 | 수신 방식(`--listen`) | 프록시 대상 |
|---|---|---|
| 이 호스트에서 직접 실행 | 루프백 TCP `127.0.0.1:8090`(기본값) | `http://127.0.0.1:8090` |
| Docker 컨테이너(bridge 네트워크) | Unix 소켓 `unix:/run/maru/maru.sock` | 컨테이너에 `/run/maru`를 바인드 마운트하고 소켓으로 연결 |
| 그 밖에 꼭 필요한 경우 | `--allow-non-loopback`과 `0.0.0.0:8090` 등 | 해당 주소 |

- 컨테이너 안의 `127.0.0.1`은 컨테이너 자기 자신이라 호스트의 루프백에 닿지 않습니다. 그래서 Docker 안의 프록시에는 Unix 소켓을 권장합니다.
- 소켓은 서비스 사용자와 그 기본 그룹(기본값 `maru:maru`) 소유에 권한 `0660`입니다. root로 실행되는 컨테이너는 접속할 수 있지만, 비root 사용자로 실행되는 프록시 이미지나 userns-remap 환경에서는 프록시 프로세스가 그 그룹 권한을 갖도록 설정해야 합니다. 호스트에서 실행하는 프록시라면 프록시 사용자를 서비스 그룹에 추가합니다.
- `/run/maru`는 maru를 다시 시작해도 그대로 남아 컨테이너의 바인드 마운트가 끊기지 않습니다. `./install.sh --uninstall`로 지웠다면 다시 설치한 뒤 프록시 컨테이너를 다시 만들어야 합니다.

> [!WARNING]
> 루프백이 아닌 주소에서 받으면 그 주소에 닿을 수 있는 누구나 프록시와 HTTPS를 거치지 않고 접속할 수 있고, 비밀번호가 평문 HTTP로 전송됩니다. `X-Forwarded-For`·`X-Forwarded-Proto`를 위조해 로그인 시도 제한을 우회하거나 OMV 인증 로그(fail2ban)에 가짜 IP를 남길 수도 있습니다. 이 방식을 쓴다면 방화벽으로 접근을 제한해 주세요.

## 프록시 요구 사항

| 항목 | 요구 사항 |
|---|---|
| `X-Forwarded-Proto` | 브라우저가 접속한 프로토콜(`https` 또는 `http`)을 전달합니다. 백엔드는 이 값이 `https`일 때만 세션 쿠키에 `Secure`를 붙입니다. |
| `X-Forwarded-For` | 마지막 항목이 브라우저 IP여야 합니다. 백엔드는 마지막 항목만 로그인 시도 제한과 OMV 인증 로그에 씁니다. 앞쪽 항목은 브라우저가 임의로 보낼 수 있으므로 쓰지 않습니다. |
| 경로 접두사 | 접두사를 떼고 전달하고, 끝 슬래시가 없는 주소(`/desk`)는 `/desk/`로 리다이렉트합니다. 상대 경로가 `/desk/`를 기준으로 해석되기 때문입니다. |
| 캐시 헤더 | 백엔드가 보내는 `Cache-Control`과 `ETag`를 바꾸지 않고 그대로 전달합니다. 그래야 업그레이드한 뒤 새로 고치면 새 파일을 받습니다. |
| WebSocket | 웹 터미널(`/api/terminal`)이 씁니다. WebSocket을 전달하고 `Host` 헤더를 바꾸지 않아야 합니다(백엔드가 `Origin`과 `Host`가 같은지 확인합니다). 백엔드가 30초마다 ping을 보내므로 유휴 연결 제한은 그보다 길어야 합니다. |
| 요청 본문 크기 | 파일 탐색기의 올리기는 파일 하나를 요청 본문 하나로 보내고, 설정 앱의 배경 이미지는 20MB까지입니다. 크기 제한을 풀거나 올릴 파일보다 크게 맞춰 주세요(nginx는 `client_max_body_size 0`, 기본값은 1MB). 큰 파일을 주고받는 동안 본문을 보내고 받는 시간에도 제한이 없어야 합니다. |
| 응답 버퍼링 | 파일 탐색기의 복사와 하위 폴더 검색은 진행 상황을 하나의 응답으로 이어서 보냅니다(`text/event-stream`). 프록시가 응답을 모아 두지 않고 바로 넘겨야 진행 상황이 보입니다. nginx는 따로 설정하지 않아도 됩니다. |
| 응답 대기 시간 | 3분보다 길어야 합니다. 서비스 켜기·끄기 같은 조작은 OMV 작업이 끝날 때까지 최대 3분 동안 응답을 기다립니다. 프록시가 먼저 끊으면 작업은 계속되지만 화면에는 시간 초과 오류가 나옵니다. nginx 계열은 기본값이 60초이므로 `proxy_read_timeout`을 늘려 주세요. |

Caddy의 `reverse_proxy`는 기본 설정으로 이 조건을 모두 채웁니다. Caddy 앞에 다른 프록시가 없다면 `trusted_proxies`는 설정하지 마세요. 그래야 브라우저가 보낸 `X-Forwarded-For`를 Caddy가 덮어씁니다.

## Caddy 설치

Debian 13(trixie) 저장소의 Caddy를 그대로 쓸 수 있습니다.

```sh
sudo apt install caddy
```

- 패키지의 기본 설정은 80번 포트를 씁니다. OMV 웹 UI가 80번 포트를 쓰고 있으면 설치 중에 `caddy.service`를 시작하지 못했다는 메시지가 나오는데, 아래 예시로 `/etc/caddy/Caddyfile`을 바꾼 뒤 `sudo systemctl restart caddy`로 시작하면 됩니다.
- 그 뒤에 설정을 바꾸면 `sudo systemctl reload caddy`로 다시 읽고, 로그는 `journalctl -u caddy`로 봅니다.
- Caddy를 Docker로 실행한다면 [수신 방식](#수신-방식-고르기)에서 Unix 소켓을 고르고 아래 예시의 대상을 소켓으로 바꿉니다.

## Caddy 예시: OMV 기본 포트를 유지하는 경우

OMV 웹 UI가 설치할 때의 기본값대로 80번 포트(HTTP)를 쓰고 있다면, Caddy는 443번 포트(HTTPS)만 쓰게 합니다. OMV 설정은 바꾸지 않으며, OMV 웹 UI 설정(시스템 → Workbench)의 SSL/TLS는 443번 포트가 겹치지 않도록 꺼 둔 상태(기본값)여야 합니다.

- Caddy는 기본적으로 80번 포트에서 HTTP를 HTTPS로 리다이렉트하므로 OMV와 포트가 겹칩니다. 전역 옵션 `auto_https disable_redirects`를 주면 이 리다이렉트를 만들지 않고 80번 포트를 열지 않습니다. 따라서 `http://` 주소로 접속하면 OMV 웹 UI가 응답하고, 대시보드는 `https://` 주소로 접속합니다.
- 80번 포트를 쓸 수 없으므로 HTTP-01 방식의 인증서 발급은 할 수 없습니다. 로컬 네트워크에서는 다음 중 하나를 씁니다.
    - `tls internal`: Caddy가 자체 CA로 인증서를 발급합니다. 호스트 이름 대신 IP 주소에도 발급합니다. 브라우저 경고를 없애고 패스키를 쓰려면 Caddy의 루트 인증서를 접속하는 기기에 신뢰할 인증서로 등록합니다. Debian 패키지의 Caddy라면 `/var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt`, 그 밖에는 `<Caddy 데이터 디렉터리>/pki/authorities/local/root.crt`입니다.
    - DNS-01 방식: 실제로 소유한 도메인이 있다면 DNS 공급자 모듈을 넣은 Caddy로 공인 인증서를 받을 수 있습니다. Caddy 문서의 `tls` 지시어 `dns` 항목을 참고해 주세요.

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

OMV와 같은 주소의 경로 접두사 아래에 두는 경우입니다. `https://nas.example.lan/`은 Caddy를 거쳐 OMV 웹 UI로, `https://nas.example.lan/desk/`는 대시보드로 연결됩니다. 기존 `http://nas.example.lan/`도 그대로 OMV 웹 UI입니다. 이때 [OMV 워크벤치 주소](configuration.md#omv-워크벤치-열기)는 `/`로 둘 수 있습니다.

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

## Caddy 예시: Caddy를 메인 리버스 프록시로 쓰는 경우

Caddy가 80·443번 포트를 모두 맡아 HTTPS 리다이렉트와 인증서 발급·갱신을 하고, OMV 웹 UI와 마루 NAS를 모두 Caddy 뒤에 두는 구성입니다. 이때는 OMV 웹 UI 설정(시스템 → Workbench)에서 OMV 포트를 다른 값으로 옮기고 SSL/TLS를 끕니다. 아래 예시는 `8001`로 옮겼다고 가정합니다. 마루 NAS의 OMV 주소도 바꾼 포트에 맞춰 `http://127.0.0.1:8001`로 설정해야 합니다(설치 스크립트는 nginx 설정에서 포트를 찾아 제안합니다).

서브도메인 루트에 두고, 프록시가 호스트에서 실행되는 경우:

```caddyfile
dash.example.com {
	reverse_proxy 127.0.0.1:8090
}
```

Docker 컨테이너의 Caddy에서 Unix 소켓으로 연결하는 경우(컨테이너에 `- /run/maru:/run/maru` 볼륨을 추가해야 합니다):

```caddyfile
dash.example.com {
	reverse_proxy unix//run/maru/maru.sock
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
