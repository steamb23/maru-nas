# 마루 NAS 문서

English: [Maru NAS documentation](README.md)

마루 NAS는 OpenMediaVault 8 위에 올리는 데스크톱형 웹 대시보드입니다. 처음 설치한다면 저장소 README의 [빠른 시작](https://github.com/steamb23/maru-nas/blob/main/README.ko.md#빠른-시작)부터 따라 해 주세요. 이 문서는 설치한 뒤 운영하면서 찾아보는 내용을 담습니다.

## 설치와 운영

| 문서 | 내용 |
|---|---|
| [설치](install.ko.md) | 요구 사항과 OMV 버전별 차이, 빌드 도구, 설치 스크립트의 흐름과 설치되는 파일, 다시 설치, 자동화, 제거 |
| [설치 옵션](install-options.ko.md) | `install.sh`의 모든 옵션 |
| [업그레이드](upgrade.ko.md) | SSH로 업그레이드, 대시보드에서 업그레이드 |
| [리버스 프록시](reverse-proxy.ko.md) | 수신 방식, 프록시 요구 사항, Caddy 설치와 예시 |
| [설정](configuration.ko.md) | `/etc/default/maru`의 모든 항목, 웹에서 바꾸는 값, 백업, OMV 워크벤치 열기 |

## 기능

| 문서 | 내용 |
|---|---|
| [설정 앱](settings-app.ko.md) | 설정 앱의 페이지와 권한 |
| [파일 탐색기](files.ko.md) | 홈 폴더와 공유 폴더 둘러보기, 올리기·내려받기, 휴지통, 옮기기와 복사 |
| [웹 터미널](terminal.ko.md) | 대시보드 창 안의 SSH 셸 |

## 보안

| 문서 | 내용 |
|---|---|
| [권한 도우미](helper.ko.md) | 사용자 권한·root 권한이 필요한 일을 맡는 서비스와 그 보안 설계 |
| [보안 강화 모드](security.ko.md) | 민감한 기능 앞의 비밀번호 재확인과 2차 인증(패스키, 인증 앱, 복구 코드) |
| [패스키로 로그인](passkey-login.ko.md) | 비밀번호 없이 패스키로 로그인하기와 그 한계 |

이 문서는 저장소의 `docs/`에서 GitHub로 그대로 읽을 수 있고, [Zensical](https://zensical.org/)로 만든 [문서 사이트](https://steamb23.github.io/maru-nas/)로도 볼 수 있습니다. 사이트는 최신 릴리스(`stable`) 기준이고, 개발 중인 내용은 [`main`의 docs/](https://github.com/steamb23/maru-nas/tree/main/docs)에 먼저 반영됩니다. 고치는 방법은 [CONTRIBUTING.ko.md](https://github.com/steamb23/maru-nas/blob/main/CONTRIBUTING.ko.md)의 "문서"에 있습니다.
