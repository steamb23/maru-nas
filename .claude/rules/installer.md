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
  - `run.go`: 흐름(1단계 설치, `--service-install` 2단계, `--uninstall`, `--dry-run`)과 sudo 재실행(`sudo bin/onmaru-install --service-install --lang X <레지스트리가 만든 인자> [--upgrade | --reinstall] [--plain]`). root면 같은 프로세스에서 이어 간다. 2단계는 받은 값을 다시 검사한다
  - `mode.go`: 설치 방식(새 설치, 다시 설치, 업그레이드)을 정하고 알리며, 업그레이드 요약 뒤의 선택(업그레이드·설정 바꾸기·취소)과 진행 확인을 한다(아래 "설치 방식")
  - `setting.go`: 설정 타입과 레지스트리 동작(옵션 파싱, 사용법, 2단계 인자, 업그레이드 로드, 질문, 검증, 요약, 환경 파일 생성). `settings.go`: 설정 목록. 순서가 질문·요약·환경 파일 줄·사용법의 순서다. `setting_<이름>.go`: 설정 정의
  - `step.go`·`steps.go`·`step_<이름>.go`: 설치·제거 단계. User 단계는 빌드, Root 단계는 설치다
  - `system.go`: System 인터페이스와 실제 구현. `system_dryrun.go`: 파일 쓰기·root 명령을 보여 주기만 하고 읽기·확인·빌드는 실제로 한다. 테스트의 `fake_test.go`: 가짜 루트(임시 디렉터리)와 명령 기록
  - `envfile.go`: 환경 파일 읽기(source 안 함. 마지막 `KEY=` 줄, 값을 감싼 큰따옴표 한 쌍은 벗김)와 쓰기(내용이 같으면 "unchanged", 바뀌면 환경 파일만 `.bak-<시각>` 백업, 유닛은 백업 없이 덮어씀)
  - `templates/*.tmpl`: 유닛 3종(embed, `text/template`). 하드닝 줄을 바꾸는 일은 이 파일을 고치는 것이다
  - `text.go`: 문구 `L(en, ko)`와 로캘 언어 선택(`--lang`, 아니면 `LC_ALL`·`LC_MESSAGES`·`LANG`이 ko로 시작하면 ko). 문구는 쓰는 곳(설정·단계 파일) 옆에 둔다
- `internal/prompt`: Prompter(Input, Select, Confirm, Note). huh(표준 입력·출력이 터미널, `TERM≠dumb`, `--plain` 아님), 줄 단위(그 밖), 테스트용 `Script`(질문 ID별 답)와 `Fail`(질문하면 실패). 두 실제 구현 모두 잘못된 입력은 다시 묻는다

## 설치 방식

`resolveMode`가 1단계와 직접 실행한 2단계에서 같은 규칙으로 정한다. "설치되어 있음"은 유닛 파일과 `/etc/default/onmaru`가 모두 있다는 뜻이다.

- 옵션 없음
  - 둘 다 없음: 새 설치. 옵션으로 주지 않은 설정을 모두 묻고 요약 뒤 "진행할까요?"
  - 둘 다 있음: 업그레이드. 설치된 설정을 읽고(설정 옵션이 덮어쓴다) 빠진 설정만 묻는다. 요약 뒤 터미널이면 업그레이드(기본)·설정 바꾸기·취소를 고른다. "설정 바꾸기"는 그 자리에서 다시 설치로 바꿔 지금 값(설치된 값과 옵션)을 기본값으로 모든 설정을 다시 묻고 "진행할까요?"를 묻는다. "취소"는 아무것도 하지 않고 0. 비TTY와 dry-run은 묻지 않는다
  - 하나만 있음: 경고하고 다시 설치
