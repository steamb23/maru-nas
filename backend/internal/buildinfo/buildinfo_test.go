package buildinfo

import (
	"runtime/debug"
	"testing"

	"onmaru/backend/config"
)

func TestFromBuildInfo(t *testing.T) {
	got := fromBuildInfo(&debug.BuildInfo{
		GoVersion: "go1.99.0",
		Settings: []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			{Key: "vcs.time", Value: "2026-01-02T03:04:05Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	})
	if got.Version != config.Version || got.GoVersion != "go1.99.0" {
		t.Fatalf("version %q go %q", got.Version, got.GoVersion)
	}
	if got.Revision != "0123456789abcdef0123456789abcdef01234567" || got.RevisionTime != "2026-01-02T03:04:05Z" || !got.Modified {
		t.Fatalf("vcs fields %+v", got)
	}
}

func TestFromBuildInfoWithoutVCS(t *testing.T) {
	// go run이나 저장소 밖 빌드에는 커밋 정보가 없다.
	got := fromBuildInfo(nil)
	if got.Version != config.Version || got.GoVersion == "" || got.Revision != "" || got.Modified {
		t.Fatalf("%+v", got)
	}
}
