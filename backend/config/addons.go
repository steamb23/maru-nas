package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
)

// 애드온 패키지 백엔드(.claude/docs/addon-packages.md의 "백엔드 프로세스")의 경로·파일 형식·검사 함수.
// maru(백엔드 소켓에 붙고 호스트 소켓을 연다)와 설치기(유닛 템플릿, --addon-sync)가 함께 쓴다.

// 운영 경로. 백엔드 소켓은 systemd 소켓 유닛이 만들고(SocketUser=maru 서비스 사용자, 0600), 호스트 소켓은 maru가 연다.
const (
	// AddonSocketDir는 백엔드 소켓 디렉터리다. 소켓은 <dir>/<id>.sock이다.
	AddonSocketDir = "/run/maru-addon"
	// AddonHostDir는 호스트 소켓 디렉터리다(root 소유 0755. maru.service의 root ExecStartPre와 백엔드 소켓 유닛이
	// 만들고, maru.service의 ReadWritePaths=-가 maru에 쓰기를 연다). 소켓은 <dir>/<id>/host.sock이고, <id>는 백엔드 소켓
	// 유닛이 maru 서비스 사용자 소유로 만든다. 백엔드 유닛에는 <dir>/<id>만 /run/maru-host로 바인드된다.
	AddonHostDir = "/run/maru-addon-host"
	// AddonHostSocketName은 호스트 소켓 파일 이름이다.
	AddonHostSocketName = "host.sock"
	// AddonUnitHostSocket은 백엔드 유닛 안에서 보이는 호스트 소켓 경로다.
	AddonUnitHostSocket = "/run/maru-host/" + AddonHostSocketName
	// AddonSyncDir는 root 동기화(설치기 --addon-sync)의 기록 디렉터리다(root 소유, maru는 읽기만).
	AddonSyncDir = "/var/lib/maru-addons"
	// AddonSyncStatusFile은 동기화 결과다. 이 파일이 있으면 운영에서 백엔드가 켜져 있다.
	AddonSyncStatusFile = AddonSyncDir + "/status.json"
	// AddonSyncRecordFile은 root 동기화의 자기 기록이다(켜 둔 인스턴스와 데이터를 남긴 id). maru는 읽지 않는다.
	AddonSyncRecordFile = AddonSyncDir + "/state.json"
	// AddonSyncStatusMaxBytes는 동기화 결과 파일의 크기 상한이다(maru가 읽을 때).
	AddonSyncStatusMaxBytes = 64 << 10
)

// 상태 디렉터리 안의 파일. maru가 쓰고 root 동기화가 읽는다.
const (
	// AddonBackendsFile은 원하는 상태(AddonBackends)다.
	AddonBackendsFile = "addon-backends.json"
	// AddonSyncRequest는 동기화 요청이다. 경로 유닛(PathExists)이 지켜보고 동기화가 읽은 뒤 지운다.
	AddonSyncRequest = "addon-sync.json"
	// AddonBackendsMaxBytes는 원하는 상태 파일의 크기 상한이다.
	AddonBackendsMaxBytes = 64 << 10
	// AddonBackendsMax는 원하는 상태에 둘 수 있는 백엔드·purge 항목 수의 상한이다.
	AddonBackendsMax = 32
	// AddonSyncRequestBody는 maru가 쓰는 동기화 요청의 내용이다. 동기화는 내용을 보지 않고(있는지만 본다) 읽기 전에
	// 지운다(PathExists가 다시 실행하지 않게). 원하는 상태는 늘 AddonBackendsFile에서 읽는다.
	AddonSyncRequestBody = `{"format":1}` + "\n"
	// AddonSyncRequestMaxBytes는 동기화가 요청 파일을 읽을 때의 상한이다.
	AddonSyncRequestMaxBytes = 4 << 10
)

// 애드온 id와 rev의 형식. id는 systemd 인스턴스 이름과 동적 사용자 이름(User=maru-a-%i, 31자 한도)에 그대로 쓰므로
// 이 정규식이 systemctl 인자 주입도 막는다.
const (
	AddonIDPattern = `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`
	// AddonIDMaxLen은 패키지 id의 최대 길이다("maru-a-" + 24자 = 31자).
	AddonIDMaxLen = 24
	// AddonRevPattern은 설치 디렉터리 이름(<version>-<sha256 앞 8자>, 개발 패키지는 dev-<해시>)이다.
	AddonRevPattern = `^[0-9A-Za-z][0-9A-Za-z.-]{0,40}-[0-9a-f]{8}$`
)

