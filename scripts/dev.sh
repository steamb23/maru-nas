#!/usr/bin/env bash
# 개발 서버 실행: 백엔드(go run, 루프백)를 백그라운드로, Vite(LAN 공개)를 포그라운드로 띄운다.
# Ctrl+C로 Vite를 끄면 백엔드도 함께 종료된다.
#
#   scripts/dev.sh          실제 OMV(OMVDESK_OMV_URL)에 연결
#   scripts/dev.sh --fake   가짜 OMV(cmd/fakeomv, 가짜 계정)를 함께 띄워 연결
#
# frontend/.env.local의 OMVDESK_* 값을 백엔드에도 적용한다. 셸 환경 변수가 우선한다.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

fake=0
case "${1:-}" in
  "") ;;
  --fake) fake=1 ;;
  *) echo "usage: $0 [--fake]" >&2; exit 2 ;;
esac

env_file="$root/frontend/.env.local"
if [[ -f "$env_file" ]]; then
  while IFS='=' read -r key value; do
    [[ "$key" =~ ^OMVDESK_[A-Z_]+$ ]] || continue
    [[ -n "${!key:-}" ]] && continue
    value="${value%\"}"; value="${value#\"}"; value="${value%\'}"; value="${value#\'}"
    export "$key=$value"
  done < "$env_file"
fi

export OMVDESK_LISTEN="${OMVDESK_LISTEN:-127.0.0.1:8090}"
if [[ $fake == 1 ]]; then
  export FAKEOMV_LISTEN="${FAKEOMV_LISTEN:-127.0.0.1:8091}"
  export OMVDESK_OMV_URL="http://$FAKEOMV_LISTEN"
fi
# 사용자 설정(위젯 배치)은 저장소 안 .dev-state에 둔다. 가짜 계정과 실제 계정의 이름이 겹칠 수 있으므로 나눈다.
export OMVDESK_STATE_DIR="${OMVDESK_STATE_DIR:-$root/.dev-state/$([[ $fake == 1 ]] && echo fake || echo real)}"

# 이미 떠 있는 서버가 있으면 시작하지 않는다. 일부만 떠서 다른 서버(예: 실제 OMV에 연결된
# 백엔드)에 붙는 일을 막는다. Vite 포트는 vite.config.ts의 server.port와 맞춘다.
ports=("${OMVDESK_LISTEN##*:}" 5173)
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

echo "[dev] backend: http://$OMVDESK_LISTEN (omv: ${OMVDESK_OMV_URL:-http://127.0.0.1})"
(cd "$root/backend" && exec setsid go run ./cmd/omvdesk) &
pids+=($!)

cd "$root/frontend"
npx vite
