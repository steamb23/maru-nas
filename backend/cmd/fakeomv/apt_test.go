package main

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"maru/backend/internal/addons"
	"maru/backend/internal/addons/packages"
	"maru/backend/internal/api"
	"maru/backend/internal/aptindex"
)

// 가짜 루트의 업데이트 목록·dpkg 상태를 실제 읽기 코드가 읽는다.
func TestFakeAptDescriptions(t *testing.T) {
	root := t.TempDir()
	cfg, _, err := startApt(root, aptSuccess)
	if err != nil {
		t.Fatal(err)
	}
	a := packages.New(aptindex.NewReaderFS(os.DirFS(root)))
	detail := func(name string) *api.PackageDetail {
		t.Helper()
		out, err := a.Fetch(context.Background(), addons.Request{Admin: true, Query: url.Values{"detail": {name}}})
		if err != nil {
			t.Fatal(err)
		}
		return out.(api.PackagesResponse).Detail
	}
	// OMV 저장소는 업데이트 목록에, Debian 공식 저장소는 dpkg 상태에 긴 설명이 있다.
	if d := detail("openmediavault"); d == nil || d.DescriptionFrom != api.DescriptionCandidate || !strings.Contains(d.Description, "Features:\n* web based") {
		t.Errorf("openmediavault = %+v", d)
	}
	if d := detail("openssl"); d == nil || d.DescriptionFrom != api.DescriptionInstalled || !strings.Contains(d.Description, "\n * creating RSA") {
		t.Errorf("openssl = %+v", d)
	}
	if d := detail("systemd"); d == nil || !strings.Contains(d.Description, "\n\n    systemctl status") {
		t.Errorf("systemd = %+v", d)
	}
	if d := detail("linux-image-6.12.99+deb13-amd64"); d == nil || d.Description != "" {
		t.Errorf("new kernel = %+v", d)
	}
	// 변경 기록: 업데이트 목록의 모든 패키지에 설정이 있고, 파일 이름이 maru의 검사를 통과한다.
	idx, err := aptindex.NewReaderFS(os.DirFS(root)).Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range idx.Packages {
		if _, ok := cfg.Changelogs[p.Name]; !ok {
			t.Errorf("%s has no fake changelog", p.Name)
		}
		got := packages.SplitChangelog(p, cfg.Changelogs[p.Name].Text, false)
		if p.OldVersion != "" && p.Name != "docker-ce" && p.Name != "openmediavault-omvextrasorg" && p.Name != "libc-bin" && !got.Matched {
			t.Errorf("%s changelog not split at %s", p.Name, p.OldVersion)
		}
	}
}
