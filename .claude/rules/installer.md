---
paths:
  - "install.sh"
  - "installer/**"
  - "backend/config/**"
---

# 설치기 (install.sh, installer/)

사용자 설명은 `docs/ko/install.md`(설치 스크립트), `docs/ko/install-options.md`(옵션 표), `docs/ko/upgrade.md`(업그레이드). README의 빠른 시작은 처음 설치하는 흐름만 둔다.

## 구조

- `install.sh`는 부트스트랩이다. 실행한 계정으로 `installer/`를 `bin/maru-install`로 빌드해 exec한다(go가 없거나 root가 남의 저장소에서 실행하면 이미 있는 실행 파일을 쓴다). 옵션과 동작은 모두 설치기가 정한다. 진입점 이름 `./install.sh`는 README·`docs/`, 화면 문구(모든 언어), 생성 파일의 주석이 참조하므로 바꾸지 않는다
- `installer/`는 별도 Go 모듈(`maru/installer`)이다. `go.work` 없이 `replace maru/backend => ../backend`로 로컬 백엔드를 쓰고, go 지시어는 `backend/go.mod`와 같게 둔다. TUI 의존성 huh v2는 이 모듈에만 있다(`backend/go.mod`·`go.sum`에는 charm 모듈이 없어야 한다)
- 백엔드에서 쓰는 것은 공개 패키지 `backend/config`(환경 변수 키, 기본값, 검증 함수, 버전)뿐이다. `backend/internal`은 Go 규칙상 import할 수 없고, `TestBackendDependencies`가 다른 백엔드 패키지를 링크하지 않는지 본다. `backend/config`는 표준 라이브러리만 쓴다(`TestStandardLibraryOnly`)
- `internal/setup`
  - `run.go`: 흐름(1단계 설치, `--service-install` 2단계, `--uninstall`, `--dry-run`, `--upgrade-plan`)과 sudo 재실행(`sudo bin/maru-install --service-install --lang X <레지스트리가 만든 인자> [--upgrade | --reinstall] [--plain]`). root면 같은 프로세스에서 이어 간다. 2단계는 받은 값을 다시 검사한다
  - `mode.go`: 설치 방식(새 설치, 다시 설치, 업그레이드)을 정하고 알리며, 업그레이드 요약 뒤의 선택(업그레이드·설정 바꾸기·취소)과 진행 확인을 한다(아래 "설치 방식")
  - `setting.go`: 설정 타입과 레지스트리 동작(옵션 파싱, 사용법, 2단계 인자, 업그레이드 로드, 질문, 검증, 요약, 환경 파일 생성). `settings.go`: 설정 목록. 순서가 질문·요약·환경 파일 줄·사용법의 순서다. `setting_<이름>.go`: 설정 정의
  - `step.go`·`steps.go`·`step_<이름>.go`: 설치·제거 단계. User 단계는 빌드, Root 단계는 설치다
  - `setting_webupgrade.go`·`step_webupgrade.go`: 대시보드에서 업그레이드 설정(`--web-upgrade`, `--build-path`)과 설치·제거(아래 "업그레이드 서비스")
  - `upgrade_service.go`·`upgrade_exec.go`: 업그레이드 서비스(`--upgrade-service`)의 확인·적용 흐름과, 소유자 권한 명령 실행·요청·상태 파일 처리
  - `step_record.go`: 설치 기록(`config.InstallRecordFile`, `/var/lib/maru-upgrade/installed.json`, 디렉터리 0755·파일 0644). 서비스를 시작한 뒤 매번 쓰고 제거할 때 디렉터리째 지운다. 커밋은 설치하는 `bin/maru`의 빌드 정보(`System.ReadBuildInfo`)에서 읽고, 버전(`config.Version`)은 설치기 자신의 커밋(`Context.SelfBuild`)과 같을 때만 적는다. 확인하지 못한 값은 비우고 경고만 한다(설치는 실패하지 않는다)
  - `plan.go`: 업그레이드 계획(`--upgrade-plan`, 아래 "업그레이드 계획"). `upgrade_list.go`: 수동 재설치 경계와 업그레이드 뒤 할 일 목록(유지자가 고치는 곳). `version.go`: `X.Y.Z` 비교, Node.js 범위, 요구 도구 버전 상수
  - `system.go`: System 인터페이스와 실제 구현. `system_dryrun.go`: 파일 쓰기·root 명령을 보여 주기만 하고 읽기·확인·빌드는 실제로 한다. 테스트의 `fake_test.go`: 가짜 루트(임시 디렉터리)와 명령 기록
  - `envfile.go`: 환경 파일 읽기(source 안 함. 마지막 `KEY=` 줄, 값을 감싼 큰따옴표 한 쌍은 벗김)와 쓰기(내용이 같으면 "unchanged", 바뀌면 환경 파일만 `.bak-<시각>` 백업, 유닛은 백업 없이 덮어씀)
  - `templates/*.tmpl`: 유닛 5종(maru, 도우미 소켓·서비스, 업그레이드 서비스·경로. embed, `text/template`). 하드닝 줄을 바꾸는 일은 이 파일을 고치는 것이다
  - `text.go`: 문구 `L(en, ko)`와 로캘 언어 선택(`--lang`, 아니면 `LC_ALL`·`LC_MESSAGES`·`LANG`이 ko로 시작하면 ko). 문구는 쓰는 곳(설정·단계 파일) 옆에 둔다
