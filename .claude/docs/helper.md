# 권한 도우미(onmaru-helper)

대시보드(`onmaru`)는 권한 없는 전용 사용자로 돌고 권한을 올릴 수 없다(`NoNewPrivileges=yes`). OMV RPC로 할 수 없는 일 중 로그인한 사용자 자신의 Linux 권한이 필요한 일(파일 탐색기의 파일 접근)과 root가 필요한 일(웹에서 가동 설정 바꾸기 등)은 root로 도는 별도 실행 파일 `onmaru-helper`가 맡는다. 사용자 대상 설명은 `README.md`의 "권한 도우미".

## 위협 모델

- 지키려는 것: 네트워크에 닿는 onmaru가 뚫려도 곧바로 root·모든 사용자 권한이 되지 않게 한다
- 도우미가 믿는 것은 세 가지뿐이다. onmaru가 보낸 사용자 이름·그룹·경로 정책은 믿지 않는다
  1. 소켓 상대의 uid(`SO_PEERCRED`)가 onmaru 서비스 사용자(`-client-user`)인지
  2. 자기가 비밀번호를 직접 확인하고 발급한 티켓인지
  3. 자기가 `/etc/passwd`·`/etc/group`·`/etc/login.defs`에서 직접 읽은 계정·그룹
- 남는 위험(README에도 적는다): onmaru가 뚫리면 그때 로그인한 사용자의 티켓을 쓸 수 있고, 그 뒤 입력되는 비밀번호를 가로챌 수 있다. root 연산은 상승한 티켓으로 5분 동안, 정해진 목록의 연산만 가능하다
- 대시보드에서 업그레이드(`upgrade.apply`, 아래 "대시보드에서 업그레이드")를 켜면 상승한 티켓으로 할 수 있는 일이 `id`에서 "설치된 저장소로 업그레이드"로 넓어진다. 업그레이드는 저장소에서 빌드한 코드를 root로 실행하므로 저장소 소유자 계정은 업그레이드로 root 코드를 실행할 수 있다(sudo로 `./install.sh`를 실행할 때와 같다). OMV 관리자는 원래 root와 같은 일을 할 수 있으므로 관리자에게 새로운 권한은 아니며, 끌 수 있는 선택 설정(`--no-web-upgrade`)이다
- 디스크 휴지통 만들기(`trash.prepare`, 아래 "휴지통")는 상승한 티켓으로 `/srv` 아래 마운트 지점 맨 위에 1777(sticky) `.Trash` 폴더 하나만 만든다(경로는 받지 않고 디스크만 받는다). 그 디스크에서 모든 사용자가 자기 휴지통(`.Trash/<uid>`)을 만들 수 있게 되는 것뿐이고, sticky 비트 때문에 남의 휴지통 폴더는 지우거나 옮길 수 없다. OMV 관리자는 원래 root와 같은 일을 할 수 있으므로 새 권한은 아니다
- 웹 터미널의 임시 SSH 키(`ssh.authorize`)는 새 권한이 아니다. 티켓을 가진 쪽은 원래 `fs.write`로 사용자 홈의 `~/.ssh/authorized_keys`를 쓸 수 있다. 그래도 키 줄은 worker가 직접 만들고(옵션을 onmaru가 정할 수 없다) ed25519 공개 키만 받는다
- OMV 관리자는 원래 OMV RPC로 root와 같은 일을 할 수 있으므로 "OMV 관리자 + 상승 그룹 + 비밀번호 재확인"은 새 권한이 아니라 더 보수적인 겹침이다
- 2차 인증은 도우미가 직접 확인하지 않는다. onmaru가 인증을 마친 뒤에만 대기 티켓을 활성화한다(onmaru가 정상일 때 2차 인증을 건너뛸 수 없다)
  - 보안 강화 모드(패스키·TOTP·복구 코드, `.claude/rules/security.md`)도 onmaru가 확인하고 도우미에 넣지 않는다. onmaru는 정책을 채우지 못한 세션의 상승을 도우미를 부르기 전에 막을 뿐이다. 로드맵의 패스키 로그인은 브라우저가 푼 비밀번호로 지금과 같이 `session.open`을 부르므로 도우미는 바뀌지 않는다(`ROADMAP.md`)

## 구성

```
브라우저 ─HTTP→ onmaru(일반 사용자) ─Unix 소켓→ onmaru-helper(root, 소켓 활성화)
                                                  └ exec 자신 `worker` (사용자 uid·gid·보조 그룹, 또는 root)
```

