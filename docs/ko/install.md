# 설치

처음이라면 README의 [빠른 시작](https://github.com/steamb23/maru-nas/blob/main/README.ko.md#빠른-시작)을 따라 하면 됩니다. 이 문서는 그 단계마다 더 알아 둘 것을 모았습니다. 옵션 목록은 [설치 옵션](install-options.md), 설치한 뒤 바꾸는 값은 [설정](configuration.md)에 있습니다.

## 요구 사항

### OMV 버전

OpenMediaVault 8의 모든 버전(8.0 이상)에서 동작합니다. 버전에 따라 다음이 다릅니다.

- OMV의 2단계 인증은 OMV 8.5 이상에서만 쓸 수 있습니다.
- OMV 8.0·8.1은 자동 로그아웃 시간을 알려 주지 않습니다. 그래서 대시보드에 세션의 남은 시간이 보이지 않고, OMV 세션이 끝난 것은 대시보드가 주기적으로 확인할 때 알게 됩니다.

지금 버전은 `dpkg-query -W openmediavault`로 봅니다. 설치 이미지(ISO)로 막 설치했다면 이미지의 버전 그대로이므로, OMV 웹 UI의 시스템 → 업데이트 관리나 `sudo omv-upgrade`로 업데이트해 두기를 권장합니다.

### 계정과 SSH

설치는 sudo를 쓸 수 있는 일반 사용자 계정으로 합니다. OMV 설치 이미지가 만든 일반 사용자는 sudo 그룹에 들어 있습니다.

- OMV는 root와 `_ssh` 그룹 사용자에게만 SSH 접속을 허용합니다. 그 계정으로 SSH에 접속하려면 OMV 웹 UI의 사용자 편집 화면에서 `_ssh` 그룹을 더해 주세요.
- root 계정만 있는 환경이라면 root로 실행해도 됩니다(sudo 없이 이어서 설치합니다). 다만 root가 소유한 저장소로는 [대시보드에서 업그레이드](upgrade.md#대시보드에서-업그레이드)를 켤 수 없습니다.

### 빌드 도구

OMV 설치 이미지로 설치한 직후에는 아래 도구가 하나도 없지만, 모두 OMV 8(Debian 13)이 기본으로 쓰는 apt 저장소에 있습니다.

| 도구 | 필요한 버전 | apt 패키지(버전) |
|---|---|---|
| Go | 1.27.1 이상 | `golang-go`(trixie-backports 1.27, 기본 저장소 1.24) |
| Node.js | 20.19 이상 또는 22.12 이상 | `nodejs`(20.19) |
| npm | 7 이상 | `npm`(9.2). Debian에서는 `nodejs`와 따로 설치합니다 |
| make | GNU make | `make`(4.4) |
| git | | `git`(2.47) |

```sh
sudo apt update
sudo apt install git make nodejs npm
sudo apt install -t trixie-backports golang-go
```

- 마지막 줄은 OMV가 기본으로 켜 두는 Debian trixie-backports 저장소에서 Go 1.27을 설치합니다.
- `-t trixie-backports` 없이 설치하면 Go 1.24가 설치됩니다. 이때는 첫 빌드에서 Go가 1.27.1 툴체인을 proxy.golang.org에서 자동으로 받으며 `~/go/pkg/mod`에 약 330MB가 더 생깁니다. `GOTOOLCHAIN=local`로 설정했거나 proxy.golang.org에 접속할 수 없다면 backports나 [go.dev/dl](https://go.dev/dl/)의 배포본을 설치해 주세요.
- Node.js 20에서는 `npm ci`가 Vitest의 `EBADENGINE` 경고를 보입니다. 개발용 테스트 도구에 대한 경고라 빌드에는 영향이 없습니다.
- 빌드하는 동안 github.com(아이콘 서브모듈), registry.npmjs.org(npm 의존성), proxy.golang.org(Go 의존성)에 접속합니다. 도구 설치와 빌드에 약 1.1GB의 여유 공간이 필요합니다(기본 저장소의 Go 1.24라면 약 1.4GB).

## 소스 받기

`stable` 브랜치가 최신 릴리스이고 `main`은 개발 중인 코드이므로 설치에는 `stable`을 받습니다. 아이콘 서브모듈은 설치 스크립트가 받으므로 `--recurse-submodules`는 필요 없습니다.

```sh
git clone -b stable https://github.com/steamb23/maru-nas.git
cd maru-nas
```

특정 버전을 설치하려면 클론한 뒤 그 버전의 태그를 체크아웃합니다(예: `git checkout v0.10.0`). 릴리스 목록과 변경 내용은 [Releases](https://github.com/steamb23/maru-nas/releases)에 있습니다.

## 설치 스크립트

일반 사용자 계정이라면 **sudo 없이** 실행합니다.

```sh
./install.sh
```

1. **설정 입력**: 서비스 사용자, 수신 방식, OMV 주소, SMART 갱신 시각, OMV 워크벤치 주소, 패스키에 쓸 공개 주소, 권한 도우미 설치 여부, 대시보드에서 업그레이드 허용 여부를 묻고 요약을 보여 준 뒤 확인을 받습니다. 입력은 이 단계에서 모두 끝납니다.
2. **빌드**: 실행한 계정으로 빌드합니다. 처음 클론한 상태라면 아이콘 서브모듈과 npm 의존성부터 받습니다.
3. **설치**: sudo 비밀번호를 한 번 묻고 root로 설치합니다. root 계정이라면 묻지 않고 이어서 진행합니다.

이미 설치되어 있으면 설정을 묻지 않고 설치된 설정으로 업그레이드합니다([업그레이드](upgrade.md)).

- 터미널에서 실행하면 화살표 키로 고르고 값을 입력하는 화면(TUI)으로 묻고, 잘못된 값은 입력하는 동안 바로 알려 줍니다. `--plain`을 주거나 `TERM=dumb`이면 한 줄씩 묻습니다.
- 표준 입력이 터미널이 아니면(파이프, 자동화 등) 묻지 않고, 빠진 설정 옵션의 목록을 알린 뒤 멈춥니다([자동화](#자동화)).
- 안내 문구는 로캘에 따라 영어나 한국어로 나오며, `--lang`으로 고를 수도 있습니다.
- 끝나면 리버스 프록시에 넣을 주소를 알려 줍니다. 리버스 프록시는 설치 스크립트가 설정하지 않습니다([리버스 프록시](reverse-proxy.md)).

### 설치되는 것

| 경로 | 내용 |
|---|---|
| `/usr/local/bin/maru` | 대시보드 실행 파일. 서비스 사용자(기본값 `maru`, 없으면 시스템 사용자로 만듦)로 실행합니다 |
| `/etc/default/maru`, `/etc/systemd/system/maru.service` | [설정](configuration.md) 파일과 systemd 유닛 |
| `/var/lib/maru` | 대시보드 자신의 상태: 사용자 설정(위젯 배치, 바로가기, 화면 설정), 관리자의 전역 설정, 올린 배경 이미지, 보안 강화 모드의 인증 수단 |
| `/usr/local/bin/maru-helper`, `/etc/default/maru-helper`, `maru-helper.socket`·`.service` | [권한 도우미](helper.md)(기본으로 설치) |
| `/usr/local/lib/maru/maru-install`, `/etc/default/maru-upgrade`, `maru-upgrade.service`·`.path` | [대시보드에서 업그레이드](upgrade.md#대시보드에서-업그레이드)를 허용했을 때 |
| `/var/lib/maru-upgrade` | 설치한 버전·커밋 기록과 업그레이드 상태 |

권한 도우미나 대시보드에서 업그레이드를 설치하지 않기로 하면 이전에 설치한 것을 지웁니다.

### 설정 바꾸기

```sh
./install.sh --reinstall
```

지금 설정을 기본값으로 보여 주며 모든 설정을 다시 묻습니다.

- 설정이 바뀌면 기존 `/etc/default/maru`는 `/etc/default/maru.bak-<시각>`으로 백업됩니다.
- 설치기가 묻지 않는 줄은 남깁니다. 웹 터미널 줄(`MARU_TERMINAL` 등)은 기존 값을 옮기고, 손으로 넣은 다른 `MARU_*` 줄(예: `MARU_COOKIE_PATH`)은 파일 끝으로 옮깁니다.

### 자동화

설정 옵션으로 준 항목은 묻지 않습니다. 그래도 처음 설치와 `--reinstall`은 요약을 보여 주고 진행 여부를 확인하므로, 표준 입력이 터미널이 아니면 이 확인에서 멈춥니다. 확인 없이 끝나는 것은 업그레이드(`--upgrade`, 또는 옵션 없이 비TTY로 실행)와 root로 `--service-install`에 설정을 모두 준 경우입니다.

처음 설치를 자동화할 때는 `make build`로 빌드해 두고 root로 다음처럼 실행합니다. 같은 옵션에 `--dry-run`을 더하면 무엇이 바뀌는지 먼저 볼 수 있습니다.

```sh
./install.sh --service-install --user maru --listen unix:/run/maru/maru.sock \
  --omv-url http://127.0.0.1 --smart-refresh-at 00:00 --workbench-url https://nas.example.lan/ \
  --public-origin '' --no-helper
```

### root로 실행할 때

root로 실행했는데(`sudo ./install.sh`, `sudo -i`, `su` 등) 저장소의 소유자가 root가 아니면 빌드를 건너뜁니다. root로 빌드하면 저장소에 root 소유 파일이 생기고, sudo 환경의 PATH에는 사용자가 설치한 Go·npm이 없을 수 있기 때문입니다.

- 이때는 이미 있는 `bin/maru-install`을 쓰고, 기존 `bin/maru`로 설치만 할지 묻습니다. `--upgrade`이거나 표준 입력이 터미널이 아닌 업그레이드라면 묻지 않고 멈춥니다.
- PATH에 `go`가 없을 때도 이미 있는 `bin/maru-install`을 씁니다.

## 제거

```sh
./install.sh --uninstall
```

- 서비스, 실행 파일, 설정을 지웁니다. 권한 도우미와 대시보드 업그레이드 서비스를 설치했다면 함께 지우고, 설치 기록과 업그레이드 상태(`/var/lib/maru-upgrade`)도 지웁니다.
- `/var/lib/maru`와 서비스 사용자는 확인을 받은 뒤에만 지웁니다.
- `/run/maru`도 지워지므로, 컨테이너 프록시에 이 디렉터리를 마운트해 두었다면 다시 설치한 뒤 프록시 컨테이너를 다시 만들어야 합니다.
