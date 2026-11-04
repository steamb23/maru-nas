package api

import "time"

// AddonSDKLevel은 이 대시보드가 지원하는 애드온 패키지 SDK API 수준이다. 패키지 manifest의 sdk가 이보다 크면
// "호환 안 됨"이다. 하위 호환을 지키며 올린다(.claude/docs/addon-packages.md).
const AddonSDKLevel = 1

// AddonPackageView는 이 사용자에게 보이는 애드온 패키지 하나다(CapabilitiesResponse.AddonPackages).
// 프론트가 앱·위젯 목록을 만들고 프레임을 띄울 때 쓴다. 문구는 패키지가 준 평문이다(번역 키가 아니다).
type AddonPackageView struct {
	ID string `json:"id"`
	// Rev는 설치본의 저장 이름이다. 바뀌면 열린 프레임을 새 주소로 다시 띄운다.
	Rev string `json:"rev"`
	// Dev는 개발 패키지(MARU_DEV_ADDONS)인지다.
	Dev         bool              `json:"dev"`
	Version     string            `json:"version"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description,omitempty"`
	Author      string            `json:"author"`
	// Icon은 대시보드 기준 상대 주소다(api/addons/<id>/icon?rev=<rev>).
	Icon string `json:"icon"`
	// UIBase는 프레임 파일의 대시보드 기준 상대 주소다(api/addon-ui/<토큰>/<rev>/). 토큰은 이 세션과 패키지에 묶인다.
	UIBase   string               `json:"uiBase"`
	Views    AddonPackageViews    `json:"views"`
	Requires Requirements         `json:"requires"`
	Actions  []AddonPackageAction `json:"actions"`
	// Languages는 패키지가 번역을 둔 언어다.
	Languages []string `json:"languages"`
	SDK       int      `json:"sdk"`
	// Backend는 백엔드 패키지인지다.
	Backend bool `json:"backend"`
	// Elevated는 쓰기를 선언해 조작을 실행할 때 단계 확인이 필요한지다.
	Elevated bool `json:"elevated"`
}

// AddonPackageViews는 패키지의 보기다.
type AddonPackageViews struct {
	Window *AddonPackageWindow `json:"window,omitempty"`
	Widget *AddonPackageWidget `json:"widget,omitempty"`
}

// AddonPackageWindow는 창 보기다. Entry는 UIBase 기준 파일 이름이다.
type AddonPackageWindow struct {
	Entry       string `json:"entry"`
	DefaultSize Size   `json:"defaultSize"`
	MinSize     *Size  `json:"minSize,omitempty"`
}

// AddonPackageWidget은 위젯 보기다.
type AddonPackageWidget struct {
	Entry       string    `json:"entry"`
	DefaultSize GridSize  `json:"defaultSize"`
	MinSize     *GridSize `json:"minSize,omitempty"`
	MaxSize     *GridSize `json:"maxSize,omitempty"`
}

// AddonPackageAction은 패키지 조작 선언이다. 문구는 언어별 평문이다.
type AddonPackageAction struct {
	Name        string            `json:"name"`
	Admin       bool              `json:"admin"`
	Confirm     bool              `json:"confirm"`
	Label       map[string]string `json:"label,omitempty"`
	ConfirmText map[string]string `json:"confirmText,omitempty"`
}

// AddonPackagesResponse는 GET /api/site/addons의 응답이다(관리자).
type AddonPackagesResponse struct {
	Packages []AddonPackageStatus `json:"packages"`
	// CanInstall은 설치할 수 있는지(상태 디렉터리가 있는지)다.
	CanInstall bool `json:"canInstall"`
	// IndexError는 설치 기록을 읽지 못했는지다. 그동안 설치본은 모두 빠진다.
	IndexError bool `json:"indexError"`
	// MaxUploadBytes는 올릴 수 있는 패키지 파일 크기다.
	MaxUploadBytes int64 `json:"maxUploadBytes"`
	// SDK는 이 대시보드의 SDK 수준이다.
	SDK int `json:"sdk"`
}

