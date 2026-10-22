package prefs

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"maru/backend/internal/security"
)

// 시험용 값은 모두 합성한 것이다.
func sampleRecord() security.Record {
	return security.Record{
		Passkeys: []security.Passkey{{ID: "Y3JlZA", Name: "laptop", PublicKey: []byte{1, 2, 3}, Alg: security.AlgES256, Created: time.Unix(1000, 0).UTC()}},
		TOTP:     &security.TOTP{Secret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", Created: time.Unix(1000, 0).UTC()},
		Recovery: []string{"hash-a", "hash-b"},
		Handle:   []byte{9, 9},
		PasskeyLogins: []security.PasskeyLogin{{
			ID: "bG9naW4", Credential: "Y3JlZA", Share: []byte{4, 5, 6},
			Created: time.Unix(1000, 0).UTC(), ExpiresAt: time.Unix(2000, 0).UTC(),
		}},
	}
}

func securityStores(t *testing.T) map[string]security.Store {
	fs, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return map[string]security.Store{"memory": NewMemoryStore(), "file": fs}
}

func TestSecurityRoundTrip(t *testing.T) {
	for name, s := range securityStores(t) {
		t.Run(name, func(t *testing.T) {
			got, err := s.GetSecurity("alice")
			if err != nil || got.HasMethods() {
				t.Fatalf("empty: %+v %v", got, err)
			}
			if err := s.UpdateSecurity("alice", func(r *security.Record) error { *r = sampleRecord(); return nil }); err != nil {
				t.Fatal(err)
			}
			got, err = s.GetSecurity("alice")
			if err != nil || len(got.Passkeys) != 1 || got.Passkeys[0].Name != "laptop" || got.TOTP == nil ||
				len(got.Recovery) != 2 || string(got.Passkeys[0].PublicKey) != "\x01\x02\x03" ||
				len(got.PasskeyLogins) != 1 || string(got.PasskeyLogins[0].Share) != "\x04\x05\x06" ||
				!got.PasskeyLogins[0].ExpiresAt.Equal(time.Unix(2000, 0)) {
				t.Fatalf("after update: %+v %v", got, err)
			}
			// 받은 값을 고쳐도 저장값은 그대로다.
			got.Passkeys[0].PublicKey[0] = 7
			got.TOTP.Secret = "changed"
			got.Recovery[0] = "changed"
			got.PasskeyLogins[0].Share[0] = 7
			again, _ := s.GetSecurity("alice")
			if again.Passkeys[0].PublicKey[0] != 1 || again.TOTP.Secret == "changed" || again.Recovery[0] == "changed" ||
				again.PasskeyLogins[0].Share[0] != 4 {
				t.Errorf("stored value changed: %+v", again)
			}
			// apply가 실패하면 저장하지 않는다.
			boom := errors.New("boom")
			if err := s.UpdateSecurity("alice", func(r *security.Record) error { r.TOTP = nil; return boom }); !errors.Is(err, boom) {
				t.Errorf("apply error: %v", err)
			}
			if again, _ := s.GetSecurity("alice"); again.TOTP == nil {
				t.Error("failed update was saved")
			}
			if names, _ := s.ListSecurity(); !slices.Equal(names, []string{"alice"}) {
				t.Errorf("list %v", names)
			}
			// 수단이 모두 없어지면 복구 코드와 기록 자체도 없어진다.
			if err := s.UpdateSecurity("alice", func(r *security.Record) error { r.Passkeys, r.TOTP = nil, nil; return nil }); err != nil {
				t.Fatal(err)
			}
			if names, _ := s.ListSecurity(); len(names) != 0 {
				t.Errorf("empty record kept: %v", names)
			}
			if ok, err := s.DeleteSecurity("alice"); ok || err != nil {
				t.Errorf("delete missing: %v %v", ok, err)
			}
			s.UpdateSecurity("bob", func(r *security.Record) error { *r = sampleRecord(); return nil })
			if ok, err := s.DeleteSecurity("bob"); !ok || err != nil {
				t.Errorf("delete: %v %v", ok, err)
			}
			if got, _ := s.GetSecurity("bob"); got.HasMethods() {
				t.Errorf("deleted record still there: %+v", got)
			}
		})
	}
}

func TestSecurityFileLayout(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStore(dir)
	names := []string{"alice", "a.b", "../evil", "한글"}
	for _, n := range names {
		if err := s.UpdateSecurity(n, func(r *security.Record) error { *r = sampleRecord(); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	fi, err := os.Stat(filepath.Join(dir, "security"))
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("security dir: %v %v", fi, err)
	}
	fi, err = os.Stat(filepath.Join(dir, "security", "alice.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("record file: %v %v", fi, err)
	}
	// 사용자 이름을 인코딩해 디렉터리를 벗어나지 않고, 목록은 원래 이름으로 돌아온다.
	if _, err := os.Stat(filepath.Join(dir, "evil.json")); err == nil {
		t.Error("escaped the security dir")
	}
	os.WriteFile(filepath.Join(dir, "security", ".tmp-x.json"), []byte("{}"), 0o600)
	os.WriteFile(filepath.Join(dir, "security", "notes.txt"), []byte("x"), 0o600)
	got, err := s.ListSecurity()
	want := slices.Sorted(slices.Values(names))
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("list %v %v, want %v", got, err, want)
	}
}

func TestSecurityCorruptFileFailsClosed(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStore(dir)
	os.MkdirAll(filepath.Join(dir, "security"), 0o700)
	for name, body := range map[string]string{
		"broken":  "{not json",
		"future":  `{"version": 2, "totp": {"secret": "X"}}`,
		"noversn": `{"totp": {"secret": "X"}}`,
	} {
		path := filepath.Join(dir, "security", name+".json")
		os.WriteFile(path, []byte(body), 0o600)
		if _, err := s.GetSecurity(name); !errors.Is(err, ErrSecurityRecord) {
			t.Errorf("%s: get err=%v", name, err)
		}
		called := false
		if err := s.UpdateSecurity(name, func(*security.Record) error { called = true; return nil }); !errors.Is(err, ErrSecurityRecord) || called {
			t.Errorf("%s: update err=%v called=%v", name, err, called)
		}
		// 파일은 고치지도 옮기지도 않는다.
		if b, err := os.ReadFile(path); err != nil || string(b) != body {
			t.Errorf("%s: file changed: %q %v", name, b, err)
		}
	}
	// root가 지우면 되살아난다.
	if ok, err := s.DeleteSecurity("broken"); !ok || err != nil {
		t.Errorf("delete corrupt: %v %v", ok, err)
	}
	if _, err := s.GetSecurity("broken"); err != nil {
		t.Errorf("after delete: %v", err)
	}
}
