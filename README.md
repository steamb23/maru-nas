# 온마루 (Onmaru)

> A suspiciously normal desktop-like dashboard for openmediavault.

OpenMediaVault 8 위에 올리는 데스크톱형 웹 대시보드입니다. 창·작업표시줄·바탕화면 위젯으로 NAS 가동 현황을 확인하고, 컨테이너와 핵심 서비스를 간단히 켜고 끌 수 있습니다. 심층 설정은 OMV 기본 웹 UI에서 합니다.

- 로컬 네트워크 전용입니다. 인터넷에 노출하는 구성을 전제로 하지 않습니다.
- 로그인은 OMV 계정으로 합니다. 백엔드가 OMV `rpc.php`에 로그인을 위임하며, 설정 변경도 로그인한 사용자의 OMV 세션으로만 합니다.
- Go 실행 파일 하나(프론트엔드 포함)를 systemd 서비스로 실행하고, 앞단의 리버스 프록시가 HTTPS를 맡습니다.

## 빌드

Go와 Node.js(npm)가 필요합니다. 일반 사용자 계정으로 실행해 주세요.

```sh
git submodule update --init --depth 1
npm --prefix frontend ci
make build        # bin/onmaru
```

## 설치

```sh
sudo ./install.sh
```

인터랙티브 설치 스크립트입니다. 서비스 사용자, 수신 방식, OMV 주소, SMART 갱신 시각을 물어본 뒤 다음 작업을 합니다.

- 시스템 사용자를 만들고(기본값 `onmaru`), `bin/onmaru`를 `/usr/local/bin/onmaru`에 설치합니다.
- 설정 파일 `/etc/default/onmaru`와 systemd 유닛 `/etc/systemd/system/onmaru.service`를 작성한 뒤 서비스를 시작합니다.
- 사용자 설정(위젯 배치)은 `/var/lib/onmaru`(`StateDirectory=`)에 저장됩니다.

스크립트는 빌드를 하지 않으므로 먼저 `make build`를 실행해 주세요. 다시 실행하면 기존 설정을 기본값으로 보여 주며, 새로 빌드한 실행 파일로 업그레이드할 때도 같은 명령을 사용합니다. 안내 문구는 로캘에 따라 영어 또는 한국어로 표시되며, `--lang en|ko`로 직접 고를 수도 있습니다.

제거하려면 `sudo ./install.sh --uninstall`을 실행합니다. `/var/lib/onmaru`와 서비스 사용자는 확인을 받은 뒤에만 삭제합니다.

리버스 프록시는 설치 스크립트가 설정하지 않습니다. 아래 "리버스 프록시" 항목을 참고해 주세요.

## 설정

`/etc/default/onmaru`의 환경 변수나 명령줄 플래그로 설정합니다. 값을 바꾼 뒤에는 `sudo systemctl restart onmaru`로 다시 시작해 주세요.

| 플래그 | 환경 변수 | 기본값 | 설명 |
|---|---|---|---|
| `-listen` | `ONMARU_LISTEN` | `127.0.0.1:8090` | 수신 주소입니다. 루프백 `host:port` 또는 `unix:/절대/경로.sock`(Unix 소켓, 권한 `0660`)을 지정합니다. |
| `-allow-non-loopback` | `ONMARU_ALLOW_NON_LOOPBACK` | `false` | 루프백이 아닌 TCP 주소(`0.0.0.0:8090` 등)도 허용합니다. 위험하니 아래 설명을 먼저 읽어 주세요. |
| `-omv-url` | `ONMARU_OMV_URL` | `http://127.0.0.1` | OMV 웹 UI(nginx) 주소입니다. 백엔드가 `<주소>/rpc.php`로 로그인을 위임합니다. 루프백 주소만 허용합니다. OMV 웹 UI 포트를 바꿨다면 포트까지 적어 주세요(설치 스크립트는 nginx 설정에서 포트를 찾아 제안합니다). |
| `-cookie-path` | `ONMARU_COOKIE_PATH` | `/` | 브라우저가 보는 대시보드 경로(세션 쿠키의 Path)입니다. 경로 접두사 아래에 둘 때 좁힐 수 있습니다. |
| `-state-dir` | `ONMARU_STATE_DIR`, `STATE_DIRECTORY` | (없음) | 사용자 설정을 저장할 디렉터리입니다(`users/<사용자 이름>.json`). 비워 두면 메모리에만 보관하므로 재시작하면 사라집니다. systemd의 `StateDirectory=`가 `STATE_DIRECTORY`를 넘겨줍니다. |
| `-smart-refresh-at` | `ONMARU_SMART_REFRESH_AT` | `00:00` | SMART 정보를 하루 한 번 다시 가져오는 기준 시각입니다(`HH:MM`, 서버 로컬 시간). 마지막 조회가 이 시각 이전이면 다음 접속 때 다시 가져옵니다. SMART 조회는 대기 중인 디스크를 깨우므로 자동 주기 조회는 하지 않습니다. |

