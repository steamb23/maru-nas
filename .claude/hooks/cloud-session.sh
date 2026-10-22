#!/usr/bin/env bash
# Claude Code SessionStart 훅(.claude/settings.json). 클라우드 세션(CLAUDE_CODE_REMOTE=true)에서만 동작하고
# 로컬 세션에서는 바로 끝난다. 규칙은 .claude/rules/cloud.md.
#
#   startup·resume: 아이콘 서브모듈과 frontend 의존성을 준비한다(이미 있으면 건너뛴다)
#   모든 경우(compact 포함): .claude/rules/cloud.md 본문을 내보낸다. 경로 범위 규칙은 압축하면 빠지므로 다시 넣는다
#
# SessionStart 훅의 표준 출력은 그대로 Claude의 문맥이 되고 10,000자까지만 들어간다.
# 그래서 명령 출력은 로그 파일로 보내고 결과만 한 줄씩 알린다. 실패해도 규칙은 내보내도록 늘 0으로 끝난다.
set -uo pipefail

[[ ${CLAUDE_CODE_REMOTE:-} == true ]] || exit 0

root="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
log="${TMPDIR:-/tmp}/maru-cloud-session.log"

# 훅 입력(JSON 한 줄)의 source: startup, resume, compact 등
event="$(sed -n 's/.*"source"[[:space:]]*:[[:space:]]*"\([a-z]*\)".*/\1/p' | head -n 1)"

status=()
if [[ $event != compact ]]; then
  if [[ -n $(ls -A "$root/vendor/icons/colloid" 2>/dev/null) ]]; then
    status+=("icon submodule: present")
  elif git -C "$root" submodule update --init --depth 1 >>"$log" 2>&1; then
    status+=("icon submodule: fetched")
  else
    status+=("icon submodule: FAILED (see $log)")
  fi

  # package-lock.json이 그대로면 npm ci를 다시 하지 않는다
  stamp="$root/frontend/node_modules/.cloud-session.sha256"
  want="$(sha256sum "$root/frontend/package-lock.json" | cut -d' ' -f1)"
  if [[ -f $stamp && $(<"$stamp") == "$want" ]]; then
    status+=("frontend dependencies: present")
  elif npm --prefix "$root/frontend" ci >>"$log" 2>&1; then
    echo "$want" >"$stamp"
    status+=("frontend dependencies: installed")
  else
    status+=("frontend dependencies: FAILED (see $log)")
  fi
fi

if ((${#status[@]})); then
  echo "Cloud session setup (.claude/hooks/cloud-session.sh):"
  printf -- '- %s\n' "${status[@]}"
  echo
fi
# 앞머리(paths)를 빼고 본문만 내보낸다
awk 'NR == 1 && $0 == "---" { fm = 1; next } fm && $0 == "---" { fm = 0; next } !fm' "$root/.claude/rules/cloud.md"
exit 0
