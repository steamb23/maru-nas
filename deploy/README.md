# deploy

운영 배포용 설정 예시를 두는 곳이다. 아직 작성하지 않았다.

예정된 파일:

- systemd 유닛: `bin/omvdesk`를 전용 일반 사용자로 실행, `127.0.0.1:8090`에 바인딩
- Caddy 설정 예시: 같은 도메인에서 `/desk/*`는 백엔드로(`handle_path`로 `/desk` 접두사 제거), 나머지는 OMV로 리버스 프록시

설치는 사용자가 직접 한다. 도메인·IP는 이 디렉터리에 커밋하지 않는다.

## 백엔드 설정 (2단계 로그인 이후)

| 플래그 | 환경 변수 | 기본값 | 설명 |
|---|---|---|---|
| `-listen` | `OMVDESK_LISTEN` | `127.0.0.1:8090` | 수신 주소. 루프백만 허용 |
| `-omv-url` | `OMVDESK_OMV_URL` | `http://127.0.0.1` | OMV 웹 UI(nginx) 주소. 백엔드가 `<주소>/rpc.php`로 로그인을 위임한다. 루프백만 허용. OMV 웹 UI 포트를 바꿨다면 포트까지 적는다 |
| `-cookie-path` | `OMVDESK_COOKIE_PATH` | `/desk` | 브라우저가 보는 대시보드 경로(세션 쿠키 Path) |
| `-state-dir` | `OMVDESK_STATE_DIR`, `STATE_DIRECTORY` | (없음) | 사용자 설정(위젯 배치 등)을 저장할 디렉터리. `users/<사용자 이름>.json`으로 저장한다. 비우면 메모리에만 두어 재시작하면 사라진다 |

앞단 프록시(Caddy)가 지켜야 할 점:

- `X-Forwarded-Proto: https`를 넘긴다. 백엔드는 이 값이 있을 때만 세션 쿠키에 `Secure`를 붙인다. Caddy `reverse_proxy`는 기본으로 넘긴다
- `X-Forwarded-For`에 브라우저 IP를 넣는다(Caddy 기본 동작). 백엔드는 마지막 항목을 로그인 시도 제한과 OMV 인증 로그에 쓴다. Caddy 앞에 다른 프록시를 두지 않는다면 `trusted_proxies`는 설정하지 않는다(브라우저가 보낸 값을 덮어쓰게)
- 세션은 백엔드 메모리에만 있으므로 백엔드를 재시작하면 모든 사용자가 다시 로그인한다

systemd 유닛에서는 `StateDirectory=omvdesk`를 쓴다. systemd가 `/var/lib/omvdesk`를 서비스 사용자 소유로 만들고 `STATE_DIRECTORY`로 알려 주므로 `-state-dir`를 따로 주지 않아도 된다. 이 디렉터리는 대시보드 자신의 상태만 담으며 OMV 설정과 무관하다. 백업할 때 함께 복사하면 사용자별 위젯 배치가 유지된다.
