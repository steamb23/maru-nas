# <img src="frontend/src/assets/logo/maru-lockup.svg" alt="마루 NAS (Maru NAS)" height="80">

> A suspiciously normal desktop-like dashboard for openmediavault.

[![CI](https://github.com/steamb23/maru-nas/actions/workflows/ci.yml/badge.svg)](https://github.com/steamb23/maru-nas/actions/workflows/ci.yml)
[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue)](LICENSE)

English: [README.md](README.md)

OpenMediaVault 8 위에 올리는 데스크톱형 웹 대시보드입니다. 창·작업표시줄·바탕화면 위젯으로 NAS 가동 현황을 확인하고, 컨테이너와 핵심 서비스를 간단히 켜고 끌 수 있습니다. 심층 설정은 OMV 기본 웹 UI에서 합니다.

- 로그인은 OMV 계정으로 합니다. 설정 변경도 로그인한 사용자의 OMV 세션으로만 합니다.
- Go 실행 파일 하나(프론트엔드 포함)를 systemd 서비스로 실행하고, 앞단의 리버스 프록시가 HTTPS를 맡습니다.

> [!WARNING]
> 로컬 네트워크 전용입니다. 인터넷에 노출하는 구성을 전제로 하지 않으며, 인터넷에 노출되었을 때 어떤 보안 취약점이 드러날지 충분히 검증되지 않았습니다.

## 주요 기능

- **현황 창과 위젯**: 시스템 정보, 저장소, 네트워크, 온도, 디스크 건강(SMART)을 창이나 바탕화면 위젯으로 봅니다.
- **간단한 조작**: 컨테이너(compose 플러그인)를 시작·중지·재시작하고, 핵심 서비스를 켜고 끕니다. 관리자는 프로필 메뉴에서 NAS를 종료하거나 다시 시작하거나 절전 상태로 둡니다.
- **[파일 탐색기](docs/ko/files.md)**: 홈 폴더와 공유 폴더를 사용자마다의 권한으로 둘러보고, 올리고, 내려받고, 휴지통으로 지웁니다.
- **[웹 터미널](docs/ko/terminal.md)**: 로그인한 계정으로 이 NAS의 SSH 셸을 창 안에 엽니다.
- **[시스템 업데이트](docs/ko/system-updates.md)**: OMV·Debian 패키지 업데이트를 확인하고 모두 설치합니다. 창을 닫아도 서버에서 끝까지 진행되고 결과를 알림으로 알려 줍니다.
- **[보안 강화 모드](docs/ko/security.md)**: 민감한 기능 앞에서 비밀번호 재확인이나 패스키·인증 앱으로 한 번 더 확인합니다. [패스키로 로그인](docs/ko/passkey-login.md)도 됩니다.
- **[대시보드에서 업그레이드](docs/ko/upgrade.md#대시보드에서-업그레이드)**: SSH 없이 설정 앱에서 새 버전으로 올립니다.
- **[OMV 워크벤치 연결](docs/ko/configuration.md#omv-워크벤치-열기)**: 심층 설정이 필요하면 OMV 웹 UI를 새 탭으로 엽니다.

## 빠른 시작

막 설치한 OMV 8에 SSH로 접속한 상태에서 시작합니다. 빌드 도구와 리버스 프록시가 하나도 없어도 아래를 차례로 따라 하면 브라우저에서 로그인할 수 있습니다.

필요한 것:

- OpenMediaVault 8.0 이상([버전별 차이](docs/ko/install.md#omv-버전))
- SSH로 접속한 일반 사용자(sudo 그룹). OMV 설치 이미지가 만든 사용자가 그렇습니다([계정과 SSH](docs/ko/install.md#계정과-ssh))
- 빌드하는 동안 인터넷 접속(github.com, registry.npmjs.org, proxy.golang.org)과 약 1.1GB의 여유 공간

### 1. 빌드 도구 설치

```sh
sudo apt update
sudo apt install git make nodejs npm
sudo apt install -t trixie-backports golang-go
```

마지막 줄은 OMV가 기본으로 켜 둔 Debian backports에서 Go 1.27을 받습니다. `go version`이 1.27 이상, `node --version`이 20.19 이상이면 됩니다. 기본 저장소의 Go 1.24를 쓰는 경우는 [빌드 도구](docs/ko/install.md#빌드-도구)를 봐 주세요.

### 2. 받아서 설치

```sh
git clone -b stable https://github.com/steamb23/maru-nas.git
cd maru-nas
./install.sh
```

**sudo 없이** 실행합니다. 설정을 묻고 요약을 확인받은 뒤 빌드하며, 설치 단계에서 sudo 비밀번호를 한 번 묻습니다. 질문은 대부분 기본값 그대로 두고, 아래 두 가지만 3단계의 프록시 예시에 맞춰 답합니다.

| 질문 | 답 |
|---|---|
| 브라우저에서 OMV 워크벤치를 열 주소 | `/` |
| 패스키에 쓸 대시보드 공개 주소 | `-`(패스키를 끕니다. 브라우저가 믿는 HTTPS 도메인이 생기면 `./install.sh --reinstall`로 바꿉니다) |

### 3. 리버스 프록시(Caddy) 설치

대시보드는 이 NAS 안(`127.0.0.1:8090`)에서만 받으므로, 브라우저가 접속할 HTTPS는 리버스 프록시가 맡습니다. Debian 저장소의 Caddy를 설치합니다.

```sh
sudo apt install caddy
```

OMV 웹 UI가 80번 포트를 쓰고 있어서 설치 중에 `caddy.service`를 시작하지 못했다는 메시지가 나올 수 있습니다. 아래처럼 설정을 바꾸면 됩니다.

`/etc/caddy/Caddyfile`의 내용을 다음으로 바꿉니다(예: `sudo nano /etc/caddy/Caddyfile`). `nas.example.lan`은 지금 브라우저에서 OMV에 접속하는 주소(호스트 이름이나 IP 주소)로 바꿔 주세요.

```caddyfile
{
	auto_https disable_redirects
}

nas.example.lan {
	tls internal
	redir /desk /desk/ 308
	handle_path /desk/* {
		reverse_proxy 127.0.0.1:8090
	}
	handle {
		reverse_proxy 127.0.0.1:80
	}
}
```

```sh
sudo systemctl restart caddy
```

Caddy는 443번 포트만 쓰고 OMV 설정은 바꾸지 않습니다. `https://<주소>/desk/`는 대시보드로, 그 밖의 `https://<주소>/`는 OMV 웹 UI로 연결되고, 지금 쓰는 `http://<주소>/`의 OMV 웹 UI도 그대로입니다. OMV 웹 UI 설정의 SSL/TLS는 꺼 둔 상태(기본값)여야 합니다.

### 4. 접속

브라우저에서 `https://<주소>/desk/`를 열고 OMV 계정으로 로그인합니다.

- `tls internal`은 Caddy가 직접 만든 인증서라 처음에는 브라우저가 경고합니다. 경고를 없애거나 패스키를 쓰려면 Caddy의 루트 인증서를 기기에 등록합니다([리버스 프록시](docs/ko/reverse-proxy.md#caddy-예시-omv-기본-포트를-유지하는-경우)).
- 서브도메인, Docker의 Caddy, 다른 프록시로 구성하는 방법도 [리버스 프록시](docs/ko/reverse-proxy.md)에 있습니다.

### 업그레이드와 제거

저장소에서 실행하면 설치된 설정으로 업그레이드합니다. 관리자는 설정 앱의 "업데이트" 페이지에서 대시보드로 업그레이드할 수도 있습니다([업그레이드](docs/ko/upgrade.md)).

```sh
git pull
./install.sh
```

제거는 `./install.sh --uninstall`입니다.

## 문서

설치한 뒤 운영하면서 찾아보는 내용은 [문서 사이트](https://steamb23.github.io/maru-nas/ko/)에 있습니다. 사이트는 보통 최신 릴리스(`stable`) 기준이지만, 릴리스 사이에는 개발 중인 `main`의 문서가 먼저 올라가 있을 수 있습니다. 같은 문서를 저장소의 [docs/ko/](docs/ko/README.md)에서 GitHub로도 읽을 수 있습니다.

| 문서 | 내용 |
|---|---|
| [설치](docs/ko/install.md) | 요구 사항과 OMV 버전별 차이, 빌드 도구, 설치 스크립트, 다시 설치, 자동화, 제거 |
| [설치 옵션](docs/ko/install-options.md) | `install.sh`의 모든 옵션 |
| [업그레이드](docs/ko/upgrade.md) | SSH로 업그레이드, 대시보드에서 업그레이드 |
| [리버스 프록시](docs/ko/reverse-proxy.md) | 수신 방식, 프록시 요구 사항, Caddy 설치와 예시 |
| [설정](docs/ko/configuration.md) | `/etc/default/maru`의 모든 항목, 웹에서 바꾸는 값, 백업, OMV 워크벤치 열기 |
| [설정 앱](docs/ko/settings-app.md) | 설정 앱의 페이지와 권한 |
| [파일 탐색기](docs/ko/files.md) | 보이는 폴더와 권한, 올리기·내려받기, 휴지통, 옮기기와 복사 |
| [웹 터미널](docs/ko/terminal.md) | 켜기, 인증, SSH 설정과 보안 |
| [시스템 업데이트](docs/ko/system-updates.md) | 업데이트 확인과 모두 설치, 작업 진행과 결과 알림, 한계 |
| [권한 도우미](docs/ko/helper.md) | 사용자 권한·root 권한이 필요한 일을 맡는 서비스와 그 보안 설계 |
| [보안 강화 모드](docs/ko/security.md) | 비밀번호 재확인과 2차 인증(패스키, 인증 앱, 복구 코드), 복구 |
| [패스키로 로그인](docs/ko/passkey-login.md) | 비밀번호 없이 패스키로 로그인하기와 그 한계 |

## 기여하기

기여 방법(개발 환경, 브랜치와 풀 리퀘스트, 번역, 문서)은 [CONTRIBUTING.md](CONTRIBUTING.md)(영어)와 [CONTRIBUTING.ko.md](CONTRIBUTING.ko.md)에 있습니다. 새 기능을 시작하기 전에 이슈를 열어 설계를 먼저 맞춰 주세요.

개발 서버는 실제 OMV 없이 가짜 OMV에 연결해 띄울 수 있습니다. 필요한 도구는 빠른 시작의 1단계와 같습니다. NAS가 아닌 PC라면 [go.dev/dl](https://go.dev/dl/)과 [nodejs.org](https://nodejs.org/)의 배포본을 쓰고, 테스트(`make check`)에 쓰는 Vitest 5는 Node.js 22.12 이상을 공식 지원합니다.

```sh
git clone https://github.com/steamb23/maru-nas.git
cd maru-nas
git submodule update --init --depth 1
npm --prefix frontend ci
make dev-fake
```

`http://<호스트>:5173/`에 접속해 가짜 계정 `admin`(비밀번호 `admin`)으로 로그인합니다. 다른 가짜 계정과 자주 쓰는 명령은 CONTRIBUTING에 있습니다.

- 풀 리퀘스트는 `main`을 대상으로 올립니다. `main`은 개발 중인 코드이고 `stable`은 최신 릴리스를 가리킵니다.
- 문서는 `docs/`를 고치고 [Zensical](https://zensical.org/)로 미리 볼 수 있습니다(CONTRIBUTING.ko.md의 "문서").

## 라이선스

GNU General Public License v3.0으로 배포됩니다([LICENSE](LICENSE)). 함께 포함된 아이콘 등 제3자 에셋의 출처와 라이선스는 [vendor/icons/README.md](vendor/icons/README.md)에 있습니다.