- `internal/prompt`: Prompter(Input, Select, Confirm, Note). huh(표준 입력·출력이 터미널, `TERM≠dumb`, `--plain` 아님), 줄 단위(그 밖), 테스트용 `Script`(질문 ID별 답)와 `Fail`(질문하면 실패). 두 실제 구현 모두 잘못된 입력은 다시 묻는다

## 설치 방식

`resolveMode`가 1단계와 직접 실행한 2단계에서 같은 규칙으로 정한다. "설치되어 있음"은 유닛 파일과 `/etc/default/maru`가 모두 있다는 뜻이다.

- 옵션 없음
  - 둘 다 없음: 새 설치. 옵션으로 주지 않은 설정을 모두 묻고 요약 뒤 "진행할까요?"
  - 둘 다 있음: 업그레이드. 설치된 설정을 읽고(설정 옵션이 덮어쓴다) 빠진 설정만 묻는다. 요약 뒤 터미널이면 업그레이드(기본)·설정 바꾸기·취소를 고른다. "설정 바꾸기"는 그 자리에서 다시 설치로 바꿔 지금 값(설치된 값과 옵션)을 기본값으로 모든 설정을 다시 묻고 "진행할까요?"를 묻는다. "취소"는 아무것도 하지 않고 0. 비TTY와 dry-run은 묻지 않는다
  - 하나만 있음: 경고하고 다시 설치
- `--reinstall`: 다시 설치. 설치된 값을 기본값으로 옵션으로 주지 않은 설정을 모두 묻고 "진행할까요?". 새 시스템에서는 새 설치와 같다
- `--upgrade`(스크립트용): 업그레이드. 설치되어 있지 않으면 1로 끝낸다. 요약 뒤 아무것도 묻지 않는다
- `--upgrade`와 `--reinstall`을 함께, 또는 둘 중 하나를 `--uninstall`·`--upgrade-plan`과 함께 주면 사용법 오류(2). 동작 옵션(`--uninstall`, `--service-install`, `--upgrade-plan`)은 하나만 받는다
- 업그레이드(자동 감지와 `--upgrade` 모두)에서는 Force SSL/TLS·포트 사용 중을 경고만 하고 묻지 않으며, 수신 주소와 서비스 사용자가 그대로면 리버스 프록시 안내를 줄인다. root가 남의 저장소에서 빌드하려 하면 `--upgrade`와 비TTY 자동 업그레이드는 멈추고, 터미널의 자동 업그레이드는 기존 실행 파일로 설치할지 묻는다
- 업그레이드에서 기존 설치의 값이 `Validate`를 통과하지 못하면(`dropInvalid`) 그 설정을 빠진 것으로 본다. 터미널이면 경고하고 기존 값이 아닌 기본값(`withoutInstalled`)으로 다시 묻고(수신 주소는 수신 방식부터), 비TTY면 그 옵션과 오류를 알리고 2로 끝낸다(옵션으로 주거나 `--reinstall`)
- 2단계에는 1단계가 끝난 방식을 넘긴다. 업그레이드면 `--upgrade`, 다시 설치(설정 바꾸기 포함)면 `--reinstall`, 새 설치면 없음. 2단계가 기존 설치를 보고 방식을 다시 정하지 않게 한다

