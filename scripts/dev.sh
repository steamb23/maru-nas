#!/usr/bin/env bash
# 개발 서버 실행: 백엔드(go run, 루프백)를 백그라운드로, Vite(LAN 공개)를 포그라운드로 띄운다.
# Ctrl+C로 Vite를 끄면 백엔드도 함께 종료된다.
#
#   scripts/dev.sh          실제 OMV(ONMARU_OMV_URL)에 연결
#   scripts/dev.sh --fake   가짜 OMV(cmd/fakeomv, 가짜 계정)를 함께 띄워 연결
#
# frontend/.env.local의 ONMARU_*·DEV_* 값을 환경 변수로 내보내 백엔드와 Vite에 적용한다. 셸 환경 변수가 우선한다.
# git 워크트리에서 실행하면 주 체크아웃의 frontend/.env.local과 .dev-state를 함께 쓴다.
# 워크트리 자신의 frontend/.env.local이 있으면 그 값이 우선하고, ONMARU_STATE_DIR로 상태 디렉터리를 따로 둘 수 있다.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# 주 체크아웃: 워크트리가 공유하는 .git의 상위 디렉터리. bare 저장소·서브모듈이거나 git이 없으면 이 저장소를 쓴다.
main_root="$root"
if common="$(git -C "$root" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" &&
  [[ "$(basename "$common")" == .git ]]; then
  main_root="$(dirname "$common")"
fi

fake=0
case "${1:-}" in
  "") ;;
  --fake) fake=1 ;;
  *) echo "usage: $0 [--fake]" >&2; exit 2 ;;
esac

# 먼저 읽은 파일의 값이 우선한다(이미 설정된 키는 건너뛴다). Vite는 frontend/.env.local만 직접 읽으므로
# 주 체크아웃의 DEV_* 값도 여기서 내보내야 한다. 포트 검사(DEV_PORT)도 Vite와 같은 값을 보게 된다.
env_files=("$root/frontend/.env.local")
[[ $main_root != "$root" ]] && env_files+=("$main_root/frontend/.env.local")
for env_file in "${env_files[@]}"; do
  [[ -f "$env_file" ]] || continue
  while IFS='=' read -r key value; do
    [[ "$key" =~ ^(ONMARU|DEV)_[A-Z_]+$ ]] || continue
    [[ -n "${!key:-}" ]] && continue
    value="${value%\"}"; value="${value#\"}"; value="${value%\'}"; value="${value#\'}"
    export "$key=$value"
  done < "$env_file"
done

export ONMARU_LISTEN="${ONMARU_LISTEN:-127.0.0.1:8090}"
if [[ $fake == 1 ]]; then
  export FAKEOMV_LISTEN="${FAKEOMV_LISTEN:-127.0.0.1:8091}"
  export ONMARU_OMV_URL="http://$FAKEOMV_LISTEN"
  # "OMV 워크벤치" 링크 앱은 가짜 OMV의 안내 페이지를 연다. 루프백 주소라 NAS 안의 브라우저에서만 열린다.
  export ONMARU_WORKBENCH_URL="http://$FAKEOMV_LISTEN/"
fi
# 사용자 설정(위젯 배치)은 주 체크아웃의 .dev-state에 둔다. 가짜 계정과 실제 계정의 이름이 겹칠 수 있으므로 나눈다.
export ONMARU_STATE_DIR="${ONMARU_STATE_DIR:-$main_root/.dev-state/$([[ $fake == 1 ]] && echo fake || echo real)}"

# 이미 떠 있는 서버가 있으면 시작하지 않는다. 일부만 떠서 다른 서버(예: 실제 OMV에 연결된
# 백엔드)에 붙는 일을 막는다. Vite 포트(DEV_PORT)는 vite.config.ts의 server.port와 같은 기본값을 쓴다.
ports=("${ONMARU_LISTEN##*:}" "${DEV_PORT:-5173}")
[[ $fake == 1 ]] && ports+=("${FAKEOMV_LISTEN##*:}")
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

echo "[dev] backend: http://$ONMARU_LISTEN (omv: ${ONMARU_OMV_URL:-http://127.0.0.1}, workbench: ${ONMARU_WORKBENCH_URL:-off})"
echo "[dev] state: $ONMARU_STATE_DIR"
(cd "$root/backend" && exec setsid go run ./cmd/onmaru) &
pids+=($!)

cd "$root/frontend"
npx vite
