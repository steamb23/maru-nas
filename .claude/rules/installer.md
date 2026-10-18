---
paths:
  - "install.sh"
  - "installer/**"
  - "backend/config/**"
---

# 설치기 (install.sh, installer/)

사용자 설명은 `README.md`의 "설치 스크립트"와 "업그레이드".

## 구조

- `install.sh`는 부트스트랩이다. 실행한 계정으로 `installer/`를 `bin/onmaru-install`로 빌드해 exec한다(go가 없거나 root가 남의 저장소에서 실행하면 이미 있는 실행 파일을 쓴다). 옵션과 동작은 모두 설치기가 정한다. 진입점 이름 `./install.sh`는 README, 화면 문구(모든 언어), 생성 파일의 주석이 참조하므로 바꾸지 않는다
- `installer/`는 별도 Go 모듈(`onmaru/installer`)이다. `go.work` 없이 `replace onmaru/backend => ../backend`로 로컬 백엔드를 쓰고, go 지시어는 `backend/go.mod`와 같게 둔다. TUI 의존성 huh v2는 이 모듈에만 있다(`backend/go.mod`·`go.sum`에는 charm 모듈이 없어야 한다)
- 백엔드에서 쓰는 것은 공개 패키지 `backend/config`(환경 변수 키, 기본값, 검증 함수)뿐이다. `backend/internal`은 Go 규칙상 import할 수 없고, `TestBackendDependencies`가 다른 백엔드 패키지를 링크하지 않는지 본다. `backend/config`는 표준 라이브러리만 쓴다(`TestStandardLibraryOnly`)
- `internal/setup`
  - `run.go`: 흐름(1단계 설치, `--service-install` 2단계, `--upgrade`, `--uninstall`, `--dry-run`)과 sudo 재실행(`sudo bin/onmaru-install --service-install --lang X <레지스트리가 만든 인자> [--upgrade] [--plain]`). root면 같은 프로세스에서 이어 간다. 2단계는 받은 값을 다시 검사한다
  - `setting.go`: 설정 타입과 레지스트리 동작(옵션 파싱, 사용법, 2단계 인자, 업그레이드 로드, 질문, 검증, 요약, 환경 파일 생성). `settings.go`: 설정 목록. 순서가 질문·요약·환경 파일 줄·사용법의 순서다. `setting_<이름>.go`: 설정 정의
  - `step.go`·`steps.go`·`step_<이름>.go`: 설치·제거 단계. User 단계는 빌드, Root 단계는 설치다
  - `system.go`: System 인터페이스와 실제 구현. `system_dryrun.go`: 파일 쓰기·root 명령을 보여 주기만 하고 읽기·확인·빌드는 실제로 한다. 테스트의 `fake_test.go`: 가짜 루트(임시 디렉터리)와 명령 기록
  - `envfile.go`: 환경 파일 읽기(source 안 함. 마지막 `KEY=` 줄, 값을 감싼 큰따옴표 한 쌍은 벗김)와 쓰기(내용이 같으면 "unchanged", 바뀌면 환경 파일만 `.bak-<시각>` 백업, 유닛은 백업 없이 덮어씀)
  - `templates/*.tmpl`: 유닛 3종(embed, `text/template`). 하드닝 줄을 바꾸는 일은 이 파일을 고치는 것이다
  - `text.go`: 문구 `L(en, ko)`와 로캘 언어 선택(`--lang`, 아니면 `LC_ALL`·`LC_MESSAGES`·`LANG`이 ko로 시작하면 ko). 문구는 쓰는 곳(설정·단계 파일) 옆에 둔다
- `internal/prompt`: Prompter(Input, Select, Confirm, Note). huh(표준 입력·출력이 터미널, `TERM≠dumb`, `--plain` 아님), 줄 단위(그 밖), 테스트용 `Script`(질문 ID별 답)와 `Fail`(질문하면 실패). 두 실제 구현 모두 잘못된 입력은 다시 묻는다

## 설정을 더할 때

1. `setting_<이름>.go`에 `Setting`을 정의한다. `Name`(플래그 이름), `Flag`, `Arg`, `Usage`, `Env`(키는 `backend/config`의 상수, 주석은 영어), `Default`(기존 설치 → 감지값 → 고정값), `Load`(`--upgrade`에서 기존 설치의 값. 없으면 그 설정만 묻는다), `Prompt` 또는 `Ask`, `Validate`, `Summary`, 필요하면 `When`·`Notice`
   - 백엔드와 같은 규칙이면 `backend/config`의 함수로 검사하고, 설치기만의 더 엄격한 규칙(사용자 이름, Unix 소켓 경로, IP만 받는 주소 등)은 설정 정의에 둔다
   - 설정 정의 안의 함수에서 다른 설정 값은 `name*` 상수로 읽는다(설정 변수를 직접 가리키면 패키지 초기화 순환이 생긴다)
