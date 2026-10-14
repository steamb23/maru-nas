package server

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name, remote, xff, want string
	}{
		{"tcp", "192.0.2.10:51234", "", "192.0.2.10"},
		{"xff last entry", "127.0.0.1:51234", "198.51.100.1, 203.0.113.7", "203.0.113.7"},
		{"xff invalid falls back", "127.0.0.1:51234", "not-an-ip", "127.0.0.1"},
		{"unix socket with xff", "@", "203.0.113.7", "203.0.113.7"},
		{"unix socket without xff", "@", "", ""},
		{"empty remote", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/session", nil)
			r.RemoteAddr = c.remote
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIP(r); got != c.want {
				t.Fatalf("clientIP = %q, want %q", got, c.want)
			}
		})
	}
}
