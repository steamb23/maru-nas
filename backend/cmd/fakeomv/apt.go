package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"maru/backend/internal/aptindex"
	"maru/backend/internal/omv/omvfake"
)

// 가짜 APT 시나리오. FAKEAPT_SCENARIO로 고른다. maru는 작업 출력을 1초마다 읽고 가짜 작업은 읽을 때마다 출력 조각
// 하나씩 나아가므로, 조각 수가 곧 작업 시간(초)이다.
const (
	// aptSuccess는 확인·설치가 성공한다. 확인은 목록을 다시 채우고, 설치는 목록을 비우고 재부팅 권장을 남긴다.
	aptSuccess = "success"
	// aptSlow는 설치가 90초쯤 걸린다(창을 닫거나 로그아웃한 동안 끝나는 경우를 확인).
	aptSlow = "slow"
	// aptFail은 설치가 apt 오류로 끝난다.
	aptFail = "fail"
	// aptCoreRemoval은 설치가 openmediavault 제거 보호로 멈춘다(OMV 8.5.10부터).
	aptCoreRemoval = "core-removal"
	// aptLocked는 다른 프로세스가 잠금을 쥐고 있어 확인·설치를 시작하지 않는다.
	aptLocked = "locked"
	// aptDirty는 apt 설정이 적용 대기라 확인을 시작하지 않는다(설치는 된다).
	aptDirty = "dirty"
)

// startApt는 가짜 루트에 처음 목록을 쓰고 시나리오의 가짜 Apt 설정을 만든다. dirty면 apt 모듈을 적용 대기로 둔다.
func startApt(root, scenario string) (omvfake.AptConfig, bool, error) {
	if err := writeAptIndex(root, fakePackages); err != nil {
		return omvfake.AptConfig{}, false, err
	}
	if err := writeReboot(root, nil); err != nil {
		return omvfake.AptConfig{}, false, err
	}
	cfg := omvfake.DefaultAptConfig()
	// 확인은 목록을 다시 쓴다(새 업데이트를 찾은 것처럼 처음 목록으로 되돌린다).
	cfg.Update.OnDone = func() { logErr(writeAptIndex(root, fakePackages)) }
	// 설치는 목록을 비우고, 커널처럼 재부팅을 요청하는 패키지를 재부팅 권장 파일에 남긴다.
	cfg.Upgrade.OnDone = func() {
		logErr(writeAptIndex(root, nil))
		var reboot []string
		for _, p := range fakePackages {
			if p.reboot {
				reboot = append(reboot, p.name)
			}
		}
		logErr(writeReboot(root, reboot))
	}
	dirty := false
	switch scenario {
	case aptSuccess:
	case aptSlow:
		var chunks []string
		for i := range 90 {
			chunks = append(chunks, fmt.Sprintf("Unpacking fake-package-%02d (1.0-%d) ...\n", i, i))
		}
		cfg.Upgrade.Chunks = append(chunks, cfg.Upgrade.Chunks...)
	case aptFail:
		cfg.Upgrade.Fail = omvfake.AptExecFailedMessage
		cfg.Upgrade.Chunks = append(cfg.Upgrade.Chunks, "dpkg: error processing package openmediavault (--configure):\n installed openmediavault package post-installation script subprocess returned error exit status 1\n")
	case aptCoreRemoval:
		cfg.Upgrade.Fail = omvfake.AptCoreRemovalMessage
		cfg.Upgrade.Chunks = nil
	case aptLocked:
		cfg.Locked = true
	case aptDirty:
		dirty = true
	default:
		return omvfake.AptConfig{}, false, fmt.Errorf("unknown FAKEAPT_SCENARIO %q (success, slow, fail, core-removal, locked, dirty)", scenario)
	}
	return cfg, dirty, nil
}

func logErr(err error) {
	if err != nil {
		log.Printf("fake apt: %v", err)
	}
}

// fakePackage는 가짜 업데이트 목록의 패키지 하나다. 값은 모두 합성이다.
type fakePackage struct {
	name, old, version, repo, section, summary string
	size                                       int64
	// reboot이면 설치한 뒤 재부팅 권장 파일에 이름을 남긴다(커널 등).
	reboot bool
}

