# 데이터 출처와 OMV 플러그인 의존성

3단계 애드온(창·위젯)의 각 항목을 어디서 가져올지 정리한 문서다. 애드온 구조는 [addons.md](addons.md)를 본다.

- 조사 기준일: 2026-10-14, OMV 8.5.11 ("synchrony")
- 설치된 플러그인의 RPC 이름·역할은 NAS의 `/usr/share/openmediavault/engined/rpc/*.inc`에서 확인했다
- 설치되지 않은 플러그인은 각 저장소 HEAD의 `engined/rpc/*.inc`와 synchrony apt 인덱스에서 확인했다. 실제 배포 버전과 메서드가 조금 다를 수 있으므로, 구현 전에 설치 후 소스를 다시 확인한다
- 역할 표기: **모두**는 `OMV_ROLE_EVERYONE`(로그인한 모든 사용자), **admin**은 `OMV_ROLE_ADMINISTRATOR`

## 원칙

- 권한이 필요 없는 직접 읽기(`/proc`, `/sys`)를 1차 출처로 쓴다. 플러그인과 역할에 상관없이 항상 동작한다
- root가 필요한 정보는 OMV RPC로만 가져온다. 서비스 계정에 권한을 더 주지 않는다
- 여러 출처를 합칠 때는 응답에 출처를 함께 담고, 같은 센서가 중복되지 않게 한다(예: hwmon의 `coretemp`와 cputemp 플러그인의 CPU 온도)

## 항목별 출처

| 항목 | 직접 읽기 (1차) | 코어 RPC | 플러그인 RPC |
|---|---|---|---|
| 디스크 용량 | `/proc/mounts` + `statfs` | `FileSystemMgmt.enumerateMountedFilesystems` (admin): 라벨, 장치, OMV가 관리하는 파일시스템 여부 | zfs, mergerfs, md 등 (아래 표) |
| SMART | 불가 (`smartctl`은 root 필요) | `Smart.getList` (admin): 디스크마다 `overallstatus`, `temperature` 포함. 세부 항목은 `Smart.getAttributes`, `Smart.getInformation` (admin) | - |
| 온도 | `/sys/class/hwmon/*/temp*_input` (CPU `coretemp`·`k10temp`, NVMe, `acpitz` 등) | SATA 디스크 온도는 `Smart.getList` (admin)로만 얻는다. `drivetemp` 커널 모듈이 로드되어 있으면 hwmon으로도 보인다 | `CpuTemp.get`/`get2`~`get4`/`getSensors` (모두), `TempMon.getAll` (모두) |
| 네트워크 | `/proc/net/dev` (누적 바이트·패킷). 속도는 프론트가 두 응답의 차이로 계산한다(백엔드는 단조 시각 `sampledAtMs`를 함께 준다). 링크 상태·속도는 `/sys/class/net/<if>/operstate`, `speed` | `Network.enumerateDevicesList` (admin): 주소, 링크, 속도, 누적 카운터 | - |
| 컨테이너 | 하지 않음 (소켓 접근은 root와 같은 권한) | 없음 | `Compose.getContainers` (모두), 상세는 admin 전용 메서드 (아래 "컨테이너 런타임") |

### 주의할 응답 형태

- `Compose.getContainers`의 첫 행은 컨테이너가 아니다. `name`이 `Docker` 또는 `Podman`이고 `status`는 런타임 상태(`running`/`stopped`/`not installed`)를 담은 가상 행이다. 그 뒤 행이 `docker container ls --all` 결과(`image`, `name`, `status`)다
- 가상 행의 `status`는 `gettext`로 번역되므로 OMV 언어 설정에 따라 문자열이 바뀐다. 이 값을 문자열 비교로 판단하지 않는다. 런타임 종류는 번역되지 않는 `name`(`Docker`/`Podman`)으로 판단한다
- `Smart.getList`는 디스크마다 `smartctl`을 실행하므로 느리다. OMV workbench는 백그라운드 버전(`getListBg`)을 쓴다. 짧은 주기로 폴링하지 않는다
- `Smart.getList`는 `rpc.common.getlist` 형식의 파라미터(`start`, `limit` 등)를 받는다
- `CpuTemp.get`은 플러그인 설정에 등록된 스크립트 결과를 그대로 돌려준다. 센서 이름과 단위는 사용자 설정에 따라 달라진다
- `Network.enumerateDevicesList`의 통계는 누적값이다. 속도를 계산하려면 직접 읽기와 마찬가지로 차이를 구해야 하므로, 속도에는 `/proc/net/dev`를 쓴다
- 그래프 플러그인(diskstats, 코어 perfstats/collectd, cputemp·tempmon의 RRD 페이지)은 admin 전용 `Rrd.getGraph`로 PNG만 준다. 수치가 없으므로 데이터 출처로 쓰지 않는다

### 3-3단계에서 쓴 출처

