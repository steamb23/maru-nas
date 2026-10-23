---
paths:
  - "backend/internal/terminal/**"
  - "backend/internal/server/terminal*.go"
  - "frontend/src/terminal/**"
---

# 웹 터미널

사용자 설명은 `docs/en/terminal.md`, 도우미 쪽 임시 키 연산은 `.claude/docs/helper.md`의 "임시 SSH 키".

## 설계

- **SSH 방식이다.** maru는 같은 호스트의 sshd(루프백)에 붙는 SSH 클라이언트(`golang.org/x/crypto/ssh`)이고, PTY는 sshd가 할당한다(`pty-req`). maru 쪽에서 PTY·셸 프로세스를 직접 띄우지 않으며(유닛의 `DevicePolicy=closed`를 바꾸지 않는다), 도우미에 셸·명령 실행 연산을 두지 않는다
  - TTY 중계를 택하지 않은 이유: 도우미의 셸 연산은 "임의 명령 실행 연산 금지" 규칙에 걸리고, 도우미가 띄운 셸은 도우미의 샌드박스(`AF_UNIX`만, `ProtectSystem=strict`, `NoNewPrivileges`)를 물려받아 네트워크·sudo가 막힌다. 하드닝 없는 root 데몬으로 `login`을 띄우는 것은 sshd를 새 root 코드로 다시 만드는 셈이다
  - sshd는 원래 LAN에 열려 있으므로 maru가 뚫려도 새로 얻는 권한이 없다. 누가 어떻게 들어갈 수 있는지(`AllowGroups`, 인증 방식, root 로그인)는 OMV의 SSH 설정이 정하고 maru는 sshd 설정을 읽기만 한다
- 접속 계정은 대시보드 세션 사용자로 고정한다. 다른 계정·root를 고르는 기능을 만들지 않는다(범용 SSH 게이트웨이가 되지 않게)
- 주소: `-ssh-addr`(기본 `auto` = `sshd_config`의 첫 `Port`로 `127.0.0.1`)는 루프백 IP만 받는다(`terminal.CheckAddr`). `sshd_config`는 첫 `Match` 줄부터 읽지 않는다(`ParseSSHDConfig`)
- 호스트 키: 접속할 때마다 `-ssh-host-keys`(`/etc/ssh/ssh_host_*_key.pub`)를 읽어 그중 하나와 같은 서버 키만 받는다. TOFU·known_hosts를 쓰지 않는다. `ssh.FixedHostKey`는 키 하나만 받으므로 쓰지 않는다
- 인증 순서(`Dialer.Dial`, 서버가 허용하는 것만 시도된다): 도우미 임시 키 → keyboard-interactive → password
  - 임시 키: ed25519 키를 메모리에서 만들어 `ssh.authorize`로 넣고, 핸드셰이크가 끝나면 성공·실패와 관계없이 `ssh.revoke`로 지운다. 넣지 못하면 오류 대신 빈 서명자를 돌려줘 다음 방법으로 넘어간다(도우미 오류 코드는 `Error.KeyCode` 힌트)
  - 비밀번호·keyboard-interactive는 WebSocket `prompt`로 브라우저에 묻고 답을 그대로 넘긴다. 저장·기록하지 않는다. x/crypto는 방법 하나가 오류를 내도 다음 방법을 시도하므로, 한 번 취소하면 다시 묻지 않게 기억한다(`authState.canceled`)
  - sshd·fail2ban에는 접속지가 루프백으로만 보이므로 비밀번호 실패는 maru가 센다(`handler.terminalLimiter`, 로그인과 따로 둔 `auth.Limiter`). 물어서 인증이 실패한 접속 하나가 실패 하나다
  - 그룹 소속(`AllowGroups`)은 접속을 막는 데 쓰지 않고 실패 안내에만 쓴다(`NotAllowedGroups`). sshd가 최종 판단한다