// fakePackages는 가짜 업데이트 목록이다.
var fakePackages = []fakePackage{
	{"openmediavault", "8.5.11-1", "8.5.12-1", "openmediavault/synchrony", "admin", "openmediavault - The open network attached storage solution", 4_194_304, false},
	{"openmediavault-omvextrasorg", "8.0.2", "8.0.3", "omv-extras/synchrony", "admin", "OMV-Extras.org Package Repositories for OpenMediaVault", 81_920, false},
	{"linux-image-6.12.99+deb13-amd64", "", "6.12.99-1", "Debian-Security/trixie-security", "kernel", "Linux 6.12 for 64-bit PCs (signed)", 104_857_600, true},
	{"linux-image-amd64", "6.12.98-1", "6.12.99-1", "Debian-Security/trixie-security", "kernel", "Linux for 64-bit PCs (meta-package)", 2_048, false},
	{"libc6", "2.41-12", "2.41-99", "Debian/trixie", "libs", "GNU C Library: Shared libraries", 2_867_200, false},
	{"libc-bin", "2.41-12", "2.41-99", "Debian/trixie", "libs", "GNU C Library: Binaries", 634_880, false},
	{"openssl", "3.5.1-1", "3.5.1-2", "Debian-Security/trixie-security", "utils", "Secure Sockets Layer toolkit - cryptographic utility", 1_464_320, false},
	{"libssl3t64", "3.5.1-1", "3.5.1-2", "Debian-Security/trixie-security", "libs", "Secure Sockets Layer toolkit - shared libraries", 2_351_104, false},
	{"systemd", "257.7-1", "257.8-1", "Debian/trixie", "admin", "system and service manager", 3_145_728, false},
	{"dbus", "1.16.2-2", "1.16.2-3", "Debian/trixie", "admin", "simple interprocess messaging system (system message bus)", 98_304, true},
	{"docker-ce", "5:28.3.2-1~debian.13~trixie", "5:28.3.3-1~debian.13~trixie", "Docker CE/trixie", "admin", "Docker: the open-source application container engine", 20_971_520, false},
	{"tzdata", "2025b-4", "2025c-1", "Debian/trixie-updates", "localization", "time zone and daylight-saving time data", 266_240, false},
}

// writeAptIndex는 가짜 루트에 upgradeindex.json을 쓴다(OMV omv-mkaptidx의 형식에서 maru가 읽는 필드만).
func writeAptIndex(root string, pkgs []fakePackage) error {
	entries := make([]map[string]any, 0, len(pkgs))
	for _, p := range pkgs {
		arch := "amd64"
		if p.section == "admin" && strings.HasPrefix(p.name, "openmediavault") {
			arch = "all"
		}
		file := fmt.Sprintf("pool/main/%c/%s/%s_%s_%s.deb", p.name[0], p.name, p.name, strings.ReplaceAll(p.version, ":", "%3a"), arch)
		entries = append(entries, map[string]any{
			"name": p.name, "package": p.name, "version": p.version, "oldversion": p.old,
			"repository": p.repo, "section": p.section, "summary": p.summary, "filename": file,
			"size": p.size, "installedsize": p.size, "architecture": arch,
		})
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(root, aptindex.IndexPath), b)
}

// writeReboot은 가짜 루트에 재부팅 권장 파일을 쓴다. 이름이 없으면 지운다.
func writeReboot(root string, names []string) error {
	path := filepath.Join(root, aptindex.RebootPath)
	pkgs := filepath.Join(root, aptindex.RebootPkgsPath)
	if len(names) == 0 {
		os.Remove(pkgs)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := writeAtomic(pkgs, []byte(strings.Join(names, "\n")+"\n")); err != nil {
		return err
	}
	return writeAtomic(path, []byte("*** System restart required ***\n"))
}

// writeAtomic은 같은 디렉터리의 임시 파일에 쓴 뒤 이름을 바꾼다(디렉터리는 만든다).
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fake-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