## 업그레이드 계획(--upgrade-plan)

대시보드에서 업그레이드(`.claude/docs/upgrade.md`)는 대상 커밋의 설치기를 저장소 소유자 계정으로 임시 빌드해 `--upgrade-plan`을 실행하고, 그 JSON(`config.UpgradePlan`)으로 물을 설정과 수동 재설치 여부를 정한다. 형식은 `backend/config/upgrade.go`에 있다.

- 읽기만 한다. 파일을 쓰지 않고, 실행하는 명령은 `node --version`(`System.Output`)뿐이다. `resolveMode` 전에 분기하므로 설치 방식 안내·일부 설치 경고를 찍지 않고 아무것도 묻지 않는다
- 표준 출력에는 JSON만 쓰고 경고는 오류 출력에 쓴다(`install.sh`의 안내도 오류 출력이다). 계획을 냈으면 수동 사유가 있어도 0, 만들지 못하면 1
- 실행 중에 저장소 파일을 읽지 않는다(실행 파일 위치가 저장소가 아닐 수 있다). 요구 도구 버전은 `version.go`의 상수이고 원본과 같은지는 테스트가 본다(`TestRequireGo`: 두 `go.mod`의 go 지시어, `TestRequireNode`: `frontend/package.json`의 `engines.node`). Go는 계획을 낸 설치기를 빌드할 수 있었다면 충분하므로 확인하지 않는다
- 설정은 업그레이드처럼 읽는다: 옵션 값과 기존 설치의 값을 `dropInvalid`로 검사하고 `missing`을 질문으로 만든다. 일반 질문(`Prompt`) 설정은 자동으로 웹 질문이 되고(문구는 `PromptSpec`의 두 언어, 기본값은 `defaultValue`, 검사 오류는 `Lang`만 바꾼 Context 사본으로 두 언어), 특수한 질문(`Ask`, 지금은 수신 주소) 설정은 수동 사유가 된다. 옵션을 주어 다시 계획하면 입력값 검사가 되며, 정하지 않은 예·아니요에 딸린 설정(상승 그룹)은 기본값(예)으로 보고 함께 묻다가 답을 옵션으로 주면 정리된다
- 수동 사유: 설치되어 있지 않음(이것만 내고 끝), 쓸 수 있는 설치 기록 없음(없음·읽기 실패·모르는 형식·버전이나 커밋이 빔), 다운그레이드, 목록의 경계, `Ask` 설정, Node.js 없음·범위 밖. 사유를 모두 모으고, 명령은 하나라도 `--reinstall`이면 `./install.sh --reinstall`, 아니면 `./install.sh`
- JSON에서 필드를 빼거나 뜻을 바꾸면 `config.UpgradePlanFormat`(설치 기록은 `InstallRecordFormat`)을 올린다. 읽는 쪽이 몰라도 되는 필드를 더할 때는 그대로 둔다

## 업그레이드 서비스(--web-upgrade, --upgrade-service)

설계는 `.claude/docs/upgrade.md`, 사용자 설명은 `docs/ko/upgrade.md`의 "대시보드에서 업그레이드". 경로·형식은 `backend/config`(`upgrade.go`, `upgrade_request.go`)에 있다.