| 패키지 | 역할 |
|---|---|
| `internal/helper/proto` | 프레임 프로토콜, 요청·응답·결과 타입, 오류 코드, 범위별 연산 목록(`Actions`) |
| `internal/helper/daemon` | root 부모: 소켓 활성화, 상대 uid 확인, 티켓, 실패 제한, 계정·상승 정책, worker 실행, 감사 로그, 유휴 종료 |
| `internal/helper/worker` | 자식 프로세스의 연산 구현(`id`, `fs.*`, `ssh.*`, `upgrade.apply`, `trash.prepare`). 연결을 fd 3으로 받아 응답·본문을 직접 주고받는다 |
| `internal/helper/client` | onmaru 쪽 클라이언트 |
| `internal/helper/helperfake` | 테스트·`make dev-fake`용 가짜 도우미(진짜 데몬 + 가짜 계정·비밀번호 + 프로세스 없는 worker) |
| `cmd/onmaru-helper` | 실행 파일(`CGO_ENABLED=0`). onmaru 웹 코드는 링크되지 않는다 |

## 프로토콜

- 연결 하나에 요청 하나. 프레임 = 4바이트 big-endian 길이 + JSON 머리, 그 뒤 머리의 `size`만큼 원시 본문. 요청 머리는 64KiB, 응답 머리는 8MiB(목록)까지다. 부모는 버퍼링 없이 머리만 읽고 본문은 worker가 읽는다
- 요청 머리는 모르는 필드를 거부한다. 비밀번호와 티켓이 들어 있으므로 요청을 로그에 남기지 않는다
- 티켓 관리: `session.open`(사용자, 비밀번호, pending), `session.activate`, `session.close`, `session.reset`, `session.status`, `session.elevate`(비밀번호), `session.drop`
- 연산: `run`(티켓, 범위 `user`|`root`, 연산, 인자, `size`). 연산 목록은 `proto.Actions`
  - 사용자 범위: `id`(티켓 사용자의 홈 `home`도 준다), `fs.list`, `fs.stat`, `fs.read`(offset·length, 응답 뒤 스트림), `fs.write`(요청 뒤 스트림, 같은 폴더 임시 파일에 받은 뒤 이름 바꾸기), `fs.mkdir`, `fs.rename`, `fs.remove`(`recursive`), 파일 탐색기의 `fs.names`·`fs.statmany`·`fs.info`(아래 "파일 탐색기 연산"), `ssh.authorize`·`ssh.revoke`(웹 터미널의 임시 SSH 키, 아래 "임시 SSH 키"), 휴지통의 `fs.trash`·`fs.trashdirs`·`fs.trashlist`·`fs.trashrestore`·`fs.trashpurge`·`fs.trashempty`(아래 "휴지통")
  - root 범위: `id`, `upgrade.apply`(대시보드에서 업그레이드의 적용 요청, 아래 "대시보드에서 업그레이드"), `trash.prepare`(디스크 맨 위의 공유 휴지통 `.Trash` 만들기, 아래 "휴지통")
- 오류는 코드만 준다(`proto.Code*`). 경로·사용자 이름 같은 세부는 도우미 로그에만 남긴다. onmaru는 코드를 HTTP 오류 코드(`api.Err*`)로 바꾼다

## 티켓과 상승

