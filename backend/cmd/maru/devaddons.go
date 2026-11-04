package main

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/addons"
	"maru/backend/internal/web"
)

// envDevAddons는 개발 전용 환경 변수다. 콜론으로 나눈 풀린 애드온 패키지 디렉터리이며, 설치하지 않고 그대로
// 불러온다(파일을 고치면 다음 capabilities 요청에서 다시 읽는다). 플래그·가동 설정·설치기에는 넣지 않는다.
const envDevAddons = "MARU_DEV_ADDONS"

// envDevAddonSDK는 개발 전용 환경 변수다. 애드온 프레임의 _maru/ 파일(sdk.js, maru.css)을 실행 파일에 포함한
// 빌드 대신 이 디렉터리에서 준다(scripts/dev.sh가 vite build --watch로 채운다).
const envDevAddonSDK = "MARU_DEV_ADDON_SDK"

// newPackages는 애드온 패키지 관리자다. 상태 디렉터리가 있으면 <state>/addons에 설치한다.
func newPackages(base *addons.Registry, stateDir string, plugins addonpkg.PluginTable) *addonpkg.Manager {
	opt := addonpkg.Options{Arch: runtime.GOARCH, Plugins: plugins}
	if stateDir != "" {
		opt.Store = addonpkg.NewStore(filepath.Join(stateDir, "addons"))
	}
	if dirs := os.Getenv(envDevAddons); dirs != "" {
		for _, d := range strings.Split(dirs, ":") {
			if d = strings.TrimSpace(d); d != "" {
				opt.DevDirs = append(opt.DevDirs, d)
			}
		}
		log.Printf("addon packages: dev directories %v (%s)", opt.DevDirs, envDevAddons)
	}
	m := addonpkg.NewManager(base, opt)
	list, err := m.List()
	if err != nil {
		log.Printf("addon packages: %v", err)
	}
	for _, s := range list {
		if s.State != addonpkg.StateActive && s.State != addonpkg.StateDisabled {
			log.Printf("addon package %q: %s (%s): %s", s.ID, s.State, s.Reason, s.Detail)
		}
	}
	return m
}

// addonSDK는 애드온 프레임에 줄 SDK 런타임 파일이다.
func addonSDK() fs.FS {
	if dir := os.Getenv(envDevAddonSDK); dir != "" {
		log.Printf("addon sdk: dev directory %s (%s)", dir, envDevAddonSDK)
		return os.DirFS(dir)
	}
	return web.AddonSDK()
}
