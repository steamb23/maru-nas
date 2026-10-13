#!/usr/bin/env bash
# 개발 서버 실행: 백엔드(go run, 루프백)를 백그라운드로, Vite(LAN 공개)를 포그라운드로 띄운다.
# Ctrl+C로 Vite를 끄면 백엔드도 함께 종료된다.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
export OMVDESK_LISTEN="${OMVDESK_LISTEN:-127.0.0.1:8090}"

if [[ ! -d "$root/frontend/node_modules" ]]; then
  echo "[dev] frontend 의존성 설치 중"
  npm --prefix "$root/frontend" install
fi

backend_pid=""
cleanup() {
  if [[ -n "$backend_pid" ]] && kill -0 "$backend_pid" 2>/dev/null; then
    # go run은 자식 프로세스로 실제 바이너리를 띄우므로 프로세스 그룹 전체에 신호를 보낸다.
    kill -TERM -- "-$backend_pid" 2>/dev/null || kill -TERM "$backend_pid" 2>/dev/null || true
    wait "$backend_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

echo "[dev] backend: http://$OMVDESK_LISTEN"
# setsid로 별도 프로세스 그룹을 만들어 cleanup에서 한 번에 종료한다.
(cd "$root/backend" && exec setsid go run ./cmd/omvdesk) &
backend_pid=$!

cd "$root/frontend"
npx vite