- 세션: 대시보드 세션 하나에 `ssh.Client` 하나, 창마다 채널(`NewSession`) 하나다(`Manager`). 인증은 첫 창에서만 하고, 같은 세션의 다른 창(같은 탭의 새 창, 다른 탭)은 기다렸다가 함께 쓴다. 세션당 창 4개(sshd `MaxSessions` 10보다 작게), 클라이언트 32개까지. 마지막 창이 닫히거나 세션이 끝나면(`auth.Store.OnRemove`, 접속 중이면 질문 대기도 취소) 닫는다
- 허용: `-terminal`(기본 끔)과 설정 앱의 덮어쓰기(`site.json`의 `server.terminal`). 꺼져 있으면 `/api/terminal`이 403 `terminal_disabled`, 끄는 순간 `Manager.CloseAll`로 열린 터미널을 닫는다. 프론트는 `capabilities.terminal`로 앱을 숨긴다
- 보안 강화 정책(`security.terminal`)을 채우지 못한 세션은 WebSocket을 받은 뒤 `status closed reason=security_required`(+ `required`)로 닫는다. 정책을 올리면 조건을 채우지 못하는 세션의 터미널을 `CloseSession(id, ReasonSecurityRequired)`로 닫는다. 프론트는 닫힘 막대에 확인 단추를 두고 확인한 뒤 다시 연결한다(`.claude/rules/security.md`)
- 프로토콜(`api.Terminal*Message`): 바이너리 프레임은 입출력 바이트, 텍스트 프레임은 JSON 제어 메시지(`resize`, `answer`, `cancel` / `status`, `prompt`). Origin은 coder/websocket 기본 검사(Host와 같아야 함). 30초마다 ping. 브라우저 메시지 하나는 64KiB까지라 프론트가 입력을 16KiB씩 나눈다
- 세션 연장은 프론트의 키 입력(`useSessionKeeper`의 document capture `keydown`)으로 된다. 출력만으로는 연장하지 않는다(`docs/en/terminal.md`가 tmux를 권한다)

## 프론트

- `src/terminal/`: `TerminalWindow.tsx`(xterm.js, FitAddon, 지연 청크), 순수 모듈 `protocol.ts`(메시지·주소), `promptLine.ts`(질문 답을 받는 줄 편집기), `messages.ts`(닫힌 이유 → 안내 문구), `theme.ts`(CSS 토큰에서 계산한 배경·글자색 + 밝은·어두운 ANSI 팔레트)
- xterm.js는 vendor 청크에서 빼고(`vite.config.ts`) 스타일(`xterm.css`)은 `TerminalWindow.tsx`가 import한다(`frontend.md`의 CSS 예외). 여백·남는 줄 높이는 우리 CSS가 터미널 배경으로 칠한다
- 한 탭에서 터미널 창을 여러 개 연다(`apps.ts`의 `multiInstance`. 바로가기·서랍 클릭과 작업표시줄 버튼 메뉴의 "새 창"이 늘 새 창을 연다. `frontend-shell.md`의 "바로가기와 모든 앱"). 창마다 `TerminalWindow`가 따로 WebSocket을 열어 채널 하나를 쓴다. 세션당 창 수 제한은 다른 탭과 함께 세므로 프론트에서 미리 막지 않고, 넘치면 그 창 안에 `too_many_windows` 안내와 다시 연결 단추가 나온다
  - 개발 모드(StrictMode)는 effect를 두 번 돌려 창마다 연결을 한 번 열었다 닫는다. 닫힌 연결이 아직 세어지는 사이에 다시 연결하므로, 상한 바로 앞의 창은 개발 모드에서만 `too_many_windows`가 날 수 있다(다시 연결하면 된다)

## 테스트

- Go: `sshfake`(x/crypto 서버, 장난감 셸)와 `helperfake`를 이어 인증 순서·임시 키·취소·호스트 키·창 공유를 시험한다(`internal/terminal`). 서버 테스트는 `envOptions{terminal: true, sshd: ...}`가 `httptest.Server`에 붙여 실제 WebSocket으로 시험한다(`internal/server/terminal_test.go`)
  - 가짜 sshd는 window-change 요청과 입력을 다른 고루틴에서 처리하므로, 크기를 바꾼 직후의 `size` 출력은 반영될 때까지 다시 묻는다
- `make dev-fake`: `FAKESSH_LISTEN`(기본 `127.0.0.1:8092`, Claude는 8192)과 `FAKESSH_AUTH`(예: `publickey`)
- 실제 sshd·도우미: 순정 OMV를 설치한 테스트 VM에서 확인한다(VM 구성은 `CLAUDE.local.md`). 실제 NAS의 확인은 사용자가 한다