// AddonPackageStatus는 관리자 목록의 패키지 하나다.
type AddonPackageStatus struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
	Rev     string `json:"rev,omitempty"`
	Dev     bool   `json:"dev"`
	Enabled bool   `json:"enabled"`
	// State는 active(켜져 있고 쓰는 중), disabled(꺼짐), rejected(충돌·금지 표로 받지 않음), invalid(파일을 읽을 수 없음)다.
	State string `json:"state" tstype:"'active' | 'disabled' | 'rejected' | 'invalid'"`
	// Reason은 rejected·invalid의 이유 코드, Detail은 영어 설명이다.
	Reason      string            `json:"reason,omitempty"`
	Detail      string            `json:"detail,omitempty"`
	Name        map[string]string `json:"name,omitempty"`
	Description map[string]string `json:"description,omitempty"`
	Author      string            `json:"author,omitempty"`
	AuthorURL   string            `json:"authorUrl,omitempty"`
	SHA256      string            `json:"sha256,omitempty"`
	InstalledBy string            `json:"installedBy,omitempty"`
	InstalledAt *time.Time        `json:"installedAt,omitempty"`
	UpdatedAt   *time.Time        `json:"updatedAt,omitempty"`
	Permissions *AddonPermissions `json:"permissions,omitempty"`
	Icon        string            `json:"icon,omitempty"`
}

// AddonPermissions는 패키지가 선언한 권한 요약이다(검토 화면).
type AddonPermissions struct {
	Admin   bool     `json:"admin"`
	Plugins []string `json:"plugins"`
	Read    []string `json:"read"`
	Write   []string `json:"write"`
	Actions []string `json:"actions"`
	Backend bool     `json:"backend"`
	Network []string `json:"network"`
	// Risks는 위험 코드다(omv_write, backend, backend_shared_data, net_lan, net_loopback).
	Risks []string `json:"risks"`
	// Elevated는 설치·켜기·쓰기 조작에 단계 확인이 필요한지다.
	Elevated bool `json:"elevated"`
}

// AddonPermissionsDiff는 업데이트로 더해지거나 빠지는 권한이다.
type AddonPermissionsDiff struct {
	AdminAdded     bool     `json:"adminAdded"`
	AdminRemoved   bool     `json:"adminRemoved"`
	AddedPlugins   []string `json:"addedPlugins"`
	AddedRead      []string `json:"addedRead"`
	RemovedRead    []string `json:"removedRead"`
	AddedWrite     []string `json:"addedWrite"`
	RemovedWrite   []string `json:"removedWrite"`
	AddedActions   []string `json:"addedActions"`
	BackendAdded   bool     `json:"backendAdded"`
	BackendRemoved bool     `json:"backendRemoved"`
	AddedNetwork   []string `json:"addedNetwork"`
	RemovedNetwork []string `json:"removedNetwork"`
	// Widens는 권한이 넓어지는지다.
	Widens bool `json:"widens"`
}

// AddonPendingInstall은 POST /api/site/addons/uploads의 응답이다. 아직 설치하지 않았다.
type AddonPendingInstall struct {
	PendingID        string             `json:"pendingId"`
	ExpiresInSeconds int                `json:"expiresInSeconds"`
	Package          AddonPackageStatus `json:"package"`
	// Installed는 같은 id의 설치본이 있으면 그 상태다(업데이트).
	Installed *AddonPackageStatus   `json:"installed,omitempty"`
	Diff      *AddonPermissionsDiff `json:"diff,omitempty"`
	// Warnings는 author_changed, downgrade, reinstall, same_digest다.
	Warnings []string `json:"warnings"`
}

// AddonEnabledRequest는 PUT /api/site/addons/{id}의 본문이다.
type AddonEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// AddonStorage는 GET·PUT /api/preferences/addons/{id}의 본문이다. Data는 패키지가 정한 JSON 값이다.
type AddonStorage struct {
	Data any `json:"data" tstype:"unknown"`
}