세션은 백엔드 메모리에만 있으므로 백엔드를 재시작하면 모든 사용자가 다시 로그인해야 합니다. `/var/lib/onmaru`에는 대시보드 자신의 상태만 들어 있으며 OMV 설정과는 관계가 없습니다. 백업할 때 이 디렉터리를 함께 복사하면 사용자별 위젯 배치가 유지됩니다.

## 리버스 프록시

대시보드를 둘 호스트와 경로는 프록시 설정에서 정합니다. 별도 서브도메인의 루트에 두어도 되고, 같은 도메인의 경로 접두사(예: `/desk/`) 아래에 두어도 됩니다. 프론트엔드는 상대 경로로 빌드되어 있습니다.

### 수신 방식 고르기

| 프록시 위치 | 수신 방식 | 프록시 대상 |
|---|---|---|
| 이 호스트에서 직접 실행 | 루프백 TCP `127.0.0.1:8090` | `http://127.0.0.1:8090` |
| Docker 컨테이너(bridge 네트워크) | Unix 소켓 `unix:/run/onmaru/onmaru.sock` | 컨테이너에 `/run/onmaru`를 바인드 마운트하고 소켓으로 연결 |
| 그 밖에 꼭 필요한 경우 | `-allow-non-loopback`과 `0.0.0.0:8090` 등 | 해당 주소 |

- 컨테이너 안의 `127.0.0.1`은 컨테이너 자기 자신을 가리키므로 호스트의 루프백에 닿지 않습니다. 그래서 Docker 안의 프록시에는 Unix 소켓을 권장합니다.
- 소켓은 `onmaru:onmaru` 소유에 권한 `0660`입니다. 컨테이너의 root는 접속할 수 있고, 호스트에서 실행하는 프록시라면 프록시 사용자를 서비스 그룹(`onmaru`)에 추가해 주세요.
- systemd 유닛은 `RuntimeDirectoryPreserve=yes`로 `/run/onmaru`를 재시작 사이에도 유지합니다. 디렉터리가 새로 만들어지면 컨테이너의 바인드 마운트가 이전 디렉터리를 계속 가리켜 연결이 끊기기 때문입니다.
- **루프백이 아닌 주소에서 수신하는 것은 위험합니다.** 그 주소에 닿을 수 있는 누구나 프록시와 HTTPS를 거치지 않고 접속할 수 있으며, 비밀번호가 평문 HTTP로 전송됩니다. 또한 `X-Forwarded-For`·`X-Forwarded-Proto`를 위조해 로그인 시도 제한을 우회하거나 OMV 인증 로그(fail2ban)에 가짜 IP를 남길 수 있습니다. 이 방식을 사용한다면 방화벽으로 접근을 제한해 주세요.

### 프록시 요구 사항

- `X-Forwarded-Proto: https`를 전달해야 합니다. 백엔드는 이 값이 있을 때만 세션 쿠키에 `Secure`를 붙입니다(Caddy `reverse_proxy`의 기본 동작입니다).
- `X-Forwarded-For`에 브라우저 IP를 넣어야 합니다(Caddy의 기본 동작입니다). 백엔드는 마지막 항목을 로그인 시도 제한과 OMV 인증 로그에 사용합니다. Caddy 앞에 다른 프록시가 없다면 `trusted_proxies`는 설정하지 마세요. 그래야 브라우저가 보낸 값을 Caddy가 덮어씁니다.
- 경로 접두사 아래에 둘 때는 접두사를 떼고 전달하고, 끝 슬래시가 없는 주소(`/desk`)를 `/desk/`로 리다이렉트해 주세요. 상대 경로가 `/desk/`를 기준으로 해석되기 때문입니다.

### Caddy 예시

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
		reverse_proxy 127.0.0.1:8001   # OMV 웹 UI
	}
}
```

## 개발

개발 명령과 설계 문서는 `CLAUDE.md`와 `.claude/` 디렉터리에 있습니다. `make dev-fake`를 실행하면 가짜 OMV에 연결된 개발 서버가 뜹니다.

## 라이선스

`LICENSE` 파일을 참고해 주세요.
