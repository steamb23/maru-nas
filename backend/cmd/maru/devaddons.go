package main

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"maru/backend/config"
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

// envDevAddonBackends는 개발 전용 환경 변수다. 1이면 애드온 패키지 백엔드를 개발 경로로 켠다: maru가 백엔드 패키지마다
// <state>/addon-host/<id>.sock(0600)을 열고, 개발자가 띄운 백엔드의 <state>/addon-dev/<id>.sock에 붙는다
// (.claude/docs/addon-packages.md의 "켜짐과 꺼짐"). 상태 디렉터리가 있어야 한다. 운영 감지보다 먼저 본다. 플래그·가동
// 설정·설치기에는 넣지 않는다(운영 백엔드는 설치기 설정 --addon-backends가 켠다).
const envDevAddonBackends = "MARU_DEV_ADDON_BACKENDS"

// addonBackends는 이 실행에서 패키지 백엔드를 켤 경로와 root 동기화 파일 위치다(.claude/docs/addon-packages.md의
// "켜짐과 꺼짐"). 개발 환경 변수(MARU_DEV_ADDON_BACKENDS=1)가 먼저이고, 그때는 동기화가 없다(개발자가 백엔드를 띄운다).
// 아니면 설치기의 --addon-backends가 켠 설치인지 본다. 둘 다 아니면 nil, nil(백엔드 꺼짐)이다. 시작할 때 한 번 본다.
func addonBackends(stateDir string) (*addonpkg.BackendPaths, *addonpkg.SyncPaths) {
	if os.Getenv(envDevAddonBackends) == "1" {
		return devAddonBackends(stateDir), nil
	}
	return installedAddonBackends(stateDir, config.InstalledStateDir, config.AddonSyncStatusFile)
}

// installedAddonBackends는 운영 백엔드의 경로다. 상태 디렉터리가 설치한 상태 디렉터리(installedDir, 백엔드 유닛과 경로
// 유닛이 이 경로를 쓴다)이고 동기화 결과(statusFile)가 일반 파일이면 켠다. 설치기가 백엔드를 켤 때 maru를 다시 시작하기
// 전에 빈 결과를 써 두므로, 결과가 있으면 켜진 설치다. 개발 서버의 상태 디렉터리(.dev-state)는 이 경로가 아니라 운영
// 파일을 쓰지 않는다.
func installedAddonBackends(stateDir, installedDir, statusFile string) (*addonpkg.BackendPaths, *addonpkg.SyncPaths) {
	if stateDir == "" || filepath.Clean(stateDir) != installedDir {
		return nil, nil
	}
	fi, err := os.Lstat(statusFile)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("addon backends: %v; backends stay disabled", err)
		}
		return nil, nil
	}
	if !fi.Mode().IsRegular() {
		log.Printf("addon backends: %s is not a regular file; backends stay disabled", statusFile)
		return nil, nil
	}
	paths := addonpkg.InstalledBackendPaths()
	sync := addonpkg.InstalledSyncPaths(stateDir)
	sync.Status = statusFile
	log.Printf("addon backends: on (%s)", statusFile)
	return &paths, &sync
}

// devAddonBackends는 개발 백엔드의 경로다. 상태 디렉터리가 없으면 nil이다(백엔드 꺼짐).
func devAddonBackends(stateDir string) *addonpkg.BackendPaths {
	if stateDir == "" {
		log.Printf("addon backends: %s needs a state directory; backends stay disabled", envDevAddonBackends)
		return nil
	}
	for _, d := range []string{addonpkg.DevBackendDir, addonpkg.DevHostDir} {
		dir := filepath.Join(stateDir, d)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			log.Printf("addon backends: %v; backends stay disabled", err)
			return nil
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			log.Printf("addon backends: %v; backends stay disabled", err)
			return nil
		}
	}
	paths := addonpkg.DevBackendPaths(stateDir)
	log.Printf("addon backends: dev sockets under %s and %s (%s)",
		filepath.Join(stateDir, addonpkg.DevBackendDir), filepath.Join(stateDir, addonpkg.DevHostDir), envDevAddonBackends)
	return &paths
}

// newPackages는 애드온 패키지 관리자다. 상태 디렉터리가 있으면 <state>/addons에 설치한다. backends는 이 설치에서
// 패키지 백엔드가 켜져 있는지다(서버의 Config.AddonHost와 같은 값).
func newPackages(base *addons.Registry, stateDir string, plugins addonpkg.PluginTable, backends bool) *addonpkg.Manager {
	opt := addonpkg.Options{Arch: runtime.GOARCH, Plugins: plugins, BackendsEnabled: backends}
	if stateDir != "" {
		opt.Store = addonpkg.NewStore(filepath.Join(stateDir, config.AddonsDirName))
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