- 설정: `--web-upgrade`(`FlagToggle`, 도우미를 설치할 때만)와 `--build-path`(묻지 않는 `FlagValue`). 켜면 `stepWebUpgrade`가 설치기 복사본(`/usr/local/lib/maru/maru-install`, `bin/maru-install`에서), `/etc/default/maru-upgrade`(저장소 경로·빌드 계정·빌드 PATH), `maru-upgrade.service`·`.path`를 쓰고, 도우미 유닛에 `RuntimeDirectory=maru-upgrade`가 들어간다. 끄거나 도우미를 지우면(제거 포함) `removeWebUpgrade`가 경로 유닛을 끄고 이것들과 남은 요청·상태 파일을 지운다. 돌고 있는 업그레이드 서비스는 멈추지 않는다
  - 값: 설정 파일이 있으면 켜짐이다. 이 설정이 없던 버전의 설치는 꺼진 것으로 읽어 묻지 않는다. 새 설치의 기본값은 저장소 소유자가 일반 사용자일 때 켜짐이고, root 소유 저장소에서는 켤 수 없다(`Validate`). 소유자는 `System.FileOwner(c.RepoDir)`로 읽는다
  - 빌드 PATH: sudo가 PATH를 바꾸므로 1단계(`ActionInstall`)는 기존 값을 읽지 않고 `System.Which`로 git·go·node·npm·make를 찾은 디렉터리 + `/usr/local/bin:/usr/bin:/bin`을 `Registry.fill`로 채워 2단계에 넘긴다. 2단계를 직접 실행하면(대시보드 업그레이드 포함) 기존 값을 쓴다
  - 설정 파일 키(`config.EnvUpgrade*`)와 서비스 시간 상한(`config.UpgradeServiceTimeout`, 유닛의 `TimeoutStartSec`로 렌더), 설치한 상태 디렉터리(`config.InstalledStateDir`)는 maru도 쓰므로 `backend/config`에 둔다
  - 경로 유닛은 바뀌었을 때만 다시 시작한다. 업그레이드 서비스 유닛은 어떤 단계도 시작·재시작하지 않는다(그 서비스 안에서 설치기가 돈다)
- `--upgrade-service`(root, 다른 옵션 없음): 저장소 밖의 복사본으로 돌므로 `c.RepoDir`를 쓰지 않고 저장소·계정·PATH는 설정 파일에서만 정한다. 요청 하나를 처리한다(적용이 먼저)
  - 요청은 내용과 상관없이 먼저 읽고 지운다(`PathExists=`가 다시 실행하지 않게). maru가 쓰는 확인 요청은 `O_NOFOLLOW`·4KiB, root가 쓰는 설정 파일·적용 요청은 64KiB까지 읽는다. 설정 파일이 없으면 요청만 지우고 상태 파일을 만들지 않는다
  - 소유자 명령은 uid·gid·보조 그룹으로 강등하고 새 세션, 새 환경 변수(HOME·PATH 등, 프롬프트 끔)로 빌드 PATH에서 찾아 실행한다. root로 실행하는 것은 마지막의 `bin/maru-install --service-install --upgrade --plain --lang en <설정>`뿐이다
  - 확인 요청의 채널(`config.UpgradeChannel*`, 정한 값만)은 원격 확인만 따른다. 채널이 있으면 `origin`의 그 브랜치를 refspec으로 이름을 정해 가져와 대상으로 삼고, 지금 브랜치와 다르면 `Switch`로 fast-forward·설치된 커밋에서 이어짐 검사 없이 덮어쓴다(로컬 채널 브랜치에 원격에 없는 커밋이 있으면 `diverged`). 적용은 `checkout -B <채널> <대상>`과 추적 설정을 한다(`.claude/docs/upgrade.md`의 "업데이트 채널"). 상태 파일의 `branch`·`detached`에 본 브랜치를 남긴다
  - 확인은 저장소 상태를 수동 재설치 사유(`config.UpgradeReason*`)로 담고, 대상 커밋의 `installer`·`backend`만 `git archive`로 소유자 임시 디렉터리에 꺼내 빌드해 `--upgrade-plan`을 실행한다(임시 디렉터리가 소유자 것이어야 `--web-upgrade` 검사가 통과한다). 적용은 마지막 확인·설치 기록·작업 트리·커밋 관계를 다시 보고 설정 값을 계획으로 다시 검사한 뒤(빌드 전) fast-forward, 서브모듈, `stepBuild`(소유자), 빌드 결과의 커밋 확인, 설치 순서다
  - 작업 실패는 상태 파일(`Error`, 로그 끝부분)에 남기고 0으로 끝낸다. 단계가 바뀔 때마다(설치 단계 전 포함) 상태 파일을 쓴다
  - 업그레이드 서비스의 동작을 바꾸면 이전 버전의 서비스가 새 버전을 올린다는 점을 생각한다. 이전 서비스가 못 하는 일이 생기면 `manualUpgrades`에 항목을 더한다
- 시험: 설정·파일·명령은 가짜 루트(`webupgrade_test.go`, 골든 `helper-web-upgrade`), 서비스는 실제 git 저장소(bare 원격 + 클론)와 계획을 내는 가짜 설치기(`upgrade_service_test.go`, 권한을 바꾸지 않음)로 본다. git·go·make·npm·tar가 없으면 건너뛴다

