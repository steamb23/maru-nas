package setup

import "testing"

// OMV nginx 설정(합성)에서 평문 포트와 Force SSL/TLS를 찾는다.
func TestNginxDetection(t *testing.T) {
	cases := []struct{ conf, url string }{
		{"", "http://127.0.0.1"},
		{"server {\n    listen 80 default_server;\n    listen [::]:80 default_server;\n}\n", "http://127.0.0.1"},
		{"server {\n    listen *:8001 default_server;\n    listen [::]:8001 default_server;\n}\n", "http://127.0.0.1:8001"},
		{"server {\n    listen 443 ssl;\n    listen [::]:8443 ssl http2;\n    listen 127.0.0.1:8080;\n}\n", "http://127.0.0.1:8080"},
		{"server {\n\tlisten\t8002;\n}\n", "http://127.0.0.1:8002"},
		{"server {\n    listen unix:/run/x.sock;\n    # listen 9000;\n}\n", "http://127.0.0.1"},
	}
	for _, tc := range cases {
		f := newFake(t)
		if tc.conf != "" {
			f.put(t, nginxWebGUI, tc.conf, 0o644)
		}
		if got := detectOMVURL(f); got != tc.url {
			t.Errorf("%q: %s, want %s", tc.conf, got, tc.url)
		}
	}
	f := newFake(t)
	if omvForcesHTTPS(f) {
		t.Error("no config forces https")
	}
	f.put(t, nginxWebGUI, "server {\n    listen 80;\n    return 301 https://$host:443$request_uri;\n}\n", 0o644)
	if !omvForcesHTTPS(f) {
		t.Error("redirect not detected")
	}
}