// AddonInstancePattern은 설치 인스턴스(패키지 기록의 installUID, 처음 설치할 때 만든 16바이트 무작위 값의 16진수)다.
const AddonInstancePattern = `^[0-9a-f]{32}$`

var (
	addonIDPattern       = regexp.MustCompile(AddonIDPattern)
	addonRevPattern      = regexp.MustCompile(AddonRevPattern)
	addonInstancePattern = regexp.MustCompile(AddonInstancePattern)
)

// ValidAddonID는 id가 패키지 id 형식인지다.
func ValidAddonID(id string) bool {
	return len(id) <= AddonIDMaxLen && addonIDPattern.MatchString(id)
}

// ValidAddonRev는 rev가 설치 디렉터리 이름 형식인지다.
func ValidAddonRev(rev string) bool {
	return addonRevPattern.MatchString(rev) && !strings.Contains(rev, "..")
}

// ValidAddonInstance는 v가 설치 인스턴스 형식인지다.
func ValidAddonInstance(v string) bool {
	return addonInstancePattern.MatchString(v)
}

// 백엔드 네트워크 권한. 비면 네트워크가 없다.
const (
	AddonNetworkLAN      = "lan"
	AddonNetworkLoopback = "loopback"
)

// ValidAddonNetwork는 network가 받는 네트워크 권한 값인지다.
func ValidAddonNetwork(network string) bool {
	return network == AddonNetworkLAN || network == AddonNetworkLoopback
}

// AddonUnitPrefix는 네트워크 권한에 맞는 유닛 템플릿 이름이다(maru-addon, maru-addon-lan, maru-addon-loopback,
// maru-addon-lan-loopback). 인스턴스는 <prefix>@<id>.socket·.service다. network는 검사한 값이어야 한다.
func AddonUnitPrefix(network []string) string {
	name := "maru-addon"
	if slices.Contains(network, AddonNetworkLAN) {
		name += "-lan"
	}
	if slices.Contains(network, AddonNetworkLoopback) {
		name += "-loopback"
	}
	return name
}

// AddonBackends는 maru가 쓰는 원하는 상태다(<state>/addon-backends.json).
type AddonBackends struct {
	Format   int            `json:"format"`
	Backends []AddonBackend `json:"backends"`
	// Purge는 백엔드 데이터(StateDirectory)를 지울 id다. 같은 id가 Backends에 있으면 동기화가 무시한다.
	Purge []string `json:"purge"`
}

// AddonBackend는 켜 둘 백엔드 하나다.
type AddonBackend struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Rev     string `json:"rev"`
	// Instance는 그 패키지의 설치 인스턴스(installUID)다. 업데이트해도 같고, 지웠다가 다시 설치하면 바뀐다. 동기화는
	// 같은 id의 백엔드 데이터가 다른 인스턴스의 것이면 켜기 전에 지운다(다시 설치한 패키지, 같은 id의 다른 작성자가
	// 이전 데이터를 읽지 못하게. 사용자별 저장값과 같은 규칙).
	Instance string   `json:"instance"`
	Network  []string `json:"network"`
}

// AddonBackendsFormat은 AddonBackends.Format의 현재 값이다.
const AddonBackendsFormat = 1