- 티켓: 256비트 난수, 도우미 메모리에 SHA-256 키로만 보관. 유휴 24시간, 최대 1000개. 상태 확인(`session.status`)도 사용으로 치므로 실제 수명은 onmaru 세션이 정한다(세션이 끝나면 삭제 훅이 닫는다). 유휴 기한은 onmaru가 닫지 못한 티켓의 상한이다. 사용할 때마다 계정이 그대로인지(같은 uid, 정책 통과) 다시 확인하고 아니면 폐기한다. 도우미가 다시 시작하면 모두 사라진다
- 발급 조건(OMV `UserMgmt.authUser`와 같다): 비밀번호(`unix_chkpwd <user> nonull` + `chkexpiry`), uid 0 아님, `openmediavault-admin`이거나 `UID_MIN..UID_MAX` 안
- onmaru는 OMV 로그인에 성공하면 같은 비밀번호로 `open`한다(5초 제한, 실패해도 로그인은 진행하고 도우미 기능만 없다). 2차 인증이면 `pending` 티켓을 `auth.Pending`에 두고 인증을 마치면 `activate`해 세션으로 옮긴다(`Store.ClaimPending`). 세션이 어떻게 끝나든 `auth.Store`의 삭제 훅(`OnRemove`, `OnRemovePending`)이 `close`한다. onmaru가 시작할 때 `reset`으로 이전 실행의 티켓을 지운다
- 상승: `openmediavault-admin`과 상승 그룹(`-elevate-group`, 기본 `sudo`)에 모두 속한 사용자가 비밀번호를 다시 확인하면 그 티켓에 5분. 절대 시간이고 연산마다 늘리지 않는다. root 연산마다 그룹을 다시 읽어 빠졌으면 상승을 거둔다
- 실패 제한(도우미 자체): 사용자별 5분 안 3번 실패부터 30초에서 두 배씩 최대 15분, 사용자별 동시 확인 1개, 전체 4개, 실패 응답 전 1초. 확인 자체를 못 한 시도(`unix_chkpwd` 실행 실패)는 실패로 세지 않는다(도구가 고장 났을 때 정상 사용자가 잠기지 않게). 상승할 수 없는 계정은 비밀번호를 확인하기 전에 거절한다. `unix_chkpwd`는 `pam_faillock`을 거치지 않는다(OMV의 webgui PAM은 `deny=3 unlock_time=0`이라 그 길로 확인하면 계정이 영구히 잠길 수 있다)
- Go로 shadow 해시를 직접 검증하지 않는다(stdlib에 crypt/yescrypt가 없다). `sudo -S`로 확인하는 방식은 setuid 실행이 필요해 도우미의 `NoNewPrivileges`를 꺼야 하고 sudoers에 없는 OMV 관리자(내장 admin)는 확인할 수 없어 쓰지 않았다

## worker

- 부모(root)는 머리를 읽고 티켓·범위·연산 허용·상승을 확인한 뒤, `/proc/self/exe worker`를 `SysProcAttr{Credential, Setsid, Pdeathsig: SIGKILL}`, 빈 환경 변수, cwd `/`로 실행한다. 작업(`worker.Job`: 범위, 연산, 인자, 본문 크기, 사용자 이름·홈)은 stdin, 연결은 `ExtraFiles`(fd 3)로 넘긴다. 부모는 사용자 데이터를 읽거나 쓰지 않는다
- 사용자 범위는 사용자 uid·기본 gid·보조 그룹(`os/user`, 로컬 파일)으로 강등한다. root 범위도 도우미의 보조 그룹은 넘기지 않는다. worker는 시작할 때 범위와 실제 uid가 맞는지 다시 확인한다(사용자 범위가 root로 돌면 거절)
- 결과 코드는 종료 코드(0 성공, 2 연산 실패 + stderr `code=<코드>`, 그 밖은 실행 오류)로 부모에 알려 감사 로그에 쓴다
- 감사 로그(journal)는 비밀번호 확인 결과, 모든 root 연산, 쓰기 연산의 경로, 내려받기(`fs.read`)의 연산·결과를 남긴다. 정보 읽기 연산(`fs.list`, `fs.stat`, `fs.names`, `fs.statmany`, `fs.info`, `fs.trashdirs`, `fs.trashlist`, `daemon.quietActions`)은 파일 탐색기가 폴더를 열고 스크롤할 때마다 부르므로 남기지 않는다. 휴지통 쓰기 연산은 휴지통 디렉터리와 이름(`trash=`, `name=`, 되돌리기는 `to=`)을, `trash.prepare`는 디스크(`disk=`)를 남긴다
- 시간 제한: 목록·정보 30초(부모), 삭제(`fs.remove`, `fs.trashpurge`, `fs.trashempty`) 10분(큰 폴더를 통째로 지우면 30초를 넘을 수 있다. 넘으면 worker를 끝내므로 일부만 지운 채 남고, 다시 지우면 이어서 지운다), 스트림 6시간(부모) + 60초 무진행(worker의 연결 마감 시각). 클라이언트는 각각 조금 더 기다린다
- 동시 실행: 전체 16개, 사용자당 4개. 연결은 동시 64개

## 파일 연산의 경로 정책

