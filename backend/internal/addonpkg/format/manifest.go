// Package format은 애드온 패키지 manifest(manifest.json)의 형식과 의미 검사다. 압축 파일·저장소와 상관없는
// 순수 계층이라 SDK의 구조 스키마(sdk/js/schema/manifest.schema.json)도 여기서 만든다.
// 설계는 .claude/docs/addon-packages.md의 "manifest".
package format

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"maru/backend/internal/api"
)

// Version은 manifest 형식 번호(format 필드)다.
const Version = 1

// MaxManifestBytes는 manifest.json 크기의 상한이다.
const MaxManifestBytes = 64 << 10

// Text는 언어 코드 → 문구다. en은 반드시 있어야 한다.
type Text map[string]string

// Manifest는 패키지 manifest다. 모르는 필드는 받지 않는다(Decode).
type Manifest struct {
	// Schema는 편집기가 쓰는 JSON 스키마 주소다(sdk/js/schema/manifest.schema.json). 검사하지 않고 무시한다.
	Schema      string   `json:"$schema,omitempty"`
	Format      int      `json:"format"`
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	SDK         int      `json:"sdk"`
	Name        Text     `json:"name"`
	Description Text     `json:"description,omitempty"`
	Author      Author   `json:"author"`
	Icon        string   `json:"icon"`
	Views       Views    `json:"views"`
	Requires    Requires `json:"requires,omitzero"`
	RPC         RPC      `json:"rpc,omitzero"`
	Actions     []Action `json:"actions,omitempty"`
	// Backend가 있으면 백엔드 패키지다.
	Backend *Backend `json:"backend,omitempty"`
}

// Author는 패키지를 만든 사람이다. 검토 화면에 평문으로 보인다.
type Author struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Views는 패키지의 보기다. 하나 이상 있어야 한다.
type Views struct {
	Window *WindowView `json:"window,omitempty"`
	Widget *WidgetView `json:"widget,omitempty"`
}

// WindowView는 창 보기다. 크기는 픽셀이다.
type WindowView struct {
	// Entry는 ui/ 바로 아래의 HTML 파일 이름이다.
	Entry       string    `json:"entry"`
	DefaultSize api.Size  `json:"defaultSize"`
	MinSize     *api.Size `json:"minSize,omitempty"`
}

// WidgetView는 바탕화면 위젯 보기다. 크기는 칸이다.
type WidgetView struct {
	Entry       string        `json:"entry"`
	DefaultSize api.GridSize  `json:"defaultSize"`
	MinSize     *api.GridSize `json:"minSize,omitempty"`
	MaxSize     *api.GridSize `json:"maxSize,omitempty"`
}

// Requires는 보기를 쓰는 데 필요한 조건이다.
type Requires struct {
	Admin   bool     `json:"admin,omitempty"`
	Plugins []Plugin `json:"plugins,omitempty"`
}

// Plugin은 필요한 OMV 플러그인과 그 플러그인이 설치하는 RPC 파일
// (/usr/share/openmediavault/engined/rpc/<파일>)이다. 둘 다 있어야 설치된 것으로 본다.
type Plugin struct {
	Package string `json:"package"`
	RPCFile string `json:"rpcFile"`
}

// RPC는 부르는 OMV RPC("Service.method")다. 쓰기는 조작으로만 부른다.
type RPC struct {
	Read  []string `json:"read,omitempty"`
	Write []string `json:"write,omitempty"`
}

// Action은 조작 선언이다. 구현은 패키지 백엔드에 있다.
type Action struct {
	Name    string `json:"name"`
	Admin   bool   `json:"admin,omitempty"`
	Confirm bool   `json:"confirm,omitempty"`
	// TimeoutSeconds는 조작 전체에 주는 시간이다. 0이면 DefaultActionTimeout이다.
	TimeoutSeconds int  `json:"timeoutSeconds,omitempty"`
	Label          Text `json:"label,omitempty"`
	ConfirmText    Text `json:"confirmText,omitempty"`
}

// Backend는 패키지 백엔드 선언이다.
type Backend struct {
	// Network는 허용할 네트워크다(NetworkLAN, NetworkLoopback). 비면 네트워크가 없다.
	Network []string `json:"network"`
}

// 백엔드 네트워크 권한.
const (
	NetworkLAN      = "lan"
	NetworkLoopback = "loopback"
)

// ErrManifest는 manifest를 읽을 수 없다는 뜻이다(형식이 틀림, 모르는 필드, 크기 초과).
var ErrManifest = errors.New("invalid manifest")

// Decode는 manifest.json을 읽는다. 모르는 필드와 뒤에 남은 값은 받지 않는다. 의미 검사는 Validate가 한다.
func Decode(data []byte) (*Manifest, error) {
	if len(data) > MaxManifestBytes {
		return nil, fmt.Errorf("%w: larger than %d bytes", ErrManifest, MaxManifestBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing data after the manifest object", ErrManifest)
	}
	return &m, nil
}

// HasBackend는 백엔드 패키지인지다.
func (m *Manifest) HasBackend() bool { return m.Backend != nil }

// NetworkHas는 백엔드가 name 네트워크를 허용받았는지다.
func (m *Manifest) NetworkHas(name string) bool {
	if m.Backend == nil {
		return false
	}
	for _, n := range m.Backend.Network {
		if n == name {
			return true
		}
	}
	return false
}
