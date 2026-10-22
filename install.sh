#!/usr/bin/env bash
# 온마루(Onmaru) 설치 스크립트. Go 설치기(installer/ → bin/maru-install)를 빌드하거나 찾아 실행한다.
# 옵션과 동작은 모두 설치기가 정한다: ./install.sh --help
#
# 설치기는 실행한 계정으로 빌드한다. root가 일반 사용자 소유 저장소에서 실행하면(sudo ./install.sh, sudo -i, su)
# 저장소에 root 소유 파일이 생기지 않게 빌드하지 않고 이미 있는 bin/maru-install을 쓴다(설치기도 같은 규칙으로
# maru를 빌드하지 않는다). go가 없을 때도 이미 있는 bin/maru-install을 쓴다.
set -euo pipefail

self=$(readlink -f "${BASH_SOURCE[0]}")
repo=$(dirname "$self")
installer=$repo/bin/maru-install

# 안내 문구 언어: --lang en|ko, 없으면 로캘(LC_ALL, LC_MESSAGES, LANG)이 ko로 시작하면 한국어. 설치기와 같은 규칙이다.
lang=""
prev=""
for arg in "$@"; do
  case $arg in
    --lang=*) lang=${arg#--lang=} ;;
    *) [[ $prev == --lang ]] && lang=$arg ;;
  esac
  prev=$arg
done
if [[ $lang != en && $lang != ko ]]; then
  locale=${LC_ALL:-${LC_MESSAGES:-${LANG:-}}}
  if [[ $locale == ko* ]]; then lang=ko; else lang=en; fi
fi

t() { if [[ $lang == ko ]]; then printf '%s' "$2"; else printf '%s' "$1"; fi; }
# 이 스크립트의 안내는 모두 오류 출력에 쓴다. 표준 출력은 설치기의 것이다(--upgrade-plan의 JSON 등).
blue="" red="" reset=""
if [[ -t 2 ]]; then blue=$'\033[1;34m' red=$'\033[1;31m' reset=$'\033[0m'; fi
info() { printf '%s==>%s %s\n' "$blue" "$reset" "$(t "$1" "$2")" >&2; }
die() { printf '%s%s%s %s\n' "$red" "$(t error: 오류:)" "$reset" "$(t "$1" "$2")" >&2; exit 1; }

owner_uid=$(stat -c %u "$repo")
if [[ $EUID -eq 0 && $owner_uid != 0 ]]; then
  owner=$(stat -c %U "$repo")
  [[ -x $installer ]] ||
    die "running as root on a repository owned by $owner, and $installer does not exist. Run $self as that user without sudo." \
      "root로 $owner 소유 저장소에서 실행했는데 $installer 가 없습니다. 그 사용자로 sudo 없이 $self 를 실행하세요."
elif command -v go >/dev/null 2>&1; then
  info "Building the installer ($installer)" "설치기 빌드 중($installer)"
  (cd "$repo/installer" && go build -trimpath -o "$installer" .) ||
    die "could not build the installer (installer/). See the messages above." \
      "설치기(installer/)를 빌드하지 못했습니다. 위의 메시지를 확인하세요."
elif [[ ! -x $installer ]]; then
  die "'go' not found in PATH and $installer does not exist. Install Go (README.md, \"Build and install\"), or copy bin/ built elsewhere with 'make build'." \
    "PATH에서 'go'를 찾을 수 없고 $installer 도 없습니다. Go를 설치하거나(README.md의 \"빌드 및 설치\"), 다른 곳에서 'make build'로 빌드한 bin/을 가져오세요."
fi

exec "$installer" "$@"