// ParseAddonBackends는 원하는 상태 파일을 엄격하게 읽는다(모르는 필드, 크기, 형식, 개수, 중복을 거부한다).
// root 동기화가 maru가 쓴 파일을 믿지 않고 이 함수로 다시 검사한다.
func ParseAddonBackends(data []byte) (AddonBackends, error) {
	var b AddonBackends
	if len(data) > AddonBackendsMaxBytes {
		return b, fmt.Errorf("addon backends: larger than %d bytes", AddonBackendsMaxBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return AddonBackends{}, fmt.Errorf("addon backends: %w", err)
	}
	// dec.More는 값 뒤에 붙은 ]·}를 놓치므로 다음 토큰이 끝인지 본다.
	if _, err := dec.Token(); err != io.EOF {
		return AddonBackends{}, errors.New("addon backends: trailing data")
	}
	if err := b.Validate(); err != nil {
		return AddonBackends{}, err
	}
	return b, nil
}

// Validate는 원하는 상태의 값을 검사한다.
func (b AddonBackends) Validate() error {
	if b.Format != AddonBackendsFormat {
		return fmt.Errorf("addon backends: unsupported format %d", b.Format)
	}
	if len(b.Backends) > AddonBackendsMax || len(b.Purge) > AddonBackendsMax {
		return fmt.Errorf("addon backends: more than %d entries", AddonBackendsMax)
	}
	seen := make(map[string]bool, len(b.Backends))
	for _, e := range b.Backends {
		if !ValidAddonID(e.ID) {
			return fmt.Errorf("addon backends: invalid id %q", e.ID)
		}
		if seen[e.ID] {
			return fmt.Errorf("addon backends: duplicate id %q", e.ID)
		}
		seen[e.ID] = true
		if !ValidAddonRev(e.Rev) || !strings.HasPrefix(e.Rev, e.Version+"-") {
			return fmt.Errorf("addon backends: %s: invalid rev %q for version %q", e.ID, e.Rev, e.Version)
		}
		if !ValidAddonInstance(e.Instance) {
			return fmt.Errorf("addon backends: %s: invalid instance", e.ID)
		}
		for i, n := range e.Network {
			if !ValidAddonNetwork(n) || slices.Contains(e.Network[:i], n) {
				return fmt.Errorf("addon backends: %s: invalid network %q", e.ID, n)
			}
		}
	}
	purged := make(map[string]bool, len(b.Purge))
	for _, id := range b.Purge {
		if !ValidAddonID(id) || purged[id] {
			return fmt.Errorf("addon backends: invalid purge id %q", id)
		}
		purged[id] = true
	}
	return nil
}

// AddonBackendsDigest는 원하는 상태 파일 내용의 SHA-256(16진수)이다. maru와 동기화가 같은 함수로 계산한다.
func AddonBackendsDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// AddonSyncStatusFormat은 AddonSyncStatus.Format의 현재 값이다.
const AddonSyncStatusFormat = 1

// AddonSyncStatus는 root 동기화의 결과다(AddonSyncStatusFile, 0644). 설치기가 백엔드를 켤 때 빈 결과(Digest 없음)를
// 먼저 써 두므로 maru가 시작하자마자 켜진 것을 안다.
type AddonSyncStatus struct {
	Format    int       `json:"format"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Error는 동기화 전체가 실패한 이유다(원하는 상태 파일을 읽거나 검사하지 못함 등). 비면 항목별 결과를 본다.
	Error string `json:"error,omitempty"`
	// Digest는 동기화가 맞춘 원하는 상태 파일의 SHA-256(16진수)이다. maru는 자기가 쓴 파일과 비교해 반영 여부를 안다.
	Digest   string               `json:"digest"`
	Backends []AddonBackendStatus `json:"backends"`
	// Purged는 백엔드 데이터를 지운 것을 확인한 id다. maru는 이 id를 원하는 상태의 Purge에서 뺀다.
	Purged []string `json:"purged"`
}

// AddonBackendStatus는 백엔드 하나의 동기화 결과다.
type AddonBackendStatus struct {
	ID    string `json:"id"`
	Rev   string `json:"rev"`
	Unit  string `json:"unit"`
	State string `json:"state"`
	// Error는 실패한 systemctl 동작의 요약이다(State가 AddonBackendFailed일 때).
	Error string `json:"error,omitempty"`
}

// AddonBackendStatus.State 값.
const (
	AddonBackendEnabled = "enabled"
	AddonBackendFailed  = "failed"
)

// ParseAddonSyncStatus는 동기화 결과 파일을 읽는다. root가 쓴 파일이지만 크기를 묶고 형식·id를 검사한다. 모르는 필드는
// 무시한다(설치기 복사본이 maru보다 먼저 바뀔 수 있다).
func ParseAddonSyncStatus(data []byte) (AddonSyncStatus, error) {
	var st AddonSyncStatus
	if len(data) > AddonSyncStatusMaxBytes {
		return st, fmt.Errorf("addon sync status: larger than %d bytes", AddonSyncStatusMaxBytes)
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return AddonSyncStatus{}, fmt.Errorf("addon sync status: %w", err)
	}
	if st.Format != AddonSyncStatusFormat {
		return AddonSyncStatus{}, fmt.Errorf("addon sync status: unsupported format %d", st.Format)
	}
	if len(st.Backends) > AddonBackendsMax || len(st.Purged) > AddonBackendsMax {
		return AddonSyncStatus{}, fmt.Errorf("addon sync status: more than %d entries", AddonBackendsMax)
	}
	for _, b := range st.Backends {
		if !ValidAddonID(b.ID) {
			return AddonSyncStatus{}, fmt.Errorf("addon sync status: invalid id %q", b.ID)
		}
	}
	for _, id := range st.Purged {
		if !ValidAddonID(id) {
			return AddonSyncStatus{}, fmt.Errorf("addon sync status: invalid purged id %q", id)
		}
	}
	return st, nil
}
