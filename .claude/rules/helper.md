---
paths:
  - "backend/internal/helper/**"
  - "backend/cmd/onmaru-helper/**"
  - "backend/internal/server/helper*.go"
  - "frontend/src/settings/HelperSection.tsx"
  - "frontend/src/shell/elevation*.ts*"
  - "installer/internal/setup/templates/onmaru-helper.*"
---

# 권한 도우미

설계·위협 모델·연산 추가 절차는 `.claude/docs/helper.md`. 사용자 설명은 `README.md`의 "권한 도우미".

- 도우미는 root로 돈다. onmaru(클라이언트)가 보낸 사용자 이름·그룹·정책을 믿지 않는다. 믿는 것은 상대 uid(`SO_PEERCRED`), 자기가 발급한 티켓, 자기가 읽은 `/etc/passwd`·`/etc/group`·`/etc/login.defs`뿐이다
- 임의 명령·셸 실행 연산을 만들지 않는다. root 연산은 `proto.Actions[ScopeRoot]`의 타입 있는 연산으로만 더하고, 브라우저가 보낸 값이 exec 인자로 그대로 가지 않게 한다. 지금 root 연산은 `id`뿐이다. 웹 터미널은 도우미가 아니라 sshd를 거친다
- 웹 터미널의 임시 SSH 키(`ssh.authorize`·`ssh.revoke`, 사용자 범위)는 키 줄을 worker가 직접 만들고 공개 키(ed25519)만 받는다. 옵션·만료 시각을 onmaru가 정하게 하지 않는다(`.claude/docs/helper.md`의 "임시 SSH 키")
- root 부모(`daemon`)는 사용자 데이터를 읽거나 쓰지 않는다. 연산은 worker 자식 프로세스가 하고, 사용자 범위는 시작 전에 사용자 uid·gid·보조 그룹으로 강등한다. worker는 범위와 실제 uid가 맞는지 스스로 다시 확인한다
- 비밀번호 확인은 `unix_chkpwd`(`daemon.ChkPwd`)로만 한다. Go로 shadow를 직접 검증하지 않고, OMV의 webgui PAM 스택(`pam_faillock deny=3 unlock_time=0`)을 거치지 않는다. 실패 제한은 도우미의 `limiter`가 한다
- 비밀번호·티켓은 로그·응답·테스트 픽스처에 남기지 않는다. 오류는 코드만 주고(`proto.Code*`) 세부는 도우미 로그에만 남긴다. 감사 로그는 root 연산과 쓰기 연산의 경로, 읽기 연산의 연산·결과만 남긴다
- 도우미 실행 파일은 `CGO_ENABLED=0`으로 빌드한다(`Makefile`). onmaru 웹 코드(`internal/server`, `net/http` 서버 등)를 `cmd/onmaru-helper`에 import하지 않는다. `cmd/onmaru-helper`의 `TestNoWebDependencies`가 `go list -deps`로 검사한다
- `helperfake`는 테스트·`make dev-fake` 전용이다. 운영 바이너리(`cmd/onmaru`, `cmd/onmaru-helper`)에서 import하지 않는다. 프로세스 없이 worker를 돌리는 Spawner는 권한을 바꾸지 않으므로 운영 코드에 두지 않는다
- 유닛의 권한(`CapabilityBoundingSet`, `ReadWritePaths` 등)은 연산이 실제로 필요한 만큼만 둔다. 늘리면 설치기의 유닛 템플릿(`installer/internal/setup/templates/onmaru-helper.service.tmpl`)과 골든(`.claude/rules/installer.md`), `README.md`의 보안 설계, `.claude/docs/helper.md`를 함께 고친다
- onmaru 쪽: 티켓은 `auth.Session.HelperTicket`에만 두고 응답에 넣지 않는다. 세션 삭제 훅이 폐기하므로 세션을 지우는 새 경로를 만들어도 따로 닫지 않는다. 2차 인증을 마친 대기는 `ClaimPending`으로 꺼낸다(`DeletePending`은 티켓을 폐기한다)
- 상승 API는 관리자 전용(`requireAdmin`)이다. root 권한이 필요한 화면은 `elevation_required`를 받으면 `requestElevation`(`shell/elevationDialog.tsx`)으로 상승한 뒤 다시 시도한다
- 보안 강화 정책(`security.elevation`)을 채우지 못한 세션의 상승은 도우미를 부르기 전에 403 `security_required`로 막는다. `ElevationForm`이 확인(`requestStepUp`) 뒤 같은 비밀번호로 다시 시도한다. 확인은 onmaru가 하고 도우미에 넣지 않는다(`.claude/rules/security.md`)

## 테스트

- root 없이 테스트한다. 데몬은 가짜 계정·`Verifier`·프로세스 없는 Spawner로, 실행 경로(`ExecSpawner`)는 테스트 실행 파일을 worker로 띄워(자기 uid, 보조 그룹 그대로) 확인한다(`daemon_test.go`의 `TestMain`)
- `unix_chkpwd` 래퍼는 인자·stdin을 확인하는 가짜 스크립트로 시험한다. 실제 비밀번호를 쓰지 않는다
- worker 파일 연산은 임시 디렉터리와 합성 mountinfo로 시험한다(심볼릭 링크 탈출, FIFO, 기준 삭제 거부 포함)
- 서버 테스트는 `envOptions{helper: true}`(helperfake)로 로그인 티켓·2차 인증·상승 API를 시험한다
- 실제 설치에서의 확인(소켓 권한, 강등, 유닛 하드닝)은 사용자가 한다. CLAUDE.md "나중에 확인할 것"의 체크리스트를 쓴다