## 수동 재설치 항목과 업그레이드 뒤 할 일

- 대시보드 업그레이드로 넘을 수 없는 변경을 하면 `upgrade_list.go`의 `manualUpgrades`에 항목(`Below`: 그 변경이 들어가는 버전, `Reason`: 영어·한국어 이유, `Command`: `config.UpgradeCommandInstall` 또는 `config.UpgradeCommandReinstall`)을 더한다. 아직 `Version`을 올리지 않았어도 다음 버전으로 적는다
  - 예: 서비스 사용자나 소켓 경로가 바뀔 때, 새 root 구성 요소(유닛, root 쪽 설정 파일)가 생길 때, 업그레이드 서비스 자체의 동작이 바뀔 때, 사용자가 꼭 문서를 읽어야 할 때
  - 새 설정 하나로 끝나는 변경은 `Prompt`로 묻게 하면 항목이 필요 없다
- 업그레이드한 뒤 사용자가 할 일(프록시 컨테이너 다시 만들기 등)은 `upgradeNotes`에 `Since`와 함께 더한다
- 두 목록의 항목은 `TestUpgradeListsValid`가 검사한다(버전 꼴, 두 언어 문구, 명령)

## 설정을 더할 때

1. `setting_<이름>.go`에 `Setting`을 정의한다. 가능하면 `Prompt`로 묻는다(대시보드에서 업그레이드할 때 웹 질문이 된다. 위 "업그레이드 계획"). `Name`(플래그 이름), `Flag`, `Arg`, `Usage`, `Env`(키는 `backend/config`의 상수, 주석은 영어), `Default`(기존 설치 → 감지값 → 고정값), `Load`(업그레이드에서 기존 설치의 값. 없거나 `Validate`를 통과하지 못하면 그 설정만 묻는다), `Prompt` 또는 `Ask`, `Validate`, `Summary`, 필요하면 `When`·`Notice`
   - 묻지 않고 설치기가 정하는 값(빌드 PATH 등)은 `Prompt`·`Ask` 없이 `Default`를 두면 질문 뒤 `Registry.fill`이 채운다
   - 백엔드와 같은 규칙이면 `backend/config`의 함수로 검사하고, 설치기만의 더 엄격한 규칙(사용자 이름, Unix 소켓 경로, IP만 받는 주소 등)은 설정 정의에 둔다
   - 설정 정의 안의 함수에서 다른 설정 값은 `name*` 상수로 읽는다(설정 변수를 직접 가리키면 패키지 초기화 순환이 생긴다)
2. `settings.go`의 목록에 한 줄 더한다
3. 백엔드 플래그도 생기면 `backend/config`에 키·기본값을 두고 `cmd/maru/main.go`의 플래그와 `runtimeEntry` 목록에서 쓴다
4. `docs/ko/install-options.md`의 옵션 표(`TestDocsListEveryOption`), 백엔드 플래그면 `docs/ko/configuration.md`의 항목 표(두 문서 모두 영어 `docs/en/`도 함께), `setting_test.go`의 검사 표와 왕복 표에 더한다
5. 환경 파일 출력이 바뀌면 골든(`testdata/golden/*/want`)도 고친다. 골든은 bash `install.sh`의 출력에서 시작했으므로 바꾼 이유를 커밋 메시지에 적는다

## 단계를 더할 때

- `step_<이름>.go`에 `func(c *Context) error`를 만들고 `steps.go`의 `installSteps` 또는 `uninstallSteps`에 `Phase`·`When`과 함께 한 줄 더한다
- 시스템을 바꾸는 일은 `c.Sys`로만 한다. 파일은 `WriteFile`·`InstallFile`·`Remove`, 명령은 `Run`(출력을 버리면 `RunQuiet`·`RunSilent`), 읽기 전용 확인은 `Check`. 그래야 `--dry-run`과 가짜 루트 테스트가 그대로 동작한다
- 설정 파일은 `c.writeConfig`로 쓴다. 질문은 `c.confirm`(dry-run은 예로 보고 진행, 비TTY는 오류)이나 `c.P`로 한다

## 불변 조건

