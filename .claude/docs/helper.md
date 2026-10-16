# 권한 도우미(onmaru-helper)

대시보드(`onmaru`)는 권한 없는 전용 사용자로 돌고 권한을 올릴 수 없다(`NoNewPrivileges=yes`). OMV RPC로 할 수 없는 일 중 로그인한 사용자 자신의 Linux 권한이 필요한 일(파일 탐색기의 파일 접근)과 root가 필요한 일(웹에서 가동 설정 바꾸기 등)은 root로 도는 별도 실행 파일 `onmaru-helper`가 맡는다. 사용자 대상 설명은 `README.md`의 "권한 도우미".

## 위협 모델

- 지키려는 것: 네트워크에 닿는 onmaru가 뚫려도 곧바로 root·모든 사용자 권한이 되지 않게 한다
- 도우미가 믿는 것은 세 가지뿐이다. onmaru가 보낸 사용자 이름·그룹·경로 정책은 믿지 않는다
  1. 소켓 상대의 uid(`SO_PEERCRED`)가 onmaru 서비스 사용자(`-client-user`)인지
  2. 자기가 비밀번호를 직접 확인하고 발급한 티켓인지
  3. 자기가 `/etc/passwd`·`/etc/group`·`/etc/login.defs`에서 직접 읽은 계정·그룹
- 남는 위험(README에도 적는다): onmaru가 뚫리면 그때 로그인한 사용자의 티켓을 쓸 수 있고, 그 뒤 입력되는 비밀번호를 가로챌 수 있다. root 연산은 상승한 티켓으로 5분 동안, 정해진 목록의 연산만 가능하다
- OMV 관리자는 원래 OMV RPC로 root와 같은 일을 할 수 있으므로 "OMV 관리자 + 상승 그룹 + 비밀번호 재확인"은 새 권한이 아니라 더 보수적인 겹침이다
- 2차 인증은 도우미가 직접 확인하지 않는다. onmaru가 인증을 마친 뒤에만 대기 티켓을 활성화한다(onmaru가 정상일 때 2차 인증을 건너뛸 수 없다)

## 구성

```
브라우저 ─HTTP→ onmaru(일반 사용자) ─Unix 소켓→ onmaru-helper(root, 소켓 활성화)
                                                  └ exec 자신 `worker` (사용자 uid·gid·보조 그룹, 또는 root)
```

| 패키지 | 역할 |
|---|---|
| `internal/helper/proto` | 프레임 프로토콜, 요청·응답·결과 타입, 오류 코드, 범위별 연산 목록(`Actions`) |
| `internal/helper/daemon` | root 부모: 소켓 활성화, 상대 uid 확인, 티켓, 실패 제한, 계정·상승 정책, worker 실행, 감사 로그, 유휴 종료 |
| `internal/helper/worker` | 자식 프로세스의 연산 구현(`id`, `fs.*`). 연결을 fd 3으로 받아 응답·본문을 직접 주고받는다 |
| `internal/helper/client` | onmaru 쪽 클라이언트 |
| `internal/helper/helperfake` | 테스트·`make dev-fake`용 가짜 도우미(진짜 데몬 + 가짜 계정·비밀번호 + 프로세스 없는 worker) |
| `cmd/onmaru-helper` | 실행 파일(`CGO_ENABLED=0`). onmaru 웹 코드는 링크되지 않는다 |

## 프로토콜

- 연결 하나에 요청 하나. 프레임 = 4바이트 big-endian 길이 + JSON 머리, 그 뒤 머리의 `size`만큼 원시 본문. 요청 머리는 64KiB, 응답 머리는 8MiB(목록)까지다. 부모는 버퍼링 없이 머리만 읽고 본문은 worker가 읽는다
- 요청 머리는 모르는 필드를 거부한다. 비밀번호와 티켓이 들어 있으므로 요청을 로그에 남기지 않는다
- 티켓 관리: `session.open`(사용자, 비밀번호, pending), `session.activate`, `session.close`, `session.reset`, `session.status`, `session.elevate`(비밀번호), `session.drop`
- 연산: `run`(티켓, 범위 `user`|`root`, 연산, 인자, `size`). 연산 목록은 `proto.Actions`
  - 사용자 범위: `id`, `fs.list`, `fs.stat`, `fs.read`(offset·length, 응답 뒤 스트림), `fs.write`(요청 뒤 스트림, 같은 폴더 임시 파일에 받은 뒤 이름 바꾸기), `fs.mkdir`, `fs.rename`, `fs.remove`(`recursive`)
  - root 범위: `id`