- 경로는 절대·정리된 경로(`path.Clean`과 같음), NUL 없음, 4096바이트 이하
- 기준 디렉터리 = 사용자 홈(`/`가 아닐 때) + `/srv` 아래 마운트 지점(`/proc/self/mountinfo`). `/srv` 자체와 그 안의 루트 파일시스템 디렉터리(`/srv/salt`, `/srv/pillar`)는 기준이 아니다. 가장 긴 기준을 골라 `os.OpenRoot`로 가두므로 `..`·심볼릭 링크로 기준 밖에 나갈 수 없다(`outside_roots`)
- 실제로 열 수 있는지는 커널이 사용자 권한(소유권·ACL)으로 정한다. 기준 정책은 onmaru가 뚫렸을 때 닿는 범위를 줄이는 겹침이다
- 공유 폴더 단위로 보이는 일은 onmaru의 파일 API(파일 탐색기)가 한다. OMV의 공유 폴더 RPC(`ShareMgmt.*`)는 모두 관리자 전용이라 일반 사용자 세션으로는 목록을 받을 수 없으므로, 관리자 세션이 받은 목록을 전역 설정(`site.json`)에 스냅숏으로 둔다. 스냅숏은 처음 보이는 폴더의 이름표일 뿐이고 도우미는 믿지 않는다(범위는 위 기준이 정한다). OMV 공유 폴더 권한(Privileges)은 SMB 같은 서비스에만 적용되므로 따르지 않는다(`ROADMAP.md`의 파일 탐색기)
- 루트 파일시스템의 공유 폴더(openmediavault-sharerootfs, 예: `/kvm`)는 기준 밖이다. 파일 탐색기에서 열려면 root가 정하는 추가 기준 경로 설정을 도우미에 더한다. onmaru가 보낸 경로는 믿지 않으므로 도우미 쪽 설정이어야 하고, 설치기가 묻고(설정 레지스트리에 설정 하나로 더한다) 유닛 템플릿의 `ReadWritePaths`에도 더한다(`ProtectSystem=strict`라 없으면 읽기 전용이다). `/`는 받지 않는다. mergerfs 풀(`/srv/mergerfs/<이름>`)은 이미 기준 안이다
- 기준 디렉터리 자체는 지우거나 옮기거나 덮어쓸 수 없다. 이름 바꾸기는 같은 기준 안에서만 하고(다른 기준이거나, 같은 기준 안이라도 다른 파일시스템이라 `EXDEV`면 `cross_device`) 대상이 있으면 `exists`다. 공유 폴더는 기준이 아니므로 공유 폴더 자신을 지우거나 옮기지 않게 하는 일은 onmaru의 파일 API가 한다(`.claude/rules/files.md`의 `root_folder`)
- 읽기는 일반 파일만(`O_NONBLOCK`으로 열어 FIFO에서 멈추지 않음). `fs.list`는 1만 개까지(`truncated`). umask는 0002(OMV Samba의 create mask 0664 / directory mask 0775)
- `os.Root`는 중간 폴더를 읽기(`O_RDONLY|O_DIRECTORY`)로 열며 따라가므로, 경로 중간에 들어가기(x)만 되고 목록(r)은 볼 수 없는 폴더가 있으면 그 아래는 다룰 수 없다(`permission_denied`). 파일 탐색기는 목록을 볼 수 있는 폴더 안만 연다
- 덮어쓰기는 새 inode로 바꾸므로 소유자는 올린 사용자가 되고 ACL은 폴더의 기본 ACL을 따른다. rwx 권한 비트는 되도록 유지하고(소유자일 때만 바뀐다) setuid·setgid·sticky는 옮기지 않는다. 덮어쓰지 않을 때는 하드 링크로 같은 이름이 없는지 원자적으로 확인하고, 하드 링크를 못 쓰는 파일시스템이면 다시 확인한 뒤 이름을 바꾼다
- 올리기가 실패하면 worker가 임시 파일(`.onmaru-upload-<난수>.part`)을 지운다. 시간 초과로 worker가 강제 종료되면(SIGKILL) 지우지 못하고 남는다. 이 접두사의 이름으로는 올릴 수 없다

## 파일 탐색기 연산(fs.names, fs.statmany, fs.info)

1만 개가 넘는 폴더도 다루기 위해 이름을 먼저 읽고, 정보는 보이는 범위만 읽는다(`ROADMAP.md`의 파일 탐색기). 정렬·캐시·보이는 범위 계산은 onmaru(`internal/files`)가 한다.

