#!/usr/bin/env bash
# 온마루(Onmaru) 설치 스크립트. 빌드한 뒤 systemd 서비스로 설치하거나 다시 설치(업그레이드)한다.
#
#   ./install.sh                    1단계: 실행한 계정으로 빌드한다(일반 사용자 또는 root만 있는 환경의 root)
#                                   2단계: root가 아니면 sudo로 자신을 --service-install로 다시 실행한다
#   ./install.sh --no-build         빌드를 건너뛰고 기존 bin/onmaru를 설치한다
#   ./install.sh --uninstall        제거한다(root가 아니면 sudo로 다시 실행한다)
#   ./install.sh --service-install  2단계만 실행한다(root 전용). 설정 질문, 사용자 생성, 유닛 작성, 시작
#   --lang en|ko                    안내 문구 언어. 생략하면 로캘(LC_ALL, LC_MESSAGES, LANG)로 고른다.
#                                   sudo로 다시 실행할 때는 명시적으로 넘겨 같은 언어를 유지한다
#
# 일반 사용자가 sudo ./install.sh로 실행하면 root로 빌드하지 않는다(저장소에 root 소유 파일이 생기고,
# sudo의 PATH에는 사용자의 go·npm이 없을 수 있다). 이때는 기존 실행 파일로 설치만 할지 묻는다.
#
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

root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SELF=$root/$(basename "${BASH_SOURCE[0]}")
readonly SELF
readonly BIN_SRC=$root/bin/onmaru
readonly DIST_INDEX=$root/backend/internal/web/static/dist/index.html

# 안내 문구 언어(en, ko). main에서 정한다.
LANG_UI=en

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

# env_get KEY: 기존 환경 파일에서 값을 읽는다(source 하지 않는다).
env_get() {
  [[ -f $ENV_FILE ]] || return 0
  sed -n "s/^$1=//p" "$ENV_FILE" | tail -n 1 | sed 's/^"\(.*\)"$/\1/'
}

is_loopback_tcp() {
  [[ $1 =~ ^127\.[0-9]+\.[0-9]+\.[0-9]+:[0-9]+$ || $1 =~ ^\[::1\]:[0-9]+$ ]]
}

# detect_omv_url: OMV 웹 UI(nginx)의 수신 포트로 루프백 주소를 만든다.
# 80/443을 앞단 프록시에 내주고 OMV를 다른 포트로 옮긴 경우가 있어 설정 파일에서 읽는다.
detect_omv_url() {
  local port=""
  if [[ -r $NGINX_WEBGUI ]]; then
    port=$(sed -nE '/ssl/d; s/^[[:space:]]*listen[[:space:]]+([^;[:space:]]*:)?([0-9]+)([[:space:];].*)?$/\2/p' "$NGINX_WEBGUI" | head -n 1)
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
사용법: $0 [--no-build | --uninstall | --service-install] [--lang en|ko]

  (옵션 없음)        빌드한 뒤 설치하거나 업그레이드합니다. 설치 단계에서 필요하면 sudo를 사용합니다
  --no-build         빌드를 건너뛰고 기존 bin/onmaru를 설치합니다
  --uninstall        서비스, 실행 파일, 설정을 제거합니다
  --service-install  빌드 없이 설치 단계만 실행합니다(root 전용)
  --lang en|ko       안내 문구 언어(기본값은 로캘에 따름)

일반 사용자는 sudo 없이 실행하세요. 빌드는 그 사용자로 하고, 설치 단계만 sudo로 실행합니다.
EOF
  else
    cat <<EOF
Usage: $0 [--no-build | --uninstall | --service-install] [--lang en|ko]

  (no option)        build, then install or upgrade. Uses sudo for the install step when needed
  --no-build         skip the build and install the existing bin/onmaru
  --uninstall        remove the service, binary and settings
  --service-install  run only the install step, without building (root only)
  --lang en|ko       language of the messages (default: from the locale)

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
  if [[ -z $(ls -A "$root/vendor/icons/colloid" 2>/dev/null) ]]; then
    command -v git >/dev/null 2>&1 || die "$(t "'git' is required to fetch the icon submodule" "아이콘 서브모듈을 받으려면 'git'이 필요합니다")"
    info "$(t "Fetching the icon submodule" "아이콘 서브모듈 받는 중")"
    git -C "$root" submodule update --init --depth 1
  fi
  if [[ ! -d $root/frontend/node_modules ]]; then
    info "$(t "Installing frontend dependencies" "프론트엔드 의존성 설치 중")"
    npm --prefix "$root/frontend" ci
  fi
  make -C "$root" build
  info "$(t "Build finished: $BIN_SRC" "빌드 완료: $BIN_SRC")"
}

