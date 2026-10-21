---
paths:
  - ".claude/hooks/cloud-session.sh"
  - ".claude/settings.json"
---

# 클라우드 세션

Claude Code 클라우드 세션(`CLAUDE_CODE_REMOTE=true`)은 NAS가 아니라 저장소를 새로 클론한 일회용 Ubuntu VM에서 root로 돈다. 저장소의 `CLAUDE.md`·`.claude/rules`·`.claude/docs`만 따라오고 `CLAUDE.local.md`, 사용자 설정, 메모리, OMV, VM 테스트 환경은 없다. 이 파일은 `CLAUDE.md`의 규칙 가운데 클라우드에서 달라지는 것만 적는다. SessionStart 훅(`.claude/hooks/cloud-session.sh`)이 클라우드 세션에서만 이 내용을 문맥에 넣는다.

## 준비

- 훅이 아이콘 서브모듈(`git submodule update --init --depth 1`)과 `npm --prefix frontend ci`를 준비하고 결과를 함께 알린다. 준비 결과가 문맥에 없거나(훅이 돌지 않은 경우) 실패했다고 알리면 같은 명령을 직접 실행해 원인을 확인한다
- 아이콘 서브모듈을 받지 못하면 `make build`와 아이콘을 쓰는 Vitest 테스트가 실패한다. 우회하지 말고 사용자에게 알린다
- 도구 버전이 `CLAUDE.md` "명령어"의 요구 사항에 못 미쳐 실패하면 `go version`·`node -v` 결과와 함께 알린다

## 브랜치와 커밋

- 워크트리를 만들지 않고 세션이 정한 브랜치에서 작업한다. 브랜치 이름 규칙(`<종류>/<짧은-설명>`)보다 세션의 브랜치가 우선한다
- 작업 단위마다 커밋해 그 브랜치에 push한다. `main`에는 병합하거나 push하지 않는다. GitHub 프록시는 push할 브랜치를 가리지 않으므로 이 규칙은 Claude가 지킨다
- 태그 push는 프록시가 거부하므로 "버전"의 릴리스 절차는 하지 않는다
- `gh pr`·`gh issue`는 GraphQL을 써서 프록시가 막는다. GitHub 작업은 내장 GitHub 도구나 `gh api`(REST)로 한다

## 할 수 없는 것과 대신 하는 것

- `sudo` 금지 규칙은 NAS를 지키기 위한 것이므로 이 VM에서 개발 도구를 설치하는 것은 괜찮다. 나머지 "절대 규칙"(OMV 설정 파일, `omv-rpc`, 비밀 값 취급)은 그대로 지킨다
- OMV 소스: `/usr/share/openmediavault`가 없다. OMV GitHub 저장소(`openmediavault/openmediavault`)의 `deb/openmediavault/` 아래에서 같은 경로를 읽는다(`raw.githubusercontent.com`). 플러그인 RPC는 각 플러그인의 GitHub 저장소를 본다
- 테스트는 테스트 전략 1(Go)·2(Vitest)와 `make build`까지 한다
  - root로 돌므로 도우미를 띄우는 Go 테스트와 일반 사용자를 전제로 한 설치기 테스트 일부는 건너뛴다(`.claude/rules/helper.md`의 "테스트")
  - `make dev-fake`는 root에서 가짜 도우미가 시작하지 않아 쓸 수 없고 브라우저도 없으므로 테스트 전략 3은 하지 않는다
  - 테스트 전략 4(실제 OMV)와 VM 테스트 환경의 확인도 할 수 없다
- 여기서 할 수 없는 확인은 아래 "로컬 확인 프롬프트"로 넘긴다

## 로컬 확인 프롬프트

작업을 마칠 때 클라우드에서 할 수 없는 확인이 남았으면, 브랜치를 push한 뒤 마지막 응답에 아래 두 가지를 출력한다.

1. **로컬 Claude Code 세션에서 실행할 확인 프롬프트**: VM 테스트 환경이 있는 로컬 세션이 그대로 실행할 수 있게 쓴다. 다음 중 하나라도 해당하면 쓴다
   - 클라우드에서 건너뛴 Go 테스트가 있는 영역(권한 도우미, 웹 터미널, 설치기)을 바꿨다
   - 화면이 바뀌었다(테스트 전략 3)
   - HTTPS 도메인, root 동작, 실제 sshd·PAM이 필요한 기능을 바꿨다(VM 테스트 환경)
2. **사용자 확인 체크리스트**: 실제 OMV(테스트 전략 4)처럼 사용자만 할 수 있는 확인

확인 프롬프트를 전달하는 방법:

- `spawn_task`(`mcp__ccd_session__spawn_task`)를 쓸 수 있으면 그 카드로 전달한다. 사용자가 Claude 데스크톱 앱에서 카드를 눌러 로컬 클론에서 새 세션으로 시작한다. 제목은 `Verify <브랜치> locally`처럼 동사로 시작하는 짧은 문구, tldr은 무엇을 왜 확인하는지 한두 문장, prompt는 아래 틀을 채운 본문이다. 마지막 응답에는 프롬프트를 다시 붙이지 않고 카드를 띄웠다고만 적는다
- 그 도구가 없거나 호출이 실패하면 마지막 응답에 아래 틀을 채운 코드 블록 하나를 출력한다. 사용자가 로컬 세션에 붙여 넣는다

프롬프트에는 VM 주소·도메인·계정을 쓰지 않는다. 로컬 세션이 `CLAUDE.local.md`에서 읽는다(이 저장소는 공개된다). 로그인해도 되는 대상과 로그인 전 확인은 로컬의 `CLAUDE.md` "테스트 전략"과 `CLAUDE.local.md`를 따르게 한다.

프롬프트 틀(`<…>`를 채우고, 해당하지 않는 줄은 뺀다):

```text
클라우드 세션에서 작업한 `<브랜치>` 브랜치(`<커밋 해시>`)를 이 환경에서 확인해 줘.

변경 요약:
- <바뀐 기능과 주요 파일>

클라우드에서 한 확인:
- `make check`, `make build`: <결과>
- root라서 건너뛴 테스트: <패키지, 없으면 "없음">

준비:
- `git fetch origin <브랜치>` 뒤 `git worktree add .claude/worktrees/<이름> <브랜치>`로 워크트리를 만들어 들어가고, 서브모듈과 `npm --prefix frontend ci`를 실행한다
- 일반 사용자로 `make check`를 실행해 클라우드에서 건너뛴 테스트까지 돌린다

확인 방법:
- <화면 확인: 테스트 전략 3의 개발 서버(Claude용 포트) / 보안·도우미·sshd 확인: `make build` 뒤 CLAUDE.local.md의 VM 테스트 환경에 배포>
- 로그인 대상과 로그인 전 확인은 CLAUDE.md "테스트 전략"과 CLAUDE.local.md를 따른다

확인 항목:
1. <조작> → <기대 결과>
2. <조작> → <기대 결과>

항목마다 통과·실패와 근거(스크린숏, 로그, 명령 출력)를 보고해 줘. 문제를 찾으면 고치기 전에 먼저 알려 주고, 이 브랜치에 push하지 마.
```