- `fs.names`(`{path}`): 이름과 종류만 읽는다. `os.Root`로 연 폴더의 `ReadDir`는 `d_type`을 알아도 항목마다 `lstatat`을 하므로(Go `os/file_unix.go`의 `newUnixDirent`) getdents(`syscall.ReadDirent`)를 직접 읽고 `linux_dirent64`를 파싱한다(`parseDirents`). `.`·`..`·`d_ino` 0은 뺀다
  - 결과 머리는 `NamesResult{count, truncated, dirModTimeNs, writable}`이고 이름 목록은 머리 뒤 본문(항목마다 종류 1바이트 `d`/`f`/`l`/`o`/`?` + 이름 + NUL, `proto.AppendName`·`ParseNames`)이다. 응답 머리(8MiB)에 넣지 않는다
  - 상한은 `proto.MaxNames`(30만 개)와 본문 `MaxNamesBytes`(32MiB)이고 넘으면 그 앞까지(디렉터리 순서)와 `truncated`다
  - 종류를 주지 않는 파일시스템(`DT_UNKNOWN`)의 항목만 `Lstat`하고, 15초(`unknownTypeBudget`)가 지나면 나머지는 `?`로 둔다(연산 시간 제한 30초 안에 끝나게). 읽는 사이 지워진 항목은 뺀다
  - `dirModTimeNs`는 폴더 자신의 수정 시각(나노초, 항목이 더해지거나 빠지면 바뀐다), `writable`은 그 폴더에 `faccessat(W_OK|X_OK)`다
- `fs.statmany`(`{dir, names[]}`): 한 폴더 바로 안의 이름 `proto.MaxStatMany`(512)개까지 `Lstat`한다. 이름은 `proto.ValidName`(빈 이름·`.`·`..`·`/`·NUL·255바이트 초과 거부)으로 검사하므로 그 폴더 밖을 가리킬 수 없다. 폴더를 `OpenRoot`로 한 번 열어 이름마다 경로를 다시 따라가지 않는다. 읽지 못한 항목은 그 항목만 `code`(지워졌으면 `not_found`)이고, 폴더(심볼릭 링크 아님)는 `O_PATH`로 연 폴더 fd 기준 `faccessat(R_OK|X_OK)`이 실패하면 `locked`다. 심볼릭 링크 대상은 기준 Root로 따라간다(`fillLink`)
- `fs.info`(`{path}`): `Entry`와 uid·gid, `/etc/passwd`·`/etc/group`에서 찾은 소유자·그룹 이름(없으면 빈 값), ACL 유무(`acl`), 내 권한(`access{read, write}`, 폴더는 R|X·W|X). 부모 폴더를 `O_PATH|O_DIRECTORY`로 열고 그 fd 기준으로 `faccessat`, `/proc/self/fd/<fd>/<이름>`에 `lgetxattr`(`system.posix_acl_access`·`posix_acl_default`, stdlib에 래퍼가 없어 `SYS_LGETXATTR`를 직접 부름)을 한다. 확인하지 못하면 `acl`을 비운다
  - **심볼릭 링크는 권한·ACL을 보지 않는다.** `faccessat`은 마지막 구성 요소를 따라가고(`AT_SYMLINK_NOFOLLOW`는 faccessat2가 필요하다) 기준 밖 대상의 정보가 샐 수 있기 때문이다. `fs.statmany`의 `locked`도 `Lstat`이 폴더라고 한 항목만 본다(그 사이 링크로 바뀌는 경쟁은 참·거짓 하나만 새므로 받아들인다)
- 쓰는 시스템 호출(`getdents64`, `faccessat`, `newfstatat`, `lgetxattr`)은 유닛의 `SystemCallFilter=@system-service`(`@file-system` 포함)에 들어 있어 유닛은 바꾸지 않았다
- 셋 다 일반 연산(30초)이고 감사 로그에 남기지 않는다. onmaru 쪽은 `client.Client.Names`(머리 뒤 본문을 읽는 별도 연결)·`StatMany`·`Info`

## 휴지통(fs.trash 등, trash.prepare)

FreeDesktop 휴지통 규격을 trash-cli의 `trash-put`과 같은 곳·형식으로 쓴다. 그래서 서버의 `trash-list`·`trash-restore`·`trash-empty`가 같은 휴지통을 다룬다. 구현은 `worker/trash.go`.

