package setup

import (
	"strings"
	"testing"
)

func TestEnvFile(t *testing.T) {
	e := ParseEnv([]byte("# c\nONMARU_A=1\nONMARU_A=\"two\"\nONMARU_B=\nONMARU_Q=\"\nONMARU_X=a\"b\" c\n#ONMARU_C=3\n ONMARU_D=4\nexport ONMARU_E=5\n"))
	for key, want := range map[string]string{"ONMARU_A": "two", "ONMARU_B": "", "ONMARU_Q": `"`, "ONMARU_X": `a"b" c`} {
		if v, ok := e.Get(key); !ok || v != want {
			t.Errorf("%s = %q %v, want %q", key, v, ok, want)
		}
	}
	for _, key := range []string{"ONMARU_C", "ONMARU_D", "ONMARU_E", "ONMARU"} {
		if e.Has(key) {
			t.Errorf("%s found", key)
		}
	}
	got := e.Unmanaged(map[string]bool{"ONMARU_A": true})
	want := []string{"ONMARU_B=", `ONMARU_Q="`, `ONMARU_X=a"b" c`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("unmanaged %q", got)
	}
	var none *EnvFile
	if none.Has("X") || none.Unmanaged(nil) != nil || none.Value("X") != "" {
		t.Error("nil env file")
	}
}

func TestUnitUser(t *testing.T) {
	if u := unitUser([]byte("[Service]\nType=simple\nUser=dash\nUser=x\n")); u != "dash" {
		t.Errorf("user %q", u)
	}
	if u := unitUser([]byte("[Service]\n")); u != "" {
		t.Errorf("user %q", u)
	}
}

func TestHealthRequest(t *testing.T) {
	for listen, want := range map[string]string{
		"127.0.0.1:8090":               "http://127.0.0.1:8090/api/health",
		"0.0.0.0:8090":                 "http://127.0.0.1:8090/api/health",
		":8090":                        "http://127.0.0.1:8090/api/health",
		"[::]:8090":                    "http://[::1]:8090/api/health",
		"[::1]:9000":                   "http://[::1]:9000/api/health",
		"192.0.2.10:8090":              "http://192.0.2.10:8090/api/health",
		"unix:/run/onmaru/onmaru.sock": "http://localhost/api/health",
	} {
		if _, got := healthRequest(listen); got != want {
			t.Errorf("%s: %s, want %s", listen, got, want)
		}
	}
}

func TestDiffLines(t *testing.T) {
	var b strings.Builder
	writeDiff(&b, []byte("a\nb\nc\n"), []byte("a\nB\nc\nd\n"))
	want := "    " + "  a\n    - b\n    + B\n      c\n    + d\n"
	if b.String() != want {
		t.Errorf("diff\n%q\nwant\n%q", b.String(), want)
	}
}

func TestShellQuote(t *testing.T) {
	if got := commandLine("sudo", []string{"/x/onmaru-install", "--workbench-url", "", "a b", "it's"}); got != `sudo /x/onmaru-install --workbench-url '' 'a b' 'it'\''s'` {
		t.Errorf("got %s", got)
	}
}

func TestDetectLang(t *testing.T) {
	for env, want := range map[[3]string]string{
		{"", "", ""}:                       "en",
		{"", "", "ko_KR.UTF-8"}:            "ko",
		{"C", "", "ko_KR.UTF-8"}:           "en",
		{"", "ko_KR.UTF-8", "en_US.UTF-8"}: "ko",
		{"ko", "", ""}:                     "ko",
	} {
		got := DetectLang(func(k string) string {
			return map[string]string{"LC_ALL": env[0], "LC_MESSAGES": env[1], "LANG": env[2]}[k]
		})
		if got != want {
			t.Errorf("%v: %s, want %s", env, got, want)
		}
	}
}

func TestDisplayWidth(t *testing.T) {
	if displayWidth("서비스 사용자") != 13 || displayWidth("listen") != 6 || padRight("수신", 6) != "수신  " {
		t.Error("width")
	}
}