2. `settings.go`의 목록에 한 줄 더한다
3. 백엔드 플래그도 생기면 `backend/config`에 키·기본값을 두고 `cmd/onmaru/main.go`의 플래그와 `runtimeEntry` 목록에서 쓴다
4. `README.md`의 설치 옵션 표(`TestReadmeListsEveryOption`)와 설정 표, `setting_test.go`의 검사 표와 왕복 표에 더한다
5. 환경 파일 출력이 바뀌면 골든(`testdata/golden/*/want`)도 고친다. 골든은 bash `install.sh`의 출력에서 시작했으므로 바꾼 이유를 커밋 메시지에 적는다

## 단계를 더할 때

- `step_<이름>.go`에 `func(c *Context) error`를 만들고 `steps.go`의 `installSteps` 또는 `uninstallSteps`에 `Phase`·`When`과 함께 한 줄 더한다
- 시스템을 바꾸는 일은 `c.Sys`로만 한다. 파일은 `WriteFile`·`InstallFile`·`Remove`, 명령은 `Run`(출력을 버리면 `RunQuiet`·`RunSilent`), 읽기 전용 확인은 `Check`. 그래야 `--dry-run`과 가짜 루트 테스트가 그대로 동작한다
- 설정 파일은 `c.writeConfig`로 쓴다. 질문은 `c.confirm`(dry-run은 예로 보고 진행, 비TTY는 오류)이나 `c.P`로 한다

## 불변 조건

- 생성하는 파일(유닛 3종, `/etc/default/onmaru`, `/etc/default/onmaru-helper`)은 골든과 바이트 단위로 같고 명령 순서(`commands`)도 같아야 한다(`TestGoldenServiceInstall`). 생성 파일의 주석은 영어로 쓰고 진입점을 `install.sh`라고 부른다
- 의도한 차이는 레지스트리가 관리하지 않는 기존 `ONMARU_*` 줄을 파일 끝 "Kept from the previous file" 주석 아래에 보존하는 것뿐이다. 그런 줄이 없으면 주석도 쓰지 않는다
- bash `install.sh`가 만든 설치(같은 경로·환경 변수 키·유닛의 `User=`)를 `--upgrade`로 읽는다. 질문 없는 업그레이드와 설정을 모두 준 `--service-install`(README의 자동화 예시, 테스트 VM의 `helper-test/setup.sh`)은 질문 없이 끝나야 한다
- 비TTY에서 설정 질문이 필요하면 멈추지 않고 빠진 옵션 목록을 알린 뒤 2로 끝낸다. 확인 질문(진행할까요, 그래도 계속할까요)도 비TTY에서는 묻지 않고 오류로 끝낸다. `--upgrade`, 설정을 모두 준 `--service-install`, `--dry-run`은 진행 확인을 하지 않는다
- 종료 코드: 잘못된 옵션·비TTY의 빠진 설정 2, 오류 1, "진행할까요?"·제거 확인에서 아니요 0, "그래도 계속할까요?"·기존 실행 파일로 설치할지 확인에서 아니요 1, 중단(Ctrl+C) 130
- 유닛 하드닝을 바꾸면 `README.md`의 보안 설계, `.claude/docs/helper.md`, `.claude/rules/helper.md`도 함께 본다
- 테스트는 root·sudo·실제 systemctl을 쓰지 않는다(가짜 루트와 명령 기록). 실제 설치는 사용자가 하거나 테스트 VM에서 확인한다
- `ROADMAP.md`의 항목(도우미 추가 기준 경로 질문 등)은 요청하기 전에 구현하지 않는다. 설정 하나와 템플릿 한 줄로 더할 수 있는 구조만 유지한다
- 새 의존성을 더하지 않는다. TUI는 huh v2(함께 오는 Bubble Tea·Lip Gloss 포함)만 쓴다

## 아직 확인하지 않은 것

- huh 화면(질문, 선택, 입력 중 검사, 요약, Ctrl+C)은 사용자가 실제 터미널에서 확인한다. 자동 확인은 가상 터미널에서 첫 질문이 뜨고 Ctrl+C로 130이 되는 것까지다
- 이 NAS의 실제 설치에서 sudo를 거치는 업그레이드. 순정 OMV 8.3.1 테스트 VM에서는 2026-10-18에 확인했다: bash `install.sh`(HEAD)로 다시 쓴 파일 위에서 `--upgrade`(질문 0개, 파일 5종 "unchanged", 백업 없음), `--no-helper`(도우미 실행 파일·유닛·환경 파일 삭제, 소켓 꺼짐, `ONMARU_HELPER_SOCKET=`)와 `--helper` 복원(bash 결과와 같음), sudo 그룹 일반 사용자의 `--upgrade`(sudo 2단계 재실행, bash 결과와 같음), root가 남의 저장소에서 빌드하려 할 때 `--upgrade` 중단(1), `--uninstall`(상태 디렉터리·사용자 남김), 비TTY에서 빠진 옵션(2), 설정을 모두 준 `--service-install`(골든 `helper-loopback`과 같음). 모두 상태 확인 성공