- **넣을 곳**(`fs.trash`, `chooseTrash`): 홈 기준 안의 항목은 홈과 같은 파일시스템(`st_dev`)이면 홈 휴지통(`~/.local/share/Trash`, 없으면 0700으로 만든다). 그 밖은 그 항목이 든 가장 깊은 마운트 지점(topdir, 모든 마운트에서 찾는다)의 `.Trash/<uid>`(`.Trash`가 심볼릭 링크가 아닌 sticky 폴더이고 `<uid>`가 사용자 소유일 때, 없으면 0700으로 만든다), 아니면 topdir의 `.Trash-<uid>`(없으면 0700으로 만든다)다. 둘 다 안 되면 `no_trash`
  - `/srv` 아래 디스크의 항목은 홈과 같은 파일시스템이어도 디스크 휴지통을 쓴다. 기준 디렉터리마다 `os.Root`로 가두므로 기준 사이로는 옮기지 않기 때문이다(`trash-put`과 다른 점)
  - 다른 디스크로 복사하지 않고 같은 `os.Root` 안에서 이름만 바꾼다. 휴지통 자신이나 그것을 품은 폴더, 기준 디렉터리 자신은 넣지 않는다
- **이름과 trashinfo**: `info/<이름>.trashinfo`를 `O_EXCL`로 먼저 만들어 이름을 정하고(겹치거나 `files/<이름>`이 있으면 `_1`, `_2` … 1000까지), 그 이름으로 항목을 옮긴다. 옮기지 못하면 trashinfo를 지운다. 내용은 `[Trash Info]` / `Path=` / `DeletionDate=`이고, `Path`는 비예약 문자(`A-Za-z0-9-_.~`)와 `/`만 두고 나머지 바이트를 `%XX`로 쓴다(홈 휴지통은 절대 경로, 디스크 휴지통은 topdir 기준 상대 경로). `DeletionDate`는 시간대 없는 지역 시각(`2006-01-02T15:04:05`)이다(규격에 시간대가 없어, 다른 시간대에서 쓴 항목은 읽을 때 그만큼 어긋난다). 형식은 테스트가 바이트 단위로 확인한다
- **찾기**: `fs.trashlist`와 휴지통을 가리키는 연산은 사용자의 휴지통 디렉터리(홈 휴지통, 기준 안 마운트 지점마다 `.Trash/<uid>`와 `.Trash-<uid>`, 심볼릭 링크가 아니고 사용자 소유인 것)를 worker가 직접 찾는다. onmaru가 보낸 휴지통 경로는 그 목록과 같을 때만 받는다(`findTrashDir`)
  - `fs.trashlist`: trashinfo와 `files/` 항목이 짝을 이룬 것만 담는다. 지운 시각이 늦은 것부터, 1만 개·결과 약 6MiB까지(`truncated`). 이름·원래 경로가 UTF-8이 아니면 `badName`이다
  - `fs.trashdirs`: 기준 디렉터리마다 휴지통 종류(`home`, `shared`, `own`, `none`)와 공유 `.Trash` 유무, 모든 휴지통의 trashinfo 수(`items`, 이름만 센다)다. 만들지 않고 보기만 한다(`own`은 `.Trash-<uid>`가 있거나 맨 위에 쓸 수 있을 때). `shared`는 `.Trash`만 보고 정하므로, 넣을 때 `.Trash/<uid>`가 남의 것이면 `.Trash-<uid>`로 간다
- **되돌리기**(`fs.trashrestore`): 받은 경로(`to`)로만 되돌린다. 원래 경로를 onmaru가 보이는 폴더로 먼저 거르게 하기 위해서다(onmaru가 목록의 원래 경로나 새 이름을 보낸다). `to`는 휴지통과 같은 기준이어야 하고(아니면 `cross_device`), 있으면 `exists`, 폴더가 없으면 만든다(`trash-restore`와 같다). 옮긴 뒤 trashinfo를 지운다
- **영구 삭제·비우기**(`fs.trashpurge`, `fs.trashempty`): `files/<이름>`(`RemoveAll`, 링크는 따라가지 않는다)을 지운 뒤 trashinfo를 지운다. 비우기는 모든 휴지통의 `files`·`info` 안을 지우고 휴지통 폴더는 둔다. 시간 제한은 삭제와 같이 10분이다
- **디스크 휴지통 만들기**(`trash.prepare`, root 범위): OMV 데이터 디스크 맨 위는 보통 root 소유라 사용자가 `.Trash-<uid>`를 만들 수 없다. 관리자가 설정 앱에서 상승한 뒤 부르면 그 디스크 맨 위에 `.Trash`를 만들고 1777로 둔다
  - 디스크는 `/srv` 아래 마운트 지점(도우미의 기준, 사용자 홈 제외)만 받는다. worker에는 `CAP_DAC_OVERRIDE`·`CAP_FOWNER`가 없어 root도 소유자로서만 할 수 있다(맨 위가 root 소유가 아니면 `permission_denied`)
  - 맨 위에 사용자가 쓸 수 있는 디스크에서는 사용자가 `.Trash`를 미리 심볼릭 링크로 둘 수 있으므로, `Lstat`으로 심볼릭 링크·폴더가 아닌 것을 거절하고(`not_dir`), `O_NOFOLLOW|O_DIRECTORY`로 연 fd가 root 소유 폴더인지 다시 확인한 뒤 그 fd에 `fchmod`한다. 이미 있으면 권한만 고친다
  - 유닛 권한은 바꾸지 않았다(`/srv`는 이미 `ReadWritePaths`이고 새 capability가 필요 없다)
