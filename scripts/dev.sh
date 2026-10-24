#!/usr/bin/env bash
# 개발 서버 실행: 백엔드(go run, 루프백)를 백그라운드로, Vite(LAN 공개)를 포그라운드로 띄운다.
# Ctrl+C로 Vite를 끄면 백엔드도 함께 종료된다.
#
#   scripts/dev.sh                    실제 OMV(MARU_OMV_URL)에 연결
#   scripts/dev.sh --fake             가짜 OMV(cmd/fakeomv, 가짜 계정)와 가짜 권한 도우미를 함께 띄워 연결
#   scripts/dev.sh --branch <브랜치>  그 브랜치가 체크아웃된 git 워크트리의 코드로 실행(--fake와 함께 쓸 수 있다)
#
# frontend/.env.local의 MARU_*·DEV_* 값을 환경 변수로 내보내 백엔드와 Vite에 적용한다. 셸 환경 변수가 우선한다.
# git 워크트리에서 실행하거나 --branch로 워크트리를 고르면 주 체크아웃의 frontend/.env.local과 .dev-state를 함께 쓴다.
# 워크트리 자신의 frontend/.env.local이 있으면 그 값이 우선하고, MARU_STATE_DIR로 상태 디렉터리를 따로 둘 수 있다.
# --branch는 이 스크립트가 그 워크트리의 backend·frontend를 실행하므로, 대상 브랜치의 dev.sh 변경은 쓰이지 않는다.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

usage() { echo "usage: $0 [--fake] [--branch <branch>]" >&2; exit 2; }

fake=0
branch=""
while (($#)); do
  case "$1" in
    --fake) fake=1 ;;
    --branch) (($# >= 2)) && [[ -n $2 ]] || usage; branch="$2"; shift ;;
    *) usage ;;
  esac
  shift
done

# --branch: git worktree list에서 refs/heads/<브랜치>가 체크아웃된 워크트리를 찾아 root로 쓴다.
# 한 브랜치는 워크트리 하나에만 체크아웃되므로 결과는 하나뿐이다.
if [[ -n $branch ]]; then
  target=""
  available=()
  path=""
  while IFS= read -r line; do
    case "$line" in
      "worktree "*) path="${line#worktree }" ;;
      "branch refs/heads/"*)
        name="${line#branch refs/heads/}"
        available+=("  $name  $path")
        [[ $name == "${branch#refs/heads/}" ]] && target="$path"
        ;;
    esac
  done < <(git -C "$root" worktree list --porcelain)
  if [[ -z $target ]]; then
    echo "[dev] no git worktree has branch '$branch' checked out. Worktrees:" >&2
    printf '%s\n' "${available[@]}" >&2
    exit 2
  fi
  root="$target"
fi

# 주 체크아웃: 워크트리가 공유하는 .git의 상위 디렉터리. bare 저장소·서브모듈이거나 git이 없으면 이 저장소를 쓴다.
main_root="$root"
if common="$(git -C "$root" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" &&
  [[ "$(basename "$common")" == .git ]]; then
  main_root="$(dirname "$common")"
fi

# 먼저 읽은 파일의 값이 우선한다(이미 설정된 키는 건너뛴다). Vite는 frontend/.env.local만 직접 읽으므로
# 주 체크아웃의 DEV_* 값도 여기서 내보내야 한다. 포트 검사(DEV_PORT)도 Vite와 같은 값을 보게 된다.
env_files=("$root/frontend/.env.local")
[[ $main_root != "$root" ]] && env_files+=("$main_root/frontend/.env.local")
for env_file in "${env_files[@]}"; do
  [[ -f "$env_file" ]] || continue
  while IFS='=' read -r key value; do
    [[ "$key" =~ ^(MARU|DEV)_[A-Z_]+$ ]] || continue
    [[ -n "${!key:-}" ]] && continue
    value="${value%\"}"; value="${value#\"}"; value="${value%\'}"; value="${value#\'}"
    export "$key=$value"
  done < "$env_file"
done

export MARU_LISTEN="${MARU_LISTEN:-127.0.0.1:8090}"
if [[ $fake == 1 ]]; then
  export FAKEOMV_LISTEN="${FAKEOMV_LISTEN:-127.0.0.1:8091}"
  export MARU_OMV_URL="http://$FAKEOMV_LISTEN"
  # "OMV 워크벤치" 링크 앱은 가짜 OMV의 안내 페이지를 연다. 루프백 주소라 NAS 안의 브라우저에서만 열린다.
  export MARU_WORKBENCH_URL="http://$FAKEOMV_LISTEN/"
