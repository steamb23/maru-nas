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
  - `run.go`: 흐름(1단계 설치, `--service-install` 2단계, `--uninstall`, `--dry-run`, `--upgrade-plan`, root 서비스가 실행하는 `--upgrade-service`·`--addon-sync`)과 sudo 재실행(`sudo bin/maru-install --service-install --lang X <레지스트리가 만든 인자> [--upgrade | --reinstall] [--plain]`). root면 같은 프로세스에서 이어 간다. 2단계는 받은 값을 다시 검사한다
  - `mode.go`: 설치 방식(새 설치, 다시 설치, 업그레이드)을 정하고 알리며, 업그레이드 요약 뒤의 선택(업그레이드·설정 바꾸기·취소)과 진행 확인을 한다(아래 "설치 방식")
  - `setting.go`: 설정 타입과 레지스트리 동작(옵션 파싱, 사용법, 2단계 인자, 업그레이드 로드, 질문, 검증, 요약, 환경 파일 생성). `settings.go`: 설정 목록. 순서가 질문·요약·환경 파일 줄·사용법의 순서다. `setting_<이름>.go`: 설정 정의
  - `step.go`·`steps.go`·`step_<이름>.go`: 설치·제거 단계. User 단계는 빌드, Root 단계는 설치다
  - `setting_webupgrade.go`·`step_webupgrade.go`: 대시보드에서 업그레이드 설정(`--web-upgrade`, `--build-path`)과 설치·제거(아래 "업그레이드 서비스")
  - `setting_addon_backends.go`·`step_addon_backends.go`·`addon_record.go`·`addon_sync.go`: 애드온 백엔드 설정(`--addon-backends`), 유닛 설치·제거, root 동기화의 기록·잠금, 동기화(`--addon-sync`)(아래 "애드온 백엔드")
  - `step_installer_copy.go`: 두 root 서비스(업그레이드 서비스, 애드온 백엔드 동기화)가 함께 실행하는 root 소유 설치기 복사본(`/usr/local/lib/maru/maru-install`)
  - `upgrade_service.go`·`upgrade_exec.go`: 업그레이드 서비스(`--upgrade-service`)의 확인·적용 흐름과, 소유자 권한 명령 실행·요청·상태 파일 처리
  - `step_record.go`: 설치 기록(`config.InstallRecordFile`, `/var/lib/maru-upgrade/installed.json`, 디렉터리 0755·파일 0644). 서비스를 시작한 뒤 매번 쓰고 제거할 때 디렉터리째 지운다. 커밋은 설치하는 `bin/maru`의 빌드 정보(`System.ReadBuildInfo`)에서 읽고, 버전(`config.Version`)은 설치기 자신의 커밋(`Context.SelfBuild`)과 같을 때만 적는다. 확인하지 못한 값은 비우고 경고만 한다(설치는 실패하지 않는다)
  - `plan.go`: 업그레이드 계획(`--upgrade-plan`, 아래 "업그레이드 계획"). `upgrade_list.go`: 수동 재설치 경계와 업그레이드 뒤 할 일 목록(유지자가 고치는 곳). `version.go`: `X.Y.Z` 비교, Node.js 범위, 요구 도구 버전 상수
  - `system.go`: System 인터페이스와 실제 구현. `system_dryrun.go`: 파일 쓰기·root 명령을 보여 주기만 하고 읽기·확인·빌드는 실제로 한다. 테스트의 `fake_test.go`: 가짜 루트(임시 디렉터리)와 명령 기록
  - `envfile.go`: 환경 파일 읽기(source 안 함. 마지막 `KEY=` 줄, 값을 감싼 큰따옴표 한 쌍은 벗김)와 쓰기(내용이 같으면 "unchanged", 바뀌면 환경 파일만 `.bak-<시각>` 백업, 유닛은 백업 없이 덮어씀)
  - `templates/*.tmpl`: 유닛 템플릿 9개(maru, 도우미 소켓·서비스, 업그레이드 서비스·경로, 애드온 백엔드 소켓·서비스(네트워크 단계 4개로 그려 유닛 파일 8개), 애드온 동기화 경로·서비스. embed, `text/template`). 하드닝 줄을 바꾸는 일은 이 파일을 고치는 것이다
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