- 오류는 코드만 준다(`proto.Code*`). 경로·사용자 이름 같은 세부는 도우미 로그에만 남긴다. onmaru는 코드를 HTTP 오류 코드(`api.Err*`)로 바꾼다

## 티켓과 상승

- 티켓: 256비트 난수, 도우미 메모리에 SHA-256 키로만 보관. 유휴 24시간, 최대 1000개. 사용할 때마다 계정이 그대로인지(같은 uid, 정책 통과) 다시 확인하고 아니면 폐기한다. 도우미가 다시 시작하면 모두 사라진다
- 발급 조건(OMV `UserMgmt.authUser`와 같다): 비밀번호(`unix_chkpwd <user> nonull` + `chkexpiry`), uid 0 아님, `openmediavault-admin`이거나 `UID_MIN..UID_MAX` 안
- onmaru는 OMV 로그인에 성공하면 같은 비밀번호로 `open`한다(5초 제한, 실패해도 로그인은 진행하고 도우미 기능만 없다). 2차 인증이면 `pending` 티켓을 `auth.Pending`에 두고 인증을 마치면 `activate`해 세션으로 옮긴다(`Store.ClaimPending`). 세션이 어떻게 끝나든 `auth.Store`의 삭제 훅(`OnRemove`, `OnRemovePending`)이 `close`한다. onmaru가 시작할 때 `reset`으로 이전 실행의 티켓을 지운다
- 상승: `openmediavault-admin`과 상승 그룹(`-elevate-group`, 기본 `sudo`)에 모두 속한 사용자가 비밀번호를 다시 확인하면 그 티켓에 5분. 절대 시간이고 연산마다 늘리지 않는다. root 연산마다 그룹을 다시 읽어 빠졌으면 상승을 거둔다
- 실패 제한(도우미 자체): 사용자별 5분 안 3번 실패부터 30초에서 두 배씩 최대 15분, 사용자별 동시 확인 1개, 전체 4개, 실패 응답 전 1초. `unix_chkpwd`는 `pam_faillock`을 거치지 않는다(OMV의 webgui PAM은 `deny=3 unlock_time=0`이라 그 길로 확인하면 계정이 영구히 잠길 수 있다)
- Go로 shadow 해시를 직접 검증하지 않는다(stdlib에 crypt/yescrypt가 없다). `sudo -S`로 확인하는 방식은 setuid 실행이 필요해 도우미의 `NoNewPrivileges`를 꺼야 하고 sudoers에 없는 OMV 관리자(내장 admin)는 확인할 수 없어 쓰지 않았다

## worker

- 부모(root)는 머리를 읽고 티켓·범위·연산 허용·상승을 확인한 뒤, `/proc/self/exe worker`를 `SysProcAttr{Credential, Setsid, Pdeathsig: SIGKILL}`, 빈 환경 변수, cwd `/`로 실행한다. 작업(`worker.Job`: 범위, 연산, 인자, 본문 크기, 사용자 이름·홈)은 stdin, 연결은 `ExtraFiles`(fd 3)로 넘긴다. 부모는 사용자 데이터를 읽거나 쓰지 않는다
- 사용자 범위는 사용자 uid·기본 gid·보조 그룹(`os/user`, 로컬 파일)으로 강등한다. root 범위도 도우미의 보조 그룹은 넘기지 않는다. worker는 시작할 때 범위와 실제 uid가 맞는지 다시 확인한다(사용자 범위가 root로 돌면 거절)
- 결과 코드는 종료 코드(0 성공, 2 연산 실패 + stderr `code=<코드>`, 그 밖은 실행 오류)로 부모에 알려 감사 로그에 쓴다
- 시간 제한: 목록·정보 30초(부모), 스트림 6시간(부모) + 60초 무진행(worker의 연결 마감 시각)
- 동시 실행: 전체 16개, 사용자당 4개. 연결은 동시 64개

## 파일 연산의 경로 정책