fi
# 사용자 설정(위젯 배치)은 주 체크아웃의 .dev-state에 둔다. 가짜 계정과 실제 계정의 이름이 겹칠 수 있으므로 나눈다.
export MARU_STATE_DIR="${MARU_STATE_DIR:-$main_root/.dev-state/$([[ $fake == 1 ]] && echo fake || echo real)}"
if [[ $fake == 1 ]]; then
  # 가짜 권한 도우미는 가짜 OMV 프로세스가 상태 디렉터리의 소켓에 띄운다(파일 연산은 helper-root 아래에서만).
  mkdir -p "$MARU_STATE_DIR"
  export FAKEHELPER_SOCKET="$MARU_STATE_DIR/helper.sock"
  export MARU_HELPER_SOCKET="$FAKEHELPER_SOCKET"
  # 웹 터미널은 가짜 sshd에 붙는다. 호스트 공개 키는 가짜 OMV 프로세스가 시작할 때마다 새로 만들어 쓴다.
  # 받는 인증 방법은 FAKESSH_AUTH(예: publickey)로 고른다. 웹 터미널 허용은 설정 앱이나 MARU_TERMINAL로 켠다.
  export FAKESSH_LISTEN="${FAKESSH_LISTEN:-127.0.0.1:8092}"
  export FAKESSH_HOST_KEY="$MARU_STATE_DIR/ssh_host_ed25519_key.pub"
  export MARU_SSH_ADDR="$FAKESSH_LISTEN"
  export MARU_SSH_HOST_KEYS="$FAKESSH_HOST_KEY"
  # 대시보드에서 업그레이드는 가짜 업그레이드 서비스(가짜 OMV 프로세스)가 이 가짜 루트 아래의 설정·상태 파일을 쓴다.
  # 시나리오는 FAKEUPGRADE_SCENARIO(web, manual, latest, check-fail, apply-fail, disabled)로 고른다.
  export MARU_DEV_UPGRADE_ROOT="${MARU_DEV_UPGRADE_ROOT:-$MARU_STATE_DIR/upgrade-root}"
  # 파일 탐색기는 가짜 도우미의 가짜 디스크(helper-root/srv 아래 폴더)를 데이터 디스크로 본다. 가짜 OMV가 그 아래에
  # 합성 파일과 공유 폴더를 만들고, FAKEFILES_LARGE=<n>이면 큰 폴더도 만든다.
  export MARU_DEV_FILES_SRV="${MARU_DEV_FILES_SRV:-$MARU_STATE_DIR/helper-root/srv}"
  # 알림은 가짜 OMV 프로세스가 이 파일에 쓴 가짜 알림을 maru가 읽는다. 시나리오는 FAKENOTICES(sample, none, many,
  # rotate), rotate 주기는 FAKENOTICES_STEP(기본 30s)이다.
  export MARU_DEV_NOTICES="${MARU_DEV_NOTICES:-$MARU_STATE_DIR/dev-notices.json}"
  # 시스템 업데이트 앱과 APT·재부팅 알림은 가짜 OMV 프로세스가 이 가짜 루트에 쓴 업데이트 목록(upgradeindex.json)과
  # 재부팅 권장 파일을 읽는다. 시나리오는 FAKEAPT_SCENARIO(success, fail, core-removal, locked, dirty, slow)다.
  export MARU_DEV_NOTIFY_ROOT="${MARU_DEV_NOTIFY_ROOT:-$MARU_STATE_DIR/notify-root}"
fi

# 이미 떠 있는 서버가 있으면 시작하지 않는다. 일부만 떠서 다른 서버(예: 실제 OMV에 연결된
# 백엔드)에 붙는 일을 막는다. Vite 포트(DEV_PORT)는 vite.config.ts의 server.port와 같은 기본값을 쓴다.
ports=("${MARU_LISTEN##*:}" "${DEV_PORT:-5173}")
[[ $fake == 1 ]] && ports+=("${FAKEOMV_LISTEN##*:}" "${FAKESSH_LISTEN##*:}")
for port in "${ports[@]}"; do
  if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
    echo "[dev] port $port is already in use. Stop the running dev server first." >&2
    exit 1
  fi
done

if [[ ! -d "$root/frontend/node_modules" ]]; then
  echo "[dev] frontend 의존성 설치 중"
  npm --prefix "$root/frontend" install
fi

pids=()
cleanup() {
  for pid in "${pids[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      # go run은 자식 프로세스로 실제 바이너리를 띄우므로 프로세스 그룹 전체에 신호를 보낸다.
      kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT INT TERM

# setsid로 별도 프로세스 그룹을 만들어 cleanup에서 한 번에 종료한다.
if [[ $fake == 1 ]]; then
  echo "[dev] fake omv: http://$FAKEOMV_LISTEN"
  (cd "$root/backend" && exec setsid go run ./cmd/fakeomv) &
  pids+=($!)
fi

echo "[dev] backend: http://$MARU_LISTEN (omv: ${MARU_OMV_URL:-http://127.0.0.1}, workbench: ${MARU_WORKBENCH_URL:-off}, helper: ${MARU_HELPER_SOCKET:-off}, ssh: ${MARU_SSH_ADDR:-auto})"
[[ $root != "$main_root" ]] && echo "[dev] worktree: $root"
echo "[dev] state: $MARU_STATE_DIR"
(cd "$root/backend" && exec setsid go run ./cmd/maru) &
pids+=($!)

cd "$root/frontend"
npx vite