- 설정: `--web-upgrade`(`FlagToggle`, 도우미를 설치할 때만)와 `--build-path`(묻지 않는 `FlagValue`). 켜면 `stepWebUpgrade`가 `/etc/default/maru-upgrade`(저장소 경로·빌드 계정·빌드 PATH), `maru-upgrade.service`·`.path`를 쓰고, 도우미 유닛에 `RuntimeDirectory=maru-upgrade`가 들어간다. 끄거나 도우미를 지우면(제거 포함) `removeWebUpgrade`가 경로 유닛을 끄고 이것들과 남은 요청·상태 파일을 지운다. 돌고 있는 업그레이드 서비스는 멈추지 않는다
  - 설치기 복사본(`/usr/local/lib/maru/maru-install`, `bin/maru-install`에서)은 애드온 백엔드의 동기화도 실행하므로 `stepInstallerCopy`가 따로 맡는다. 둘 중 하나라도 켜져 있으면 설치할 때마다 바꾸고, 둘 다 꺼야 지운다(디렉터리는 비었을 때만). 설치·제거 모두 웹 업그레이드 → 애드온 백엔드 → 복사본 순서다(복사본을 실행하는 유닛을 먼저 끈다). 복사본이 늘 지금 설치기이므로 이전 버전의 서비스가 새 버전을 올려도 다음 실행부터 새 동작이다
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

## 애드온 백엔드(--addon-backends, --addon-sync)

설계는 `.claude/docs/addon-packages.md`의 "백엔드 프로세스"·"root 동기화", 사용자 설명은 `docs/ko/install.md`의 "애드온 백엔드". 경로·형식은 `backend/config`(`addons.go`, `addons_units.go`)에 있다.

- 설정: `--addon-backends`(`FlagToggle`, 기본 꺼짐, `Prompt`·`Ask` 없음). 새 설치와 업그레이드에서 묻지 않고 업그레이드 계획에도 나오지 않는다. 값은 동기화 디렉터리(`config.AddonSyncDir`)가 있는지로 읽는다(`Installed.AddonBackends`, `Load`가 늘 정한 값을 돌려준다). 환경 파일 줄은 없고 도우미와 상관없다
  - 이 디렉터리가 켜짐의 기준이므로 끄는 쪽·잠금이 디렉터리를 새로 만들면 안 된다(다음 업그레이드가 켜진 것으로 읽는다)
