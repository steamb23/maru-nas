#!/usr/bin/env bash
# 온마루(Onmaru) 설치 스크립트. 빌드한 뒤 systemd 서비스로 설치하거나 다시 설치(업그레이드)한다.
#
#   ./install.sh                    1단계: 실행한 계정으로 빌드한다(일반 사용자 또는 root만 있는 환경의 root)
#                                   2단계: root가 아니면 sudo로 자신을 --service-install로 다시 실행한다
#   ./install.sh --no-build         빌드를 건너뛰고 기존 bin/onmaru를 설치한다
#   ./install.sh --upgrade          기존 설치의 설정 그대로 묻지 않고 빌드·설치한다. 기존 설치에 없는 설정
#                                   (새 버전에서 생긴 설정)만 묻는다. sudo 비밀번호는 필요하면 묻는다
#   ./install.sh --uninstall        제거한다(root가 아니면 sudo로 다시 실행한다)
#   ./install.sh --service-install  2단계만 실행한다(root 전용). 설정 질문, 사용자 생성, 유닛 작성, 시작
#   --lang en|ko                    안내 문구 언어. 생략하면 로캘(LC_ALL, LC_MESSAGES, LANG)로 고른다.
#                                   sudo로 다시 실행할 때는 명시적으로 넘겨 같은 언어를 유지한다
#
# 일반 사용자가 sudo ./install.sh로 실행하면 root로 빌드하지 않는다(저장소에 root 소유 파일이 생기고,
# sudo의 PATH에는 사용자의 go·npm이 없을 수 있다). 이때는 기존 실행 파일로 설치만 할지 묻는다.
#
# 권한 도우미(onmaru-helper, root로 도는 소켓 활성화 서비스)는 물어서 고른 경우에만 설치한다(--helper).
# 리버스 프록시 설정은 하지 않는다. README.md의 "리버스 프록시"를 참고한다.
# 생성하는 설정 파일(유닛, /etc/default/onmaru)의 주석은 언어와 상관없이 영어로 쓴다.
set -euo pipefail

readonly SERVICE=onmaru
readonly BIN_DST=/usr/local/bin/onmaru
readonly UNIT_FILE=/etc/systemd/system/onmaru.service
readonly ENV_FILE=/etc/default/onmaru
readonly STATE_DIR=/var/lib/onmaru
readonly RUNTIME_DIR=/run/onmaru
readonly SOCKET_PATH=$RUNTIME_DIR/onmaru.sock
readonly NGINX_WEBGUI=/etc/nginx/sites-enabled/openmediavault-webgui
readonly HELPER=onmaru-helper
readonly HELPER_BIN_DST=/usr/local/bin/onmaru-helper
readonly HELPER_SOCKET_UNIT=/etc/systemd/system/onmaru-helper.socket
readonly HELPER_SERVICE_UNIT=/etc/systemd/system/onmaru-helper.service
readonly HELPER_ENV_FILE=/etc/default/onmaru-helper
# 도우미 소켓은 Docker 프록시에 바인드 마운트하는 $RUNTIME_DIR과 다른 디렉터리에 둔다.
readonly HELPER_SOCKET=/run/onmaru-helper/helper.sock

SELF=$(readlink -f "${BASH_SOURCE[0]}")
repo_dir=$(dirname "$SELF")
readonly SELF repo_dir
readonly BIN_SRC=$repo_dir/bin/onmaru
readonly HELPER_BIN_SRC=$repo_dir/bin/onmaru-helper
readonly DIST_INDEX=$repo_dir/backend/internal/web/static/dist/index.html

# 안내 문구 언어(en, ko). main에서 정한다.
LANG_UI=en

# --upgrade로 실행했는지(1). 기존 설치의 설정을 쓰고 확인 질문을 하지 않는다.
UPGRADE=0

# t EN KO: 현재 언어의 문구를 출력한다.
t() {
  if [[ $LANG_UI == ko ]]; then printf '%s' "$2"; else printf '%s' "$1"; fi
}

detect_lang() {
  local l=${LC_ALL:-${LC_MESSAGES:-${LANG:-}}}
  [[ $l == ko* ]] && echo ko || echo en
}

is_root() { [[ $EUID -eq 0 ]]; }

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m%s:\033[0m %s\n' "$(t warning 경고)" "$*" >&2; }
die() { printf '\033[1;31m%s:\033[0m %s\n' "$(t error 오류)" "$*" >&2; exit 1; }

# ask PROMPT DEFAULT: 입력이 비면 기본값을 쓴다. 결과는 REPLY에 둔다.
ask() {
  local prompt=$1 def=${2-}
  if [[ -n $def ]]; then
    read -r -p "$prompt [$def]: " REPLY || exit 1
    REPLY=${REPLY:-$def}
  else
    read -r -p "$prompt: " REPLY || exit 1
  fi
}

# confirm PROMPT DEFAULT(y|n)
confirm() {
  local prompt=$1 def=${2:-n} hint
  [[ $def == y ]] && hint="Y/n" || hint="y/N"
  read -r -p "$prompt [$hint]: " REPLY || exit 1
  REPLY=${REPLY:-$def}
  [[ ${REPLY,,} == y || ${REPLY,,} == yes ]]
}

# write_file PATH BACKUP(yes|no): 표준 입력을 PATH(0644)에 쓴다. 내용이 같으면 그대로 두고,
# 달라졌을 때 BACKUP이 yes면 기존 파일을 PATH.bak-<시각>으로 남긴다.
write_file() {
  local path=$1 keep_backup=$2 tmp
  tmp=$(mktemp)
  cat >"$tmp"
  if [[ -f $path ]] && cmp -s "$tmp" "$path"; then
    rm -f -- "$tmp"
    info "$(t "$path unchanged" "$path 변경 없음")"
    return
  fi
  if [[ -f $path && $keep_backup == yes ]]; then
    cp -p -- "$path" "$path.bak-$(date +%Y%m%d-%H%M%S)"
  fi
  install -o root -g root -m 0644 "$tmp" "$path"
  rm -f -- "$tmp"
  info "$(t "Wrote $path" "$path 작성")"
}

# env_get KEY [FILE]: 기존 환경 파일(기본 $ENV_FILE)에서 값을 읽는다(source 하지 않는다).
env_get() {
  local file=${2:-$ENV_FILE}
  [[ -f $file ]] || return 0
  sed -n "s/^$1=//p" "$file" | tail -n 1 | sed 's/^"\(.*\)"$/\1/'
}

# valid_group: 도우미의 상승 그룹 이름(onmaru-helper -elevate-group과 같은 규칙)
valid_group() {
  [[ $1 =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$ ]]
}

readonly PORT_RE='[1-9][0-9]{0,4}'
readonly IPV4_RE='[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}'

valid_port() {
  [[ $1 =~ ^$PORT_RE$ ]] && (( $1 <= 65535 ))
}

is_loopback_tcp() {
  [[ $1 =~ ^127\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}:($PORT_RE)$ || $1 =~ ^\[::1\]:($PORT_RE)$ ]] && valid_port "${BASH_REMATCH[1]}"
}

# is_any_tcp: 3번 모드의 host:port. host는 IP만 받는다(백엔드도 IP만 받는다). 비우면 모든 주소다.
is_any_tcp() {
  [[ $1 =~ ^(\[[0-9a-fA-F:]+\]|$IPV4_RE)?:($PORT_RE)$ ]] && valid_port "${BASH_REMATCH[2]}"
}