- 디스크 용량: `/proc/mounts` + statfs + `/dev/disk/by-label`(직접 읽기만). `FileSystemMgmt.enumerateMountedFilesystems`(admin)는 쓰지 않았다. 라벨은 by-label로 충분하고, 일반 사용자에게도 같은 화면을 보여 주기 위해서다. OMV가 관리하는 파일시스템 표시가 필요해지면 관리자용 보강 provider 후보다
- 네트워크: 직접 읽기만. `Network.enumerateDevicesList`(admin)는 주소·설정 표시가 필요해지면 관리자용 보강 provider 후보다
- 온도: hwmon, `CpuTemp.get`(hwmon에 CPU 센서가 없을 때만), SMART 캐시의 디스크 온도(관리자)
- SMART: `Smart.getList`(admin)를 하루 한 번·새로고침 때만. 백그라운드 버전 `getListBg`는 쓰지 않았다(백엔드 캐시가 같은 역할)
- 컨테이너: `Compose.getContainers`(모두)

## 호환 후보 플러그인

"설치"는 이 NAS에 설치되어 있는지를 뜻한다. 3단계는 설치된 플러그인을 먼저 구현한다.

| 패키지 | 저장소 | OMV 8 버전 | 설치 | RPC → 쓸 만한 메서드 (역할) | 항목 |
|---|---|---|---|---|---|
| openmediavault (코어) | 공식 | 8.5.11 | O | `FileSystemMgmt`, `Smart`, `Network` (위 표, 모두 admin) | 용량, SMART, 온도, 네트워크 |
| openmediavault-compose | omv-extras | 8.2.2 | O | `Compose.getContainers`/`getContainersTerm` (모두), `getContainerList`/`getServicesList`/`getStats`/`getImages`/`getVolumes`/`getNetworks` (admin) | 컨테이너 |
| openmediavault-cputemp | omv-extras | 8.2.2 | O | `CpuTemp.get`/`get2`~`get4`/`getSensors` (모두, lm-sensors 사용) | 온도 |
| openmediavault-mergerfs | omv-extras | 8.0.3 | O | `Mergerfs.getList`/`getPoolPaths` (admin) | 용량 |
| openmediavault-bcache | 공식 | 8.0.2 | O | `bcache.getCacheStatistic`/`getBackingStatistic` (admin) | 용량 |
| openmediavault-kvm | omv-extras | 8.3.4 | O | `Kvm.getVmNameStateList`/`getLxcNameStateList`/`getVirtTop` (admin) | 가상 머신·LXC (목록 밖, 후보) |
| openmediavault-tempmon | omv-extras (OMV 8 신규) | 8.0.4 | X | `TempMon.getAll` (모두): 사용자 정의 센서 스크립트 값, `getSensorsList` (admin): 센서 이름 | 온도 |
| openmediavault-zfs | omv-extras | 8.1.1 | X | `zfs.getPoolHealth`/`getPoolStatus`/`listPools`/`getScrubStatus`/`getArcStats` (admin) | 용량 |
| openmediavault-md | 공식 (코어와 별도 패키지) | 8.1.6 | X | `MdMgmt.getList`/`getDetail` (admin): RAID 상태 | 용량 |
| openmediavault-lvm2 | 공식 | 8.0.3 | X | `LogicalVolumeMgmt.getVolumeGroupsList`/`getLogicalVolumesList` (admin) | 용량 |
| openmediavault-snapraid, -snapraid-aio | omv-extras | 8.0.3 / 8.1 | X | `SnapRaid.getArrayList`/`getDriveList` (admin). 상태 조회는 명령 실행 메서드(`executeCommand`)를 거치므로 읽기 전용 단계에서는 쓰지 않는다 | 용량 |
| openmediavault-luksencryption | omv-extras | 8.1.6 | X | `LuksMgmt.getContainersList` (admin): 열림·잠김 상태 | 용량 |
| openmediavault-remotemount | omv-extras | 8.1.10 | X | `RemoteMount.getList` (admin) | 용량 |
| openmediavault-podman | 공식 | 8.0.1 | X | `Podman.containerList`/`containerStats`/`podList`/`podStats`/`imageList`/`volumeList` (admin). `containerList`는 `-a` 없이 실행해 실행 중인 컨테이너만 나오는 것으로 보인다(미확인) | 컨테이너 |
| openmediavault-k8s | 공식 | 8.2.7 | X | `Kubectl.getList`/`getStats`/`getEvents`, `Crictl.images` (admin). k3s/containerd이며 Docker와 별개 | 컨테이너 (별도 런타임) |
| openmediavault-microvm | omv-extras (OMV 8 신규) | 8.0.0 | X | `MicroVm.getVmList`/`getVmNameStateList` (admin) | 가상 머신 (후보) |
| openmediavault-nut | 공식 | 8.0.7 | X | `Nut.getStats`/`getVariables` (admin): UPS 부하·배터리 | UPS (목록 밖, 후보) |
| openmediavault-hddfanctrl | omv-extras | 8.0.3 | X | `HddFanCtrl.getHddList`/`getFansList` (admin): 팬·디스크 설정만, 온도 값은 없는 것으로 보인다(미확인) | 온도 (약함) |
| openmediavault-wireguard | omv-extras | 8.0.3 | X | `Wireguard.getTunnelList`/`getClientList` (admin): 설정만, 실시간 핸드셰이크·트래픽 없음 | 네트워크 (약함) |
| openmediavault-iperf3 | omv-extras | 8.0 | X | `iPerf3.getJobList`/`getLog` (admin). 측정 실행은 조작이다 | 네트워크 (약함) |