- onmaru 쪽은 `client.Client`의 `Trash`, `TrashDirs`, `TrashList`, `TrashRestore`, `TrashPurge`, `TrashEmpty`, `TrashPrepare`

## 임시 SSH 키(ssh.authorize, ssh.revoke)

- 웹 터미널(`internal/terminal`)이 공개 키만 받는 sshd에도 비밀번호를 다시 묻지 않고 접속하려고 쓴다. onmaru가 메모리에서 만든 ed25519 키의 공개 키를 넘기면, 사용자 범위 worker가 `<홈>/.ssh/authorized_keys`에 `restrict,pty,from="127.0.0.1,::1",expiry-time="<UTC+2분>Z" ssh-ed25519 <키> onmaru-ephemeral` 줄을 넣는다. onmaru는 인증이 끝나면 성공·실패와 관계없이 `ssh.revoke`로 지운다
- 공개 키는 와이어 형식(`proto.ParseSSHPublicKey`)으로 직접 확인한다(도우미에 x/crypto를 링크하지 않는다). 주석·옵션·다른 키 종류는 받지 않는다
- 다른 줄은 바이트 그대로 두고, 이 주석과 옵션 접두사를 가진 줄만 고친다. 만료됐거나 만료 시각을 읽을 수 없는 onmaru 줄은 다음 연산이 지운다(시작 시 정리 없음)
- 홈은 `os.OpenRoot`로 가둔다. 홈이 `/`이거나 없으면 `no_home`, 홈이 다른 사용자에게 쓰기를 허용하거나 남의 소유면 sshd(`StrictModes`)가 키를 무시하므로 `ssh_strict_modes`로 거절하고 고치지 않는다. `~/.ssh`(없으면 0700으로 만듦)와 파일은 사용자 소유일 때 그룹·다른 사용자 쓰기만 뺀다(worker umask가 0002라 직접 정한다). `authorized_keys`가 심볼릭 링크 등 일반 파일이 아니면 `not_regular`
- 같은 폴더 임시 파일(`.onmaru-keys-*`)에 쓰고 이름을 바꾸며, 동시에 고쳐 쓰지 않게 `~/.ssh` 디렉터리 fd에 `flock`을 건다
- 감사 로그에는 공개 키 대신 SHA256 지문(`key=SHA256:...`)을 남긴다. sshd 로그의 `Accepted publickey ... SHA256:...`와 맞춰 볼 수 있다
- 유닛 권한은 바꾸지 않았다(`ReadWritePaths=-/srv -/home`)

## 대시보드에서 업그레이드(upgrade.apply)

