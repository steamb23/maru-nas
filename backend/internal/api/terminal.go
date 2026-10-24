package api

// 웹 터미널(GET /api/terminal, WebSocket). 바이너리 프레임은 터미널 입출력 바이트이고, 텍스트 프레임은
// 아래 JSON 제어 메시지다. 쿼리 cols·rows는 처음 창 크기다.

// TerminalClientMessage는 브라우저가 보내는 제어 메시지다.
//   - resize: 창 크기(Cols, Rows)가 바뀌었다
//   - answer: 마지막 prompt의 답(Answers, 질문 수와 같다)
//   - cancel: 마지막 prompt를 취소한다(접속을 그만둔다)
type TerminalClientMessage struct {
	Type    string   `json:"type" tstype:"'resize' | 'answer' | 'cancel'"`
	Cols    int      `json:"cols,omitempty"`
	Rows    int      `json:"rows,omitempty"`
	Answers []string `json:"answers,omitempty"`
}

// TerminalServerMessage는 서버가 보내는 제어 메시지다.
//   - status: 접속 상태(State). closed면 Reason과 ExitCode·힌트가 붙는다
//   - prompt: sshd가 묻는 질문. 답은 answer로 보낸다
type TerminalServerMessage struct {
	Type  string `json:"type" tstype:"'status' | 'prompt'"`
	State string `json:"state,omitempty" tstype:"'connecting' | 'connected' | 'closed'"`
	// Reason은 닫힌 이유다. exit(셸이 끝남, ExitCode), 세션·설정 이유(session_ended, disabled,
	// connection_lost, security_required), 접속 실패(ssh_unavailable, ssh_host_key, ssh_auth_failed, ssh_failed,
	// canceled, too_many_windows, busy, rate_limited) 중 하나다.
	Reason   string `json:"reason,omitempty" tstype:"'exit' | 'session_ended' | 'disabled' | 'connection_lost' | 'security_required' | 'ssh_unavailable' | 'ssh_host_key' | 'ssh_auth_failed' | 'ssh_failed' | 'canceled' | 'too_many_windows' | 'busy' | 'rate_limited'"`
	ExitCode *int   `json:"exitCode,omitempty"`
	// KeyCode는 도우미 임시 키를 쓰지 못한 이유(도우미 오류 코드, 또는 sshd가 키를 받지 않았으면
	// key_rejected)다. ssh_auth_failed의 힌트다.
	KeyCode string `json:"keyCode,omitempty"`
	// NotAllowedGroups는 사용자가 sshd의 AllowGroups 중 어느 그룹에도 없을 때 그 그룹 목록이다.
	NotAllowedGroups []string `json:"notAllowedGroups,omitempty"`
	// RetryAfterSeconds는 rate_limited일 때 다시 시도할 수 있을 때까지의 초다.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`
	// Required는 security_required일 때 거쳐야 하는 단계다. 정책이 바뀌어 열린 터미널이 닫혔을 때는 없다.
	Required string `json:"required,omitempty" tstype:"'password' | 'twoFactor'"`
	// Kind는 prompt의 종류다. password면 질문 문구를 프론트가 현지화하고, keyboard-interactive면
	// sshd(PAM)가 준 Instruction과 Prompts의 문구를 그대로 보여 준다.
	Kind        string           `json:"kind,omitempty" tstype:"'password' | 'keyboard-interactive'"`
	Instruction string           `json:"instruction,omitempty"`
	Prompts     []TerminalPrompt `json:"prompts,omitempty"`
}

// TerminalPrompt는 질문 하나다. Echo가 false면 입력을 화면에 보이지 않는다(비밀번호).
type TerminalPrompt struct {
	Text string `json:"text"`
	Echo bool   `json:"echo"`
}
