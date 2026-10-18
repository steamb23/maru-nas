package config

import "testing"

func TestCheckListen(t *testing.T) {
	cases := []struct {
		addr             string
		allowNonLoopback bool
		wantErr          bool
	}{
		{addr: "127.0.0.1:8090"},
		{addr: "[::1]:8090"},
		{addr: "unix:/run/onmaru/onmaru.sock"},
		{addr: "unix:onmaru.sock", wantErr: true},
		{addr: "0.0.0.0:8090", wantErr: true},
		{addr: ":8090", wantErr: true},
		{addr: "192.0.2.1:8090", wantErr: true},
		{addr: "localhost:8090", wantErr: true},
		{addr: "127.0.0.1", wantErr: true},
		{addr: "0.0.0.0:8090", allowNonLoopback: true},
		{addr: ":8090", allowNonLoopback: true},
		{addr: "127.0.0.1", allowNonLoopback: true, wantErr: true},
	}
	for _, c := range cases {
		err := CheckListen(c.addr, c.allowNonLoopback)
		if (err != nil) != c.wantErr {
			t.Errorf("CheckListen(%q, %v) = %v, wantErr %v", c.addr, c.allowNonLoopback, err, c.wantErr)
		}
	}
}

func TestCheckOMVURL(t *testing.T) {
	for _, ok := range []string{"http://127.0.0.1", "http://127.0.0.1:8001", "https://localhost/", "http://[::1]:80"} {
		if err := CheckOMVURL(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "127.0.0.1", "ftp://127.0.0.1", "http://192.0.2.1", "http://nas.example.lan", "http://[::]:80"} {
		if CheckOMVURL(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCheckHelperSocket(t *testing.T) {
	for _, ok := range []string{"", "/run/onmaru-helper/helper.sock"} {
		if err := CheckHelperSocket(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"helper.sock", "unix:/run/x.sock", "/run/../x.sock", "/run/x/"} {
		if CheckHelperSocket(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCheckWorkbenchURL(t *testing.T) {
	cases := []struct {
		raw     string
		wantErr bool
	}{
		{raw: ""},
		{raw: "https://nas.example.lan/"},
		{raw: "http://192.0.2.10:8001"},
		{raw: "https://nas.example.lan/omv/"},
		{raw: "HTTPS://nas.example.lan/"},
		{raw: "/"},
		{raw: "/omv/"},
		{raw: "../"},
		{raw: "./omv/"},
		{raw: "nas.example.lan/", wantErr: true},
		{raw: "nas.example.lan:8001/", wantErr: true},
		{raw: "omv/", wantErr: true},
		{raw: "//nas.example.lan/", wantErr: true},
		{raw: "https:///omv/", wantErr: true},
		{raw: "https://user:pass@nas.example.lan/", wantErr: true},
		{raw: "https://nas.example.lan/?a=1", wantErr: true},
		{raw: "https://nas.example.lan/#/dashboard", wantErr: true},
		{raw: "https://nas.example.lan/a b", wantErr: true},
		{raw: "javascript:alert(1)", wantErr: true},
		{raw: "ftp://nas.example.lan/", wantErr: true},
	}
	for _, c := range cases {
		err := CheckWorkbenchURL(c.raw)
		if (err != nil) != c.wantErr {
			t.Errorf("CheckWorkbenchURL(%q) = %v, wantErr %v", c.raw, err, c.wantErr)
		}
	}
}

func TestParseRefreshAt(t *testing.T) {
	if h, m, err := ParseRefreshAt("06:30"); err != nil || h != 6 || m != 30 {
		t.Errorf("%d:%d %v", h, m, err)
	}
	for _, bad := range []string{"24:00", "12:60", "noon", ""} {
		if _, _, err := ParseRefreshAt(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParsePublicOrigin(t *testing.T) {
	type rp struct{ id, origin string }
	for raw, want := range map[string]rp{
		"https://desk.example.lan":        {"desk.example.lan", "https://desk.example.lan"},
		"https://Desk.Example.LAN/":       {"desk.example.lan", "https://desk.example.lan"},
		"https://desk.example.lan:8443":   {"desk.example.lan", "https://desk.example.lan:8443"},
		"https://desk.example.lan:443/":   {"desk.example.lan", "https://desk.example.lan"},
		"https://localhost:5174":          {"localhost", "https://localhost:5174"},
		"https://xn--9n2bp8q.example.lan": {"xn--9n2bp8q.example.lan", "https://xn--9n2bp8q.example.lan"},
	} {
		id, origin, err := ParsePublicOrigin(raw)
		if err != nil || id != want.id || origin != want.origin {
			t.Errorf("%q: %q %q %v", raw, id, origin, err)
		}
	}
	for _, raw := range []string{
		"", "desk.example.lan", "http://desk.example.lan", "https://", "https://192.168.0.10", "https://[::1]:443",
		"https://desk.example.lan/desk/", "https://desk.example.lan?x=1", "https://desk.example.lan#a",
		"https://user@desk.example.lan", "https://desk.example.lan.", "https:desk.example.lan", "https://desk.example.lan?",
		"https://desk.example.lan:99999", "https://desk.example.lan:0",
	} {
		if _, _, err := ParsePublicOrigin(raw); err == nil {
			t.Errorf("%q accepted", raw)
		}
	}
}

func TestValidGroupName(t *testing.T) {
	for _, ok := range []string{"sudo", "openmediavault-admin", "_ssh", "wheel.2"} {
		if !ValidGroupName(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "-sudo", ".x", "a b", "sudo;id"} {
		if ValidGroupName(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
