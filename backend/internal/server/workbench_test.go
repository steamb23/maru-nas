package server

import "testing"

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
