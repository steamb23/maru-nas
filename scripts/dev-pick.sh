#!/usr/bin/env bash
# 대화형 개발 서버 실행: git 워크트리(브랜치)와 OMV 연결(실제·가짜)을 번호로 골라 scripts/dev.sh --branch로 띄운다.
# make dev BRANCH=<브랜치>·make dev-fake BRANCH=<브랜치>를 브랜치 이름 입력 없이 실행하기 위한 것이다.
#
#   scripts/dev-pick.sh           워크트리와 OMV 연결을 모두 묻는다
#   scripts/dev-pick.sh --fake    가짜 OMV로 정하고 워크트리만 묻는다(--real은 실제 OMV)
#
# 브랜치가 체크아웃되지 않은 워크트리(detached HEAD)와 디렉터리가 없어진 워크트리는 고를 수 없으므로 목록에서 뺀다.
# 환경 변수(DEV_PORT, ONMARU_* 등)는 그대로 dev.sh에 넘어간다.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

usage() { echo "usage: $0 [--fake | --real]" >&2; exit 2; }

mode=""
while (($#)); do
  case "$1" in
    --fake) mode=fake ;;
    --real) mode=real ;;
    *) usage ;;
  esac
  shift
done

# 프롬프트를 보이고 한 줄을 reply에 읽는다. 입력이 끝나면(EOF, Ctrl+D) 취소로 보고 끝낸다.
reply=""
ask() {
  if ! IFS= read -r -p "$1" reply; then
    echo >&2
    echo "[dev] cancelled" >&2
    exit 1
  fi
}

branches=()
paths=()
path=""
while IFS= read -r line; do
  case "$line" in
    "worktree "*) path="${line#worktree }" ;;
    "branch refs/heads/"*)
      if [[ -d $path ]]; then
        branches+=("${line#branch refs/heads/}")
        paths+=("$path")
      fi
      ;;
  esac
done < <(git -C "$root" worktree list --porcelain)

count=${#branches[@]}
if ((count == 0)); then
  echo "[dev] no git worktree with a branch checked out" >&2
  exit 1
fi

width=0
for name in "${branches[@]}"; do
  if ((${#name} > width)); then width=${#name}; fi
done

echo "[dev] worktrees:"
for i in "${!branches[@]}"; do
  info="$(git -C "${paths[i]}" log -1 --format='%<(14)%cr  %<(50,trunc)%s' 2>/dev/null || true)"
  printf '  %2d) %-*s  %s\n' $((i + 1)) "$width" "${branches[i]}" "$info"
done

if ((count == 1)); then
  choice=1
else
  while :; do
    ask "Worktree [1-$count]: "
    if [[ $reply =~ ^[0-9]+$ ]] && ((10#$reply >= 1 && 10#$reply <= count)); then
      choice=$((10#$reply))
      break
    fi
    echo "[dev] enter a number from the list" >&2
  done
fi
branch="${branches[choice - 1]}"

if [[ -z $mode ]]; then
  echo "[dev] OMV:"
  echo "   1) real  (make dev)"
  echo "   2) fake  (make dev-fake)"
  while :; do
    ask "OMV [1]: "
    case "$reply" in
      "" | 1) mode=real; break ;;
      2) mode=fake; break ;;
      *) echo "[dev] enter 1 or 2" >&2 ;;
    esac
  done
fi

args=(--branch "$branch")
if [[ $mode == fake ]]; then args=(--fake "${args[@]}"); fi
echo "[dev] scripts/dev.sh ${args[*]}"
exec "$root/scripts/dev.sh" "${args[@]}"