- 켤 때(`stepAddonBackends` → `installAddonBackends`): 네트워크 단계마다(`addonTiers`, 이름은 `config.AddonUnitPrefix`) 템플릿 `maru-addon.socket`·`maru-addon.service`를 `<단계>@.socket`·`.service`로, `maru-addons.path`·`.service`를 쓰고 단계별로 바뀌었는지 남긴다. 동기화 결과(`config.AddonSyncStatusFile`)가 없으면 빈 결과(digest 없음, 0644)를 둔다. 이 파일이 디렉터리(0755)를 만들고, maru는 시작할 때 이 파일로 켜졌음을 알므로 maru를 다시 시작하기 전에 둔다
  - 호스트 소켓 디렉터리(`config.AddonHostDir`): 백엔드 서비스가 `<id>`를 바인드하므로(바인드 원본은 PID 1이 호스트에서 심볼릭 링크를 따라 푼다) 그 경로에 maru가 바꿀 수 있는 구성 요소가 없어야 한다. 그래서 maru 유닛의 `RuntimeDirectory=`에 넣지 않고, 켰을 때만 `ExecStartPre=+install -d -o root -g root -m 0755 <dir>`(root, 샌드박스 밖, 고정 인자)와 `ReadWritePaths=-<dir>`(`ProtectSystem=strict` 아래에서 `<id>` 안에 소켓을 열게)를 더한다. systemd(257, 2026-11-05 테스트 VM)는 `+` 명령에도 마운트 이름공간을 먼저 만들어 `-` 없는 `ReadWritePaths`의 경로가 없으면 그 `ExecStartPre`부터 `226/NAMESPACE`로 실패하므로 `-`를 붙인다. 이름공간은 명령마다 따로 만들므로 `ExecStartPre`가 만든 디렉터리는 `ExecStart`의 이름공간에서 쓰기 가능하다. `<id>`는 소켓 템플릿의 `ExecStartPre`가 root로 만들어 서비스 사용자(0755)에게 준다(부팅 때도 소켓이 maru보다 먼저 시작한다). maru는 `<id>`를 만들지 않고 그 안에 소켓만 연다
  - `BindReadOnlyPaths=<state>/addons/%i`의 원본은 maru 상태 디렉터리 안이라 maru가 바꿀 수 있지만, `ExecStart`가 그 안에서 풀려야 하므로 maru가 이미 쓸 수 있는 디렉터리만 쓸모가 있다(넓어지는 범위가 없다)