# valid_omv_url: 루프백 http(s) URL(백엔드 -omv-url과 같은 조건)
valid_omv_url() {
  [[ $1 =~ ^https?://(127\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|localhost|\[::1\])(:($PORT_RE))?/?$ ]] || return 1
  [[ -z ${BASH_REMATCH[3]} ]] || valid_port "${BASH_REMATCH[3]}"
}

# valid_workbench_url: 백엔드 -workbench-url과 같은 조건. 빈 값(끔), http(s)://호스트[/경로],
# 또는 /·./·../로 시작하는 경로(대시보드 기준). query·fragment·공백·사용자 정보는 받지 않는다.
valid_workbench_url() {
  local u=$1
  [[ -z $u ]] && return 0
  [[ $u == *[?#[:space:]]* ]] && return 1
  # scheme은 대소문자를 가리지 않는다(백엔드의 url.Parse와 같게).
  [[ ${u,,} =~ ^https?://[^/@]+(/.*)?$ ]] && return 0
  [[ $u == //* ]] && return 1
  [[ $u == /* || $u == . || $u == .. || $u == ./* || $u == ../* ]]
}

# omv_forces_https: OMV의 "Force SSL/TLS"가 켜져 있으면 평문 포트가 https로 리다이렉트한다.
# 백엔드의 OMV 클라이언트는 자체 서명 인증서를 받지 않으므로 이 구성에서는 로그인할 수 없다.
omv_forces_https() {
  [[ -r $NGINX_WEBGUI ]] && grep -q 'return 301 https://' "$NGINX_WEBGUI"
}

# detect_omv_url: OMV 웹 UI(nginx)의 수신 포트로 루프백 주소를 만든다.
# 80/443을 앞단 프록시에 내주고 OMV를 다른 포트로 옮긴 경우가 있어 설정 파일에서 읽는다.
detect_omv_url() {
  local port=""
  if [[ -r $NGINX_WEBGUI ]]; then
    port=$(sed -nE '/ssl/d; s/^[[:space:]]*listen[[:space:]]+([^;[:space:]]*:)?([0-9]+)([[:space:];].*)?$/\2/; T; p; q' "$NGINX_WEBGUI")
  fi
  if [[ -z $port || $port == 80 ]]; then
    echo "http://127.0.0.1"
  else
    echo "http://127.0.0.1:$port"
  fi
}

usage() {
  if [[ $LANG_UI == ko ]]; then
    cat <<EOF
사용법: $0 [--upgrade] [--no-build | --uninstall | --service-install] [설정 옵션] [--lang en|ko]

  (옵션 없음)        설정을 묻고 빌드한 뒤 설치하거나 업그레이드합니다. 설치 단계에서 필요하면 sudo를 사용합니다
  --upgrade          기존 설치의 설정 그대로 묻지 않고 빌드한 뒤 업그레이드합니다
                     기존 설치에 없는 설정(새 버전에서 생긴 설정)만 묻습니다
  --no-build         빌드를 건너뛰고 기존 bin/onmaru를 설치합니다
  --uninstall        서비스, 실행 파일, 설정을 제거합니다
  --service-install  빌드 없이 설치 단계만 실행합니다(root 전용). 설정 옵션을 모두 주면 묻지 않습니다
  --lang en|ko       안내 문구 언어(기본값은 로캘에 따름)

설정 옵션(주지 않은 항목만 묻습니다):
  --user NAME                서비스 사용자(기본값 onmaru)
  --listen ADDR              127.0.0.1:8090, unix:$SOCKET_PATH, 또는 host:port
  --allow-non-loopback       루프백이 아닌 --listen 주소를 허용합니다(위험)
  --omv-url URL              OMV 웹 UI 주소(루프백)
  --workbench-url URL        브라우저에서 OMV 웹 UI를 열 주소(빈 값이면 OMV 워크벤치 앱을 숨깁니다)
  --smart-refresh-at HH:MM   SMART 정보를 매일 다시 가져올 시각
  --helper | --no-helper     권한 도우미(root로 도는 onmaru-helper)를 설치하거나 설치하지 않습니다(제거)
  --elevate-group GROUP      root 권한 상승에 openmediavault-admin과 함께 필요한 그룹(기본값 sudo)

일반 사용자는 sudo 없이 실행하세요. 빌드는 그 사용자로 하고, 설치 단계만 sudo로 실행합니다.
EOF
  else
    cat <<EOF
Usage: $0 [--upgrade] [--no-build | --uninstall | --service-install] [settings] [--lang en|ko]

  (no option)        ask for settings, build, then install or upgrade. Uses sudo for the install step when needed
  --upgrade          build and upgrade with the installed settings, without questions
                     only settings missing from the installation (new in this version) are asked
  --no-build         skip the build and install the existing bin/onmaru
  --uninstall        remove the service, binary and settings
  --service-install  run only the install step, without building (root only). No questions if all settings are given
  --lang en|ko       language of the messages (default: from the locale)

Settings (only the missing ones are asked):
  --user NAME                service user (default onmaru)
  --listen ADDR              127.0.0.1:8090, unix:$SOCKET_PATH, or host:port
  --allow-non-loopback       allow a non-loopback --listen address (dangerous)
  --omv-url URL              OMV web UI address (loopback)
  --workbench-url URL        OMV web UI address for browsers (empty hides the OMV Workbench app)
  --smart-refresh-at HH:MM   daily SMART refresh time
  --helper | --no-helper     install, or do not install (remove), the privileged helper (onmaru-helper, runs as root)
  --elevate-group GROUP      group required together with openmediavault-admin to elevate to root (default sudo)

Normal users should run it without sudo: the build runs as that user and only the install step uses sudo.
EOF
  fi
}

# build: 1단계. 실행한 계정으로 빌드한다. 처음 클론한 상태면 서브모듈과 npm 의존성부터 준비한다.
build() {
  info "$(t "Building as $(id -un)" "$(id -un) 계정으로 빌드")"
  local tool
  for tool in make go npm; do
    command -v "$tool" >/dev/null 2>&1 ||
      die "$(t "'$tool' not found in PATH. Install it, or build elsewhere and use --no-build." \
        "PATH에서 '$tool'을 찾을 수 없습니다. 설치하거나, 다른 곳에서 빌드한 뒤 --no-build를 사용하세요.")"
  done
  if [[ -z $(ls -A "$repo_dir/vendor/icons/colloid" 2>/dev/null) ]]; then
    command -v git >/dev/null 2>&1 || die "$(t "'git' is required to fetch the icon submodule" "아이콘 서브모듈을 받으려면 'git'이 필요합니다")"
    info "$(t "Fetching the icon submodule" "아이콘 서브모듈 받는 중")"
    git -C "$repo_dir" submodule update --init --depth 1
  fi
  if [[ ! -d $repo_dir/frontend/node_modules ||
    $repo_dir/frontend/package-lock.json -nt $repo_dir/frontend/node_modules/.package-lock.json ]]; then
    info "$(t "Installing frontend dependencies" "프론트엔드 의존성 설치 중")"
    npm --prefix "$repo_dir/frontend" ci
  fi
  make -C "$repo_dir" build
  info "$(t "Build finished: $BIN_SRC" "빌드 완료: $BIN_SRC")"
}

# as_root ACTION(service-install|uninstall): root면 바로 실행하고, 아니면 sudo로 자신을 다시 실행한다.
# service-install이면 1단계에서 정한 설정을 옵션으로 넘겨 2단계가 다시 묻지 않게 한다.
as_root() {
  local action=$1
  if is_root; then
    run_root "$action"
    return
  fi
  local args=()
  if [[ $action == service-install ]]; then
    mapfile -t args < <(settings_args)
    # 2단계도 확인 질문 없이 진행한다.
    if (( UPGRADE )); then
      args+=(--upgrade)
    fi
  fi
  command -v sudo >/dev/null 2>&1 ||
    die "$(t "root privileges are required and sudo is not installed. Run this as root." \
      "root 권한이 필요한데 sudo가 없습니다. root로 실행하세요.")"
  info "$(t "Root privileges are needed for the next step; running sudo" "다음 단계에는 root 권한이 필요해 sudo를 실행합니다")"
  exec sudo "$SELF" "--$action" --lang "$LANG_UI" "${args[@]}"
}

run_root() {
  command -v systemctl >/dev/null 2>&1 || die "$(t "systemd is required" "systemd가 필요합니다")"
  case $1 in
    service-install) install_service ;;
    uninstall) uninstall ;;
  esac
}

# remove_helper: 권한 도우미의 소켓·서비스를 멈추고 유닛, 실행 파일, 설정을 지운다. 없으면 아무것도 하지 않는다.
remove_helper() {
  [[ -f $HELPER_SOCKET_UNIT || -f $HELPER_SERVICE_UNIT || -f $HELPER_BIN_DST || -f $HELPER_ENV_FILE ]] || return 0
  info "$(t "Removing the privileged helper" "권한 도우미 제거")"
  systemctl disable --now "$HELPER.socket" "$HELPER.service" 2>/dev/null || true
  rm -f -- "$HELPER_SOCKET_UNIT" "$HELPER_SERVICE_UNIT" "$HELPER_BIN_DST" "$HELPER_ENV_FILE"
  systemctl daemon-reload
}

# is_own_system_user NAME: install.sh가 만든 형태의 계정인지 본다. 기존 공용 계정을 지정했을 수 있으므로
# 시스템 UID 범위이고 홈이 /nonexistent일 때만 제거를 제안한다.
is_own_system_user() {
  local entry uid home
  entry=$(getent passwd "$1") || return 1
  uid=$(cut -d: -f3 <<<"$entry")
  home=$(cut -d: -f6 <<<"$entry")
  (( uid > 0 && uid < 1000 )) && [[ $home == /nonexistent ]]
}

uninstall() {
  info "$(t "Uninstalling $SERVICE" "$SERVICE 제거")"
  confirm "$(t "Stop the service and remove $UNIT_FILE, $BIN_DST and $ENV_FILE (and the privileged helper if installed)?" \
    "서비스를 멈추고 $UNIT_FILE, $BIN_DST, $ENV_FILE 파일(설치했다면 권한 도우미도)을 지울까요?")" n || exit 0
  if systemctl list-unit-files "$SERVICE.service" >/dev/null 2>&1; then
    systemctl disable --now "$SERVICE.service" 2>/dev/null || true
  fi
  remove_helper
  local user=""
  [[ -f $UNIT_FILE ]] && user=$(sed -n '/^User=/{s///p;q}' "$UNIT_FILE")
  rm -f -- "$UNIT_FILE" "$BIN_DST" "$ENV_FILE"
  systemctl daemon-reload
  # RuntimeDirectoryPreserve=yes라 서비스를 멈춰도 남는다. 재부팅하면 어차피 사라진다.
  rm -rf -- "$RUNTIME_DIR"
  info "$(t "Removed service, binary and settings (backups $ENV_FILE.bak-* are kept)" \
    "서비스, 실행 파일, 설정을 지웠습니다(백업 $ENV_FILE.bak-* 은 남겨 둡니다)")"

  if [[ -d $STATE_DIR ]] && confirm "$(t "Also delete $STATE_DIR (dashboard settings and uploaded backgrounds)?" \
    "$STATE_DIR(대시보드 설정과 올린 배경 이미지)도 지울까요?")" n; then
    rm -rf -- "$STATE_DIR"
    info "$(t "Removed $STATE_DIR" "$STATE_DIR 삭제")"
  fi
  if [[ -n $user ]] && is_own_system_user "$user" && confirm "$(t "Also delete the system user '$user'?" \
    "시스템 사용자 '$user'도 지울까요?")" n; then
    userdel "$user"
    info "$(t "Removed user $user" "사용자 $user 삭제")"
  fi
  info "$(t "If a reverse proxy pointed at onmaru, remove that configuration as well." \
    "리버스 프록시에 onmaru 설정이 있다면 그것도 지워 주세요.")"
}

# 설정값. 명령줄 옵션(--user 등)으로 받거나 ask_settings로 묻는다. 비어 있으면 아직 정하지 않은 것이다.
# 설정을 새로 더하면 load_installed_settings에서도 기존 설치의 값을 읽는다. 기존 설치에 그 값이 없으면
# --upgrade는 그 설정만 묻는다.
S_USER=""
S_LISTEN=""
S_ALLOW=false
S_OMV=""
S_SMART=""
# OMV 워크벤치 주소는 빈 값(끔)도 정한 값이므로 정했는지를 따로 둔다.
S_WORKBENCH=""
S_WORKBENCH_SET=false
# 권한 도우미 설치 여부(yes|no)와 상승 그룹. 설치하지 않으면 그룹은 쓰지 않는다.
S_HELPER=""
S_ELEVATE_GROUP=""

settings_complete() {
  [[ -n $S_USER && -n $S_LISTEN && -n $S_OMV && -n $S_SMART && $S_WORKBENCH_SET == true ]] &&
    [[ $S_HELPER == no || ($S_HELPER == yes && -n $S_ELEVATE_GROUP) ]]
}

# settings_args: 2단계(--service-install)로 넘길 옵션을 한 줄에 하나씩 출력한다.
settings_args() {
  printf '%s\n' --user "$S_USER" --listen "$S_LISTEN" --omv-url "$S_OMV" --smart-refresh-at "$S_SMART" \
    --workbench-url "$S_WORKBENCH"
  if [[ $S_ALLOW == true ]]; then
    printf '%s\n' --allow-non-loopback
  fi
  if [[ $S_HELPER == yes ]]; then
    printf '%s\n' --helper --elevate-group "$S_ELEVATE_GROUP"
  elif [[ $S_HELPER == no ]]; then
    printf '%s\n' --no-helper
  fi
}

# load_installed_settings: --upgrade. 기존 설치(유닛, 환경 파일)의 설정을 옵션으로 받지 않은 항목에 채운다.
# 기존 설치에 없는 설정은 비워 두어 ask_settings가 그것만 묻게 한다. 일반 사용자도 읽을 수 있다(0644).
load_installed_settings() {
  [[ -f $UNIT_FILE && -f $ENV_FILE ]] ||
    die "$(t "--upgrade needs an existing installation ($UNIT_FILE, $ENV_FILE). Run $SELF without --upgrade to install." \
      "--upgrade는 기존 설치($UNIT_FILE, $ENV_FILE)가 있어야 합니다. 처음 설치라면 --upgrade 없이 $SELF 를 실행하세요.")"
  [[ -n $S_USER ]] || S_USER=$(sed -n '/^User=/{s///p;q}' "$UNIT_FILE")
  # 루프백 외 주소 허용은 수신 주소와 짝이므로 수신 주소를 기존 설치에서 읽을 때만 함께 읽는다.
  if [[ -z $S_LISTEN ]]; then
    S_LISTEN=$(env_get ONMARU_LISTEN)
    if [[ $(env_get ONMARU_ALLOW_NON_LOOPBACK) == true ]]; then
      S_ALLOW=true
    fi
  fi
  [[ -n $S_OMV ]] || S_OMV=$(env_get ONMARU_OMV_URL)
  [[ -n $S_SMART ]] || S_SMART=$(env_get ONMARU_SMART_REFRESH_AT)
  # 빈 값도 정한 값(끔)이므로 키가 있는지로 판단한다.
  if [[ $S_WORKBENCH_SET != true ]] && grep -q '^ONMARU_WORKBENCH_URL=' "$ENV_FILE"; then
    S_WORKBENCH=$(env_get ONMARU_WORKBENCH_URL)
    S_WORKBENCH_SET=true
  fi
  # 도우미를 설치하지 않기로 한 설치도 ONMARU_HELPER_SOCKET=(빈 값)을 남기므로 키가 있으면 정한 것이다.
  if [[ -z $S_HELPER ]] && grep -q '^ONMARU_HELPER_SOCKET=' "$ENV_FILE"; then
    if [[ -n $(env_get ONMARU_HELPER_SOCKET) && -f $HELPER_SERVICE_UNIT ]]; then
      S_HELPER=yes
    else
      S_HELPER=no
    fi
  fi
  if [[ $S_HELPER == yes && -z $S_ELEVATE_GROUP ]]; then
    S_ELEVATE_GROUP=$(env_get ONMARU_HELPER_ELEVATE_GROUP "$HELPER_ENV_FILE")
  fi
}

# ask_settings: 아직 정하지 않은 설정만 묻는다. 기존 설치가 있으면 그 값을 기본값으로 쓴다.
# 일반 사용자도 실행할 수 있다(읽는 파일은 모두 0644).
ask_settings() {
  if (( UPGRADE )); then
    info "$(t "Upgrading with the installed settings." "기존 설치의 설정으로 업그레이드합니다.")"
    if ! settings_complete; then
      info "$(t "Some settings are new in this version. Please answer them." \
        "이 버전에서 새로 생긴 설정이 있습니다. 그 설정만 입력해 주세요.")"
    fi
  elif [[ -f $UNIT_FILE ]]; then
    info "$(t "Existing installation found. Current settings are used as defaults." \
      "기존 설치를 찾았습니다. 현재 설정을 기본값으로 씁니다.")"
  fi

  # 1. 서비스 사용자
  if [[ -z $S_USER ]]; then
    local user_def=""
    if [[ -f $UNIT_FILE ]]; then
      user_def=$(sed -n '/^User=/{s///p;q}' "$UNIT_FILE")
    fi
    ask "$(t "Service user (created as a system user if missing)" "서비스 사용자(없으면 시스템 사용자로 만듭니다)")" \
      "${user_def:-$SERVICE}"
    S_USER=$REPLY
  fi

  # 2. 수신 방식
  if [[ -z $S_LISTEN ]]; then
    ask_listen
  fi

  # 3. OMV 주소
  if omv_forces_https; then
    warn "$(t "OMV redirects HTTP to HTTPS (\"Force SSL/TLS\" is on). onmaru talks to OMV over plain HTTP on loopback and cannot log in this way. Turn off Force SSL/TLS in the OMV web UI (System > Workbench) first." \
      "OMV가 HTTP를 HTTPS로 리다이렉트합니다(\"Force SSL/TLS\" 켜짐). onmaru는 루프백의 평문 HTTP로 OMV에 접속하므로 이 상태로는 로그인할 수 없습니다. 먼저 OMV 웹 UI(System > Workbench)에서 Force SSL/TLS를 끄세요.")"
    # --upgrade는 묻지 않고 알리기만 한다(이미 설치된 구성이다).
    if (( ! UPGRADE )); then
      confirm "$(t "Continue anyway?" "그래도 계속할까요?")" n || exit 1
    fi
  fi
  if [[ -z $S_OMV ]]; then
    local omv_def
    omv_def=$(env_get ONMARU_OMV_URL)
    ask "$(t "OMV web UI address (loopback, detected from nginx)" "OMV 웹 UI 주소(루프백, nginx 설정에서 찾음)")" \
      "${omv_def:-$(detect_omv_url)}"
    S_OMV=$REPLY
  fi

  # 4. SMART 갱신 시각
  if [[ -z $S_SMART ]]; then
    local smart_def
    smart_def=$(env_get ONMARU_SMART_REFRESH_AT)
    ask "$(t "Daily SMART refresh time (HH:MM, local time)" "SMART 정보를 매일 다시 가져올 시각(HH:MM, 서버 시간)")" \
      "${smart_def:-00:00}"
    S_SMART=$REPLY
  fi

  # 5. OMV 워크벤치 주소(브라우저 기준). ask는 빈 입력이면 기본값을 쓰므로 끄는 값은 -로 받는다.
  if [[ $S_WORKBENCH_SET != true ]]; then
    local wb_def
    wb_def=$(env_get ONMARU_WORKBENCH_URL)
    t "The dashboard's \"OMV Workbench\" app opens the OMV web UI in a new browser tab at this address.
  Enter the address you open OMV with in the browser, e.g. https://nas.example.lan/
  (if the dashboard is under a path on the same host as OMV, / also works). Enter - to hide the app." \
      "대시보드의 \"OMV 워크벤치\" 앱이 이 주소로 OMV 웹 UI를 새 탭에 엽니다.
  브라우저에서 OMV에 접속하는 주소를 입력하세요. 예: https://nas.example.lan/
  (대시보드를 OMV와 같은 호스트의 경로 아래에 두었다면 /도 됩니다). -를 입력하면 앱을 숨깁니다."
    echo
    ask "$(t "OMV Workbench address for browsers (- to turn off)" "브라우저에서 OMV 워크벤치를 열 주소(끄려면 -)")" \
      "${wb_def:--}"
    [[ $REPLY == - ]] && REPLY=""
    S_WORKBENCH=$REPLY
    S_WORKBENCH_SET=true
  fi

  # 6. 권한 도우미
  if [[ -z $S_HELPER ]]; then
    echo
    t "The privileged helper (onmaru-helper) is a separate service that runs as root.
  It lets onmaru act with each signed-in user's own Linux account (for example for file access) and,
  after an administrator enters the password again, run a fixed set of root operations for 5 minutes.
  Without it, features that need it are unavailable. See README.md, \"Privileged helper\"." \
      "권한 도우미(onmaru-helper)는 root로 도는 별도 서비스입니다.
  로그인한 사용자 자신의 Linux 계정 권한으로(예: 파일 접근), 그리고 관리자가 비밀번호를 다시 입력하면
  5분 동안 정해진 root 연산을 onmaru가 실행할 수 있게 합니다. 설치하지 않으면 그런 기능을 쓸 수 없습니다.
  README.md의 \"권한 도우미\"를 참고하세요."
    echo
    local helper_def=n
    [[ -f $HELPER_SERVICE_UNIT ]] && helper_def=y
    if confirm "$(t "Install the privileged helper?" "권한 도우미를 설치할까요?")" "$helper_def"; then
      S_HELPER=yes
    else
      S_HELPER=no
    fi
  fi
  if [[ $S_HELPER == yes && -z $S_ELEVATE_GROUP ]]; then
    local group_def
    group_def=$(env_get ONMARU_HELPER_ELEVATE_GROUP "$HELPER_ENV_FILE")
    ask "$(t "Group that may elevate to root (members must also be in openmediavault-admin)" \
      "root 권한으로 상승할 수 있는 그룹(openmediavault-admin에도 속해야 합니다)")" "${group_def:-sudo}"
    S_ELEVATE_GROUP=$REPLY
  fi
}

ask_listen() {
  local cur_listen mode_def
  cur_listen=$(env_get ONMARU_LISTEN)
  if [[ $cur_listen == unix:* ]]; then
    mode_def=2
  elif [[ -n $cur_listen ]] && ! is_loopback_tcp "$cur_listen"; then
    mode_def=3
  else
    mode_def=1
  fi
  if [[ $LANG_UI == ko ]]; then
    cat <<EOF

리버스 프록시가 onmaru에 어떻게 접속하나요?
  1) 루프백 TCP (127.0.0.1:8090)
     프록시가 이 호스트에서 직접 실행될 때(컨테이너가 아닐 때).
  2) Unix 소켓 ($SOCKET_PATH)
     프록시가 Docker 컨테이너일 때: $RUNTIME_DIR 을 컨테이너에 마운트합니다.
  3) 모든 주소의 TCP (위험)
     그 주소에 닿는 누구나 onmaru에 직접 접속할 수 있습니다.
EOF
  else
    cat <<EOF

How should the reverse proxy reach onmaru?
  1) TCP on loopback (127.0.0.1:8090)
     The proxy runs on this host (not in a container).
  2) Unix socket ($SOCKET_PATH)
     The proxy runs in a Docker container: mount $RUNTIME_DIR into it.
  3) TCP on any address (DANGEROUS)
     Anyone who can reach the address talks to onmaru directly.
EOF
  fi
  ask "$(t "Choose 1-3" "1-3 중 선택")" "$mode_def"
  case $REPLY in
    1)
      ask "$(t "Listen address" "수신 주소")" "$(is_loopback_tcp "$cur_listen" && echo "$cur_listen" || echo 127.0.0.1:8090)"
      S_LISTEN=$REPLY
      S_ALLOW=false
      is_loopback_tcp "$S_LISTEN" || die "$(t "not a loopback address: $S_LISTEN (use option 3 for other addresses)" \
        "루프백 주소가 아닙니다: $S_LISTEN (다른 주소는 3번을 고르세요)")"
      ;;
    2)
      S_LISTEN=unix:$SOCKET_PATH
      S_ALLOW=false
      ;;
    3)
      ask "$(t "Listen address" "수신 주소")" "$([[ $mode_def == 3 ]] && echo "$cur_listen" || echo 0.0.0.0:8090)"
      S_LISTEN=$REPLY
      is_any_tcp "$S_LISTEN" || die "$(t "invalid address: $S_LISTEN (use IP:port, e.g. 0.0.0.0:8090)" \
        "주소가 올바르지 않습니다: $S_LISTEN (IP:포트 형식, 예: 0.0.0.0:8090)")"
      if [[ $LANG_UI == ko ]]; then
        cat <<'EOF' >&2

  경고: 루프백이 아닌 주소에서 수신합니다.
  - 직접 접속하는 클라이언트는 리버스 프록시와 HTTPS를 거치지 않습니다. 비밀번호가 평문 HTTP로 오갑니다.
  - X-Forwarded-For / X-Forwarded-Proto를 위조할 수 있습니다. 로그인 시도 제한을 우회할 수 있고
    OMV 인증 로그(fail2ban)에 가짜 IP가 남을 수 있습니다.
  이 방식을 쓴다면 방화벽으로 접근을 제한하세요.

EOF
      else
        cat <<'EOF' >&2

  WARNING: listening on a non-loopback address.
  - Clients that connect directly bypass the reverse proxy and HTTPS. Passwords travel in plain HTTP.
  - X-Forwarded-For / X-Forwarded-Proto can be forged: the login rate limit can be bypassed
    and OMV auth logs (fail2ban) may record a fake IP.
  Restrict access with a firewall if you choose this.

EOF
      fi
      read -r -p "$(t "Type 'yes' to accept the risk: " "위험을 감수하려면 'yes'를 입력하세요: ")" REPLY || exit 1
      [[ $REPLY == yes ]] || die "$(t "aborted" "중단했습니다")"
      S_ALLOW=true
      ;;
    *) die "$(t "invalid choice: $REPLY" "잘못된 선택입니다: $REPLY")" ;;
  esac
}

# validate_settings: 옵션으로 받은 값도 같은 규칙으로 검사한다(2단계는 1단계의 값을 다시 검사한다).
validate_settings() {
  [[ $S_USER =~ ^[a-z_][a-z0-9_-]*$ ]] || die "$(t "invalid user name: $S_USER" "사용자 이름이 올바르지 않습니다: $S_USER")"
  [[ $S_USER != root ]] || die "$(t "do not run onmaru as root" "onmaru를 root로 실행하지 마세요")"
  if [[ $S_LISTEN == unix:* ]]; then
    # 소켓 디렉터리는 systemd RuntimeDirectory=가 만들고 ProtectSystem=strict에서도 쓸 수 있는 곳이어야 한다.
    [[ $S_LISTEN == "unix:$SOCKET_PATH" ]] || die "$(t "the Unix socket must be unix:$SOCKET_PATH, got $S_LISTEN" \
      "Unix 소켓은 unix:$SOCKET_PATH 이어야 합니다: $S_LISTEN")"
  elif ! is_loopback_tcp "$S_LISTEN"; then
    is_any_tcp "$S_LISTEN" || die "$(t "invalid address: $S_LISTEN (use IP:port, e.g. 0.0.0.0:8090)" \
      "주소가 올바르지 않습니다: $S_LISTEN (IP:포트 형식, 예: 0.0.0.0:8090)")"
    [[ $S_ALLOW == true ]] || die "$(t "$S_LISTEN is not a loopback address; add --allow-non-loopback to accept the risk" \
      "$S_LISTEN 은 루프백 주소가 아닙니다. 위험을 감수하려면 --allow-non-loopback을 추가하세요")"
  fi
  valid_omv_url "$S_OMV" ||
    die "$(t "OMV address must be a loopback http(s) URL: $S_OMV" "OMV 주소는 루프백 http(s) URL이어야 합니다: $S_OMV")"
  [[ $S_SMART =~ ^([01][0-9]|2[0-3]):[0-5][0-9]$ ]] || die "$(t "invalid time: $S_SMART" "시각이 올바르지 않습니다: $S_SMART")"
  [[ $S_HELPER == yes || $S_HELPER == no ]] || die "$(t "choose --helper or --no-helper" "--helper 또는 --no-helper를 고르세요")"
  if [[ $S_HELPER == yes ]]; then
    valid_group "$S_ELEVATE_GROUP" || die "$(t "invalid group name: $S_ELEVATE_GROUP" "그룹 이름이 올바르지 않습니다: $S_ELEVATE_GROUP")"
  fi
  valid_workbench_url "$S_WORKBENCH" ||
    die "$(t "OMV Workbench address must start with http:// or https://, or be a path starting with / or ./ or ../ (no query or fragment): $S_WORKBENCH" \
      "OMV 워크벤치 주소는 http:// 또는 https://로 시작하거나 /·./·../로 시작하는 경로여야 합니다(query·fragment 없이): $S_WORKBENCH")"
}

# check_settings: validate_settings. --upgrade에서 기존 설치의 값이 이 버전의 검사를 통과하지 못하면
# 고치는 방법을 먼저 알린다(검사가 엄격해졌거나 손으로 고친 설정 파일).
check_settings() {
  if (( UPGRADE )) && ! (validate_settings) >/dev/null 2>&1; then
    warn "$(t "the installed settings do not pass this version's checks. Run $SELF without --upgrade to fix them." \
      "기존 설치의 설정이 이 버전의 검사를 통과하지 못했습니다. --upgrade 없이 $SELF 를 실행해 고치세요.")"
  fi
  validate_settings
}

show_summary() {
  local allow_note="" workbench=${S_WORKBENCH:-"($(t "off" "끔"))"} helper
  [[ $S_ALLOW == true ]] && allow_note="  ($(t "non-loopback allowed" "루프백 외 주소 허용"))"
  if [[ $S_HELPER == yes ]]; then
    helper="$(t "install" "설치") ($HELPER_BIN_DST, $(t "elevate group" "상승 그룹") $S_ELEVATE_GROUP)"
  else
    helper=$(t "do not install" "설치 안 함")
  fi
  if [[ $LANG_UI == ko ]]; then
    cat <<EOF

요약
  실행 파일     $BIN_SRC -> $BIN_DST
  서비스 사용자 $S_USER
  수신          $S_LISTEN$allow_note
  OMV 주소      $S_OMV
  OMV 워크벤치  $workbench
  SMART 갱신    $S_SMART
  권한 도우미   $helper
  상태 디렉터리 $STATE_DIR
  설정 파일     $UNIT_FILE, $ENV_FILE

EOF
  else
    cat <<EOF

Summary
  binary        $BIN_SRC -> $BIN_DST
  service user  $S_USER
  listen        $S_LISTEN$allow_note
  OMV address   $S_OMV
  OMV Workbench $workbench
  SMART refresh $S_SMART
  helper        $helper
  state dir     $STATE_DIR
  files         $UNIT_FILE, $ENV_FILE

EOF
  fi
}

require_binary() {
  [[ -x $BIN_SRC ]] || die "$(t "$BIN_SRC not found. Run $SELF without --no-build to build it first." \
    "$BIN_SRC 파일이 없습니다. --no-build 없이 $SELF 를 실행해 먼저 빌드하세요.")"
  if [[ $S_HELPER == yes ]]; then
    [[ -x $HELPER_BIN_SRC ]] || die "$(t "$HELPER_BIN_SRC not found. Run $SELF without --no-build to build it first." \
      "$HELPER_BIN_SRC 파일이 없습니다. --no-build 없이 $SELF 를 실행해 먼저 빌드하세요.")"
  fi
  if [[ ! -f $DIST_INDEX ]]; then
    warn "$(t "frontend build not found ($DIST_INDEX). The binary will serve a 'frontend not built' page." \
      "프론트엔드 빌드가 없습니다($DIST_INDEX). 화면 대신 'frontend not built' 안내만 나옵니다.")"
  fi
}

# install_service: 2단계(root). 설정이 모두 정해져 있으면 묻지 않고 설치한다.
# root가 옵션 없이 --service-install을 직접 실행했다면 여기서 묻는다(--upgrade면 기존 설치에 없는 설정만).
install_service() {
  require_binary
  if (( UPGRADE )); then
    load_installed_settings
  fi
  if ! settings_complete; then
    ask_settings
    check_settings
    show_summary
    if (( ! UPGRADE )); then
      confirm "$(t "Proceed?" "진행할까요?")" y || exit 0
    fi
  else
    check_settings
  fi
  # 설정을 물은 뒤에야 도우미를 설치할지 알 수 있으므로 실행 파일을 다시 확인한다.
  require_binary
  local user=$S_USER listen=$S_LISTEN allow=$S_ALLOW omv_url=$S_OMV smart_at=$S_SMART workbench_url=$S_WORKBENCH group

  if ! id -u "$user" >/dev/null 2>&1; then
    info "$(t "Creating system user $user" "시스템 사용자 $user 생성")"
    useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin "$user"
  fi
  # 기존 계정은 이름과 같은 그룹이 없을 수 있다(예: adduser --system의 기본 그룹 nogroup).
  group=$(id -gn "$user") || die "$(t "cannot find the primary group of $user" "$user 의 기본 그룹을 찾을 수 없습니다")"

  check_port_free "$listen"

  # 업그레이드에서 수신 주소와 서비스 사용자가 그대로면 리버스 프록시 설정도 그대로 맞으므로 안내를 줄인다.
  # 파일을 다시 쓰기 전에 기존 값과 비교한다.
  local proxy_unchanged=no
  if (( UPGRADE )) && [[ -f $UNIT_FILE && $(env_get ONMARU_LISTEN) == "$listen" &&
    $(sed -n '/^User=/{s///p;q}' "$UNIT_FILE") == "$user" ]]; then
    proxy_unchanged=yes
  fi

  info "$(t "Installing $BIN_DST" "$BIN_DST 설치")"
  install -o root -g root -m 0755 "$BIN_SRC" "$BIN_DST"

  local helper_socket="" helper_deps=""
  if [[ $S_HELPER == yes ]]; then
    install_helper "$user"
    helper_socket=$HELPER_SOCKET
    helper_deps=$'\nWants=onmaru-helper.socket\nAfter=onmaru-helper.socket'
  else
    remove_helper
  fi

  write_file "$ENV_FILE" yes <<EOF
# onmaru settings. Written by install.sh; re-run it to change them.
# See README.md for every option.
ONMARU_LISTEN=$listen
ONMARU_ALLOW_NON_LOOPBACK=$allow
ONMARU_OMV_URL=$omv_url
ONMARU_SMART_REFRESH_AT=$smart_at
ONMARU_WORKBENCH_URL=$workbench_url
# Privileged helper socket; empty means the helper is not installed.
ONMARU_HELPER_SOCKET=$helper_socket
EOF

  # 특정 IP에 바인딩하면 부팅 중 그 주소가 생긴 뒤에 시작해야 한다.
  local unit_after="After=network.target"
  if [[ $allow == true ]]; then
    unit_after=$'Wants=network-online.target\nAfter=network-online.target'
  fi

  # 유닛은 위 값으로 언제든 다시 만들 수 있으므로 백업하지 않는다.
  write_file "$UNIT_FILE" no <<EOF
# Written by onmaru install.sh
[Unit]
Description=Onmaru dashboard for openmediavault
$unit_after$helper_deps

[Service]
Type=simple
User=$user
Group=$group
EnvironmentFile=$ENV_FILE
ExecStart=$BIN_DST
Restart=on-failure
RestartSec=5

# Per-user preferences (STATE_DIRECTORY is passed to onmaru)
StateDirectory=$SERVICE
# Unix socket directory. Preserve keeps the same directory across restarts so a
# container bind mount of $RUNTIME_DIR keeps working.
RuntimeDirectory=$SERVICE
RuntimeDirectoryPreserve=yes

NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
# PrivateDevices=yes would hide /dev/disk/by-label (filesystem labels); deny device access instead.
DevicePolicy=closed
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictSUIDSGID=yes
RestrictRealtime=yes
LockPersonality=yes
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  if [[ $S_HELPER == yes ]]; then
    # onmaru가 시작할 때 도우미에 연결하므로 소켓을 먼저 연다. 이미 돌던 도우미는 새 실행 파일로 다시 시작한다
    # (티켓이 사라지므로 사용자는 다시 로그인한다. onmaru도 다시 시작하므로 어차피 다시 로그인한다).
    systemctl enable --now "$HELPER.socket" >/dev/null
    if systemctl is-active --quiet "$HELPER.service"; then
      systemctl restart "$HELPER.service"
    fi
  fi
  systemctl enable "$SERVICE.service" >/dev/null
  if systemctl is-active --quiet "$SERVICE.service"; then
    info "$(t "Restarting $SERVICE" "$SERVICE 재시작")"
    systemctl restart "$SERVICE.service"
  else
    info "$(t "Starting $SERVICE" "$SERVICE 시작")"
    systemctl start "$SERVICE.service"
  fi

  health_check "$listen"
  if [[ $S_HELPER == yes ]] && ! systemctl is-active --quiet "$HELPER.socket"; then
    warn "$(t "$HELPER.socket is not active. See: systemctl status $HELPER.socket" \
      "$HELPER.socket 이(가) 켜져 있지 않습니다. 확인: systemctl status $HELPER.socket")"
  fi
  next_steps "$listen" "$group" "$proxy_unchanged"
}

# install_helper USER: 권한 도우미 실행 파일, 설정, 소켓·서비스 유닛을 쓴다. 도우미는 root로 돌지만
# USER(onmaru 서비스 사용자)의 연결만 받고, 사용자 권한으로 강등하는 데 필요한 권한만 남긴다.
install_helper() {
  local user=$1
  if ! getent group "$S_ELEVATE_GROUP" >/dev/null; then
    warn "$(t "group '$S_ELEVATE_GROUP' does not exist. Nobody can elevate to root until it is created." \
      "그룹 '$S_ELEVATE_GROUP'이(가) 없습니다. 그룹을 만들기 전에는 아무도 root 권한으로 상승할 수 없습니다.")"
  fi
  [[ -x /usr/sbin/unix_chkpwd ]] || warn "$(t "/usr/sbin/unix_chkpwd not found (package libpam-modules-bin); the helper will not start." \
    "/usr/sbin/unix_chkpwd가 없습니다(libpam-modules-bin 패키지). 도우미가 시작하지 않습니다.")"
  info "$(t "Installing $HELPER_BIN_DST" "$HELPER_BIN_DST 설치")"
  install -o root -g root -m 0755 "$HELPER_BIN_SRC" "$HELPER_BIN_DST"

  write_file "$HELPER_ENV_FILE" yes <<EOF
# onmaru-helper settings. Written by install.sh; re-run it to change them.
# Only this user may connect to the helper socket.
ONMARU_HELPER_CLIENT_USER=$user
# Members of openmediavault-admin who are also in this group may elevate to root.
ONMARU_HELPER_ELEVATE_GROUP=$S_ELEVATE_GROUP
EOF

  write_file "$HELPER_SOCKET_UNIT" no <<EOF
# Written by onmaru install.sh
[Unit]
Description=Onmaru privileged helper socket

[Socket]
ListenStream=$HELPER_SOCKET
# Only the onmaru service user can connect (the helper also checks the peer uid).
SocketUser=$user
SocketGroup=root
SocketMode=0600
DirectoryMode=0755
RemoveOnStop=yes

[Install]
WantedBy=sockets.target
EOF

  write_file "$HELPER_SERVICE_UNIT" no <<EOF
# Written by onmaru install.sh
[Unit]
Description=Onmaru privileged helper
Requires=$HELPER.socket
After=$HELPER.socket

[Service]
Type=simple
EnvironmentFile=$HELPER_ENV_FILE
ExecStart=$HELPER_BIN_DST

# Runs as root, but may only drop privileges to a user (SETUID/SETGID) and stop its own
# workers (KILL). Add capabilities here only together with a new root operation.
CapabilityBoundingSet=CAP_SETUID CAP_SETGID CAP_KILL
NoNewPrivileges=yes
ProtectSystem=strict
# User-scope file operations write on data disks and in home directories.
ReadWritePaths=-/srv -/home
PrivateTmp=yes
PrivateDevices=yes
PrivateNetwork=yes
RestrictAddressFamilies=AF_UNIX
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
SystemCallArchitectures=native
SystemCallFilter=@system-service
UMask=0077
EOF
}

# check_port_free: TCP 수신이면 다른 프로세스(예: make dev의 백엔드)가 포트를 쓰고 있는지 미리 알린다.
# 서비스가 이미 돌고 있다면 그 포트는 자기 것이므로 건너뛴다.
check_port_free() {
  local listen=$1
  [[ $listen != unix:* ]] || return 0
  command -v ss >/dev/null 2>&1 || return 0
  systemctl is-active --quiet "$SERVICE.service" && return 0
  if [[ -n $(ss -Hltn "sport = :${listen##*:}") ]]; then
    warn "$(t "port ${listen##*:} is already in use by another process (for example a 'make dev' backend). The service will fail to start until it is free." \
      "포트 ${listen##*:}을(를) 다른 프로세스가 쓰고 있습니다(예: 'make dev' 백엔드). 비워질 때까지 서비스가 시작되지 않습니다.")"
    # --upgrade는 묻지 않고 알리기만 한다. 포트가 비면 systemd가 다시 시작한다(Restart=on-failure).
    if (( ! UPGRADE )); then
      confirm "$(t "Continue anyway?" "그래도 계속할까요?")" n || exit 1
    fi
  fi
}

health_check() {
  local listen=$1
  # Type=simple이라 start는 곧바로 죽는 프로세스에도 성공한다. 잠시 뒤 실제 상태를 본다.
  sleep 2
  if ! systemctl is-active --quiet "$SERVICE.service"; then
    warn "$(t "$SERVICE is not running. See: systemctl status $SERVICE; journalctl -u $SERVICE -e" \
      "$SERVICE 이(가) 실행 중이 아닙니다. 확인: systemctl status $SERVICE; journalctl -u $SERVICE -e")"
    return
  fi
  if ! command -v curl >/dev/null 2>&1; then
    info "$(t "curl not found; skipping health check. Check: systemctl status $SERVICE" \
      "curl이 없어 상태 확인을 건너뜁니다. 확인: systemctl status $SERVICE")"
    return
  fi
  local args=() url
  if [[ $listen == unix:* ]]; then
    args=(--unix-socket "${listen#unix:}")
    url=http://localhost/api/health
  else
    local host=${listen%:*} port=${listen##*:}
    case $host in
      "" | 0.0.0.0) host=127.0.0.1 ;;
      "[::]") host="[::1]" ;;
    esac
    url=http://$host:$port/api/health
  fi
  for _ in 1 2 3 4 5; do
    if curl -fsS "${args[@]}" "$url" >/dev/null 2>&1; then
      info "$(t "Health check OK" "상태 확인 성공")"
      return
    fi
    sleep 1
  done
  warn "$(t "health check failed. See: journalctl -u $SERVICE -e" "상태 확인 실패. 로그: journalctl -u $SERVICE -e")"
}

# next_steps LISTEN GROUP PROXY_UNCHANGED(yes|no): 끝난 뒤의 안내. 리버스 프록시 설정이 그대로 맞으면
# (수신 주소·서비스 사용자를 바꾸지 않은 업그레이드) 로그 위치만 알린다.
next_steps() {
  local listen=$1 group=$2 proxy_unchanged=${3:-no}
  echo
  info "$(t "Done. Logs: journalctl -u $SERVICE -f" "완료. 로그: journalctl -u $SERVICE -f")"
  if [[ $proxy_unchanged == yes ]]; then
    return
  fi
  t "Next: point your reverse proxy (HTTPS) at onmaru. See README.md, 'Reverse proxy'." \
    "다음 단계: 리버스 프록시(HTTPS)가 onmaru를 가리키게 설정하세요. README.md의 '리버스 프록시'를 참고하세요."
  echo
  if [[ $listen == unix:* ]]; then
    echo "  - Caddy:  reverse_proxy unix/${listen#unix:}"
    echo "  - $(t "Proxy in Docker: bind-mount $RUNTIME_DIR:$RUNTIME_DIR into the container." \
      "Docker 안의 프록시: 컨테이너에 $RUNTIME_DIR:$RUNTIME_DIR 을 바인드 마운트하세요.")"
    echo "  - $(t "Proxy on this host: its user must be in the '$group' group (socket mode 0660)." \
      "호스트의 프록시: 프록시 사용자가 '$group' 그룹에 있어야 합니다(소켓 권한 0660).")"
  else
    # 모든 주소에서 수신하면 프록시에는 실제로 닿는 주소를 적어야 한다.
    local host=${listen%:*}
    case $host in
      "" | 0.0.0.0 | "[::]") host="<$(t "this-host-IP" "이-호스트-IP")>" ;;
    esac
    echo "  - Caddy:  reverse_proxy http://$host:${listen##*:}"
  fi
}

main() {
  LANG_UI=$(detect_lang)
  local action=install do_build=1
  while (($#)); do
    # --name=value 형식은 --name value로 나눈다.
    case $1 in
      --user=* | --listen=* | --omv-url=* | --smart-refresh-at=* | --workbench-url=* | --elevate-group=* | --lang=*)
        set -- "${1%%=*}" "${1#*=}" "${@:2}"
        ;;
    esac
    case $1 in
      --uninstall) action=uninstall ;;
      --service-install) action=service-install ;;
      --no-build) do_build=0 ;;
      --upgrade) UPGRADE=1 ;;
      --lang)
        [[ ${2:-} == en || ${2:-} == ko ]] || { usage >&2; exit 2; }
        LANG_UI=$2
        shift
        ;;
      --user | --listen | --omv-url | --smart-refresh-at)
        [[ $# -ge 2 && -n $2 ]] || { usage >&2; exit 2; }
        case $1 in
          --user) S_USER=$2 ;;
          --listen) S_LISTEN=$2 ;;
          --omv-url) S_OMV=$2 ;;
          --smart-refresh-at) S_SMART=$2 ;;
        esac
        shift
        ;;
      # 빈 값(끔)도 받는다.
      --workbench-url)
        [[ $# -ge 2 ]] || { usage >&2; exit 2; }
        S_WORKBENCH=$2
        S_WORKBENCH_SET=true
        shift
        ;;
      --allow-non-loopback) S_ALLOW=true ;;
      --helper) S_HELPER=yes ;;
      --no-helper) S_HELPER=no ;;
      --elevate-group)
        [[ $# -ge 2 && -n $2 ]] || { usage >&2; exit 2; }
        S_ELEVATE_GROUP=$2
        shift
        ;;
      -h | --help) usage; exit 0 ;;
      *) usage >&2; exit 2 ;;
    esac
    shift
  done
  if (( UPGRADE )) && [[ $action == uninstall ]]; then
    usage >&2
    exit 2
  fi
  case $action in
    uninstall)
      as_root uninstall
      ;;
    service-install)
      is_root || die "$(t "--service-install must run as root. Run $SELF without options instead." \
        "--service-install은 root로만 실행할 수 있습니다. 대신 옵션 없이 $SELF 를 실행하세요.")"
      run_root service-install
      ;;
    install)
      # root로 다른 사용자의 저장소를 빌드하면 root 소유 파일이 생긴다(sudo, sudo -i, su 모두).
      # 저장소 소유자가 root가 아니면 root로 빌드하지 않는다.
      if (( do_build )) && is_root && [[ $(stat -c %u "$repo_dir") != 0 ]]; then
        # --upgrade는 묻지 않으므로, 빌드하지 않은 예전 실행 파일을 몰래 설치하지 않고 멈춘다.
        if (( UPGRADE )); then
          die "$(t "running as root on a repository owned by $(stat -c %U "$repo_dir"): the build would create root-owned files. Run $SELF --upgrade as that user without sudo, or add --no-build to install the existing $BIN_SRC." \
            "root로 $(stat -c %U "$repo_dir") 소유 저장소에서 실행했습니다. root로 빌드하면 root 소유 파일이 생깁니다. 그 사용자로 sudo 없이 $SELF --upgrade 를 실행하거나, 기존 $BIN_SRC 를 설치하려면 --no-build를 더하세요.")"
        fi
        warn "$(t "running as root on a repository owned by $(stat -c %U "$repo_dir"): the build is skipped to avoid root-owned files. To build, run $SELF as that user without sudo." \
          "root로 $(stat -c %U "$repo_dir") 소유 저장소에서 실행되어 빌드를 건너뜁니다(root 소유 파일이 생기지 않게). 빌드하려면 그 사용자로 sudo 없이 $SELF 를 실행하세요.")"
        confirm "$(t "Install the existing $BIN_SRC instead?" "대신 기존 $BIN_SRC 로 설치할까요?")" n || exit 1
        do_build=0
      fi
      if (( ! do_build )); then
        require_binary
      fi
      # --upgrade는 기존 설치의 설정을 쓰고, 그 설치에 없는 설정만 아래 ask_settings가 묻는다.
      if (( UPGRADE )); then
        load_installed_settings
      fi
      # 질문은 빌드 전에 모두 받는다. 2단계는 이 값을 옵션으로 받아 묻지 않고 설치한다.
      ask_settings
      check_settings
      show_summary
      if (( ! UPGRADE )); then
        confirm "$(t "Proceed?" "진행할까요?")" y || exit 0
      fi
      if (( do_build )); then
        build
      fi
      require_binary
      as_root service-install
      ;;
  esac
}

main "$@"
