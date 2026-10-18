// Package buildinfo는 온마루 버전(config.Version)과 실행 파일의 빌드 정보를 준다.
package buildinfo

import (
	"runtime"
	"runtime/debug"

	"onmaru/backend/config"
	"onmaru/backend/internal/api"
)

// Read는 지금 실행 파일의 빌드 정보다. go build는 git 저장소 안에서 빌드하면 커밋 정보를 넣지만
// go run은 넣지 않으므로 커밋·시각이 빠질 수 있다.
func Read() api.AboutResponse {
	bi, _ := debug.ReadBuildInfo()
	return fromBuildInfo(bi)
}

// fromBuildInfo는 Go가 실행 파일에 넣은 빌드 정보에서 응답을 만든다. bi가 nil이어도 된다.
func fromBuildInfo(bi *debug.BuildInfo) api.AboutResponse {
	resp := api.AboutResponse{Version: config.Version, GoVersion: runtime.Version()}
	if bi == nil {
		return resp
	}
	if bi.GoVersion != "" {
		resp.GoVersion = bi.GoVersion
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			resp.Revision = s.Value
		case "vcs.time":
			resp.RevisionTime = s.Value
		case "vcs.modified":
			resp.Modified = s.Value == "true"
		}
	}
	return resp
}