- daemon-reload 뒤(`stepAddonBackendsPath`): 동기화 잠금을 쥐고 경로 유닛을 켠다(바뀌었으면 다시 시작). 템플릿이 바뀐 단계의 켜진 인스턴스(`knownAddonInstances`: 기록 다음에 `sockets.target.wants`의 링크에만 있는 것. 기록을 읽지 못해도 링크로 찾은 것은 다룬다)를 `try-restart`한다(유닛 설정은 시작할 때만 적용되므로). 소켓 템플릿이 바뀌면 소켓을(서비스는 `Requires=`라 함께 다시 시작해 새 소켓을 받는다), 서비스 템플릿만 바뀌면 서비스를. 실패는 경고만 한다. 동기화 서비스는 시작하지 않는다
- 끌 때(`removeAddonBackends`, 제거 포함): 유닛 파일도 동기화 디렉터리도 없으면 명령 없이 끝낸다. 있으면 경로 유닛을 끄고, 잠금을 쥔 채(디렉터리가 있을 때) `knownAddonInstances`(링크는 단계 이름과 id 형식에 맞는 것만)를 멈춘 뒤 소켓을 끄고(`stop` 다음 `disable`. `disable --now`는 유닛 파일이 없으면 멈추지 않는다) 남은 링크를 지운다. 이어서 고정 패턴(`maru-addon@*`, `maru-addon-*@*`의 `.socket`·`.service`)으로 아직 올라와 있는 인스턴스를 멈추고, 유닛 파일·남은 요청(일반 파일이 아니면 따라가지 않고 통째로, `RemoveAll`)을 지우고 daemon-reload, 동기화·백엔드 소켓·호스트 소켓 디렉터리를 지운다. 백엔드 데이터(`/var/lib/private/maru-addon`, `/var/lib/maru-addon`)는 남긴다. 제거는 `files` 단계(maru 유닛 삭제) 전에 끄고, 데이터는 상태 디렉터리 질문 다음에 따로 묻는다(`uninstall-addon-data`)
- 동기화 기록(`config.AddonSyncRecordFile`, root 0600, `addon_record.go`): `{format: 1, backends: [{id, unit, rev, instance}], data: [{id, instance}]}`. `backends`는 켜 둔 인스턴스(32개까지), `data`는 끄면서 데이터를 남긴 id와 그 데이터의 설치 인스턴스(128개까지)이고 한 id는 한쪽에만 있다. 읽을 때마다 엄격하게 검사한다(모르는 필드, 뒤에 남은 값(`json.Decoder.More`가 놓치는 `]`·`}` 포함, 다음 토큰이 `io.EOF`여야 한다), 64KiB, id·단계·rev·인스턴스 형식). 데이터의 인스턴스는 이 기록으로만 증명하므로, 데이터가 있는데 기록에 없는 id는 다른 인스턴스의 데이터로 보고 그 id를 켜기 전에 지운다. 켜지 않는 id의 데이터는 purge가 아니면 지우지 않는다(`--no-addon-backends`와 제거가 기록을 지우므로 껐다 켜면 남긴 데이터는 그 패키지의 백엔드를 다시 켤 때 지워진다. 제거의 데이터 질문과 `docs/ko/install.md`가 이를 알린다). `data`가 상한이면 원하는 상태에 없는 id는 새로 적지 않고(그 id를 다시 켤 때 지워질 뿐이다), 원하는 상태에 있는 id(단계를 바꾸거나 다시 켜려고 잠깐 끈 것)는 원하는 상태에 없는 항목 하나를 밀어내고 적는다(켜기가 실패해도 다음 동기화가 같은 설치의 데이터를 지우지 않게. 원하는 id는 32개까지라 밀어낼 항목이 늘 있다)
- 잠금: 동기화 기록을 읽거나 쓰거나 인스턴스를 켜고 끄는 root 쪽(설치기의 `--service-install`·`--uninstall`, `--addon-sync`)은 모두 `config.AddonSyncLockFile`(flock, `System.Lock`)을 잡는다. 잡은 뒤 경로가 같은 파일인지 다시 보고, 지워졌으면 다시 연다(디렉터리째 지워졌으면 실패). 설치기는 바로 잡지 못하면 기다린다고 알린 뒤 동기화의 시간 상한(`addonSyncTimeout`)보다 조금 더 기다린다. 유닛 파일과 빈 결과를 쓰는 동안에는 잡지 않는다(기록을 건드리지 않는다). dry-run은 잠그지 않는다
- 유닛 하드닝: 백엔드 서비스 템플릿은 `maru.service`보다 느슨하지 않아야 한다. `TestAddonBackendUnitsNotLooserThanMaru`가 `maru.service`의 하드닝 지시어마다 같은 값이나 더 엄격한 지시어(`DevicePolicy=closed` → `PrivateDevices=yes`, 주소 계열은 부분 집합)를 요구하므로, `maru.service`에 하드닝을 더하면 백엔드 템플릿에도 더한다. 백엔드에는 `maru.service`에 없는 것도 둔다(`PrivateIPC=yes`, 백엔드마다 따로 두는 `/dev/shm`(`TemporaryFileSystem=`. `PrivateIPC`는 POSIX 공유 메모리를 나누지 않고, `PrivateDevices`의 새 `/dev`가 호스트의 `/dev/shm`을 그대로 둘 것으로 본다) 등). 호스트 소켓 디렉터리의 소유는 `TestAddonHostDirOwnedByRoot`가 본다. 템플릿 경로는 `backend/config`의 상수에서 만들고 같은 경로를 다시 적지 않는다
- 새 root 구성 요소지만 켜는 길은 수동 `./install.sh --addon-backends`뿐이고 대시보드에서 업그레이드는 그 값을 그대로 읽어 두므로(꺼진 설치는 꺼진 채) `manualUpgrades` 항목을 두지 않았다. 켠 설치의 업그레이드는 템플릿을 새 버전으로 바꾸고 바뀐 단계의 인스턴스를 다시 시작한다
- `--addon-sync`(root, 다른 옵션 없음, `maru-addons.service`가 복사본으로 실행, `addon_sync.go`): root가 아니면 요청을 건드리지 않고 실패한다. 내부용이라 사용법과 `docs/ko/install-options.md`의 "내부용" 표(`--upgrade-service`와 함께, `TestDocsListEveryOption`)에 있다
  - 순서: 요청을 내용과 상관없이 가장 먼저 지운다(`System.TakeFile`: `O_NOFOLLOW|O_NONBLOCK`, 일반 파일, `config.AddonSyncRequestMaxBytes`. maru는 요청이 남은 시간으로 멈춤을 보고, 잠금을 기다리기 전에 지워야 잠금 실패가 경로 유닛을 곧바로 다시 부르지 않는다). 지우지 못하면(`errRequestKept`) 그렇게 경고하고 동기화는 그대로 한다(경로 유닛이 곧바로 다시 시작하고 maru에는 멈춤으로 보인다). 동기화 디렉터리가 없으면(꺼짐) 결과를 쓰지 않고 0으로 끝낸다. 그다음 잠금(`addonSyncLockWait` 1분까지 기다림. 설치기의 `addonLockWait`는 유닛의 시간 상한보다 길어 쓰지 않는다. 10분 상한 안에 이 1분과 상한 개수를 하나씩 끄는 8분(32 × `TimeoutStopSec` 15초)을 둔다. 그래도 끊기면 기록이 진행을 남겨 다음 동기화가 이어 간다. 기다리는 동안 디렉터리가 지워졌으면 0)
  - 한 차례: 원하는 상태(`<state>/config.AddonBackendsFile`)를 `System.ReadNoFollow`·64KiB로 읽고 `json.Valid`(믿지 않는 입력이라 내용 전체가 JSON 값 하나인지 따로 본다) 다음 `config.ParseAddonBackends`(뒤에 남은 값도 거부한다)로 검사한다. 파일이 없으면 빈 상태(모두 끔), 읽지 못하거나 검사에 실패하거나 기록을 읽지 못하면 아무것도 바꾸지 않고 결과의 `error`(읽은 내용이 있으면 그 digest와 함께)
  - 맞추는 순서: (1) 기록과 `sockets.target.wants`의 링크(`enabledAddonInstances`)를 맞춘다. 기록에 없는 켜진 인스턴스는 끄고(데이터는 기록에 없으므로 그 id를 켤 때 지운다), 링크가 없는 기록 항목(링크만 지우고 유닛은 돌 수 있다)은 멈춘 뒤 데이터를 남긴 것으로 옮겨 다시 켠다. 둘 중 하나라도 멈추지 못하면 그 id를 이번 차례에 막는다(`blocked`: 켜지도 데이터를 지우지도 않고 `failed`, purge는 미룸, 기록은 그대로) (2) 원하는 상태에 없는 켜진 id를 끄고 데이터와 그 인스턴스를 `data`에 남긴다 (3) 원하는 백엔드(id 순): 막힌 id는 실패, 고정 계정 확인, 인스턴스가 다르면 끄기 → `clean` → 켜기, 단계가 다르면 이전 단계 끄기 → 새 단계 켜기, rev만 다르면 소켓 확인 뒤 `try-restart <단계>@<id>.service`, 바뀐 것이 없으면 소켓 확인, 꺼져 있으면 남긴 데이터가 같은 인스턴스일 때만 그대로 켜고 아니면 `clean` 뒤 켠다 (4) purge(backends에 있는 id는 무시, 켜져 있는(끄지 못한) id와 막힌 id는 미룸, 남긴 항목이 없어도 디스크의 데이터를 지움)
  - 소켓 확인(`ensureSocket`): 켜 둔 인스턴스의 소켓을 `is-active --quiet`(`Check`)로 보고, 떠 있지 않으면(계속 죽는 백엔드가 시작 한도에 걸려 소켓까지 실패했거나 누가 멈췄다) `reset-failed <소켓> <서비스>`(실패해도 경고만) 다음 `start <소켓>`. 다시 시작하지 못하면 `failed`, rev가 바뀐 경우는 기록의 rev를 그대로 둔다. 다시 시작했으면 `try-restart`는 하지 않는다(서비스도 멈춰 있었을 것이므로(`Requires=`) 다음 연결 때 새 실행 파일로 시작한다). 서비스는 보지 않는다(연결이 없으면 멈춰 있는 것이 정상이다). 시작 뒤에 죽는 것은 알 수 없으므로 결과는 `enabled`이고, 다음 동기화(관리자의 다시 동기화 포함)가 다시 소켓을 띄운다
  - 동사: 켜기 `enable --now <단계>@<id>.socket`(실패하면 `stop`·`disable`로 되돌린다), 끄기 `stop <소켓> <서비스>` 다음 `disable <소켓>`(설치기의 `stopAndDisable`과 같은 이유로 `disable --now`를 쓰지 않는다), 데이터 지우기 `stop <그 id의 네 단계 소켓·서비스 8개>` 다음 `clean --what=state maru-addon@<id>.service`(StateDirectory는 단계와 상관없이 같은데 `clean`은 이름을 준 유닛이 멈췄는지만 보므로, 기록·링크와 상관없이 그 id의 모든 단계를 먼저 멈춘다. 멈추지 못하면 실패). 데이터가 디스크(`/var/lib/private/maru-addon/<id>`, `/var/lib/maru-addon/<id>`)에 없으면 둘 다 부르지 않고(도는 서비스는 StateDirectory가 있으므로 데이터가 있다), 부른 뒤에도 남아 있으면 실패다(다른 설치의 데이터를 둔 채 켜지 않는다)
  - 고정 계정: 켜기 전에 `maru-a-<id>` 사용자(`System.LookupUser`)·그룹(`System.GroupID`, 모두 NSS)을 찾아 uid·gid가 `config.AddonDynamicUIDMin`~`Max` 밖이면 그 항목을 `failed`로 두고 켜져 있었으면 끈다(같은 이름의 계정이 있으면 systemd가 동적 사용자 대신 그 계정으로 실행한다). `getent`가 성공했는데 줄을 읽지 못하면 두 함수 모두 오류다(계정 없음으로 보지 않는다)
  - 기록은 바꿀 때마다 바로 쓴다(도중에 끊겨도 켠 인스턴스와 남긴 데이터의 인스턴스를 잃지 않게). 쓰지 못하면 그 차례를 전체 실패로 끝낸다. 실패한 동작은 기록에 반영하지 않으므로(켜지 못한 id는 기록에 없음, `try-restart`가 실패하면 이전 rev) 다음 동기화(관리자의 다시 동기화)가 다시 시도한다. 기록의 `backends`가 32개로 차 있으면(끄지 못한 백엔드) 새로 켜지 않는다. `data`의 상한은 위 "동기화 기록"
  - 결과(`config.AddonSyncStatusFile`, 0644): `digest`는 맞춘 파일의 `config.AddonBackendsDigest`(파일이 없거나 읽지 못하면 빈 값), `backends`는 원하는 상태의 항목마다 `{id, rev, unit: <단계>@<id>.service, state, error}`(오류는 한 줄 256바이트까지, 실패한 systemctl 명령과 종료 상태), `purged`는 지운 것을 확인한 id. 전체 실패면 `backends`·`purged`는 비운다(maru는 그 결과의 항목을 보지 않는다). 쓴 뒤 원하는 상태를 다시 읽어 digest가 바뀌었으면 3번(`addonSyncRounds`)까지 다시 맞춘다
  - 작업 실패는 결과에 남기고 0으로 끝낸다. root가 아님, 잠금 실패, 결과 파일을 쓰지 못함만 1이다(`--upgrade-service`와 같다). 끄지 못한 제거·지우지 못한 purge는 결과에 칸이 없어 로그(journal)에만 남고, 기록에 그대로 있어 다음 동기화가 다시 시도한다