# as_root ACTION(service-install|uninstall): root면 바로 실행하고, 아니면 sudo로 자신을 다시 실행한다.
as_root() {
  local action=$1
  if is_root; then
    run_root "$action"
    return
  fi
  command -v sudo >/dev/null 2>&1 ||
    die "$(t "root privileges are required and sudo is not installed. Run this as root." \
      "root 권한이 필요한데 sudo가 없습니다. root로 실행하세요.")"
  info "$(t "Root privileges are needed for the next step; running sudo" "다음 단계에는 root 권한이 필요해 sudo를 실행합니다")"
  exec sudo "$SELF" "--$action" --lang "$LANG_UI"
}

run_root() {
  command -v systemctl >/dev/null 2>&1 || die "$(t "systemd is required" "systemd가 필요합니다")"
  case $1 in
    service-install) install_service ;;
    uninstall) uninstall ;;
  esac
}

uninstall() {
  info "$(t "Uninstalling $SERVICE" "$SERVICE 제거")"
  confirm "$(t "Stop the service and remove $UNIT_FILE, $BIN_DST and $ENV_FILE?" \
    "서비스를 멈추고 $UNIT_FILE, $BIN_DST, $ENV_FILE 파일을 지울까요?")" n || exit 0
  if systemctl list-unit-files "$SERVICE.service" >/dev/null 2>&1; then
    systemctl disable --now "$SERVICE.service" 2>/dev/null || true
  fi
  local user=""
  [[ -f $UNIT_FILE ]] && user=$(sed -n 's/^User=//p' "$UNIT_FILE" | head -n 1)
  rm -f -- "$UNIT_FILE" "$BIN_DST" "$ENV_FILE"
  systemctl daemon-reload
  # RuntimeDirectoryPreserve=yes라 서비스를 멈춰도 남는다. 재부팅하면 어차피 사라진다.
  rm -rf -- "$RUNTIME_DIR"
  info "$(t "Removed service, binary and settings (backups $ENV_FILE.bak-* are kept)" \
    "서비스, 실행 파일, 설정을 지웠습니다(백업 $ENV_FILE.bak-* 은 남겨 둡니다)")"

  if [[ -d $STATE_DIR ]] && confirm "$(t "Also delete $STATE_DIR (per-user widget layouts)?" \
    "$STATE_DIR(사용자별 위젯 배치)도 지울까요?")" n; then
    rm -rf -- "$STATE_DIR"
    info "$(t "Removed $STATE_DIR" "$STATE_DIR 삭제")"
  fi
  if [[ -n $user ]] && id -u "$user" >/dev/null 2>&1 && confirm "$(t "Also delete the system user '$user'?" \
    "시스템 사용자 '$user'도 지울까요?")" n; then
    userdel "$user"
    info "$(t "Removed user $user" "사용자 $user 삭제")"
  fi
  info "$(t "If a reverse proxy pointed at onmaru, remove that configuration as well." \
    "리버스 프록시에 onmaru 설정이 있다면 그것도 지워 주세요.")"
}

