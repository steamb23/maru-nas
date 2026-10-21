package proto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestRequestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	req := Request{V: Version, Op: OpRun, Ticket: "t", Scope: ScopeUser, Action: ActionList, Params: json.RawMessage(`{"path":"/x"}`), Size: 3}
	if err := WriteRequest(&buf, req); err != nil {
		t.Fatal(err)
	}
	// 머리 뒤의 본문은 읽지 않아야 한다.
	buf.WriteString("abc")
	got, err := ReadRequest(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Op != OpRun || got.Ticket != "t" || string(got.Params) != `{"path":"/x"}` || got.Size != 3 {
		t.Fatalf("request = %+v", got)
	}
	if buf.String() != "abc" {
		t.Fatalf("body after the frame = %q", buf.String())
	}
}

func frame(b []byte) *bytes.Buffer {
	var buf bytes.Buffer
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	buf.Write(n[:])
	buf.Write(b)
	return &buf
}

func TestReadRequestRejects(t *testing.T) {
	cases := map[string]*bytes.Buffer{
		"unknown field": frame([]byte(`{"v":1,"op":"run","evil":true}`)),
		"trailing data": frame([]byte(`{"v":1,"op":"run"} {}`)),
		"empty frame":   frame(nil),
		"too large":     frame(bytes.Repeat([]byte(" "), MaxRequest+1)),
		"short frame": func() *bytes.Buffer {
			b := frame([]byte(`{"v":1}`))
			b.Truncate(b.Len() - 2)
			return b
		}(),
	}
	for name, buf := range cases {
		if _, err := ReadRequest(buf); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestWriteRequestTooLarge(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteRequest(&buf, Request{Password: strings.Repeat("x", MaxRequest)}); err == nil {
		t.Fatal("oversized request written")
	}
	if buf.Len() != 0 {
		t.Fatal("partial frame written")
	}
}

func TestResponseErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteResponse(&buf, Fail(&Error{Code: CodeRateLimited, RetryAfter: 30})); err != nil {
		t.Fatal(err)
	}
	resp, err := ReadResponse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	err = resp.Err()
	if CodeOf(err) != CodeRateLimited || err.(*Error).RetryAfter != 30 {
		t.Fatalf("err = %v", err)
	}
	// 코드가 없는 실패는 internal이다.
	if CodeOf(Response{}.Err()) != CodeInternal {
		t.Fatal("empty failure is not internal")
	}
	if Fail(bytes.ErrTooLarge).Code != CodeInternal {
		t.Fatal("plain error is not internal")
	}
}

func TestSuccessDecode(t *testing.T) {
	resp, err := Success(StatusResult{User: "alice", ElevatedSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	var st StatusResult
	if err := resp.Decode(&st); err != nil || st.User != "alice" || st.ElevatedSeconds != 10 {
		t.Fatalf("decode = %+v %v", st, err)
	}
	if err := (Response{OK: true}).Decode(&st); err == nil {
		t.Fatal("empty result decoded")
	}
}

func TestAllowed(t *testing.T) {
	if !Allowed(ScopeUser, ActionWrite) || !Allowed(ScopeRoot, ActionID) {
		t.Fatal("allowed action rejected")
	}
	// root 범위에는 신원 확인, 업그레이드 요청, 디스크 휴지통 만들기만 있다. 파일 연산이 root로 열리면 안 된다.
	for _, a := range Actions[ScopeUser] {
		if a != ActionID && Allowed(ScopeRoot, a) {
			t.Errorf("%s allowed in root scope", a)
		}
	}
	if !slices.Equal(Actions[ScopeRoot], []string{ActionID, ActionUpgradeApply, ActionTrashPrepare}) ||
		Allowed(ScopeUser, ActionUpgradeApply) || Allowed(ScopeUser, ActionTrashPrepare) {
		t.Errorf("root actions %q", Actions[ScopeRoot])
	}
	if !Allowed(ScopeUser, ActionSSHAuthorize) || !Allowed(ScopeUser, ActionSSHRevoke) {
		t.Fatal("ssh key actions not allowed in user scope")
	}
	if Allowed("", ActionID) || Allowed(ScopeUser, "exec") {
		t.Fatal("unknown scope or action allowed")
	}
}

func TestDecodeParams(t *testing.T) {
	var p PathParams
	if err := DecodeParams(json.RawMessage(`{"path":"/a"}`), &p); err != nil || p.Path != "/a" {
		t.Fatalf("decode = %+v %v", p, err)
	}
	if CodeOf(DecodeParams(json.RawMessage(`{"path":"/a","x":1}`), &p)) != CodeBadRequest {
		t.Fatal("unknown param accepted")
	}
	if err := DecodeParams(nil, &p); err != nil {
		t.Fatalf("empty params: %v", err)
	}
}