- 시험: 설정·파일·명령·기록·잠금은 가짜 루트(`addon_backends_test.go`, 골든 `helper-addon-backends`)로 본다. 동기화는 `addon_sync_test.go`가 가짜 시스템의 `units`(`enable --now`·`disable`이 링크를, `enable --now`·`start`·`stop`이 `is-active` 결과를, `clean`이 데이터를 바꾼다), `fail`(명령별 실패), `cleanKeeps`, `onRun`(실행 도중 원하는 상태를 바꾸거나 잠금을 확인한다), `groupIDs`, `keepRequests`(요청을 지우지 못함)로 본다. 실제 systemd가 정하는 것(`PrivateUsers=yes`에서 maru가 보는 상대 uid, 숨긴 `/run` 아래의 resolv.conf, `IPAddressDeny`, 동적 uid가 패키지마다 다른지, 읽기 전용 tmpfs 안의 바인드 지점, 소켓의 `ExecStartPre`가 서비스의 바인드보다 먼저 도는지, `clean --what=state`가 private 아래의 디렉터리와 바깥 링크를 모두 지우는지, 도는 동안의 동적 사용자가 동기화 유닛 안의 `getent`에 범위 안 uid로 보이는지, 한 번도 올라오지 않은 인스턴스 8개에 `systemctl stop`이 0인지, 계속 죽는 백엔드의 소켓이 실패 상태가 되고 `reset-failed` + `start`로 다시 뜨는지, `PrivateDevices`의 `/dev`가 호스트의 `/dev/shm`을 바인드하고 `TemporaryFileSystem=/dev/shm`이 그 위에 백엔드만의 것을 두는지)은 테스트 VM에서 `probe`와 실제 동기화로 확인한다

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
3. 백엔드 플래그도 생기면 `backend/config`에 키·기본값을 두고 `cmd/maru/flags.go`의 플래그(`parseFlags`)와 `runtimeEntry` 목록(`options.runtime`)에서 쓴다
4. `docs/ko/install-options.md`의 옵션 표(`TestDocsListEveryOption`), 백엔드 플래그면 `docs/ko/configuration.md`의 항목 표(두 문서 모두 영어 `docs/en/`도 함께), `setting_test.go`의 검사 표와 왕복 표에 더한다
5. 환경 파일 출력이 바뀌면 골든(`testdata/golden/*/want`)도 고친다. 골든은 bash `install.sh`의 출력에서 시작했으므로 바꾼 이유를 커밋 메시지에 적는다