- 생성하는 파일(유닛 5종, `/etc/default/maru`, `/etc/default/maru-helper`, `/etc/default/maru-upgrade`)은 골든과 바이트 단위로 같고 명령 순서(`commands`)도 같아야 한다(`TestGoldenServiceInstall`). 골든은 bash `install.sh`의 출력에서 시작했고, bash에 없던 설정(대시보드에서 업그레이드)은 Go 설치기 출력으로 만든 케이스로 본다(`testdata/golden/README`). 생성 파일의 주석은 영어로 쓰고 진입점을 `install.sh`라고 부른다
- 의도한 차이는 레지스트리가 관리하지 않는 기존 `MARU_*` 줄을 파일 끝 "Kept from the previous file" 주석 아래에 보존하는 것뿐이다. 그런 줄이 없으면 주석도 쓰지 않는다
- 설치 기록은 설치 시각이 들어가므로 골든 대상이 아니다(`record_test.go`가 본다). 파일 쓰기는 명령 기록에 남지 않으므로 골든의 `commands`도 그대로다
- bash `install.sh`가 만든 설치(같은 경로·환경 변수 키·유닛의 `User=`)를 업그레이드(옵션 없음 또는 `--upgrade`)로 읽는다. 설정이 모두 있는 설치의 업그레이드는 설정 질문이 없고(옵션 없는 터미널 실행은 요약 뒤 선택 하나), 설정을 모두 준 `--service-install`(`docs/ko/install.md`의 "자동화" 예시, 테스트 VM의 설치 시험)은 설치 여부와 상관없이 질문 없이 끝나야 한다
- 비TTY에서 설정 질문이 필요하면(빠진 설정, 검사를 통과하지 못한 기존 값) 멈추지 않고 그 옵션 목록을 알린 뒤 2로 끝낸다. 확인 질문(진행할까요, 그래도 계속할까요)도 비TTY에서는 묻지 않고 오류로 끝낸다. 업그레이드(옵션 없는 비TTY 실행, `--upgrade`), 설정을 모두 준 `--service-install`, `--dry-run`은 진행 확인을 하지 않는다
- 종료 코드: 잘못된 옵션(함께 쓸 수 없는 방식·동작 옵션 포함)·비TTY의 빠진 설정 2, 오류(`--upgrade`인데 설치되어 있지 않음 포함) 1, 계획을 낸 `--upgrade-plan` 0, "진행할까요?"·제거 확인에서 아니요와 업그레이드 "취소" 0, "그래도 계속할까요?"·기존 실행 파일로 설치할지 확인에서 아니요 1, 중단(Ctrl+C) 130
- 유닛 하드닝을 바꾸면 `.claude/docs/helper.md`, `.claude/rules/helper.md`도 함께 보고, 사용자에게 보이는 영향(쓸 수 있는 경로 등)이 바뀌면 `docs/ko/helper.md`도 고친다
- maru 유닛의 `RuntimeDirectoryPreserve=yes`는 `/run/maru`를 재시작 사이에도 남긴다. 디렉터리가 새로 만들어지면 컨테이너 프록시의 바인드 마운트가 이전 디렉터리를 계속 가리켜 연결이 끊기기 때문이다. 제거(`--uninstall`)는 지우므로 다시 설치한 뒤 프록시 컨테이너를 다시 만들어야 한다(`docs/ko/install.md`의 "제거")
- 생성하는 `/etc/default/maru`의 머리 주석 `# See README.md for every option.`(`setting.go`의 `MaruEnv`)은 README가 설정 문서(`docs/en/configuration.md`)로 이어 주므로 일부러 그대로 둔다. 바꾸면 골든(bash 출력과 같은 케이스 포함)이 바뀌고 기존 설치마다 업그레이드 때 `.bak`이 한 번 생긴다. 다음에 `MaruEnv`의 내용이 어차피 바뀔 때 `docs/en/configuration.md`로 바꾼다
- 테스트는 root·sudo·실제 systemctl을 쓰지 않는다(가짜 루트와 명령 기록). 실제 설치는 사용자가 하거나 테스트 VM에서 확인한다
- `.claude/docs/roadmap.md`의 항목(도우미 추가 기준 경로 질문 등)은 요청하기 전에 구현하지 않는다. 설정 하나와 템플릿 한 줄로 더할 수 있는 구조만 유지한다
- 새 의존성을 더하지 않는다. TUI는 huh v2(함께 오는 Bubble Tea·Lip Gloss 포함)만 쓴다