### 쓰지 않거나 OMV 8에 없는 것

- 그래프만 주는 플러그인: openmediavault-diskstats (설치됨, `CollectdDisk.enumerate`는 내부에서 admin 전용 RPC를 호출하고 결과는 PNG 그래프용), 코어 perfstats/collectd
- 설정만 있는 플러그인: usbbackup, snmp, writecache, tgt, nvmetcp, autoshutdown, wol
- OMV 8에서 빠짐: omv-extras의 diskclone, downloader, flashmemory / 공식 openmediavault-s3
- 저장소 보관(archived): openmediavault-sensors, docker-gui, docker, minidlna-docker
- 아직 OMV 8용으로 배포되지 않음: openmediavault-bcachefs, openmediavault-pwmfan
- Portainer·Yacht 설치 기능: compose 플러그인에는 Portainer 스택을 옮겨오는 `importPortainerStacks`만 남아 있다(설치 기능이 없어진 것은 메서드가 없다는 점으로 추정)
- openmediavault-imagecheck: omv-extras 저장소에 올라 있지만 소스가 조직 밖(`hackitz/openmediavault-imagecheck`)에 있다. RPC는 확인하지 않았다

## 컨테이너 런타임: Docker 우선

- OMV 코어에는 컨테이너 RPC가 없다
- compose 플러그인은 자기 설정의 `podman` 값에 따라 Docker 또는 Podman(`DOCKER_HOST=unix:///run/podman/podman.sock`)을 조회한다. 메서드는 같다. Podman 지원은 실험적 기능이다(플러그인 변경 이력 기준, 도입 버전은 미확인)
- 공식 openmediavault-podman 플러그인은 Podman 전용이고 모든 메서드가 admin이다. compose 플러그인과 함께 설치할 수 있는지는 확인하지 못했다
- 지원 순서
  1. compose 플러그인 + Docker: 검증 대상
  2. compose 플러그인 + Podman: 같은 provider로 동작할 것으로 보이지만 미검증으로 표시한다
  3. 공식 podman 플러그인 provider: 필요해지면 추가할 후보
- 런타임 이름은 `getContainers` 첫 행의 `name`(`Docker`/`Podman`)으로 판단한다

## OMV 플러그인 RPC와 독립 CLI·소켓 비교

컨테이너처럼 root 권한이 필요한 정보를 OMV 플러그인 RPC로 가져올지, Docker CLI·소켓을 직접 쓸지 비교한 결과다.

| | OMV 플러그인 RPC | 독립 CLI·소켓 직접 접근 |
|---|---|---|
| 필요 권한 | 없음. 로그인 사용자의 OMV 세션으로 호출 | 서비스 계정을 `docker` 그룹에 넣어야 한다. `docker` 그룹은 root와 같은 권한이라 대시보드가 뚫리면 호스트 전체가 넘어간다. rootful Podman 소켓(`/run/podman`)은 일반 사용자가 열 수 없다 |
| 역할 구분 | OMV가 처리 (`getContainers`는 모두, 상세는 admin) | 백엔드가 역할별 노출 범위를 따로 구현해야 한다 |
| Docker·Podman | 플러그인 설정 하나로 둘 다 | 런타임마다 구현하거나 Docker 호환 API에 의존 |
| 정보량·속도 | 일반 사용자에게는 이미지·이름·상태만. PHP 프로세스와 CLI를 거쳐 느리고, 플러그인 캐시 설정의 영향을 받는다 | Engine API JSON으로 풍부하고 빠르다 |
| 의존성 | 플러그인과 로그인 세션이 필요하다. 플러그인 API가 바뀌면 따라가야 한다 | OMV와 무관하게 동작한다 |
| 4단계 조작 | "조작은 OMV RPC로" 규칙과 맞는다 | 규칙과 충돌한다. OMV 화면과 상태가 어긋날 수 있다 |

둘 다 지원하면 플러그인이 없는 환경까지 다룰 수 있다. 대신 테스트 조합이 두 배가 되고, 권한 상승 경로가 생긴다.

**결론**

- 권한이 필요 없는 직접 읽기(`/proc`, `/sys`)는 1차 출처로 쓴다
- root가 필요한 정보(SMART, SATA 디스크 온도, 컨테이너)는 OMV RPC로만 가져온다
- Docker·Podman 소켓을 직접 읽는 provider는 만들지 않는다. provider 구조이므로 포크해서 쓰는 사람이 직접 추가할 수는 있다
- compose 플러그인이 없는 환경에서는 컨테이너 창·위젯을 숨기고 "compose 플러그인 필요"로 안내한다