install_service() {
  [[ -x $BIN_SRC ]] || die "$(t "$BIN_SRC not found. Run $SELF without options to build it first." \
    "$BIN_SRC 파일이 없습니다. 먼저 옵션 없이 $SELF 를 실행해 빌드하세요.")"
  if [[ ! -f $DIST_INDEX ]]; then
    warn "$(t "frontend build not found ($DIST_INDEX). The binary will serve a 'frontend not built' page." \
      "프론트엔드 빌드가 없습니다($DIST_INDEX). 화면 대신 'frontend not built' 안내만 나옵니다.")"
    confirm "$(t "Continue anyway?" "그래도 계속할까요?")" n || exit 1
  fi

  if [[ -f $UNIT_FILE ]]; then
    info "$(t "Existing installation found. Current settings are used as defaults." \
      "기존 설치를 찾았습니다. 현재 설정을 기본값으로 씁니다.")"
  fi

  # 1. 서비스 사용자
  local user_def=""
  if [[ -f $UNIT_FILE ]]; then
    user_def=$(sed -n 's/^User=//p' "$UNIT_FILE" | head -n 1)
  fi
  ask "$(t "Service user (created as a system user if missing)" "서비스 사용자(없으면 시스템 사용자로 만듭니다)")" \
    "${user_def:-$SERVICE}"
  local user=$REPLY
  [[ $user =~ ^[a-z_][a-z0-9_-]*$ ]] || die "$(t "invalid user name: $user" "사용자 이름이 올바르지 않습니다: $user")"
  [[ $user != root ]] || die "$(t "do not run onmaru as root" "onmaru를 root로 실행하지 마세요")"

  # 2. 수신 방식
  local cur_listen cur_allow mode_def
  cur_listen=$(env_get ONMARU_LISTEN)
  cur_allow=$(env_get ONMARU_ALLOW_NON_LOOPBACK)
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
  local listen allow=false
  case $REPLY in
    1)
      ask "$(t "Listen address" "수신 주소")" "$(is_loopback_tcp "$cur_listen" && echo "$cur_listen" || echo 127.0.0.1:8090)"
      listen=$REPLY
      is_loopback_tcp "$listen" || die "$(t "not a loopback address: $listen (use option 3 for other addresses)" \
        "루프백 주소가 아닙니다: $listen (다른 주소는 3번을 고르세요)")"
      ;;
    2)
      listen=unix:$SOCKET_PATH
      ;;
    3)
      ask "$(t "Listen address" "수신 주소")" "$([[ $mode_def == 3 ]] && echo "$cur_listen" || echo 0.0.0.0:8090)"
      listen=$REPLY
      [[ $listen =~ ^(\[[0-9a-fA-F:]+\]|[0-9.]*):[0-9]+$ ]] || die "$(t "invalid address: $listen (use host:port, e.g. 0.0.0.0:8090)" \
        "주소가 올바르지 않습니다: $listen (host:port 형식, 예: 0.0.0.0:8090)")"
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
      if [[ $cur_allow != true ]]; then
        read -r -p "$(t "Type 'yes' to accept the risk: " "위험을 감수하려면 'yes'를 입력하세요: ")" REPLY || exit 1
        [[ $REPLY == yes ]] || die "$(t "aborted" "중단했습니다")"
      fi
      allow=true
      ;;
    *) die "$(t "invalid choice: $REPLY" "잘못된 선택입니다: $REPLY")" ;;
  esac

  # 3. OMV 주소
  local omv_def
  omv_def=$(env_get ONMARU_OMV_URL)
  ask "$(t "OMV web UI address (loopback, detected from nginx)" "OMV 웹 UI 주소(루프백, nginx 설정에서 찾음)")" \
    "${omv_def:-$(detect_omv_url)}"
  local omv_url=$REPLY
  [[ $omv_url =~ ^https?://(127\.[0-9.]+|localhost|\[::1\])(:[0-9]+)?/?$ ]] ||
    die "$(t "OMV address must be a loopback http(s) URL: $omv_url" "OMV 주소는 루프백 http(s) URL이어야 합니다: $omv_url")"

  # 4. SMART 갱신 시각
  local smart_def
  smart_def=$(env_get ONMARU_SMART_REFRESH_AT)
  ask "$(t "Daily SMART refresh time (HH:MM, local time)" "SMART 정보를 매일 다시 가져올 시각(HH:MM, 서버 시간)")" \
    "${smart_def:-00:00}"
  local smart_at=$REPLY
  [[ $smart_at =~ ^([01][0-9]|2[0-3]):[0-5][0-9]$ ]] || die "$(t "invalid time: $smart_at" "시각이 올바르지 않습니다: $smart_at")"

  local allow_note=""
  [[ $allow == true ]] && allow_note="  ($(t "non-loopback allowed" "루프백 외 주소 허용"))"
  if [[ $LANG_UI == ko ]]; then
    cat <<EOF

요약
  실행 파일     $BIN_SRC -> $BIN_DST
  서비스 사용자 $user
  수신          $listen$allow_note
  OMV 주소      $omv_url
  SMART 갱신    $smart_at
  상태 디렉터리 $STATE_DIR
  설정 파일     $UNIT_FILE, $ENV_FILE

EOF
  else
    cat <<EOF

Summary
  binary        $BIN_SRC -> $BIN_DST
  service user  $user
  listen        $listen$allow_note
  OMV address   $omv_url
  SMART refresh $smart_at
  state dir     $STATE_DIR
  files         $UNIT_FILE, $ENV_FILE

EOF
  fi
  confirm "$(t "Proceed?" "진행할까요?")" y || exit 0

  if ! id -u "$user" >/dev/null 2>&1; then
    info "$(t "Creating system user $user" "시스템 사용자 $user 생성")"
    useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin "$user"
  fi

  info "$(t "Installing $BIN_DST" "$BIN_DST 설치")"
  install -o root -g root -m 0755 "$BIN_SRC" "$BIN_DST"

  write_file "$ENV_FILE" yes <<EOF
# onmaru settings. Written by install.sh; re-run it to change them.
# See README.md for every option.
ONMARU_LISTEN=$listen
ONMARU_ALLOW_NON_LOOPBACK=$allow
ONMARU_OMV_URL=$omv_url
ONMARU_SMART_REFRESH_AT=$smart_at
EOF

  # 유닛은 위 값으로 언제든 다시 만들 수 있으므로 백업하지 않는다.
  write_file "$UNIT_FILE" no <<EOF
# Written by onmaru install.sh
[Unit]
Description=Onmaru dashboard for openmediavault
After=network.target

[Service]
Type=simple
User=$user
Group=$user
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
PrivateDevices=yes
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
  systemctl enable "$SERVICE.service" >/dev/null
  if systemctl is-active --quiet "$SERVICE.service"; then
    info "$(t "Restarting $SERVICE" "$SERVICE 재시작")"
    systemctl restart "$SERVICE.service"
  else
    info "$(t "Starting $SERVICE" "$SERVICE 시작")"
    systemctl start "$SERVICE.service"
  fi

  health_check "$listen"
  next_steps "$listen" "$user"
}

health_check() {
  local listen=$1
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

next_steps() {
  local listen=$1 user=$2
  echo
  info "$(t "Done. Logs: journalctl -u $SERVICE -f" "완료. 로그: journalctl -u $SERVICE -f")"
  t "Next: point your reverse proxy (HTTPS) at onmaru. See README.md, 'Reverse proxy'." \
    "다음 단계: 리버스 프록시(HTTPS)가 onmaru를 가리키게 설정하세요. README.md의 '리버스 프록시'를 참고하세요."
  echo
  if [[ $listen == unix:* ]]; then
    echo "  - Caddy:  reverse_proxy unix/${listen#unix:}"
    echo "  - $(t "Proxy in Docker: bind-mount $RUNTIME_DIR:$RUNTIME_DIR into the container." \
      "Docker 안의 프록시: 컨테이너에 $RUNTIME_DIR:$RUNTIME_DIR 을 바인드 마운트하세요.")"
    echo "  - $(t "Proxy on this host: its user must be in the '$user' group (socket mode 0660)." \
      "호스트의 프록시: 프록시 사용자가 '$user' 그룹에 있어야 합니다(소켓 권한 0660).")"
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
    case $1 in
      --uninstall) action=uninstall ;;
      --service-install) action=service-install ;;
      --no-build) do_build=0 ;;
      --lang)
        [[ ${2:-} == en || ${2:-} == ko ]] || { usage >&2; exit 2; }
        LANG_UI=$2
        shift
        ;;
      --lang=en | --lang=ko) LANG_UI=${1#--lang=} ;;
      -h | --help) usage; exit 0 ;;
      *) usage >&2; exit 2 ;;
    esac
    shift
  done
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
      # 일반 사용자가 sudo로 실행했다면 root로 빌드하지 않는다.
      if (( do_build )) && is_root && [[ -n ${SUDO_USER:-} && $SUDO_USER != root ]]; then
        warn "$(t "running under sudo: the build is skipped to avoid root-owned files in the repository. To build, run $SELF without sudo." \
          "sudo로 실행되어 빌드를 건너뜁니다(저장소에 root 소유 파일이 생기지 않게). 빌드하려면 sudo 없이 $SELF 를 실행하세요.")"
        confirm "$(t "Install the existing $BIN_SRC instead?" "대신 기존 $BIN_SRC 로 설치할까요?")" n || exit 1
        do_build=0
      fi
      if (( do_build )); then
        build
      fi
      as_root service-install
      ;;
  esac
}

main "$@"