- `--reinstall`: 다시 설치. 설치된 값을 기본값으로 옵션으로 주지 않은 설정을 모두 묻고 "진행할까요?". 새 시스템에서는 새 설치와 같다
- `--upgrade`(스크립트용): 업그레이드. 설치되어 있지 않으면 1로 끝낸다. 요약 뒤 아무것도 묻지 않는다
- `--upgrade`와 `--reinstall`을 함께, 또는 둘 중 하나를 `--uninstall`과 함께 주면 사용법 오류(2)
- 업그레이드(자동 감지와 `--upgrade` 모두)에서는 Force SSL/TLS·포트 사용 중을 경고만 하고 묻지 않으며, 수신 주소와 서비스 사용자가 그대로면 리버스 프록시 안내를 줄인다. root가 남의 저장소에서 빌드하려 하면 `--upgrade`와 비TTY 자동 업그레이드는 멈추고, 터미널의 자동 업그레이드는 기존 실행 파일로 설치할지 묻는다
- 업그레이드에서 기존 설치의 값이 `Validate`를 통과하지 못하면(`dropInvalid`) 그 설정을 빠진 것으로 본다. 터미널이면 경고하고 기존 값이 아닌 기본값(`withoutInstalled`)으로 다시 묻고(수신 주소는 수신 방식부터), 비TTY면 그 옵션과 오류를 알리고 2로 끝낸다(옵션으로 주거나 `--reinstall`)
- 2단계에는 1단계가 끝난 방식을 넘긴다. 업그레이드면 `--upgrade`, 다시 설치(설정 바꾸기 포함)면 `--reinstall`, 새 설치면 없음. 2단계가 기존 설치를 보고 방식을 다시 정하지 않게 한다

## 설정을 더할 때

1. `setting_<이름>.go`에 `Setting`을 정의한다. `Name`(플래그 이름), `Flag`, `Arg`, `Usage`, `Env`(키는 `backend/config`의 상수, 주석은 영어), `Default`(기존 설치 → 감지값 → 고정값), `Load`(업그레이드에서 기존 설치의 값. 없거나 `Validate`를 통과하지 못하면 그 설정만 묻는다), `Prompt` 또는 `Ask`, `Validate`, `Summary`, 필요하면 `When`·`Notice`
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
- bash `install.sh`가 만든 설치(같은 경로·환경 변수 키·유닛의 `User=`)를 업그레이드(옵션 없음 또는 `--upgrade`)로 읽는다. 설정이 모두 있는 설치의 업그레이드는 설정 질문이 없고(옵션 없는 터미널 실행은 요약 뒤 선택 하나), 설정을 모두 준 `--service-install`(README의 자동화 예시, 테스트 VM의 설치 시험)은 설치 여부와 상관없이 질문 없이 끝나야 한다
- 비TTY에서 설정 질문이 필요하면(빠진 설정, 검사를 통과하지 못한 기존 값) 멈추지 않고 그 옵션 목록을 알린 뒤 2로 끝낸다. 확인 질문(진행할까요, 그래도 계속할까요)도 비TTY에서는 묻지 않고 오류로 끝낸다. 업그레이드(옵션 없는 비TTY 실행, `--upgrade`), 설정을 모두 준 `--service-install`, `--dry-run`은 진행 확인을 하지 않는다
- 종료 코드: 잘못된 옵션(함께 쓸 수 없는 방식 옵션 포함)·비TTY의 빠진 설정 2, 오류(`--upgrade`인데 설치되어 있지 않음 포함) 1, "진행할까요?"·제거 확인에서 아니요와 업그레이드 "취소" 0, "그래도 계속할까요?"·기존 실행 파일로 설치할지 확인에서 아니요 1, 중단(Ctrl+C) 130
- 유닛 하드닝을 바꾸면 `README.md`의 보안 설계, `.claude/docs/helper.md`, `.claude/rules/helper.md`도 함께 본다
- 테스트는 root·sudo·실제 systemctl을 쓰지 않는다(가짜 루트와 명령 기록). 실제 설치는 사용자가 하거나 테스트 VM에서 확인한다
- `ROADMAP.md`의 항목(도우미 추가 기준 경로 질문 등)은 요청하기 전에 구현하지 않는다. 설정 하나와 템플릿 한 줄로 더할 수 있는 구조만 유지한다
- 새 의존성을 더하지 않는다. TUI는 huh v2(함께 오는 Bubble Tea·Lip Gloss 포함)만 쓴다
