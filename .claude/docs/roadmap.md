# 로드맵

나중에 개발할 기능 목록입니다.

## 파일 탐색기

1단계(처음 보이는 폴더, 이름을 먼저 읽는 목록과 가상 스크롤, 내려받기, 올리기), 2단계(폴더 만들기, 이름 바꾸기, 삭제와 trash-put 호환 휴지통, 디스크 휴지통 만들기, 끌어 놓기, 여러 항목 고르기, 보안 강화 모드의 쓰기 전용 단계), 3단계(하위 폴더까지 찾는 검색, 미리보기 창, "OMV 공유 폴더 권한 따르기" 옵션, 아이콘 보기)는 만들었습니다(`docs/en/files.md`, 규칙은 `.claude/rules/files.md`). 남은 일입니다.

- **미리보기 넘기기**: 미리보기 창에서 같은 폴더의 이전·다음 파일로 넘깁니다(←/→).
- **검색 결과에서 쓰기 작업**: 찾은 항목을 바로 끌어 옮기거나 지웁니다. 지금은 "위치 열기"로 그 폴더에 가서 합니다.
- **루트 파일시스템의 공유 폴더**(openmediavault-sharerootfs, 예: `/kvm`): 도우미의 기준 디렉터리(홈과 `/srv` 아래 마운트) 밖입니다. root가 정하는 도우미의 추가 기준 경로 설정으로 엽니다. `install.sh`가 묻고 도우미 유닛의 `ReadWritePaths`에도 더하며, `/`는 받지 않습니다. mergerfs 풀(`/srv/mergerfs/<이름>`)은 이미 기준 안에 있습니다.

## 알림

알림 틀(작업표시줄의 알림 버튼, 알림 센터, 토스트, 서버 알림 API, 클라이언트 알림)은 만들었습니다(`.claude/rules/backend.md`·`.claude/rules/frontend-shell.md`의 "알림"). 운영 출처는 아직 없습니다. 남은 일입니다.

- **시스템 패키지 업데이트(APT)와 재부팅 권장**(관리자 전용 출처): OMV RPC 없이 직접 읽습니다. 업데이트는 `/var/lib/openmediavault/apt/upgradeindex.json`(OMV `omv-mkaptidx`가 apt 후크로 다시 만들고 apt-daily가 매일 갱신, 0644), 재부팅은 `/run/reboot-required`가 있는지와 `/run/reboot-required.pkgs`입니다. OMV `System.getInformation`의 `availablePkgUpdates`·`rebootRequired`와 같은 출처이며 maru 유닛(`ProtectSystem=strict`)에서 읽힙니다.
    - 리비전은 정렬한 `이름@버전`과 `.pkgs` 줄의 해시입니다. 인덱스는 내용이 같아도 매일 다시 쓰이고 제자리에서 쓰이므로(이름 바꾸기가 아님) 깨진 파일을 읽으면 직전 결과를 유지합니다(`notify.Center`가 이미 그렇게 합니다).
    - 동작은 워크벤치의 업데이트 관리 화면(라우트는 OMV 8.0~8.5 워크벤치 소스에서 확인)과 전원 대화상자(다시 시작)입니다. 문구 키와 코드 유니온(`api.Notice.Code`)을 함께 더합니다.
    - 개발 fixture는 합성 `upgradeindex.json`·`reboot-required`를 둔 가짜 루트(`MARU_DEV_NOTIFY_ROOT`, `os.DirFS`)로 바꿉니다.
- **서버 백그라운드 작업 표시**: 알림 센터에 "백그라운드 작업" 구역을 더합니다. 브라우저 없이 서버에서 끝까지 실행할 수 있는 작업(예: 패키지 업데이트)에만 쓰고, 사용자가 작업 중에 창을 닫았거나 세션이 만료되었을 때만 보입니다. 브라우저가 진행하는 파일 복사·올리기는 대상이 아닙니다. 작업이 진행 중이면 더 자주 받습니다. 시안 B의 "나중에: 백그라운드 작업" 자리를 따릅니다.

## 웹 터미널의 sshd 쪽 보강

OMV SSH 설정의 추가 옵션에 `AuthenticationMethods publickey,keyboard-interactive`를 두고 root가 PAM OTP를 설정하면, 도우미의 임시 키만으로는 들어갈 수 없으므로 maru가 뚫려도 OTP가 필요합니다. maru의 keyboard-interactive 중계 확인이 남아 있습니다. 보안 강화 모드(`docs/en/security.md`)는 maru가 확인하므로 maru 자체가 뚫린 경우는 막지 못하며, 이 보강이 그 부분을 채웁니다.

## 웹에서 가동 설정 바꾸기

설정 앱의 서버 페이지에서 `/etc/default/maru`의 가동 설정을 바꾸고 서비스를 다시 시작합니다. 권한 도우미의 root 권한 상승(OMV 관리자이면서 상승 그룹에 속한 사용자가 비밀번호를 다시 입력)을 쓰고, 도우미에 허용된 키 변경과 재시작만 하는 root 연산을 더합니다(`.claude/docs/helper.md`의 "root 연산을 더할 때"). 수신 주소는 프록시 설정과 어긋나 접속이 끊길 수 있으므로 웹에서 바꾸지 않습니다.

- 대시보드에서 업그레이드(`.claude/docs/upgrade.md`)의 업그레이드 서비스가 root로 설치기를 실행하는 경로를 함께 쓸 수 있습니다. 설정 값을 설치기 옵션으로 넘기고 설치기가 검사합니다.

## 프로세스 조작

시스템 정보 창의 프로세스 표에서 프로세스를 종료합니다. 목록(관리자 전용)은 읽기 전용으로 만들어 두었습니다. 서비스 계정은 다른 사용자의 프로세스에 신호를 보낼 권한이 없으므로, 어떤 경로로 할지부터 정합니다.