- 업그레이드 서비스(설치기의 `--upgrade-service`, `onmaru-upgrade.service`)에 적용 요청을 넘기는 root 연산이다. 흐름은 `.claude/docs/upgrade.md`, 서비스는 `.claude/rules/installer.md`의 "업그레이드 서비스"
- 인자(`proto.UpgradeApplyParams`)는 대상 커밋(`config.ValidUpgradeTarget`, 40자리 소문자 16진수)과 설정 이름·값(`config.ValidUpgradeSetting`, 옵션 이름 꼴, 공백·제어 문자 없는 ASCII 512바이트, 32개까지)뿐이다. 명령·경로는 받지 않는다. 요청한 사용자(티켓)와 시각은 worker가 정한다
- worker는 꼴만 검사하고 요청 디렉터리(`config.UpgradeRequestDir`, `/run/onmaru-upgrade`)에 `apply.json`(0600)을 쓴다. 점으로 시작하는 임시 파일에 쓴 뒤 이름을 바꾸므로 경로 유닛은 다 쓴 요청만 본다. 처리하지 않은 요청은 새 요청으로 바꾼다. 상태 파일은 읽지 않는다. 마지막 확인과 같은 대상인지, 저장소 상태, 설정 값은 업그레이드 서비스가 모두 다시 확인한다
- 요청 디렉터리는 대시보드에서 업그레이드를 켠 도우미 유닛의 `RuntimeDirectory=onmaru-upgrade`(0700, Preserve)다. 없으면 `upgrade_disabled`다. `ProtectSystem=strict`에서도 RuntimeDirectory는 쓸 수 있으므로 `ReadWritePaths`는 더하지 않았다
- 감사 로그에는 대상 커밋과 설정 이름만 남긴다(값은 남기지 않는다)
- onmaru는 `client.Client.UpgradeApply`로 부른다(`POST /api/upgrade/apply`가 꼴을 먼저 검사한다). `upgrade_disabled`는 409, `bad_request`는 400으로 바꾼다
- 테스트와 가짜 도우미는 `worker.Config.UpgradeDir`로 디렉터리를 바꾼다(가짜 도우미는 Root 아래 `run/onmaru-upgrade`. `make dev-fake`의 가짜 업그레이드 서비스가 이 디렉터리의 요청을 처리한다)

## systemd

- `onmaru-helper.socket`: `/run/onmaru-helper/helper.sock`, `SocketUser=<onmaru 서비스 사용자>`, `SocketMode=0600`. Docker 프록시에 바인드 마운트하는 `/run/onmaru`와 디렉터리를 나눈다(컨테이너 안의 같은 uid가 소켓에 닿지 않게)
- `onmaru-helper.service`: root지만 `CapabilityBoundingSet=CAP_SETUID CAP_SETGID CAP_KILL`(강등과, uid가 다른 worker 종료), `NoNewPrivileges=yes`, `ProtectSystem=strict` + `ReadWritePaths=-/srv -/home`, `PrivateNetwork=yes`, `RestrictAddressFamilies=AF_UNIX`, `PrivateDevices=yes`, `SystemCallFilter=@system-service` 등. root라도 `CAP_DAC_OVERRIDE`가 없어 남의 파일을 마음대로 읽지 못한다(`/etc/shadow`는 소유자가 root라 `unix_chkpwd`가 읽는다)
- 대시보드에서 업그레이드를 켜면 `RuntimeDirectory=onmaru-upgrade`, `RuntimeDirectoryMode=0700`, `RuntimeDirectoryPreserve=yes`를 더한다(`upgrade.apply`의 요청 디렉터리. 유휴 종료로 도우미가 멈춰도 남는다). 끄면 이 줄을 빼고 설치기가 디렉터리를 지운다
- 소켓 활성화로 띄우고, 티켓과 연결이 없이 10분이면 스스로 끝난다(`-idle-exit`). 티켓이 있으면 끝나지 않는다
- onmaru 유닛은 바꾸지 않는다(`ProtectSystem=strict`에서도 소켓 connect는 된다). `Wants=`·`After=onmaru-helper.socket`만 더한다

## root 연산을 더할 때

1. 기능이 정말 OMV RPC로 안 되는지 먼저 확인한다(OMV 소스 `/usr/share/openmediavault/engined/rpc/`)
2. `proto`에 연산 이름과 인자·결과 타입을 더하고 `proto.Actions[ScopeRoot]`에 넣는다. 임의 명령·셸·브라우저가 보낸 경로가 exec 인자로 가는 연산은 만들지 않는다. 인자는 worker가 허용 목록으로 검사하고 실행 인자는 직접 만든다
3. `worker`에 구현하고, 필요한 권한만 유닛에 더한다(`CapabilityBoundingSet`, `ReadWritePaths` 등). 설치기의 유닛 템플릿(`installer/internal/setup/templates/onmaru-helper.service.tmpl`)과 골든, README의 보안 설계도 함께 고친다
4. 감사 로그(`daemon.audit`)에 필요한 세부가 남는지 확인한다(root 범위는 인자의 경로를 남긴다)
5. onmaru 쪽은 관리자 전용 API(`requireAdmin`)로 부르고, `elevation_required`(403)를 받은 화면은 `requestElevation`(`shell/elevationDialog.tsx`)으로 상승한 뒤 다시 시도한다
6. 테스트: `proto.Allowed`의 범위 검사, worker 단위 테스트, 데몬의 상승 필요·그룹 재확인, 서버 테스트(helperfake)