## 단계를 더할 때

- `step_<이름>.go`에 `func(c *Context) error`를 만들고 `steps.go`의 `installSteps` 또는 `uninstallSteps`에 `Phase`·`When`과 함께 한 줄 더한다
- 시스템을 바꾸는 일은 `c.Sys`로만 한다. 파일은 `WriteFile`·`InstallFile`·`Remove`, 명령은 `Run`(출력을 버리면 `RunQuiet`·`RunSilent`), 읽기 전용 확인은 `Check`. 다른 계정(maru)이 쓸 수 있는 파일은 `ReadNoFollow`로 읽고, 요청 파일은 `TakeFile`로 읽고 지운다. 그래야 `--dry-run`과 가짜 루트 테스트가 그대로 동작한다
- 설정 파일은 `c.writeConfig`로 쓴다. 질문은 `c.confirm`(dry-run은 예로 보고 진행, 비TTY는 오류)이나 `c.P`로 한다

## 불변 조건

- 생성하는 파일(유닛 파일 15개, `/etc/default/maru`, `/etc/default/maru-helper`, `/etc/default/maru-upgrade`, 빈 애드온 동기화 결과. 목록은 `fake_test.go`의 `configFiles`)은 골든과 바이트 단위로 같고 명령 순서(`commands`)도 같아야 한다(`TestGoldenServiceInstall`). 골든은 bash `install.sh`의 출력에서 시작했고, bash에 없던 설정(대시보드에서 업그레이드, 애드온 백엔드)은 Go 설치기 출력으로 만든 케이스로 본다(`testdata/golden/README`). 생성 파일의 주석은 영어로 쓰고 진입점을 `install.sh`라고 부른다
- 의도한 차이는 레지스트리가 관리하지 않는 기존 `MARU_*` 줄을 파일 끝 "Kept from the previous file" 주석 아래에 보존하는 것뿐이다. 그런 줄이 없으면 주석도 쓰지 않는다
- 설치 기록은 설치 시각이 들어가므로 골든 대상이 아니다(`record_test.go`가 본다). 파일 쓰기는 명령 기록에 남지 않으므로 골든의 `commands`도 그대로다
- bash `install.sh`가 만든 설치(같은 경로·환경 변수 키·유닛의 `User=`)를 업그레이드(옵션 없음 또는 `--upgrade`)로 읽는다. 설정이 모두 있는 설치의 업그레이드는 설정 질문이 없고(옵션 없는 터미널 실행은 요약 뒤 선택 하나), 설정을 모두 준 `--service-install`(`docs/ko/install.md`의 "자동화" 예시, 테스트 VM의 설치 시험)은 설치 여부와 상관없이 질문 없이 끝나야 한다
- 비TTY에서 설정 질문이 필요하면(빠진 설정, 검사를 통과하지 못한 기존 값) 멈추지 않고 그 옵션 목록을 알린 뒤 2로 끝낸다. 확인 질문(진행할까요, 그래도 계속할까요)도 비TTY에서는 묻지 않고 오류로 끝낸다. 업그레이드(옵션 없는 비TTY 실행, `--upgrade`), 설정을 모두 준 `--service-install`, `--dry-run`은 진행 확인을 하지 않는다
- 종료 코드: 잘못된 옵션(함께 쓸 수 없는 방식·동작 옵션 포함)·비TTY의 빠진 설정 2, 오류(`--upgrade`인데 설치되어 있지 않음 포함) 1, 계획을 낸 `--upgrade-plan` 0, "진행할까요?"·제거 확인에서 아니요와 업그레이드 "취소" 0, "그래도 계속할까요?"·기존 실행 파일로 설치할지 확인에서 아니요 1, 중단(Ctrl+C) 130
- 유닛 하드닝을 바꾸면 `.claude/docs/helper.md`, `.claude/rules/helper.md`도 함께 보고, 사용자에게 보이는 영향(쓸 수 있는 경로 등)이 바뀌면 `docs/ko/helper.md`도 고친다. 애드온 백엔드 템플릿은 `.claude/docs/addon-packages.md`의 "백엔드 프로세스"와 함께 고친다
- maru 유닛의 `RuntimeDirectoryPreserve=yes`는 `/run/maru`를 재시작 사이에도 남긴다. 디렉터리가 새로 만들어지면 컨테이너 프록시의 바인드 마운트가 이전 디렉터리를 계속 가리켜 연결이 끊기기 때문이다. 제거(`--uninstall`)는 지우므로 다시 설치한 뒤 프록시 컨테이너를 다시 만들어야 한다(`docs/ko/install.md`의 "제거")
- 생성하는 `/etc/default/maru`의 머리 주석 `# See README.md for every option.`(`setting.go`의 `MaruEnv`)은 README가 설정 문서(`docs/en/configuration.md`)로 이어 주므로 일부러 그대로 둔다. 바꾸면 골든(bash 출력과 같은 케이스 포함)이 바뀌고 기존 설치마다 업그레이드 때 `.bak`이 한 번 생긴다. 다음에 `MaruEnv`의 내용이 어차피 바뀔 때 `docs/en/configuration.md`로 바꾼다
- 테스트는 root·sudo·실제 systemctl을 쓰지 않는다(가짜 루트와 명령 기록). 실제 설치는 사용자가 하거나 테스트 VM에서 확인한다
- `.claude/docs/roadmap.md`의 항목(도우미 추가 기준 경로 질문 등)은 요청하기 전에 구현하지 않는다. 설정 하나와 템플릿 한 줄로 더할 수 있는 구조만 유지한다
- 새 의존성을 더하지 않는다. TUI는 huh v2(함께 오는 Bubble Tea·Lip Gloss 포함)만 쓴다