- 경로는 절대·정리된 경로(`path.Clean`과 같음), NUL 없음, 4096바이트 이하
- 기준 디렉터리 = 사용자 홈(`/`가 아닐 때) + `/srv` 아래 마운트 지점(`/proc/self/mountinfo`). `/srv` 자체와 그 안의 루트 파일시스템 디렉터리(`/srv/salt`, `/srv/pillar`)는 기준이 아니다. 가장 긴 기준을 골라 `os.OpenRoot`로 가두므로 `..`·심볼릭 링크로 기준 밖에 나갈 수 없다(`outside_roots`)
- 실제로 열 수 있는지는 커널이 사용자 권한(소유권·ACL)으로 정한다. 기준 정책은 onmaru가 뚫렸을 때 닿는 범위를 줄이는 겹침이다. 공유 폴더 단위로 더 좁히는 일은 onmaru의 파일 API(파일 탐색기)가 OMV RPC로 한다
- 기준 디렉터리 자체는 지우거나 옮기거나 덮어쓸 수 없다. 이름 바꾸기는 같은 기준 안에서만 하고 대상이 있으면 `exists`다
- 읽기는 일반 파일만(`O_NONBLOCK`으로 열어 FIFO에서 멈추지 않음). 목록은 1만 개까지(`truncated`). umask는 0002(OMV Samba의 create mask 0664 / directory mask 0775)
- 덮어쓰기는 새 inode로 바꾸므로 소유자는 올린 사용자가 되고 ACL은 폴더의 기본 ACL을 따른다. 권한 비트는 되도록 유지한다. 덮어쓰지 않을 때는 하드 링크로 같은 이름이 없는지 원자적으로 확인하고, 하드 링크를 못 쓰는 파일시스템이면 다시 확인한 뒤 이름을 바꾼다
- 올리기가 실패하면 worker가 임시 파일(`.onmaru-upload-<난수>.part`)을 지운다. 시간 초과로 worker가 강제 종료되면(SIGKILL) 지우지 못하고 남는다. 이 접두사의 이름으로는 올릴 수 없다

## systemd

- `onmaru-helper.socket`: `/run/onmaru-helper/helper.sock`, `SocketUser=<onmaru 서비스 사용자>`, `SocketMode=0600`. Docker 프록시에 바인드 마운트하는 `/run/onmaru`와 디렉터리를 나눈다(컨테이너 안의 같은 uid가 소켓에 닿지 않게)
- `onmaru-helper.service`: root지만 `CapabilityBoundingSet=CAP_SETUID CAP_SETGID CAP_KILL`(강등과, uid가 다른 worker 종료), `NoNewPrivileges=yes`, `ProtectSystem=strict` + `ReadWritePaths=-/srv -/home`, `PrivateNetwork=yes`, `RestrictAddressFamilies=AF_UNIX`, `PrivateDevices=yes`, `SystemCallFilter=@system-service` 등. root라도 `CAP_DAC_OVERRIDE`가 없어 남의 파일을 마음대로 읽지 못한다(`/etc/shadow`는 소유자가 root라 `unix_chkpwd`가 읽는다)
- 소켓 활성화로 띄우고, 티켓과 연결이 없이 10분이면 스스로 끝난다(`-idle-exit`). 티켓이 있으면 끝나지 않는다
- onmaru 유닛은 바꾸지 않는다(`ProtectSystem=strict`에서도 소켓 connect는 된다). `Wants=`·`After=onmaru-helper.socket`만 더한다

## root 연산을 더할 때

1. 기능이 정말 OMV RPC로 안 되는지 먼저 확인한다(OMV 소스 `/usr/share/openmediavault/engined/rpc/`)
2. `proto`에 연산 이름과 인자·결과 타입을 더하고 `proto.Actions[ScopeRoot]`에 넣는다. 임의 명령·셸·브라우저가 보낸 경로가 exec 인자로 가는 연산은 만들지 않는다. 인자는 worker가 허용 목록으로 검사하고 실행 인자는 직접 만든다
3. `worker`에 구현하고, 필요한 권한만 유닛에 더한다(`CapabilityBoundingSet`, `ReadWritePaths` 등). `install.sh`의 유닛과 README의 보안 설계도 함께 고친다
4. 감사 로그(`daemon.audit`)에 필요한 세부가 남는지 확인한다(root 범위는 인자의 경로를 남긴다)
5. onmaru 쪽은 관리자 전용 API(`requireAdmin`)로 부르고, `elevation_required`(403)를 받은 화면은 `requestElevation`(`shell/elevationDialog.tsx`)으로 상승한 뒤 다시 시도한다
6. 테스트: `proto.Allowed`의 범위 검사, worker 단위 테스트, 데몬의 상승 필요·그룹 재확인, 서버 테스트(helperfake)
